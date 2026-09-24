package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/mcp"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

func TestBotRealtimeFlow(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live bot tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE name IN ('bot-test', 'llm-test')`)
		pool.Close()
	}()
	_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE name IN ('bot-test', 'llm-test')`)

	// NapCat HTTP stub records send_msg calls.
	sent := make(chan map[string]any, 8)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/send_") {
			var params map[string]any
			_ = json.NewDecoder(r.Body).Decode(&params)
			sent <- params
			_, _ = w.Write([]byte(`{"status":"ok","retcode":0,"data":{"message_id":123}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer stub.Close()

	llmStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "喵~ 老吴在！",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer llmStub.Close()

	var accountID, botID string
	groupID := fmt.Sprintf("1000%d", time.Now().UnixNano()%100000)
	err = pool.QueryRow(ctx, `
		INSERT INTO napcat_accounts(name, qq_uin, ws_url, http_url)
		VALUES('bot-test','9bot','', $1) RETURNING id::text`, stub.URL).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE id=$1`, accountID)
	})

	err = pool.QueryRow(ctx, `INSERT INTO bot_instances(account_id, enabled, llm_api_base, llm_api_key, llm_model) VALUES($1, true, $2, 'test-key', 'mock-model') RETURNING id::text`, accountID, llmStub.URL).Scan(&botID)
	if err != nil {
		t.Fatalf("insert instance: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bot_instances WHERE id=$1`, botID)
	})

	service := NewService(pool, persistence.Repository{DB: pool}, nil)
	service.limit = time.Millisecond

	rawMessage := func(text, userQQ string) []byte {
		payload, _ := json.Marshal(map[string]any{
			"post_type": "message", "message_type": "group", "group_id": groupID,
			"user_id": userQQ, "message_id": 777, "raw_message": text,
			"sender": map[string]any{"nickname": "测试员"},
			"message": []any{map[string]any{"type": "text", "data": map[string]any{"text": text}}},
		})
		return payload
	}

	// 1. Non-whitelisted group: bot must stay silent.
	service.HandleRealtime(accountID, rawMessage("/喵", "12345"))
	select {
	case <-sent:
		t.Fatalf("bot replied in a non-whitelisted group")
	case <-time.After(300 * time.Millisecond):
	}

	// 2. Whitelist the group, then a command should be answered.
	if _, err := pool.Exec(ctx, `INSERT INTO bot_group_whitelist(bot_id, platform_group_id) VALUES($1,$2)`, botID, groupID); err != nil {
		t.Fatalf("insert whitelist: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bot_group_whitelist WHERE bot_id=$1`, botID)
	})

	service.HandleRealtime(accountID, rawMessage("@9bot 你好", "12345"))
	select {
	case params := <-sent:
		segments, _ := json.Marshal(params["message"])
		if !strings.Contains(string(segments), "老吴") && !strings.Contains(string(segments), "喵") {
			t.Fatalf("unexpected reply segments: %s", segments)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("bot did not reply to @9bot in whitelisted group")
	}

	// 3. Audit row must exist for the reply.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var auditCount int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM bot_audit_log WHERE bot_id=$1`, botID).Scan(&auditCount); err == nil && auditCount >= 1 {
			break
		}
		if time.Now().After(deadline) {
			var auditCount int
			_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM bot_audit_log WHERE bot_id=$1`, botID).Scan(&auditCount)
			t.Fatalf("expected audit rows, got %d", auditCount)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bot_audit_log WHERE bot_id=$1`, botID)
	})

	// 4. The account's own messages must never trigger a reply (echo guard).
	service.HandleRealtime(accountID, rawMessage("/喵", "9bot"))
	select {
	case <-sent:
		t.Fatalf("bot replied to its own message")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestParseMessage(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"post_type": "message", "message_type": "group", "group_id": 111, "user_id": 222,
		"message_id": 333, "raw_message": "hi",
		"message": []any{
			map[string]any{"type": "at", "data": map[string]any{"qq": "111"}},
			map[string]any{"type": "text", "data": map[string]any{"text": " 查人 10000001"}},
		},
	})
	msg, ok := parseMessage(raw)
	if !ok {
		t.Fatalf("expected parse ok")
	}
	if msg.Private || msg.GroupID != "111" || msg.UserID != "222" || msg.MessageID != "333" {
		t.Fatalf("unexpected message: %+v", msg)
	}
	if !strings.Contains(msg.Text, "@111") || !strings.Contains(msg.Text, "查人 10000001") {
		t.Fatalf("text = %q", msg.Text)
	}
}

func TestBotLLMNLUFlow(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE name IN ('bot-test', 'llm-test')`)
		pool.Close()
	}()
	_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE name IN ('bot-test', 'llm-test')`)

	// Mock OpenAI API server simulating tool calling
	step := 0
	llmStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if step == 0 {
			step++
			// Step 1: LLM decides to call query_person
			resp := map[string]any{
				"choices": []any{
					map[string]any{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []any{
								map[string]any{
									"id":   "call_1",
									"type": "function",
									"function": map[string]any{
										"name":      "sra_person_dossier",
										"arguments": `{"qq":"10000001"}`,
									},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		// Step 2: LLM receives tool result and produces natural reply
		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "查到啦！这是群老吴的资料喵~",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer llmStub.Close()

	sent := make(chan map[string]any, 8)
	napcatStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/send_") {
			var params map[string]any
			_ = json.NewDecoder(r.Body).Decode(&params)
			sent <- params
			_, _ = w.Write([]byte(`{"status":"ok","retcode":0,"data":{"message_id":999}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer napcatStub.Close()

	var accountID, botID string
	groupID := fmt.Sprintf("2000%d", time.Now().UnixNano()%100000)
	err = pool.QueryRow(ctx, `
		INSERT INTO napcat_accounts(name, qq_uin, ws_url, http_url)
		VALUES('llm-test','9llm','', $1) RETURNING id::text`, napcatStub.URL).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM napcat_accounts WHERE id=$1`, accountID)
	})

	err = pool.QueryRow(ctx, `
		INSERT INTO bot_instances(account_id, enabled, llm_api_base, llm_api_key, llm_model)
		VALUES($1, true, $2, 'test-key', 'mock-model') RETURNING id::text`, accountID, llmStub.URL).Scan(&botID)
	if err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bot_instances WHERE id=$1`, botID)
	})

	_, _ = pool.Exec(ctx, `INSERT INTO bot_group_whitelist(bot_id, platform_group_id) VALUES($1,$2)`, botID, groupID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bot_group_whitelist WHERE bot_id=$1`, botID)
	})

	repo := persistence.Repository{DB: pool}
	service := NewService(pool, repo, nil)
	service.SetMCP(&mcp.HandlerRegistry{DB: pool, Repo: &repo})
	service.limit = time.Millisecond

	raw, _ := json.Marshal(map[string]any{
		"post_type": "message", "message_type": "group", "group_id": groupID,
		"user_id": "12345", "message_id": 888, "raw_message": "@9llm 帮我看看10000001是谁",
		"sender": map[string]any{"nickname": "提问者"},
		"message": []any{
			map[string]any{"type": "at", "data": map[string]any{"qq": "9llm"}},
			map[string]any{"type": "text", "data": map[string]any{"text": " 帮我看看10000001是谁"}},
		},
	})

	service.HandleRealtime(accountID, raw)

	select {
	case params := <-sent:
		segments, _ := json.Marshal(params["message"])
		if !strings.Contains(string(segments), "群老吴") {
			t.Fatalf("expected reply to contain tool result synthesis, got: %s", segments)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for LLM bot reply")
	}

	// Verify session memory has recorded the turn (group-level shared context)
	sessionKey := fmt.Sprintf("g:%s:%s", botID, groupID)
	history := service.Memory.GetHistory(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected 2 history messages (user+assistant), got %d", len(history))
	}
	if history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("unexpected history roles: %+v", history)
	}
}

func TestBotSessionMemoryIsolation(t *testing.T) {
	mem := NewMemory(time.Minute, 10)
	mem.Append("user1", "你好", "你好呀喵")
	mem.Append("user2", "我是李四", "你好李四喵")

	h1 := mem.GetHistory("user1")
	h2 := mem.GetHistory("user2")

	if len(h1) != 2 || h1[0].Content != "你好" {
		t.Fatalf("user1 history mismatch: %+v", h1)
	}
	if len(h2) != 2 || h2[0].Content != "我是李四" {
		t.Fatalf("user2 history mismatch: %+v", h2)
	}
}

func TestAntiLoopProtection(t *testing.T) {
	service := NewService(nil, persistence.Repository{}, nil)
	service.markBotSent("mid-999", "这是机器人的自动回复喵")

	if !service.isBotSent("mid-999", "") {
		t.Fatalf("expected mid-999 to be recognized as bot sent")
	}
	if !service.isBotSent("", "这是机器人的自动回复喵") {
		t.Fatalf("expected text to be recognized as bot sent")
	}
	if service.isBotSent("mid-other", "用户发的人类消息") {
		t.Fatalf("expected human message not to be recognized as bot sent")
	}
}

func TestControlCommandsParsing(t *testing.T) {
	cases := []struct {
		input     string
		botName   string
		expAction string
		expCtrl   bool
	}{
		{"/老吴 开", "群老吴", "enable", true},
		{"/老吴 关", "群老吴", "disable", true},
		{"/老吴开", "群老吴", "enable", true},
		{"/老吴关", "群老吴", "disable", true},
		{"/老吴 开启", "群老吴", "enable", true},
		{"/老吴 关闭", "群老吴", "disable", true},
		{"/老吴 on", "群老吴", "enable", true},
		{"/老吴 off", "群老吴", "disable", true},
		{"/群老吴 启动", "群老吴", "enable", true},
		{"/群老吴 停用", "群老吴", "disable", true},
		{"老吴开门", "群老吴", "", false},
		{"@群老吴 你好", "群老吴", "", false},
	}

	for _, c := range cases {
		act, ctrl := isControlCommand(c.input, c.botName)
		if ctrl != c.expCtrl || act != c.expAction {
			t.Errorf("isControlCommand(%q, %q) = (%q, %v); expected (%q, %v)",
				c.input, c.botName, act, ctrl, c.expAction, c.expCtrl)
		}
	}
}


