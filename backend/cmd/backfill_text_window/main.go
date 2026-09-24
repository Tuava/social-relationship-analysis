package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type conversation struct {
	kind string
	id   string
}

type page struct {
	Messages []json.RawMessage `json:"messages"`
}

type message struct {
	Time       int64           `json:"time"`
	MessageSeq any             `json:"message_seq"`
	Message    json.RawMessage `json:"message"`
}

type segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type counters struct {
	conversations atomic.Int64
	pages         atomic.Int64
	seen          atomic.Int64
	matched       atomic.Int64
	saved         atomic.Int64
	failures      atomic.Int64
}

func main() {
	var accountID, startRaw, endRaw, exportPath, only string
	var workers int
	flag.StringVar(&accountID, "account", "", "NapCat account UUID")
	flag.StringVar(&startRaw, "start", "", "inclusive RFC3339 start")
	flag.StringVar(&endRaw, "end", "", "exclusive RFC3339 end")
	flag.StringVar(&exportPath, "export", "", "optional JSONL output path")
	flag.StringVar(&only, "only", "", "optional single conversation as group:ID or private:ID")
	flag.IntVar(&workers, "workers", 4, "parallel conversation workers")
	flag.Parse()
	if accountID == "" || startRaw == "" || endRaw == "" {
		fatalf("-account, -start and -end are required")
	}
	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		fatalf("parse start: %v", err)
	}
	end, err := time.Parse(time.RFC3339, endRaw)
	if err != nil || !end.After(start) {
		fatalf("invalid end: %v", err)
	}
	if workers < 1 || workers > 8 {
		fatalf("workers must be between 1 and 8")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	pool, err := persistence.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fatalf("open database: %v", err)
	}
	defer pool.Close()
	repo := persistence.Repository{DB: pool, EncryptionKey: os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")}
	account, err := repo.GetAccount(ctx, accountID)
	if err != nil {
		fatalf("load account: %v", err)
	}
	client := napcat.NewHTTPClient(account.HTTPURL, account.HTTPToken)
	normalizer := normalization.Normalizer{DB: pool}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	before, err := countTextMessages(ctx, pool, start, end)
	if err != nil {
		fatalf("count existing messages: %v", err)
	}
	run, err := repo.CreateCollectionRun(ctx, &account.ID, "targeted_text_window")
	if err != nil {
		fatalf("create collection run: %v", err)
	}
	_ = repo.UpdateCollectionRun(ctx, run.ID, "running", 1, nil)

	conversations, err := listConversations(ctx, client)
	if err != nil {
		msg := err.Error()
		_ = repo.UpdateCollectionRun(context.Background(), run.ID, "failed", 100, &msg)
		fatalf("list conversations: %v", err)
	}
	if only != "" {
		parts := strings.SplitN(only, ":", 2)
		if len(parts) != 2 || (parts[0] != "group" && parts[0] != "private") || strings.TrimSpace(parts[1]) == "" {
			fatalf("-only must be group:ID or private:ID")
		}
		conversations = []conversation{{kind: parts[0], id: strings.TrimSpace(parts[1])}}
	}
	logger.Info("targeted text collection started", "run_id", run.ID, "start", start, "end", end, "conversations", len(conversations), "workers", workers)

	jobs := make(chan conversation)
	var wg sync.WaitGroup
	var stats counters
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if err := collectConversation(ctx, client, repo, normalizer, account.ID, item, start, end, &stats); err != nil {
					stats.failures.Add(1)
					logger.Warn("conversation collection failed", "kind", item.kind, "id", item.id, "error", err)
				}
				done := stats.conversations.Add(1)
				progress := 1 + int(done*98/int64(len(conversations)))
				_ = repo.UpdateCollectionRun(context.Background(), run.ID, "running", progress, nil)
			}
		}()
	}
	for _, item := range conversations {
		select {
		case jobs <- item:
		case <-ctx.Done():
			break
		}
	}
	close(jobs)
	wg.Wait()

	after, err := countTextMessages(context.Background(), pool, start, end)
	if err != nil {
		fatalf("count collected messages: %v", err)
	}
	status := "completed"
	var runErr *string
	if stats.failures.Load() > 0 {
		status = "partial"
		msg := fmt.Sprintf("%d conversations failed", stats.failures.Load())
		runErr = &msg
	}
	_ = repo.UpdateCollectionRun(context.Background(), run.ID, status, 100, runErr)

	if exportPath != "" {
		if err := exportJSONL(context.Background(), pool, start, end, exportPath); err != nil {
			fatalf("export JSONL: %v", err)
		}
	}
	result := map[string]any{
		"run_id": run.ID, "status": status, "start": start, "end": end,
		"conversations": stats.conversations.Load(), "pages": stats.pages.Load(),
		"messages_seen": stats.seen.Load(), "text_messages_matched": stats.matched.Load(),
		"normalizations": stats.saved.Load(), "conversation_failures": stats.failures.Load(),
		"window_total_before": before, "window_total_after": after, "window_new": after - before,
		"export": exportPath,
	}
	encoded, _ := json.Marshal(result)
	fmt.Println(string(encoded))
}

func listConversations(ctx context.Context, client *napcat.HTTPClient) ([]conversation, error) {
	groupsRaw, err := client.GetGroupList(ctx)
	if err != nil {
		return nil, err
	}
	var groups []struct {
		GroupID any `json:"group_id"`
	}
	if err := decodeUseNumber(groupsRaw, &groups); err != nil {
		return nil, err
	}
	friendsRaw, err := client.GetFriendList(ctx)
	if err != nil {
		return nil, err
	}
	var friends []struct {
		UserID any `json:"user_id"`
	}
	if err := decodeUseNumber(friendsRaw, &friends); err != nil {
		return nil, err
	}
	result := make([]conversation, 0, len(groups)+len(friends))
	for _, group := range groups {
		if id := identifier(group.GroupID); id != "" {
			result = append(result, conversation{kind: "group", id: id})
		}
	}
	for _, friend := range friends {
		if id := identifier(friend.UserID); id != "" {
			result = append(result, conversation{kind: "private", id: id})
		}
	}
	return result, nil
}

func collectConversation(ctx context.Context, client *napcat.HTTPClient, repo persistence.Repository, normalizer normalization.Normalizer, accountID string, item conversation, start, end time.Time, stats *counters) error {
	var anchor any
	var previous int64
	staleRecoveries := 0
	for pageIndex := 0; pageIndex < 500; pageIndex++ {
		var raw json.RawMessage
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if item.kind == "group" {
				raw, err = client.GetGroupHistory(ctx, item.id, 100, anchor)
			} else {
				raw, err = client.GetFriendMsgHistory(ctx, item.id, 100, anchor)
			}
			if err == nil {
				break
			}
			select {
			case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err != nil {
			if strings.Contains(err.Error(), "消息undefined不存在") {
				return nil
			}
			if anchor != nil && strings.Contains(err.Error(), "消息") && strings.Contains(err.Error(), "不存在") && staleRecoveries < 3 {
				// NapCat short message IDs are temporary anchors. Re-prime the
				// conversation from its latest page when one expires mid-run.
				anchor = nil
				previous = 0
				staleRecoveries++
				continue
			}
			return err
		}
		var wrapper page
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return err
		}
		if len(wrapper.Messages) == 0 {
			return nil
		}
		stats.pages.Add(1)
		var oldest int64
		for _, payload := range wrapper.Messages {
			stats.seen.Add(1)
			var msg message
			if err := decodeUseNumber(payload, &msg); err != nil {
				continue
			}
			if oldest == 0 || (msg.Time > 0 && msg.Time < oldest) {
				oldest = msg.Time
			}
			at := time.Unix(msg.Time, 0)
			if at.Before(start) || !at.Before(end) || strings.TrimSpace(textFromSegments(msg.Message)) == "" {
				continue
			}
			stats.matched.Add(1)
			endpoint := "get_friend_msg_history:" + item.id + ":message"
			if item.kind == "group" {
				endpoint = "get_group_msg_history:" + item.id + ":message"
			}
			rawID, err := repo.SaveRaw(ctx, accountID, "napcat_http", endpoint, payload)
			if err != nil {
				return err
			}
			peer := ""
			if item.kind != "group" {
				peer = item.id
			}
			if err := normalizer.ProcessRawMessage(ctx, accountID, rawID, payload, peer); err != nil {
				return err
			}
			stats.saved.Add(1)
		}
		if oldest > 0 && time.Unix(oldest, 0).Before(start) {
			return nil
		}
		var first message
		if err := decodeUseNumber(wrapper.Messages[0], &first); err != nil {
			return err
		}
		next, ok := int64Value(first.MessageSeq)
		if !ok || next <= 0 || next == previous {
			return nil
		}
		previous = next
		anchor = next
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("page limit reached")
}

func textFromSegments(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var segments []segment
	if json.Unmarshal(raw, &segments) != nil {
		return ""
	}
	var result strings.Builder
	for _, item := range segments {
		if item.Type != "text" {
			continue
		}
		if value, ok := item.Data["text"].(string); ok {
			result.WriteString(value)
		}
	}
	return result.String()
}

func countTextMessages(ctx context.Context, db *pgxpool.Pool, start, end time.Time) (int64, error) {
	var count int64
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM messages m
		WHERE m.sent_at >= $1 AND m.sent_at < $2 AND (
			(jsonb_typeof(m.message_segments)='string' AND btrim(m.message_segments #>> '{}') <> '') OR
			(jsonb_typeof(m.message_segments)='array' AND EXISTS (
				SELECT 1 FROM jsonb_array_elements(m.message_segments) seg
				WHERE seg->>'type'='text' AND btrim(COALESCE(seg->'data'->>'text','')) <> ''
			))
		)`, start, end).Scan(&count)
	return count, err
}

func exportJSONL(ctx context.Context, db *pgxpool.Pool, start, end time.Time, target string) error {
	rows, err := db.Query(ctx, `
		SELECT m.id::text, c.conversation_type, c.platform_conversation_id, COALESCE(c.name,''),
			COALESCE(pi.platform_user_id,''), COALESCE(p.display_name,''), m.sent_at,
			CASE WHEN jsonb_typeof(m.message_segments)='string' THEN m.message_segments #>> '{}'
			ELSE COALESCE((SELECT string_agg(seg->'data'->>'text','' ORDER BY ord)
				FROM jsonb_array_elements(m.message_segments) WITH ORDINALITY AS x(seg,ord)
				WHERE seg->>'type'='text'), '') END AS text
		FROM messages m
		JOIN conversations c ON c.id=m.conversation_id
		JOIN persons p ON p.id=m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE m.sent_at >= $1 AND m.sent_at < $2
		ORDER BY m.sent_at, m.id`, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for rows.Next() {
		var id, kind, conversationID, conversationName, senderQQ, senderName, text string
		var sentAt time.Time
		if err := rows.Scan(&id, &kind, &conversationID, &conversationName, &senderQQ, &senderName, &sentAt, &text); err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		if err := encoder.Encode(map[string]any{"id": id, "conversation_type": kind, "conversation_id": conversationID, "conversation_name": conversationName, "sender_qq": senderQQ, "sender_name": senderName, "sent_at": sentAt, "text": text}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func decodeUseNumber(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(dst)
}

func identifier(value any) string {
	if number, ok := int64Value(value); ok && number > 0 {
		return fmt.Sprint(number)
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	return text
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		result, err := typed.Int64()
		return result, err == nil
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case string:
		var result int64
		_, err := fmt.Sscan(typed, &result)
		return result, err == nil
	default:
		return 0, false
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
