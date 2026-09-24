package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

// botInstancePayload is the editable portion of a bot instance.
type botInstancePayload struct {
	Enabled          *bool           `json:"enabled"`
	Name             *string         `json:"name"`
	Persona          *string         `json:"persona"`
	ContextPrompt    *string         `json:"context_prompt"`
	CmdEnableReply   *string         `json:"cmd_enable_reply"`
	CmdDisableReply  *string         `json:"cmd_disable_reply"`
	FallbackReply    *string         `json:"fallback_reply"`
	MeowSound        *string         `json:"meow_sound"`
	CommandPrefix    *string         `json:"command_prefix"`
	Greeting         *string         `json:"greeting"`
	ReplyProbability *int            `json:"reply_probability"`
	Entertainment    json.RawMessage `json:"entertainment"`
	LLMProvider      *string         `json:"llm_provider"`
	LLMAPIBase       *string         `json:"llm_api_base"`
	LLMAPIKey        *string         `json:"llm_api_key"`
	LLMModel         *string         `json:"llm_model"`
	LLMTemperature   *float64        `json:"llm_temperature"`
	LLMMaxTokens     *int            `json:"llm_max_tokens"`
	MaxToolHops      *int            `json:"max_tool_hops"`
}

func (s *Server) botInstances(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT b.id::text,b.account_id::text,a.name,a.qq_uin,b.enabled,b.name,b.persona,
			COALESCE(b.context_prompt, ''),COALESCE(b.cmd_enable_reply, ''),COALESCE(b.cmd_disable_reply, ''),COALESCE(b.fallback_reply, ''),
			b.meow_sound,b.command_prefix,b.greeting,b.reply_probability,b.entertainment,
			b.llm_provider,b.llm_api_base,b.llm_api_key,b.llm_model,b.llm_temperature,b.llm_max_tokens,
			COALESCE(b.max_tool_hops, 10),
			(SELECT count(*) FROM bot_group_whitelist g WHERE g.bot_id=b.id AND g.enabled)
		FROM bot_instances b JOIN napcat_accounts a ON a.id=b.account_id
		ORDER BY a.name`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID                string          `json:"id"`
		AccountID         string          `json:"account_id"`
		AccountName       string          `json:"account_name"`
		AccountQQ         string          `json:"account_qq"`
		Enabled           bool            `json:"enabled"`
		Name              string          `json:"name"`
		Persona           string          `json:"persona"`
		ContextPrompt     string          `json:"context_prompt"`
		CmdEnableReply    string          `json:"cmd_enable_reply"`
		CmdDisableReply   string          `json:"cmd_disable_reply"`
		FallbackReply     string          `json:"fallback_reply"`
		MeowSound         string          `json:"meow_sound"`
		CommandPrefix     string          `json:"command_prefix"`
		Greeting          string          `json:"greeting"`
		ReplyProbability  int             `json:"reply_probability"`
		Entertainment     json.RawMessage `json:"entertainment"`
		LLMProvider       string          `json:"llm_provider"`
		LLMAPIBase        string          `json:"llm_api_base"`
		LLMAPIKey         string          `json:"llm_api_key"`
		LLMModel          string          `json:"llm_model"`
		LLMTemperature    float64         `json:"llm_temperature"`
		LLMMaxTokens      int             `json:"llm_max_tokens"`
		MaxToolHops       int             `json:"max_tool_hops"`
		WhitelistedGroups int             `json:"whitelisted_groups"`
	}
	items := make([]item, 0, 8)
	for rows.Next() {
		var it item
		var ent []byte
		if err := rows.Scan(&it.ID, &it.AccountID, &it.AccountName, &it.AccountQQ, &it.Enabled, &it.Name,
			&it.Persona, &it.ContextPrompt, &it.CmdEnableReply, &it.CmdDisableReply, &it.FallbackReply,
			&it.MeowSound, &it.CommandPrefix, &it.Greeting, &it.ReplyProbability, &ent,
			&it.LLMProvider, &it.LLMAPIBase, &it.LLMAPIKey, &it.LLMModel, &it.LLMTemperature, &it.LLMMaxTokens,
			&it.MaxToolHops,
			&it.WhitelistedGroups); err != nil {
			writeError(w, 500, err)
			return
		}
		if decrypted, decryptErr := secrets.Decrypt(it.LLMAPIKey, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
			it.LLMAPIKey = maskSecret(decrypted)
		} else {
			it.LLMAPIKey = maskSecret(it.LLMAPIKey)
		}
		it.Entertainment = ent
		items = append(items, it)
	}
	writeJSON(w, 200, map[string]any{"data": items, "total": len(items)})
}

func (s *Server) botCreateInstance(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.AccountID == "" {
		writeError(w, 400, errInvalid("account_id is required"))
		return
	}
	var exists bool
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM bot_instances WHERE account_id=$1)`, payload.AccountID).Scan(&exists); err != nil {
		writeError(w, 500, err)
		return
	}
	if exists {
		writeError(w, 409, errInvalid("this account already has a bot instance"))
		return
	}
	var id string
	err := s.Repo.DB.QueryRow(r.Context(), `
		INSERT INTO bot_instances(account_id) VALUES($1) RETURNING id::text`, payload.AccountID).Scan(&id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) botUpdateInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p botInstancePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, 400, errInvalid("invalid body"))
		return
	}
	if p.Entertainment != nil {
		var corpus []string
		if err := json.Unmarshal(p.Entertainment, &corpus); err != nil {
			writeError(w, 400, errInvalid("entertainment must be an array of strings"))
			return
		}
	}
	query := `UPDATE bot_instances SET updated_at=now()`
	params := []any{}
	idx := 1
	apply := func(field string, value any) {
		query += ", " + field + "=$" + strconv.Itoa(idx)
		params = append(params, value)
		idx++
	}
	if p.Enabled != nil {
		apply("enabled", *p.Enabled)
	}
	if p.Name != nil {
		apply("name", strings.TrimSpace(*p.Name))
	}
	if p.Persona != nil {
		apply("persona", strings.TrimSpace(*p.Persona))
	}
	if p.ContextPrompt != nil {
		apply("context_prompt", strings.TrimSpace(*p.ContextPrompt))
	}
	if p.CmdEnableReply != nil {
		apply("cmd_enable_reply", strings.TrimSpace(*p.CmdEnableReply))
	}
	if p.CmdDisableReply != nil {
		apply("cmd_disable_reply", strings.TrimSpace(*p.CmdDisableReply))
	}
	if p.FallbackReply != nil {
		apply("fallback_reply", strings.TrimSpace(*p.FallbackReply))
	}
	if p.MeowSound != nil {
		apply("meow_sound", strings.TrimSpace(*p.MeowSound))
	}
	if p.CommandPrefix != nil {
		apply("command_prefix", strings.TrimSpace(*p.CommandPrefix))
	}
	if p.Greeting != nil {
		apply("greeting", strings.TrimSpace(*p.Greeting))
	}
	if p.ReplyProbability != nil {
		prob := *p.ReplyProbability
		if prob < 0 {
			prob = 0
		}
		if prob > 100 {
			prob = 100
		}
		apply("reply_probability", prob)
	}
	if p.Entertainment != nil {
		query += ", entertainment=$" + strconv.Itoa(idx) + "::jsonb"
		params = append(params, string(p.Entertainment))
		idx++
	}
	if p.LLMProvider != nil {
		apply("llm_provider", strings.TrimSpace(*p.LLMProvider))
	}
	if p.LLMAPIBase != nil {
		apply("llm_api_base", strings.TrimSpace(*p.LLMAPIBase))
	}
	if p.LLMAPIKey != nil {
		key := strings.TrimSpace(*p.LLMAPIKey)
		if key != "" && !strings.Contains(key, "***") {
			protected, protectErr := secrets.Encrypt(key, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY"))
			if protectErr != nil {
				writeError(w, 500, protectErr)
				return
			}
			key = protected
		}
		if key != "" && !strings.Contains(key, "***") {
			apply("llm_api_key", key)
		}
	}
	if p.LLMModel != nil {
		apply("llm_model", strings.TrimSpace(*p.LLMModel))
	}
	if p.LLMTemperature != nil {
		apply("llm_temperature", *p.LLMTemperature)
	}
	if p.LLMMaxTokens != nil {
		apply("llm_max_tokens", *p.LLMMaxTokens)
	}
	if p.MaxToolHops != nil {
		hops := *p.MaxToolHops
		if hops < 1 {
			hops = 1
		}
		if hops > 25 {
			hops = 25
		}
		apply("max_tool_hops", hops)
	}
	if len(params) == 0 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	query += " WHERE id=$" + strconv.Itoa(idx) + "::uuid"
	params = append(params, id)
	if _, err := s.Repo.DB.Exec(r.Context(), query, params...); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) botDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.Repo.DB.Exec(r.Context(), `DELETE FROM bot_instances WHERE id=$1`, id); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) botWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT g.id::text,g.platform_group_id,g.enabled,g.created_at,
			COALESCE(gr.group_name,''),COALESCE((SELECT count(*) FROM group_memberships gm WHERE gm.group_id=gr.id),0)
		FROM bot_group_whitelist g LEFT JOIN "groups" gr ON gr.platform_group_id=g.platform_group_id
		WHERE g.bot_id=$1 ORDER BY g.created_at DESC`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        string `json:"id"`
		GroupID   string `json:"group_id"`
		Enabled   bool   `json:"enabled"`
		GroupName string `json:"group_name"`
		Members   int    `json:"members"`
	}
	items := make([]item, 0, 32)
	for rows.Next() {
		var it item
		var createdAt interface{}
		if err := rows.Scan(&it.ID, &it.GroupID, &it.Enabled, &createdAt, &it.GroupName, &it.Members); err != nil {
			writeError(w, 500, err)
			return
		}
		items = append(items, it)
	}
	writeJSON(w, 200, map[string]any{"data": items, "total": len(items)})
}

func (s *Server) botAddWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		GroupID string `json:"group_id"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.GroupID) == "" {
		writeError(w, 400, errInvalid("group_id is required"))
		return
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	_, err := s.Repo.DB.Exec(r.Context(), `
		INSERT INTO bot_group_whitelist(bot_id,platform_group_id,enabled) VALUES($1,$2,$3)
		ON CONFLICT(bot_id,platform_group_id) DO UPDATE SET enabled=EXCLUDED.enabled`,
		id, strings.TrimSpace(payload.GroupID), enabled)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) botDeleteWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	groupID := chi.URLParam(r, "group_id")
	if _, err := s.Repo.DB.Exec(r.Context(), `DELETE FROM bot_group_whitelist WHERE bot_id=$1 AND platform_group_id=$2`, id, groupID); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) botUsersWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT id::text, user_qq, enabled, created_at
		FROM bot_user_whitelist
		WHERE bot_id=$1 ORDER BY created_at DESC`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        string `json:"id"`
		UserQQ    string `json:"user_qq"`
		Enabled   bool   `json:"enabled"`
		CreatedAt string `json:"created_at"`
	}
	items := make([]item, 0, 32)
	for rows.Next() {
		var it item
		var t time.Time
		if err := rows.Scan(&it.ID, &it.UserQQ, &it.Enabled, &t); err != nil {
			writeError(w, 500, err)
			return
		}
		it.CreatedAt = t.Format(time.RFC3339)
		items = append(items, it)
	}
	writeJSON(w, 200, map[string]any{"data": items, "total": len(items)})
}

func (s *Server) botAddUserWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		UserQQ  string `json:"user_qq"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.UserQQ) == "" {
		writeError(w, 400, errInvalid("user_qq is required"))
		return
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	_, err := s.Repo.DB.Exec(r.Context(), `
		INSERT INTO bot_user_whitelist(bot_id,user_qq,enabled) VALUES($1,$2,$3)
		ON CONFLICT(bot_id,user_qq) DO UPDATE SET enabled=EXCLUDED.enabled`,
		id, strings.TrimSpace(payload.UserQQ), enabled)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) botDeleteUserWhitelist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userQQ := chi.URLParam(r, "user_qq")
	if _, err := s.Repo.DB.Exec(r.Context(), `DELETE FROM bot_user_whitelist WHERE bot_id=$1 AND user_qq=$2`, id, userQQ); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) botAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	var total int
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM bot_audit_log`).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT l.id::text,COALESCE(l.account_id::text,''),l.platform_group_id,l.user_qq,l.trigger_type,l.command,l.reply,
			COALESCE(l.status, 'completed'), COALESCE(l.latency_ms, 0), COALESCE(l.tools_called, '{}'), COALESCE(l.error_message, ''),
			COALESCE(l.thought_trace, '[]'::jsonb),
			l.created_at, COALESCE(a.name,''),COALESCE(a.qq_uin,''),COALESCE(gr.group_name,'')
		FROM bot_audit_log l
		LEFT JOIN napcat_accounts a ON a.id=l.account_id
		LEFT JOIN "groups" gr ON gr.platform_group_id=l.platform_group_id
		ORDER BY l.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID           string          `json:"id"`
		AccountID    string          `json:"account_id"`
		GroupID      string          `json:"group_id"`
		GroupName    string          `json:"group_name"`
		UserQQ       string          `json:"user_qq"`
		TriggerType  string          `json:"trigger_type"`
		Command      string          `json:"command"`
		Reply        string          `json:"reply"`
		Status       string          `json:"status"`
		LatencyMs    int             `json:"latency_ms"`
		ToolsCalled  []string        `json:"tools_called"`
		ErrorMessage string          `json:"error_message"`
		ThoughtTrace json.RawMessage `json:"thought_trace"`
		CreatedAt    string          `json:"created_at"`
		AccountName  string          `json:"account_name"`
		AccountQQ    string          `json:"account_qq"`
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		var t interface{}
		var tools []string
		var traceBytes []byte
		if err := rows.Scan(&it.ID, &it.AccountID, &it.GroupID, &it.UserQQ, &it.TriggerType, &it.Command, &it.Reply,
			&it.Status, &it.LatencyMs, &tools, &it.ErrorMessage,
			&traceBytes,
			&t, &it.AccountName, &it.AccountQQ, &it.GroupName); err != nil {
			writeError(w, 500, err)
			return
		}
		it.ToolsCalled = tools
		if len(traceBytes) > 0 {
			it.ThoughtTrace = json.RawMessage(traceBytes)
		} else {
			it.ThoughtTrace = json.RawMessage("[]")
		}
		if ts, ok := t.(time.Time); ok {
			it.CreatedAt = ts.Format(time.RFC3339)
		} else if t != nil {
			it.CreatedAt = fmt.Sprint(t)
		}
		items = append(items, it)
	}
	writeJSON(w, 200, map[string]any{"data": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) botListSessions(w http.ResponseWriter, r *http.Request) {
	if s.Bot == nil || s.Bot.Memory == nil {
		writeJSON(w, 200, map[string]any{"data": []any{}})
		return
	}
	id := chi.URLParam(r, "id")
	sessions := s.Bot.Memory.ListSummaries(id, true)
	writeJSON(w, 200, map[string]any{
		"data":  sessions,
		"total": len(sessions),
	})
}

func (s *Server) botInjectSessionMemory(w http.ResponseWriter, r *http.Request) {
	if s.Bot == nil || s.Bot.Memory == nil {
		writeError(w, 500, fmt.Errorf("memory service not available"))
		return
	}
	var in struct {
		Key     string `json:"key"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Key == "" || in.Content == "" {
		writeError(w, 400, fmt.Errorf("key and content are required"))
		return
	}
	if in.Role == "" {
		in.Role = "assistant"
	}
	s.Bot.Memory.InjectMessage(in.Key, in.Role, in.Content)
	writeJSON(w, 200, map[string]any{"status": "injected"})
}

func (s *Server) botChatTest(w http.ResponseWriter, r *http.Request) {
	if s.Bot == nil {
		writeError(w, 500, fmt.Errorf("bot service not initialized"))
		return
	}
	id := chi.URLParam(r, "id")
	var in struct {
		Message string `json:"message"`
		UserID  string `json:"user_id"`
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	text := strings.TrimSpace(in.Message)
	if text == "" {
		writeJSON(w, 400, map[string]string{"error": "message is required"})
		return
	}
	reply, err := s.Bot.ChatTest(r.Context(), id, in.UserID, in.GroupID, text)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"reply": reply,
	})
}

func (s *Server) botClearSession(w http.ResponseWriter, r *http.Request) {
	if s.Bot == nil {
		writeError(w, 500, fmt.Errorf("bot service not initialized"))
		return
	}
	id := chi.URLParam(r, "id")
	key := r.URL.Query().Get("key")
	if key != "" {
		s.Bot.Memory.Clear(key)
		writeJSON(w, 200, map[string]any{"status": "cleared", "key": key})
		return
	}
	userID := r.URL.Query().Get("user_id")
	groupID := r.URL.Query().Get("group_id")
	if userID == "" {
		userID = "web_tester"
	}
	s.Bot.ClearSession(id, groupID, userID)
	writeJSON(w, 200, map[string]any{"status": "cleared"})
}

func errInvalid(message string) error {
	return fmt.Errorf("%s", message)
}
