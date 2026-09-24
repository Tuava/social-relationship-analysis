package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DialogueThread struct {
	ID               string                 `json:"id"`
	ConversationID   string                 `json:"conversation_id"`
	Title            string                 `json:"title"`
	TopicCategory    string                 `json:"topic_category"`
	Stance           string                 `json:"stance"`
	Summary          string                 `json:"summary"`
	ParticipantCount int                    `json:"participant_count"`
	MessageCount     int                    `json:"message_count"`
	StartedAt        time.Time              `json:"started_at"`
	EndedAt          time.Time              `json:"ended_at"`
	KeyEntities      []string               `json:"key_entities"`
	Messages         []ThreadMessageDetail  `json:"messages,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

type ThreadMessageDetail struct {
	MessageID        string     `json:"message_id"`
	SequenceIndex    int        `json:"sequence_index"`
	ReplyToMessageID *string    `json:"reply_to_message_id,omitempty"`
	SenderQQ         string     `json:"sender_qq"`
	SenderName       string     `json:"sender_name"`
	Text             string     `json:"text"`
	SentAt           *time.Time `json:"sent_at"`
}

type rawMsgItem struct {
	ID         string
	SenderQQ   string
	SenderName string
	Text       string
	SentAt     time.Time
}

// DisentangleConversation parses interleaved messages in a conversation into coherent dialogue threads using LLM.
func DisentangleConversation(ctx context.Context, pool *pgxpool.Pool, conversationID string, limit int) ([]DialogueThread, error) {
	if limit <= 0 {
		limit = 60
	}
	if limit > 150 {
		limit = 150
	}

	// 1. Fetch chronological messages for the conversation
	rows, err := pool.Query(ctx, `
		SELECT m.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, ''),
		       COALESCE(m.raw_text, ''), m.sent_at
		FROM messages m
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE m.conversation_id = $1::uuid AND m.raw_text IS NOT NULL AND TRIM(m.raw_text) != ''
		ORDER BY m.sent_at ASC NULLS LAST, m.id ASC
		LIMIT $2
	`, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch conversation messages: %w", err)
	}
	defer rows.Close()

	var msgList []rawMsgItem
	for rows.Next() {
		var item rawMsgItem
		if err := rows.Scan(&item.ID, &item.SenderQQ, &item.SenderName, &item.Text, &item.SentAt); err != nil {
			return nil, err
		}
		item.Text = sanitizeMessageText(item.Text)
		msgList = append(msgList, item)
	}
	rows.Close()

	if len(msgList) == 0 {
		return []DialogueThread{}, nil
	}

	// 2. Prepare payload for LLM
	var sb strings.Builder
	for i, m := range msgList {
		timeStr := m.SentAt.Format("15:04:05")
		name := m.SenderName
		if name == "" {
			name = m.SenderQQ
		}
		sb.WriteString(fmt.Sprintf("[%d] (ID:%s, Time:%s, %s): %s\n", i+1, m.ID, timeStr, name, m.Text))
	}

	cfg := GetActiveLLMConfig(ctx, pool)
	systemPrompt := `你是一名专业的群聊会话解缠（Multiparty Chat Disentanglement）与自然语言处理专家。
你的任务是将群聊中时间交错的多方消息流拆分重组为一个个独立的【对话小节 / 话题线程（Dialogue Threads）】。

要求：
1. 每个线程聚合围绕同一个具体话题/事件的相关消息。
2. 给出清晰的主题题目、分类（tech/gaming/finance/acgn/daily/general）、讨论立场（casual/debate/question_answer/collaboration）以及一句话摘要。
3. message_ids 请填入消息的序号（例如 ["1", "2", "3"] 或消息ID）。
4. 必须输出合法可解析的标准 JSON 结构，不要输出任何多余解释。`

	fewShotExample := `
【标准输出示例】
{
  "threads": [
    {
      "title": "大模型视觉识别限制探讨",
      "topic_category": "tech",
      "stance": "debate",
      "summary": "讨论大模型对视频逐帧解析能力与安全审查限制。",
      "key_entities": ["大模型", "视觉识别", "安全审查"],
      "message_ids": ["1", "2", "3"],
      "links": [
        {"message_id": "2", "reply_to": "1"}
      ]
    }
  ]
}`

	userPrompt := fmt.Sprintf(`待解缠群聊消息流如下：
%s

请参考标准格式输出 JSON：
`, sb.String())

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt + "\n" + fewShotExample},
		{Role: "user", Content: userPrompt},
	}

	rawResp, err := CallLLM(ctx, cfg, messages, 0.1)
	if err != nil {
		return nil, fmt.Errorf("llm disentanglement call: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResp)
	var parsed struct {
		Threads []struct {
			Title         string   `json:"title"`
			TopicCategory string   `json:"topic_category"`
			Stance        string   `json:"stance"`
			Summary       string   `json:"summary"`
			KeyEntities   []string `json:"key_entities"`
			MessageIDs    []string `json:"message_ids"`
			Links         []struct {
				MessageID string `json:"message_id"`
				ReplyTo   string `json:"reply_to"`
			} `json:"links"`
		} `json:"threads"`
	}

	if err := json.Unmarshal([]byte(cleanJSON), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal disentanglement json: %w (raw: %s)", err, cleanJSON)
	}

	msgMap := make(map[string]rawMsgItem)
	idxMap := make(map[string]rawMsgItem)
	for i, m := range msgList {
		msgMap[m.ID] = m
		idxMap[strconv.Itoa(i+1)] = m
		idxMap[fmt.Sprintf("[%d]", i+1)] = m
	}

	var results []DialogueThread

	for _, pt := range parsed.Threads {
		if len(pt.MessageIDs) == 0 {
			continue
		}

		var firstTime, lastTime time.Time
		usersSet := make(map[string]bool)
		validMsgIDs := make([]string, 0, len(pt.MessageIDs))

		for _, mid := range pt.MessageIDs {
			mid = strings.TrimSpace(mid)
			var matched rawMsgItem
			var found bool
			if m, ok := msgMap[mid]; ok {
				matched = m
				found = true
			} else if m, ok := idxMap[mid]; ok {
				matched = m
				found = true
			}

			if found {
				validMsgIDs = append(validMsgIDs, matched.ID)
				usersSet[matched.SenderQQ] = true
				if firstTime.IsZero() || matched.SentAt.Before(firstTime) {
					firstTime = matched.SentAt
				}
				if lastTime.IsZero() || matched.SentAt.After(lastTime) {
					lastTime = matched.SentAt
				}
			}
		}

		if len(validMsgIDs) == 0 {
			continue
		}

		keyEntBytes, _ := json.Marshal(pt.KeyEntities)
		if len(pt.KeyEntities) == 0 {
			keyEntBytes = []byte("[]")
		}

		var threadID string
		err := pool.QueryRow(ctx, `
			INSERT INTO dialogue_threads(
				conversation_id, title, topic_category, stance, summary,
				participant_count, message_count, started_at, ended_at, key_entities
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb
			) RETURNING id::text
		`, conversationID, pt.Title, pt.TopicCategory, pt.Stance, pt.Summary,
			len(usersSet), len(validMsgIDs), firstTime, lastTime, string(keyEntBytes)).Scan(&threadID)
		if err != nil {
			continue
		}

		linksMap := make(map[string]string)
		for _, l := range pt.Links {
			lMid := strings.TrimSpace(l.MessageID)
			lParent := strings.TrimSpace(l.ReplyTo)
			var actualChildID, actualParentID string
			if m, ok := msgMap[lMid]; ok {
				actualChildID = m.ID
			} else if m, ok := idxMap[lMid]; ok {
				actualChildID = m.ID
			}

			if m, ok := msgMap[lParent]; ok {
				actualParentID = m.ID
			} else if m, ok := idxMap[lParent]; ok {
				actualParentID = m.ID
			}

			if actualChildID != "" && actualParentID != "" {
				linksMap[actualChildID] = actualParentID
			}
		}

		var threadMsgs []ThreadMessageDetail
		for seq, mid := range validMsgIDs {
			m := msgMap[mid]
			var replyTo *string
			if p, ok := linksMap[mid]; ok {
				replyTo = &p
			}

			_, _ = pool.Exec(ctx, `
				INSERT INTO thread_messages(thread_id, message_id, sequence_index, reply_to_message_id)
				VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
				ON CONFLICT(thread_id, message_id) DO UPDATE SET
					sequence_index = EXCLUDED.sequence_index,
					reply_to_message_id = EXCLUDED.reply_to_message_id
			`, threadID, mid, seq, replyTo)

			threadMsgs = append(threadMsgs, ThreadMessageDetail{
				MessageID:        mid,
				SequenceIndex:    seq,
				ReplyToMessageID: replyTo,
				SenderQQ:         m.SenderQQ,
				SenderName:       m.SenderName,
				Text:             m.Text,
				SentAt:           &m.SentAt,
			})
		}

		results = append(results, DialogueThread{
			ID:               threadID,
			ConversationID:   conversationID,
			Title:            pt.Title,
			TopicCategory:    pt.TopicCategory,
			Stance:           pt.Stance,
			Summary:          pt.Summary,
			ParticipantCount: len(usersSet),
			MessageCount:     len(validMsgIDs),
			StartedAt:        firstTime,
			EndedAt:          lastTime,
			KeyEntities:      pt.KeyEntities,
			Messages:         threadMsgs,
			CreatedAt:        time.Now(),
		})
	}

	return results, nil
}
