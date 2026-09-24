package bot

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Memory maintains per-user / per-context rolling conversation history for continuous multi-turn dialogue.
type Memory struct {
	mu       sync.RWMutex
	sessions map[string]*userSession
	ttl      time.Duration
	maxMsgs  int
}

type userSession struct {
	Key        string
	BotID      string
	GroupID    string
	UserID     string
	Private    bool
	Messages   []chatMessage
	LastActive time.Time
}

// SessionSummary provides a human-readable snapshot of an active memory space.
type SessionSummary struct {
	Key          string        `json:"key"`
	Type         string        `json:"type"` // "private" or "group"
	BotID        string        `json:"bot_id"`
	GroupID      string        `json:"group_id,omitempty"`
	UserID       string        `json:"user_id"`
	MessageCount int           `json:"message_count"`
	TurnCount    int           `json:"turn_count"`
	LastActive   string        `json:"last_active"`
	LastQuery    string        `json:"last_query"`
	LastReply    string        `json:"last_reply"`
	Messages     []chatMessage `json:"messages,omitempty"`
}

// NewMemory creates a new conversation memory store.
func NewMemory(ttl time.Duration, maxMsgs int) *Memory {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if maxMsgs <= 0 {
		maxMsgs = 20
	}
	m := &Memory{
		sessions: make(map[string]*userSession),
		ttl:      ttl,
		maxMsgs:  maxMsgs,
	}
	// Periodic cleanup of idle conversation sessions
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		for range ticker.C {
			m.cleanup()
		}
	}()
	return m
}

// GetHistory returns the active conversation history for a given session key.
func (m *Memory) GetHistory(key string) []chatMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[key]
	if !ok {
		return nil
	}
	if time.Since(sess.LastActive) > m.ttl {
		return nil
	}

	// Return a copy to avoid data races
	history := make([]chatMessage, len(sess.Messages))
	copy(history, sess.Messages)
	return history
}

// Append records a user turn and the assistant's final response into the session.
func (m *Memory) Append(key string, userMsg, assistantMsg string) {
	if userMsg == "" && assistantMsg == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[key]
	if !ok || time.Since(sess.LastActive) > m.ttl {
		sess = &userSession{
			Key:      key,
			Messages: make([]chatMessage, 0, m.maxMsgs),
		}
		// Parse key parts: p:<botID>:<userID> or g:<botID>:<groupID> or g:<botID>:<groupID>:<userID>
		parts := strings.Split(key, ":")
		if len(parts) >= 3 && parts[0] == "p" {
			sess.Private = true
			sess.BotID = parts[1]
			sess.UserID = parts[2]
		} else if len(parts) >= 3 && parts[0] == "g" {
			sess.Private = false
			sess.BotID = parts[1]
			sess.GroupID = parts[2]
			if len(parts) >= 4 {
				sess.UserID = parts[3]
			}
		}
		m.sessions[key] = sess
	}

	if userMsg != "" {
		sess.Messages = append(sess.Messages, chatMessage{Role: "user", Content: userMsg})
	}
	if assistantMsg != "" {
		sess.Messages = append(sess.Messages, chatMessage{Role: "assistant", Content: assistantMsg})
	}

	// Keep within maximum sliding window size
	if len(sess.Messages) > m.maxMsgs {
		sess.Messages = sess.Messages[len(sess.Messages)-m.maxMsgs:]
	}
	sess.LastActive = time.Now()
}

// ListSummaries returns active memory session summaries for a given bot instance.
func (m *Memory) ListSummaries(botID string, includeMessages bool) []SessionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	res := make([]SessionSummary, 0, len(m.sessions))

	for key, sess := range m.sessions {
		if now.Sub(sess.LastActive) > m.ttl {
			continue
		}
		if botID != "" && sess.BotID != "" && sess.BotID != botID {
			continue
		}

		kind := "group"
		if sess.Private {
			kind = "private"
		}

		var lastQ, lastR string
		for i := len(sess.Messages) - 1; i >= 0; i-- {
			if sess.Messages[i].Role == "user" && lastQ == "" {
				lastQ = sess.Messages[i].Content
			}
			if sess.Messages[i].Role == "assistant" && lastR == "" {
				lastR = sess.Messages[i].Content
			}
			if lastQ != "" && lastR != "" {
				break
			}
		}

		sum := SessionSummary{
			Key:          key,
			Type:         kind,
			BotID:        sess.BotID,
			GroupID:      sess.GroupID,
			UserID:       sess.UserID,
			MessageCount: len(sess.Messages),
			TurnCount:    (len(sess.Messages) + 1) / 2,
			LastActive:   sess.LastActive.Format(time.RFC3339),
			LastQuery:    lastQ,
			LastReply:    lastR,
		}

		if includeMessages {
			history := make([]chatMessage, len(sess.Messages))
			copy(history, sess.Messages)
			sum.Messages = history
		}

		res = append(res, sum)
	}
	return res
}

// GetSessionDetail returns full conversation history for a specific session key.
func (m *Memory) GetSessionDetail(key string) (*SessionSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[key]
	if !ok || time.Since(sess.LastActive) > m.ttl {
		return nil, fmt.Errorf("session not found or expired")
	}

	kind := "group"
	if sess.Private {
		kind = "private"
	}

	history := make([]chatMessage, len(sess.Messages))
	copy(history, sess.Messages)

	return &SessionSummary{
		Key:          key,
		Type:         kind,
		BotID:        sess.BotID,
		GroupID:      sess.GroupID,
		UserID:       sess.UserID,
		MessageCount: len(sess.Messages),
		TurnCount:    (len(sess.Messages) + 1) / 2,
		LastActive:   sess.LastActive.Format(time.RFC3339),
		Messages:     history,
	}, nil
}

// InjectMessage allows manually adding context or memory into a session.
func (m *Memory) InjectMessage(key string, role, content string) {
	if content == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[key]
	if !ok || time.Since(sess.LastActive) > m.ttl {
		sess = &userSession{
			Key:      key,
			Messages: make([]chatMessage, 0, m.maxMsgs),
		}
		parts := strings.Split(key, ":")
		if len(parts) >= 3 && parts[0] == "p" {
			sess.Private = true
			sess.BotID = parts[1]
			sess.UserID = parts[2]
		} else if len(parts) >= 4 && parts[0] == "g" {
			sess.Private = false
			sess.BotID = parts[1]
			sess.GroupID = parts[2]
			sess.UserID = parts[3]
		}
		m.sessions[key] = sess
	}

	sess.Messages = append(sess.Messages, chatMessage{Role: role, Content: content})
	if len(sess.Messages) > m.maxMsgs {
		sess.Messages = sess.Messages[len(sess.Messages)-m.maxMsgs:]
	}
	sess.LastActive = time.Now()
}

// Clear explicitly resets the conversation memory for a session.
func (m *Memory) Clear(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key)
}

func (m *Memory) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.sessions {
		if now.Sub(v.LastActive) > m.ttl {
			delete(m.sessions, k)
		}
	}
}
