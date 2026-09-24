package napcat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

const (
	groupHistoryPageSize  = 100
	friendHistoryPageSize = 100
	historyMaxAttempts    = 4
	historyPagePause      = 150 * time.Millisecond
)

// callHistoryWithRetry retries a history page request with exponential
// backoff. Permanent failures (permission / not found) fail immediately so a
// genuinely unreachable group does not stall the whole run.
func callHistoryWithRetry(ctx context.Context, call func() (json.RawMessage, error)) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt < historyMaxAttempts; attempt++ {
		raw, err := call()
		if err == nil {
			return raw, nil
		}
		lastErr = err
		// A dead anchor ("消息<id>不存在") must not be retried as-is; the
		// caller falls back to a fresh no-anchor request instead.
		if isStaleAnchorError(err) || !retryableHistoryError(err) {
			return nil, err
		}
		backoff := time.Duration(250*(1<<attempt)) * time.Millisecond
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func retryableHistoryError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	permanent := []string{
		"not found", "no such", "permission", "unauthorized", "forbidden",
		"群不存在", "好友不存在", "会话不存在", "无权", "未找到", "不在此群", "拒绝",
	}
	for _, p := range permanent {
		if strings.Contains(msg, p) {
			return false
		}
	}
	return true
}

// isStaleAnchorError reports whether NapCat rejected a history request because
// the saved message_seq anchor no longer exists (account restart / re-login
// invalidates NapCat's short message IDs).
func isStaleAnchorError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "消息") && strings.Contains(msg, "不存在")
}

// isEmptyHistoryError covers NapCat's response for a conversation with no
// readable history. NapCat returns "消息undefined不存在" when the request has
// no message_seq anchor; this is different from a real stale anchor because
// there is no saved page boundary to recover from.
func isEmptyHistoryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "消息undefined不存在")
}

func historyPagePauseWait(ctx context.Context) error {
	select {
	case <-time.After(historyPagePause):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// callHistoryPageWithAnchorRecovery makes one history page request and, when
// the saved short message_seq is stale, retries the same page without an
// anchor. The fallback is deliberately contained to this request so a
// permanently broken API response cannot turn pagination into an endless loop.
func callHistoryPageWithAnchorRecovery(ctx context.Context, seq any, call func(any) (json.RawMessage, error)) (json.RawMessage, error, bool) {
	raw, err := callHistoryWithRetry(ctx, func() (json.RawMessage, error) {
		return call(seq)
	})
	if err != nil && seq == nil && isEmptyHistoryError(err) {
		return json.RawMessage(`{"messages":[]}`), nil, false
	}
	if err == nil || seq == nil || !isStaleAnchorError(err) {
		return raw, err, false
	}

	raw, err = callHistoryWithRetry(ctx, func() (json.RawMessage, error) {
		return call(nil)
	})
	if err != nil && isEmptyHistoryError(err) {
		return json.RawMessage(`{"messages":[]}`), nil, true
	}
	return raw, err, true
}

// collectGroupHistory fetches all available group messages by paginating
// backwards through message_seq. It resumes from a saved cursor if one exists.
func (c Collector) CollectGroupHistory(ctx context.Context, account domain.NapCatAccount, client *HTTPClient, groupID string) error {
	return c.collectGroupHistory(ctx, account, client, groupID, nil)
}

func (c Collector) collectGroupHistory(ctx context.Context, account domain.NapCatAccount, client *HTTPClient, groupID string, onProgress func(domain.HistoryProgress)) error {
	scope := fmt.Sprintf("group_msg_history:%s", groupID)
	endpoint := fmt.Sprintf("get_group_msg_history:%s", groupID)

	cursor := GroupHistoryCursor{HasMore: true}
	if c.Cursors.DB != nil {
		if found, _ := c.Cursors.LoadCursor(ctx, account.ID, scope, &cursor); found {
			c.Logger.Debug("resuming group history from cursor", "group_id", groupID, "last_seq", cursor.LastSeq, "seen", cursor.TotalSeen)
		}
	}

	pages := 0
	var records int64
	emit := func() {
		if onProgress != nil {
			onProgress(domain.HistoryProgress{Pages: pages, Records: records, Cursor: map[string]any{
				"scope": scope, "group_id": groupID, "message_seq": cursor.LastSeq,
				"total_seen": cursor.TotalSeen, "has_more": cursor.HasMore,
				"boundary_confirmed": cursor.BoundaryConfirmed, "boundary_reason": cursor.BoundaryReason,
			}})
		}
	}
	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if page > 0 {
			if err := historyPagePauseWait(ctx); err != nil {
				return err
			}
		}
		var seq any
		if cursor.LastSeq > 0 {
			seq = cursor.LastSeq
		}
		raw, err, recovered := callHistoryPageWithAnchorRecovery(ctx, seq, func(requestSeq any) (json.RawMessage, error) {
			return client.GetGroupHistory(ctx, groupID, groupHistoryPageSize, requestSeq)
		})
		if recovered {
			cursor.LastSeq = 0
			cursor.HasMore = true
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			if c.Logger != nil {
				c.Logger.Warn("discarded stale group history anchor", "group_id", groupID)
			}
		}
		if err != nil {
			c.logError(account, endpoint, err)
			return fmt.Errorf("%s page %d: %w", endpoint, page+1, err)
		}
		var wrapper struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			c.logError(account, endpoint+":decode", err)
			return fmt.Errorf("%s page %d decode: %w", endpoint, page+1, err)
		}
		if len(wrapper.Messages) == 0 {
			cursor.HasMore = false
			cursor.BoundaryConfirmed = true
			cursor.BoundaryReason = "empty_page"
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			emit()
			return nil
		}

		// Save and normalize each message
		for _, msg := range wrapper.Messages {
			msgBytes, _ := json.Marshal(msg)
			rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", endpoint+":message", msgBytes)
			if err != nil {
				c.logError(account, endpoint+":saveraw", err)
				continue
			}
			if err := c.Normalizer.ProcessRawMessage(ctx, account.ID, rawID, msgBytes); err != nil {
				if c.Logger != nil {
					c.Logger.Warn("normalize history message failed", "error", err)
				}
			}
		}

		cursor.TotalSeen += len(wrapper.Messages)
		pages++
		records += int64(len(wrapper.Messages))

		nextSeq, found := historyBoundary(wrapper.Messages)
		if !found || nextSeq <= 0 || nextSeq == cursor.LastSeq {
			cursor.HasMore = false
			// NapCat returned the same oldest anchor after the probe. Treat
			// this as the API's terminal history window, while preserving the
			// reason so callers do not confuse it with an empty conversation.
			cursor.BoundaryConfirmed = true
			cursor.BoundaryReason = "upstream_history_window_exhausted"
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			emit()
			return nil
		}
		cursor.LastSeq = nextSeq
		// A short page is not proof that the server reached the historical
		// boundary. Keep walking with its oldest message as the anchor; the
		// following empty page (or an explicit end marker) confirms completion.
		cursor.HasMore = true
		cursor.BoundaryConfirmed = false
		if len(wrapper.Messages) < groupHistoryPageSize {
			cursor.BoundaryReason = "short_page_probe_continuing"
		}

		// Persist cursor after each page
		if c.Cursors.DB != nil {
			_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
		}
		emit()
		if !cursor.HasMore {
			return nil
		}
	}
}

// collectFriendHistory fetches private chat messages for a single friend QQ.
func (c Collector) CollectFriendHistory(ctx context.Context, account domain.NapCatAccount, client *HTTPClient, friendQQ string) error {
	return c.collectFriendHistory(ctx, account, client, friendQQ, nil)
}

func (c Collector) collectFriendHistory(ctx context.Context, account domain.NapCatAccount, client *HTTPClient, friendQQ string, onProgress func(domain.HistoryProgress)) error {
	scope := fmt.Sprintf("friend_msg_history:%s", friendQQ)
	endpoint := fmt.Sprintf("get_friend_msg_history:%s", friendQQ)

	cursor := FriendHistoryCursor{HasMore: true}
	if c.Cursors.DB != nil {
		if found, _ := c.Cursors.LoadCursor(ctx, account.ID, scope, &cursor); found {
			c.Logger.Debug("resuming friend history from cursor", "qq", friendQQ, "last_seq", cursor.LastSeq, "seen", cursor.TotalSeen)
		}
	}

	pages := 0
	var records int64
	emit := func() {
		if onProgress != nil {
			onProgress(domain.HistoryProgress{Pages: pages, Records: records, Cursor: map[string]any{
				"scope": scope, "qq": friendQQ, "message_seq": cursor.LastSeq,
				"total_seen": cursor.TotalSeen, "has_more": cursor.HasMore,
			}})
		}
	}
	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if page > 0 {
			if err := historyPagePauseWait(ctx); err != nil {
				return err
			}
		}
		var seq any
		if cursor.LastSeq > 0 {
			seq = cursor.LastSeq
		}
		raw, err, recovered := callHistoryPageWithAnchorRecovery(ctx, seq, func(requestSeq any) (json.RawMessage, error) {
			return client.GetFriendMsgHistory(ctx, friendQQ, friendHistoryPageSize, requestSeq)
		})
		if recovered {
			cursor.LastSeq = 0
			cursor.HasMore = true
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			if c.Logger != nil {
				c.Logger.Warn("discarded stale friend history anchor", "qq", friendQQ)
			}
		}
		if err != nil {
			c.logError(account, endpoint, err)
			return fmt.Errorf("%s page %d: %w", endpoint, page+1, err)
		}
		var wrapper struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			c.logError(account, endpoint+":decode", err)
			return fmt.Errorf("%s page %d decode: %w", endpoint, page+1, err)
		}
		if len(wrapper.Messages) == 0 {
			cursor.HasMore = false
			cursor.BoundaryConfirmed = true
			cursor.BoundaryReason = "empty_page"
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			emit()
			return nil
		}

		for _, msg := range wrapper.Messages {
			msgBytes, _ := json.Marshal(msg)
			rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", endpoint+":message", msgBytes)
			if err != nil {
				c.logError(account, endpoint+":saveraw", err)
				continue
			}
			if err := c.Normalizer.ProcessRawMessage(ctx, account.ID, rawID, msgBytes, friendQQ); err != nil {
				if c.Logger != nil {
					c.Logger.Warn("normalize friend history message failed", "error", err)
				}
			}
		}

		cursor.TotalSeen += len(wrapper.Messages)
		pages++
		records += int64(len(wrapper.Messages))

		nextSeq, found := historyBoundary(wrapper.Messages)
		if !found || nextSeq <= 0 || nextSeq == cursor.LastSeq {
			cursor.HasMore = false
			cursor.BoundaryConfirmed = true
			cursor.BoundaryReason = "upstream_history_window_exhausted"
			if c.Cursors.DB != nil {
				_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
			}
			emit()
			return nil
		}
		cursor.LastSeq = nextSeq
		cursor.HasMore = true
		cursor.BoundaryConfirmed = false
		if len(wrapper.Messages) < friendHistoryPageSize {
			cursor.BoundaryReason = "short_page_probe_continuing"
		}

		if c.Cursors.DB != nil {
			_ = c.Cursors.SaveCursor(ctx, account.ID, scope, cursor)
		}
		emit()
		if !cursor.HasMore {
			return nil
		}
	}
}

// historyBoundary returns the oldest message in the current page. NapCat
// returns history pages in chronological order, while message_seq is a random
// short ID registered in the current NapCat process rather than a sortable
// sequence number.
func historyBoundary(messages []map[string]any) (int64, bool) {
	if len(messages) == 0 {
		return 0, false
	}
	return toInt64(messages[0]["message_seq"])
}

// toInt64 safely converts any numeric to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case string:
		var i int64
		_, err := fmt.Sscanf(n, "%d", &i)
		return i, err == nil
	}
	return 0, false
}
