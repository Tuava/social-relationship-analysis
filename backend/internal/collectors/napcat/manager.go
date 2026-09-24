package napcat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type Manager struct {
	repo       persistence.Repository
	logger     *slog.Logger
	mu         sync.Mutex
	cancel     map[string]context.CancelFunc
	generation map[string]uint64
	nextRun    uint64
	normalizer normalization.Normalizer
	rootCtx    context.Context
	// MessageHook receives every realtime chat message after normalization,
	// in the raw OneBot v11 shape. Used by the bot service; nil disables it.
	MessageHook func(accountID string, raw json.RawMessage)
}

func NewManager(ctx context.Context, repo persistence.Repository, logger *slog.Logger) *Manager {
	return &Manager{repo: repo, logger: logger, cancel: make(map[string]context.CancelFunc), generation: make(map[string]uint64), normalizer: normalization.Normalizer{DB: repo.DB}, rootCtx: ctx}
}

func (m *Manager) Connect(ctx context.Context, account domain.NapCatAccount) error {
	m.mu.Lock()
	if cancel := m.cancel[account.ID]; cancel != nil {
		m.mu.Unlock()
		return nil
	}
	parent := m.rootCtx
	if parent == nil {
		parent = ctx
	}
	runCtx, cancel := context.WithCancel(parent)
	m.nextRun++
	generation := m.nextRun
	m.cancel[account.ID] = cancel
	m.generation[account.ID] = generation
	m.mu.Unlock()
	client := &WSClient{URL: account.WSURL, Token: account.WSToken, Logger: m.logger}
	client.OnStatus = func(status string, err error) {
		_ = m.repo.SetAccountStatus(context.Background(), account.ID, status)
		var message *string
		if err != nil {
			value := err.Error()
			message = &value
		}
		_ = m.repo.TouchConnection(context.Background(), account.ID, "ws", status, nil, message)
		if err != nil {
			m.logger.Warn("NapCat account status", "account_id", account.ID, "error", err)
		}
	}
	client.OnEvent = func(event WSEvent) error {
		rawID, err := m.repo.SaveRaw(context.Background(), account.ID, "napcat_ws", event.PostType, event.Raw)
		if err != nil {
			return err
		}
		occurred := eventTime(event.Time)
		_ = m.repo.TouchConnection(context.Background(), account.ID, "ws", "connected", occurred, nil)
		if err := m.repo.SaveRawEvent(context.Background(), rawID, event.EventID, event.PostType, event.MessageType, occurred, event.Sequence); err != nil {
			return err
		}
		if event.PostType == "message" || event.PostType == "message_sent" {
			if err := m.normalizer.ProcessRawMessage(context.Background(), account.ID, rawID, event.Raw); err != nil {
				m.logger.Warn("normalize NapCat message failed", "error", err)
			}
			if m.MessageHook != nil {
				m.MessageHook(account.ID, event.Raw)
			}
		}
		return nil
	}
	go func() {
		err := client.Run(runCtx)
		if err != nil && runCtx.Err() == nil {
			m.logger.Warn("NapCat account stopped", "account_id", account.ID, "error", err)
		}
		m.mu.Lock()
		if m.generation[account.ID] == generation {
			delete(m.cancel, account.ID)
			delete(m.generation, account.ID)
			_ = m.repo.SetAccountStatus(context.Background(), account.ID, "disconnected")
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) Disconnect(accountID string) {
	m.mu.Lock()
	if cancel := m.cancel[accountID]; cancel != nil {
		cancel()
		delete(m.cancel, accountID)
	}
	m.mu.Unlock()
	_ = m.repo.SetAccountStatus(context.Background(), accountID, "disconnected")
}
func (m *Manager) Connected(accountID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancel[accountID] != nil
}
func eventTime(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	t := time.Unix(seconds, 0)
	return &t
}
