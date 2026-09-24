package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/mcp"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

var (
	cqImageURLRegex     = regexp.MustCompile(`\[CQ:image,[^\]]*url=([^,\]]+)[^\]]*\]`)
	cqImageGenericRegex = regexp.MustCompile(`\[CQ:image,[^\]]*\]`)
	cqReplyRegex        = regexp.MustCompile(`\[CQ:reply,[^\]]*id=([0-9\-]+)[^\]]*\]`)
)

// ThoughtStep records one incremental thinking or tool execution step for live auditing and streaming.
type ThoughtStep struct {
	Hop           int    `json:"hop"`
	Type          string `json:"type"` // "thinking", "tool_call", "tool_result"
	Tool          string `json:"tool,omitempty"`
	Arguments     string `json:"arguments,omitempty"`
	Thought       string `json:"thought,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	LatencyMs     int    `json:"latency_ms,omitempty"`
	Time          string `json:"time"`
}

func formatCQImages(s string) string {
	s = cqImageURLRegex.ReplaceAllString(s, "[附带图片URL: $1]")
	s = cqImageGenericRegex.ReplaceAllString(s, "[附带图片]")
	return s
}

// Instance is the bot configuration for one NapCat account.
type Instance struct {
	ID               string          `json:"id"`
	AccountID        string          `json:"account_id"`
	Enabled          bool            `json:"enabled"`
	Name             string          `json:"name"`
	Persona          string          `json:"persona"`
	ContextPrompt    string          `json:"context_prompt"`
	CmdEnableReply   string          `json:"cmd_enable_reply"`
	CmdDisableReply  string          `json:"cmd_disable_reply"`
	FallbackReply    string          `json:"fallback_reply"`
	MeowSound        string          `json:"meow_sound"`
	CommandPrefix    string          `json:"command_prefix"`
	Greeting         string          `json:"greeting"`
	ReplyProbability int             `json:"reply_probability"`
	Entertainment    json.RawMessage `json:"entertainment"`
	LLMProvider      string          `json:"llm_provider"`
	LLMAPIBase       string          `json:"llm_api_base"`
	LLMAPIKey        string          `json:"llm_api_key"`
	LLMModel         string          `json:"llm_model"`
	LLMTemperature   float64         `json:"llm_temperature"`
	LLMMaxTokens     int             `json:"llm_max_tokens"`
	MaxToolHops      int             `json:"max_tool_hops"`
}

// message is a parsed realtime chat message the bot may respond to.
type message struct {
	MessageID   string
	GroupID     string
	UserID      string
	Nickname    string
	Text        string
	Private     bool
	QuotedMsgID string // message_id of the quoted/replied-to message, if any
}

// Service dispatches realtime messages to bot instances and sends replies.
type Service struct {
	DB     *pgxpool.Pool
	Repo   persistence.Repository
	MCP    *mcp.HandlerRegistry
	Memory *Memory
	Logger *slog.Logger

	mu    sync.Mutex
	last  map[string]time.Time // botID:groupID -> last reply time
	limit time.Duration

	sentMu    sync.Mutex
	sentMIDs  map[string]time.Time // outgoing message_id -> timestamp
	sentTexts map[string]time.Time // outgoing reply text -> timestamp
}

func NewService(db *pgxpool.Pool, repo persistence.Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		DB:        db,
		Repo:      repo,
		Memory:    NewMemory(30*time.Minute, 20),
		Logger:    logger,
		last:      map[string]time.Time{},
		limit:     3 * time.Second,
		sentMIDs:  map[string]time.Time{},
		sentTexts: map[string]time.Time{},
	}
}

func (s *Service) SetMCP(registry *mcp.HandlerRegistry) {
	s.MCP = registry
}

func (s *Service) markBotSent(messageID, text string) {
	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	now := time.Now()
	if s.sentMIDs == nil {
		s.sentMIDs = make(map[string]time.Time)
	}
	if s.sentTexts == nil {
		s.sentTexts = make(map[string]time.Time)
	}
	if messageID != "" && messageID != "<nil>" {
		s.sentMIDs[messageID] = now
	}
	clean := strings.TrimSpace(text)
	if clean != "" {
		s.sentTexts[clean] = now
	}

	// Evict items older than 5 minutes
	for k, v := range s.sentMIDs {
		if now.Sub(v) > 5*time.Minute {
			delete(s.sentMIDs, k)
		}
	}
	for k, v := range s.sentTexts {
		if now.Sub(v) > 5*time.Minute {
			delete(s.sentTexts, k)
		}
	}
}

func (s *Service) isBotSent(messageID, text string) bool {
	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	now := time.Now()
	if messageID != "" && messageID != "<nil>" {
		if t, ok := s.sentMIDs[messageID]; ok && now.Sub(t) < 5*time.Minute {
			return true
		}
	}
	clean := strings.TrimSpace(text)
	if clean != "" {
		for sentText, t := range s.sentTexts {
			if now.Sub(t) < 5*time.Minute {
				if clean == sentText || strings.Contains(clean, sentText) || (len(clean) > 10 && strings.Contains(sentText, clean)) {
					return true
				}
			}
		}
	}
	return false
}

// HandleRealtime is wired into the NapCat manager and runs on every WS event.
// It returns immediately; all work happens in a detached goroutine so message
// ingestion is never blocked.
func (s *Service) HandleRealtime(accountID string, raw json.RawMessage) {
	msg, ok := parseMessage(raw)
	if !ok || msg.Text == "" {
		return
	}
	botQQ := s.accountQQ(accountID)
	// Anti-loop protection: drop only if sender IS the bot AND we can confirm it's our own sent message.
	// Do NOT drop just because text starts with "[回复]" — that would block all quoted-message @mentions
	// from other users (they trigger @bot inside a reply bubble, which starts with "[回复]").
	if msg.UserID == botQQ {
		if s.isBotSent(msg.MessageID, msg.Text) {
			s.Logger.Debug("dropping bot outgoing reply echo", "mid", msg.MessageID)
			return
		}
		inst, err := s.loadInstance(context.Background(), accountID)
		if err != nil || inst == nil || !inst.Enabled {
			return
		}
		_, isControl := isControlCommand(msg.Text, inst.Name)
		isMention := strings.Contains(msg.Text, "@"+botQQ) ||
			strings.Contains(msg.Text, "@"+inst.Name) ||
			strings.Contains(msg.Text, "@老吴") ||
			strings.Contains(msg.Text, "@群老吴") ||
			strings.Contains(msg.Text, "[CQ:at,qq="+botQQ+"]") ||
			strings.HasPrefix(msg.Text, "老吴") ||
			strings.HasPrefix(msg.Text, "群老吴") ||
			(inst.Name != "" && strings.HasPrefix(msg.Text, inst.Name))
		if !isControl && !isMention {
			return
		}
		s.Logger.Info("bot received message from bot owner", "group_id", msg.GroupID, "user_id", msg.UserID, "text", msg.Text, "private", msg.Private)
		go s.handle(context.Background(), accountID, *inst, msg)
		return
	}

	inst, err := s.loadInstance(context.Background(), accountID)
	if err != nil || inst == nil || !inst.Enabled {
		return
	}
	s.Logger.Info("bot received message", "group_id", msg.GroupID, "user_id", msg.UserID, "text", msg.Text, "private", msg.Private)
	go s.handle(context.Background(), accountID, *inst, msg)
}

func (s *Service) handle(ctx context.Context, accountID string, inst Instance, msg message) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("bot handler panic", "error", r)
		}
	}()

	botQQ := s.accountQQ(inst.AccountID)
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// 1. Master switch command: /老吴 开 or /老吴 关 (ONLY bot owner can execute)
	action, isControl := isControlCommand(text, inst.Name)
	if isControl {
		if msg.UserID == botQQ {
			s.handleControlCommand(ctx, accountID, inst, msg, action, text)
		} else {
			s.Logger.Info("control command rejected (not owner)", "user_id", msg.UserID, "bot_qq", botQQ, "action", action)
			_ = s.sendReply(ctx, accountID, inst, msg, "喵？这个指令只有老吴的主人才能用哦~")
		}
		return
	}

	// 2. Strict Whitelist + Mention Guard
	if !s.guard(ctx, inst, msg) {
		return
	}
	if !s.rateOK(inst.ID, msg) {
		return
	}

	// Strip @-mention or name prefix from text for natural LLM comprehension
	cleanText := text
	if botQQ != "" {
		cleanText = strings.ReplaceAll(cleanText, "@"+botQQ, "")
		cleanText = strings.ReplaceAll(cleanText, "[CQ:at,qq="+botQQ+"]", "")
	}
	if inst.Name != "" {
		cleanText = strings.ReplaceAll(cleanText, "@"+inst.Name, "")
	}
	cleanText = strings.ReplaceAll(cleanText, "@老吴", "")
	cleanText = strings.ReplaceAll(cleanText, "@群老吴", "")
	cleanText = strings.ReplaceAll(cleanText, "@全体成员", "")
	cleanText = strings.ReplaceAll(cleanText, "[CQ:at,qq=all]", "")
	cleanText = strings.TrimSpace(cleanText)
	if strings.HasPrefix(cleanText, "群老吴") {
		cleanText = strings.TrimPrefix(cleanText, "群老吴")
	} else if strings.HasPrefix(cleanText, "老吴") {
		cleanText = strings.TrimPrefix(cleanText, "老吴")
	} else if inst.Name != "" && strings.HasPrefix(cleanText, inst.Name) {
		cleanText = strings.TrimPrefix(cleanText, inst.Name)
	}
	cleanText = strings.TrimLeft(cleanText, " ,，:：!！?？ \t\r\n")
	cleanText = formatCQImages(cleanText)
	cleanText = strings.TrimSpace(cleanText)
	if strings.HasPrefix(cleanText, "[回复]") {
		cleanText = strings.TrimPrefix(cleanText, "[回复]")
		cleanText = strings.TrimSpace(cleanText)
	}

	// If this message quotes another message, fetch the original to extract any images in it.
	// This is necessary because NapCat only delivers the reply segment's data.id; the actual
	// content (especially image URLs) of the quoted message is not inlined in the WS event.
	if msg.QuotedMsgID != "" {
		if extra := s.fetchQuotedContent(ctx, accountID, msg.QuotedMsgID); extra != "" {
			if cleanText != "" {
				cleanText = cleanText + "\n" + extra
			} else {
				cleanText = extra
			}
		}
	}

	if cleanText == "" {
		cleanText = "你好"
	}
	msg.Text = cleanText

	// Record start time and insert live processing state into audit log
	start := time.Now()
	auditID := s.startAudit(ctx, inst, msg, "llm_nlu", text)

	// 100% Pure Natural Language Understanding via LLM + MCP Tools
	ctx = withToolGroupScope(ctx, msg.GroupID)
	reply, toolsUsed, thoughtTrace, err := s.generateLLMReplyWithTools(ctx, inst, msg, auditID)
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		s.Logger.Warn("bot llm reasoning failed", "account_id", accountID, "error", err)
		fallback := inst.FallbackReply
		if fallback == "" {
			fallback = "喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~"
		}
		fallback = strings.ReplaceAll(fallback, "{name}", inst.Name)
		_ = s.sendReply(ctx, accountID, inst, msg, fallback)
		s.finishAuditWithTrace(ctx, auditID, "failed", fallback, toolsUsed, latency, err.Error(), thoughtTrace)
		return
	}
	if reply == "" {
		fallback := inst.FallbackReply
		if fallback == "" {
			fallback = "喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~"
		}
		fallback = strings.ReplaceAll(fallback, "{name}", inst.Name)
		_ = s.sendReply(ctx, accountID, inst, msg, fallback)
		s.finishAuditWithTrace(ctx, auditID, "failed", fallback, toolsUsed, latency, "empty reply generated", thoughtTrace)
		return
	}

	if err := s.sendReply(ctx, accountID, inst, msg, reply); err != nil {
		s.Logger.Warn("bot reply failed", "account_id", accountID, "error", err)
		s.finishAuditWithTrace(ctx, auditID, "failed", reply, toolsUsed, latency, "send reply failed: "+err.Error(), thoughtTrace)
		return
	}
	s.finishAuditWithTrace(ctx, auditID, "completed", reply, toolsUsed, latency, "", thoughtTrace)
	s.touch(inst.ID, msg)
}

func (s *Service) handleControlCommand(ctx context.Context, accountID string, inst Instance, msg message, action, rawText string) {
	start := time.Now()
	auditID := s.startAudit(ctx, inst, msg, "command_control", rawText)

	var reply, typeDesc, targetDesc string
	var writeErr error
	if action == "enable" {
		if msg.Private {
			typeDesc = "私聊"
			targetDesc = msg.UserID
			_, writeErr = s.DB.Exec(ctx, `INSERT INTO bot_user_whitelist (bot_id, user_qq, enabled)
				VALUES ($1, $2, true)
				ON CONFLICT (bot_id, user_qq) DO UPDATE SET enabled = true`, inst.ID, msg.UserID)
			reply = inst.CmdEnableReply
			if reply == "" {
				reply = "老吴~ 喵！当前{type}【{target}】已加入白名单并开启服务！"
			}
		} else {
			typeDesc = "群聊"
			targetDesc = msg.GroupID
			_, writeErr = s.DB.Exec(ctx, `INSERT INTO bot_group_whitelist (bot_id, platform_group_id, enabled)
				VALUES ($1, $2, true)
				ON CONFLICT (bot_id, platform_group_id) DO UPDATE SET enabled = true`, inst.ID, msg.GroupID)
			reply = inst.CmdEnableReply
			if reply == "" {
				reply = "老吴~ 喵！当前{type}【{target}】已加入白名单并开启服务！"
			}
		}
	} else if action == "disable" {
		if msg.Private {
			typeDesc = "私聊"
			targetDesc = msg.UserID
			_, writeErr = s.DB.Exec(ctx, `INSERT INTO bot_user_whitelist (bot_id, user_qq, enabled)
				VALUES ($1, $2, false)
				ON CONFLICT (bot_id, user_qq) DO UPDATE SET enabled = false`, inst.ID, msg.UserID)
			reply = inst.CmdDisableReply
			if reply == "" {
				reply = "老吴~ 喵！当前{type}【{target}】已关闭服务，老吴去睡觉啦~ Zzz"
			}
		} else {
			typeDesc = "群聊"
			targetDesc = msg.GroupID
			_, writeErr = s.DB.Exec(ctx, `INSERT INTO bot_group_whitelist (bot_id, platform_group_id, enabled)
				VALUES ($1, $2, false)
				ON CONFLICT (bot_id, platform_group_id) DO UPDATE SET enabled = false`, inst.ID, msg.GroupID)
			reply = inst.CmdDisableReply
			if reply == "" {
				reply = "老吴~ 喵！当前{type}【{target}】已关闭服务，老吴去睡觉啦~ Zzz"
			}
		}
	}

	if writeErr != nil {
		s.finishAudit(ctx, auditID, "failed", "", []string{"whitelist_control"}, int(time.Since(start).Milliseconds()), writeErr.Error())
		s.Logger.Warn("whitelist update failed", "error", writeErr)
		return
	}
	reply = strings.ReplaceAll(reply, "{name}", inst.Name)
	reply = strings.ReplaceAll(reply, "{type}", typeDesc)
	reply = strings.ReplaceAll(reply, "{target}", targetDesc)

	_ = s.sendReply(ctx, accountID, inst, msg, reply)
	latency := int(time.Since(start).Milliseconds())
	s.finishAudit(ctx, auditID, "completed", reply, []string{"whitelist_control"}, latency, "")
	s.touch(inst.ID, msg)
}

func isControlCommand(text, botName string) (action string, isControl bool) {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "/")
	t = strings.TrimSpace(t)

	// Clean bot name prefix if present
	if botName != "" {
		t = strings.TrimPrefix(t, botName)
		t = strings.TrimSpace(t)
	}
	if strings.HasPrefix(t, "老吴") {
		t = strings.TrimPrefix(t, "老吴")
		t = strings.TrimSpace(t)
	} else if strings.HasPrefix(t, "群老吴") {
		t = strings.TrimPrefix(t, "群老吴")
		t = strings.TrimSpace(t)
	}

	switch strings.ToLower(t) {
	case "开", "开启", "启动", "上线", "激活", "on", "start", "enable":
		return "enable", true
	case "关", "关闭", "下线", "休眠", "禁用", "停用", "off", "stop", "disable":
		return "disable", true
	}
	return "", false
}

func (s *Service) isWhitelistedGroup(ctx context.Context, botID, groupID string) bool {
	if groupID == "" {
		return false
	}
	var enabled bool
	err := s.DB.QueryRow(ctx, `SELECT enabled FROM bot_group_whitelist WHERE bot_id=$1 AND platform_group_id=$2`, botID, groupID).Scan(&enabled)
	return err == nil && enabled
}

func (s *Service) isWhitelistedUser(ctx context.Context, botID, userQQ string) bool {
	if userQQ == "" {
		return false
	}
	var enabled bool
	err := s.DB.QueryRow(ctx, `SELECT enabled FROM bot_user_whitelist WHERE bot_id=$1 AND user_qq=$2`, botID, userQQ).Scan(&enabled)
	return err == nil && enabled
}

// guard decides whether this message should be answered by the bot.
func (s *Service) guard(ctx context.Context, inst Instance, msg message) bool {
	botQQ := s.accountQQ(inst.AccountID)

	responseMode := "whitelist_only"
	globalPrefix := ""
	if s.DB != nil {
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE(value #>> '{}', 'whitelist_only') FROM system_configs WHERE key='bot.response_mode'`).Scan(&responseMode)
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE(value #>> '{}', '') FROM system_configs WHERE key='bot.command_prefix'`).Scan(&globalPrefix)
	}

	if responseMode == "admin_only" {
		if msg.UserID != botQQ && !s.isWhitelistedUser(ctx, inst.ID, msg.UserID) {
			return false
		}
	} else if msg.Private {
		// Private chat requires explicit user whitelist or the bot account itself
		if msg.UserID != botQQ && responseMode == "whitelist_only" && !s.isWhitelistedUser(ctx, inst.ID, msg.UserID) {
			return false
		}
	} else {
		// Group chat requires explicit group whitelist if in whitelist_only mode
		if responseMode == "whitelist_only" && !s.isWhitelistedGroup(ctx, inst.ID, msg.GroupID) {
			s.Logger.Info("bot message ignored (group not in whitelist)", "bot_id", inst.ID, "group_id", msg.GroupID)
			return false
		}
	}

	prefix := globalPrefix
	if prefix == "" {
		prefix = inst.CommandPrefix
	}
	if prefix == "" {
		prefix = "/"
	}
	isCmd := strings.HasPrefix(msg.Text, prefix) || strings.HasPrefix(msg.Text, "/")

	atBot := isCmd ||
		strings.Contains(msg.Text, "@"+botQQ) ||
		strings.Contains(msg.Text, "@"+inst.Name) ||
		strings.Contains(msg.Text, "@老吴") ||
		strings.Contains(msg.Text, "@群老吴") ||
		strings.Contains(msg.Text, "[CQ:at,qq="+botQQ+"]") ||
		strings.Contains(msg.Text, "@全体成员") ||
		strings.Contains(msg.Text, "[CQ:at,qq=all]") ||
		strings.HasPrefix(msg.Text, "老吴") ||
		strings.HasPrefix(msg.Text, "群老吴") ||
		(inst.Name != "" && strings.HasPrefix(msg.Text, inst.Name))

	if !atBot {
		if inst.ReplyProbability > 0 && s.nrand(100) < inst.ReplyProbability {
			line := s.entertainLine(inst)
			if line != "" {
				go func() {
					_ = s.sendReply(context.Background(), inst.AccountID, inst, msg, line)
					s.audit(context.Background(), inst, msg, "entertainment", msg.Text, line)
				}()
			}
		}
		s.Logger.Debug("bot message ignored in whitelisted group (not @mentioned)", "bot_id", inst.ID, "group_id", msg.GroupID)
		return false
	}
	return true
}

func (s *Service) rateOK(botID string, msg message) bool {
	// Debounce rapid duplicate packets from the same user within 300ms
	key := botID + ":" + msg.UserID
	s.mu.Lock()
	defer s.mu.Unlock()
	last := s.last[key]
	if time.Since(last) < 300*time.Millisecond {
		return false
	}
	s.last[key] = time.Now()
	return true
}

func (s *Service) touch(botID string, msg message) {
	key := botID + ":" + msg.UserID
	s.mu.Lock()
	s.last[key] = time.Now()
	s.mu.Unlock()
}

func (s *Service) loadInstance(ctx context.Context, accountID string) (*Instance, error) {
	var inst Instance
	var ent []byte
	err := s.DB.QueryRow(ctx, `SELECT id::text,account_id::text,enabled,name,persona,
		COALESCE(NULLIF(context_prompt, ''), (SELECT value #>> '{}' FROM system_configs WHERE key='bot.default_context_prompt'), ''),COALESCE(cmd_enable_reply, ''),COALESCE(cmd_disable_reply, ''),COALESCE(fallback_reply, ''),
		meow_sound,command_prefix,greeting,reply_probability,entertainment,
		COALESCE(llm_provider, ''),COALESCE(llm_api_base, ''),COALESCE(llm_api_key, ''),COALESCE(llm_model, ''),COALESCE(llm_temperature, 0.7),COALESCE(llm_max_tokens, 600),
		COALESCE(max_tool_hops, 10)
		FROM bot_instances WHERE account_id=$1`, accountID).
		Scan(&inst.ID, &inst.AccountID, &inst.Enabled, &inst.Name, &inst.Persona,
			&inst.ContextPrompt, &inst.CmdEnableReply, &inst.CmdDisableReply, &inst.FallbackReply,
			&inst.MeowSound, &inst.CommandPrefix, &inst.Greeting, &inst.ReplyProbability, &ent,
			&inst.LLMProvider, &inst.LLMAPIBase, &inst.LLMAPIKey, &inst.LLMModel, &inst.LLMTemperature, &inst.LLMMaxTokens,
			&inst.MaxToolHops)
	if err != nil {
		return nil, err
	}
	inst.Entertainment = ent
	if decrypted, decryptErr := secrets.Decrypt(inst.LLMAPIKey, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
		inst.LLMAPIKey = decrypted
	}
	return &inst, nil
}

func (s *Service) loadInstanceByID(ctx context.Context, instanceID string) (*Instance, error) {
	var inst Instance
	var ent []byte
	err := s.DB.QueryRow(ctx, `SELECT id::text,account_id::text,enabled,name,persona,
		COALESCE(NULLIF(context_prompt, ''), (SELECT value #>> '{}' FROM system_configs WHERE key='bot.default_context_prompt'), ''),COALESCE(cmd_enable_reply, ''),COALESCE(cmd_disable_reply, ''),COALESCE(fallback_reply, ''),
		meow_sound,command_prefix,greeting,reply_probability,entertainment,
		COALESCE(llm_provider, ''),COALESCE(llm_api_base, ''),COALESCE(llm_api_key, ''),COALESCE(llm_model, ''),COALESCE(llm_temperature, 0.7),COALESCE(llm_max_tokens, 600),
		COALESCE(max_tool_hops, 10)
		FROM bot_instances WHERE id=$1`, instanceID).
		Scan(&inst.ID, &inst.AccountID, &inst.Enabled, &inst.Name, &inst.Persona,
			&inst.ContextPrompt, &inst.CmdEnableReply, &inst.CmdDisableReply, &inst.FallbackReply,
			&inst.MeowSound, &inst.CommandPrefix, &inst.Greeting, &inst.ReplyProbability, &ent,
			&inst.LLMProvider, &inst.LLMAPIBase, &inst.LLMAPIKey, &inst.LLMModel, &inst.LLMTemperature, &inst.LLMMaxTokens,
			&inst.MaxToolHops)
	if err != nil {
		return nil, err
	}
	inst.Entertainment = ent
	if decrypted, decryptErr := secrets.Decrypt(inst.LLMAPIKey, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
		inst.LLMAPIKey = decrypted
	}
	return &inst, nil
}

// ChatTest executes a test interaction through the bot's LLM + MCP engine for Web Sandbox.
func (s *Service) ChatTest(ctx context.Context, instanceID, userID, groupID, text string) (string, error) {
	inst, err := s.loadInstanceByID(ctx, instanceID)
	if err != nil {
		return "", fmt.Errorf("load instance: %w", err)
	}
	if inst == nil {
		return "", fmt.Errorf("instance not found")
	}
	if userID == "" {
		userID = "web_tester"
	}
	msg := message{
		MessageID: "test_" + strconv.FormatInt(time.Now().UnixMilli(), 10),
		GroupID:   groupID,
		UserID:    userID,
		Nickname:  "沙盒测试员",
		Private:   groupID == "",
		Text:      strings.TrimSpace(text),
	}
	reply, err := s.generateLLMReply(ctx, *inst, msg)
	if err != nil {
		return "", err
	}
	s.audit(ctx, *inst, msg, "playground", text, reply)
	return reply, nil
}

// ClearSession resets memory for a user session.
func (s *Service) ClearSession(botID, groupID, userID string) {
	if s.Memory == nil {
		return
	}
	var sessionKey string
	if groupID == "" {
		sessionKey = fmt.Sprintf("p:%s:%s", botID, userID)
	} else {
		sessionKey = fmt.Sprintf("g:%s:%s", botID, groupID)
	}
	s.Memory.Clear(sessionKey)
}

func (s *Service) accountQQ(accountID string) string {
	var qq string
	_ = s.DB.QueryRow(context.Background(), `SELECT qq_uin FROM napcat_accounts WHERE id=$1`, accountID).Scan(&qq)
	return qq
}

func (s *Service) audit(ctx context.Context, inst Instance, msg message, trigger, command, reply string) {
	if _, err := s.DB.Exec(ctx, `INSERT INTO bot_audit_log(bot_id,account_id,platform_group_id,user_qq,trigger_type,command,reply,message_id,status,thought_trace,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'completed','[]'::jsonb,now())`,
		inst.ID, inst.AccountID, msg.GroupID, msg.UserID, trigger, command, reply, msg.MessageID); err != nil {
		s.Logger.Warn("bot audit write failed", "error", err)
	}
}

func (s *Service) startAudit(ctx context.Context, inst Instance, msg message, trigger, command string) string {
	var id string
	err := s.DB.QueryRow(ctx, `INSERT INTO bot_audit_log(bot_id,account_id,platform_group_id,user_qq,trigger_type,command,reply,message_id,status,thought_trace,created_at)
		VALUES($1,$2,$3,$4,$5,$6,'',$7,'processing','[]'::jsonb,now()) RETURNING id::text`,
		inst.ID, inst.AccountID, msg.GroupID, msg.UserID, trigger, command, msg.MessageID).Scan(&id)
	if err != nil {
		s.Logger.Warn("bot audit start write failed", "error", err)
		return ""
	}
	return id
}

func (s *Service) updateAuditThought(ctx context.Context, auditID string, tools []string, thoughtTrace []ThoughtStep) {
	if auditID == "" || s.DB == nil {
		return
	}
	if tools == nil {
		tools = []string{}
	}
	traceJSON, _ := json.Marshal(thoughtTrace)
	_, _ = s.DB.Exec(ctx, `UPDATE bot_audit_log SET tools_called=$1, thought_trace=$2 WHERE id=$3`, tools, traceJSON, auditID)
}

func (s *Service) finishAudit(ctx context.Context, auditID, status, reply string, tools []string, latency int, errMsg string) {
	s.finishAuditWithTrace(ctx, auditID, status, reply, tools, latency, errMsg, nil)
}

func (s *Service) finishAuditWithTrace(ctx context.Context, auditID, status, reply string, tools []string, latency int, errMsg string, thoughtTrace []ThoughtStep) {
	if auditID == "" || s.DB == nil {
		return
	}
	if tools == nil {
		tools = []string{}
	}
	if thoughtTrace == nil {
		thoughtTrace = []ThoughtStep{}
	}
	traceJSON, _ := json.Marshal(thoughtTrace)
	if _, err := s.DB.Exec(ctx, `UPDATE bot_audit_log SET status=$1, reply=$2, tools_called=$3, latency_ms=$4, error_message=$5, thought_trace=$6 WHERE id=$7`,
		status, reply, tools, latency, errMsg, traceJSON, auditID); err != nil {
		s.Logger.Warn("bot audit finish update failed", "error", err)
	}
}

// accountForSend loads the HTTP credentials used to send a reply.
func (s *Service) accountForSend(ctx context.Context, accountID string) (domain.NapCatAccount, error) {
	return s.Repo.GetAccount(ctx, accountID)
}

func formatID(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case json.Number:
		return v.String()
	default:
		s := fmt.Sprint(v)
		if strings.Contains(s, "e+") || strings.Contains(s, "E+") {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return strconv.FormatInt(int64(f), 10)
			}
		}
		return strings.TrimSpace(s)
	}
}

// parseMessage extracts a chat message from a OneBot v11 WS event.
func parseMessage(raw json.RawMessage) (message, bool) {
	var event struct {
		PostType    string `json:"post_type"`
		MessageType string `json:"message_type"`
		GroupID     any    `json:"group_id"`
		UserID      any    `json:"user_id"`
		SelfID      any    `json:"self_id"`
		TargetID    any    `json:"target_id"`
		MessageID   any    `json:"message_id"`
		RealID      any    `json:"real_id"`
		MessageSeq  any    `json:"message_seq"`
		RawMessage  string `json:"raw_message"`
		Sender      struct {
			UserID   any    `json:"user_id"`
			Nickname string `json:"nickname"`
			Card     string `json:"card"`
		} `json:"sender"`
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return message{}, false
	}
	if event.PostType != "message" && event.PostType != "message_sent" {
		return message{}, false
	}

	userID := formatID(event.UserID)
	if event.PostType == "message_sent" && event.MessageType != "group" {
		target := formatID(event.TargetID)
		if target != "" && target != "0" && target != "<nil>" {
			userID = target
		}
	}
	if userID == "" || userID == "0" || userID == "<nil>" {
		userID = formatID(event.Sender.UserID)
	}
	if userID == "" || userID == "0" || userID == "<nil>" {
		userID = formatID(event.SelfID)
	}

	mid := formatID(event.MessageID)
	if mid == "" || mid == "0" || mid == "<nil>" {
		mid = formatID(event.RealID)
	}
	if mid == "" || mid == "0" || mid == "<nil>" {
		mid = formatID(event.MessageSeq)
	}

	nickname := strings.TrimSpace(event.Sender.Card)
	if nickname == "" {
		nickname = strings.TrimSpace(event.Sender.Nickname)
	}

	msg := message{
		MessageID:   mid,
		GroupID:     formatID(event.GroupID),
		UserID:      userID,
		Nickname:    nickname,
		Private:     event.MessageType != "group",
		Text:        segmentsText(event.Message),
		QuotedMsgID: extractReplyID(event.RawMessage, event.Message),
	}
	if msg.Text == "" {
		msg.Text = strings.TrimSpace(event.RawMessage)
	}
	if msg.UserID == "<nil>" || msg.UserID == "" {
		return message{}, false
	}
	return msg, true
}

// extractReplyID returns the message_id of the quoted/reply segment, if present.
func extractReplyID(rawMessage string, raw json.RawMessage) string {
	var segments []struct {
		Type string `json:"type"`
		Data struct {
			ID any `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &segments); err == nil {
		for _, seg := range segments {
			if seg.Type == "reply" {
				id := formatID(seg.Data.ID)
				if id != "" && id != "0" && id != "<nil>" {
					return id
				}
			}
		}
	}
	// Fallback to regex on rawMessage or raw json string
	if m := cqReplyRegex.FindStringSubmatch(rawMessage); len(m) > 1 {
		return m[1]
	}
	if m := cqReplyRegex.FindStringSubmatch(string(raw)); len(m) > 1 {
		return m[1]
	}
	return ""
}

// segmentsText flattens a message segment array into plain text.
// OneBot v11 image segments carry data.url (CDN link); reply segments carry
// data.id (quoted msg id) and data.text/data.sender (when NapCat inlines them).
func segmentsText(raw json.RawMessage) string {
	var segments []struct {
		Type string `json:"type"`
		Data struct {
			Text string `json:"text"`
			QQ   any    `json:"qq"`
			ID   any    `json:"id"`
			// image fields
			URL      string `json:"url"`
			File     string `json:"file"`
			FileSize int    `json:"file_size"`
			// mface/marketface (QQ market sticker) fields
			Summary string `json:"summary"`
			// reply fields (NapCat sometimes inlines the original message)
			SenderQQ   any    `json:"sender"`
			SenderName string `json:"sender_name"`
			Content    string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		return ""
	}
	var builder strings.Builder
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			builder.WriteString(seg.Data.Text)
		case "at":
			qq := formatID(seg.Data.QQ)
			if qq == "all" {
				builder.WriteString("@全体成员 ")
			} else if qq != "" && qq != "<nil>" {
				builder.WriteString("@")
				builder.WriteString(qq)
				builder.WriteString(" ")
			}
		case "face":
			builder.WriteString("[表情]")
		case "image":
			// Prefer CDN URL so the LLM (or later vision pipeline) can access the image.
			if seg.Data.URL != "" {
				builder.WriteString("[附带图片URL: ")
				builder.WriteString(seg.Data.URL)
				builder.WriteString("]")
			} else if seg.Data.File != "" {
				builder.WriteString("[图片: ")
				builder.WriteString(seg.Data.File)
				builder.WriteString("]")
			} else {
				builder.WriteString("[图片]")
			}
		case "mface", "marketface":
			// QQ market stickers. data.url is the CDN image URL; data.summary is the text label.
			if seg.Data.URL != "" {
				builder.WriteString("[附带图片URL: ")
				builder.WriteString(seg.Data.URL)
				builder.WriteString("]")
			} else if seg.Data.Summary != "" {
				builder.WriteString("[表情包: ")
				builder.WriteString(seg.Data.Summary)
				builder.WriteString("]")
			} else {
				builder.WriteString("[表情包]")
			}
		case "reply":
			// Build a readable quoted-message marker so the LLM understands who is being replied to.
			senderQQ := formatID(seg.Data.SenderQQ)
			content := strings.TrimSpace(seg.Data.Content)
			if content == "" {
				content = strings.TrimSpace(seg.Data.Text)
			}
			if senderQQ != "" && senderQQ != "<nil>" && senderQQ != "0" {
				if content != "" {
					builder.WriteString("[引用 @")
					builder.WriteString(senderQQ)
					builder.WriteString(" 的消息: \"")
					builder.WriteString(content)
					builder.WriteString("\"] ")
				} else {
					builder.WriteString("[引用 @")
					builder.WriteString(senderQQ)
					builder.WriteString(" 的消息] ")
				}
			} else if content != "" {
				builder.WriteString("[引用消息: \"")
				builder.WriteString(content)
				builder.WriteString("\"] ")
			} else {
				builder.WriteString("[回复] ")
			}
		case "record":
			builder.WriteString("[语音]")
		case "video":
			builder.WriteString("[视频]")
		default:
			builder.WriteString(" ")
		}
	}
	return strings.TrimSpace(builder.String())
}
