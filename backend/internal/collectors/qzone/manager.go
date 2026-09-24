package qzone

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type Manager struct {
	repo       persistence.Repository
	logger     *slog.Logger
	normalizer Normalizer
	rootCtx    context.Context
	mu         sync.Mutex
	cancel     map[string]context.CancelFunc
}

func NewManager(ctx context.Context, repo persistence.Repository, logger *slog.Logger, normalizer Normalizer) *Manager {
	return &Manager{repo: repo, logger: logger, normalizer: normalizer, rootCtx: ctx, cancel: map[string]context.CancelFunc{}}
}

func (m *Manager) Connect(ctx context.Context, account domain.NapCatAccount, connection domain.QZoneConnection) error {
	m.mu.Lock()
	if m.cancel[account.ID] != nil {
		m.mu.Unlock()
		return nil
	}
	parent := m.rootCtx
	if parent == nil {
		parent = ctx
	}
	runCtx, cancel := context.WithCancel(parent)
	m.cancel[account.ID] = cancel
	m.mu.Unlock()

	client := &WSClient{URL: connection.WSURL, Token: connection.AccessToken, Logger: m.logger}
	client.OnStatus = func(status string, statusErr error) {
		var message *string
		if statusErr != nil {
			value := statusErr.Error()
			message = &value
		}
		_ = m.repo.SetQZoneStatus(context.Background(), account.ID, status, nil, message)
		_ = m.repo.TouchConnection(context.Background(), account.ID, "qzone_ws", status, nil, message)
	}
	client.OnEvent = func(event Event) error {
		rawID, err := m.repo.SaveRaw(context.Background(), account.ID, "qzone_ws", eventType(event), event.Raw)
		if err != nil {
			return err
		}
		var occurred *time.Time
		if event.Time > 0 {
			value := time.Unix(event.Time, 0)
			occurred = &value
		}
		_ = m.repo.SetQZoneStatus(context.Background(), account.ID, "connected", occurred, nil)
		_ = m.repo.TouchConnection(context.Background(), account.ID, "qzone_ws", "connected", occurred, nil)
		if err := m.repo.SaveRawEvent(context.Background(), rawID, event.EventID, event.PostType, event.MessageType, occurred, 0); err != nil {
			return err
		}
		return m.normalizer.Realtime(context.Background(), account, rawID, event.Raw)
	}
	go func() {
		err := client.Run(runCtx)
		if err != nil && runCtx.Err() == nil && m.logger != nil {
			m.logger.Warn("QZone websocket stopped", "account_id", account.ID, "error", err)
		}
		m.mu.Lock()
		delete(m.cancel, account.ID)
		m.mu.Unlock()
		_ = m.repo.SetQZoneStatus(context.Background(), account.ID, "disconnected", nil, nil)
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
	_ = m.repo.SetQZoneStatus(context.Background(), accountID, "disconnected", nil, nil)
}
