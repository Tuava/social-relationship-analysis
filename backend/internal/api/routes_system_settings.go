package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/qzone"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

type SystemConfigItem struct {
	Key         string          `json:"key"`
	Category    string          `json:"category"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
	UpdatedAt   time.Time       `json:"updated_at"`
	IsMasked    bool            `json:"is_masked,omitempty"`
}

type TestLLMRequest struct {
	APIBase string `json:"api_base"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type TestLLMResponse struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Model     string `json:"model"`
	Reply     string `json:"reply"`
	Error     string `json:"error,omitempty"`
}

// maskSecret masks a sensitive token string, showing only prefix/suffix if long enough
func maskSecret(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "\"")
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "******"
	}
	return s[:3] + "******" + s[len(s)-4:]
}

// isSensitiveKey returns true if the key holds sensitive credential
func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	return strings.Contains(lower, "api_key") || strings.Contains(lower, "secret") || strings.Contains(lower, "password")
}

// systemSettingsGet handles GET /api/v1/system/settings
func (s *Server) systemSettingsGet(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT key, category, value, COALESCE(description,''), updated_at
		FROM system_configs
		ORDER BY category, key
	`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()

	items := []SystemConfigItem{}
	categorized := map[string]map[string]any{}

	for rows.Next() {
		var item SystemConfigItem
		var rawVal []byte
		if err := rows.Scan(&item.Key, &item.Category, &rawVal, &item.Description, &item.UpdatedAt); err != nil {
			continue
		}

		if isSensitiveKey(item.Key) {
			var strVal string
			if json.Unmarshal(rawVal, &strVal) == nil && strVal != "" {
				if decrypted, decryptErr := secrets.Decrypt(strVal, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
					strVal = decrypted
				}
				item.IsMasked = true
				masked := maskSecret(strVal)
				item.Value, _ = json.Marshal(masked)
			} else {
				item.Value = rawVal
			}
		} else {
			item.Value = rawVal
		}

		items = append(items, item)

		if _, ok := categorized[item.Category]; !ok {
			categorized[item.Category] = map[string]any{}
		}
		var parsedVal any
		if json.Unmarshal(item.Value, &parsedVal) == nil {
			categorized[item.Category][item.Key] = parsedVal
		}
	}

	writeJSON(w, 200, map[string]any{
		"data":        categorized,
		"items":       items,
		"queue_stats": analysis.GetGlobalQueueManager().Stats(),
	})
}

// systemSettingsUpdate handles PUT /api/v1/system/settings
func (s *Server) systemSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, fmt.Errorf("invalid json body: %w", err))
		return
	}

	tx, err := s.Repo.DB.Begin(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback(r.Context())

	for k, v := range body {
		valStr := string(v)
		// If sensitive key contains masked pattern (e.g. "sk-***" or "******"), skip updating to preserve original key
		if isSensitiveKey(k) && (strings.Contains(valStr, "***") || strings.Trim(valStr, "\"") == "") {
			continue
		}
		if isSensitiveKey(k) {
			var plain string
			if err := json.Unmarshal(v, &plain); err != nil {
				writeError(w, 400, fmt.Errorf("sensitive setting %s must be a string", k))
				return
			}
			protected, protectErr := secrets.Encrypt(plain, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY"))
			if protectErr != nil {
				writeError(w, 500, protectErr)
				return
			}
			v, _ = json.Marshal(protected)
		}

		// Infer category from key prefix (e.g. "ai.llm_model" -> "ai")
		parts := strings.Split(k, ".")
		category := "general"
		if len(parts) > 1 {
			category = parts[0]
		}

		_, err := tx.Exec(r.Context(), `
			INSERT INTO system_configs (key, category, value, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (key) DO UPDATE
			SET value = EXCLUDED.value, updated_at = NOW()
		`, k, category, v)
		if err != nil {
			writeError(w, 500, fmt.Errorf("failed to save %s: %w", k, err))
			return
		}

		if k == "crawler.qzone_interval_seconds" {
			var sec float64
			if json.Unmarshal(v, &sec) == nil {
				qzone.SetDynamicInterval(sec)
			}
		}

		if k == "ai.max_concurrency" {
			var limit int
			if json.Unmarshal(v, &limit) == nil && limit > 0 {
				analysis.GetGlobalQueueManager().SetGlobalMaxConcurrency(limit)
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, err)
		return
	}

	writeJSON(w, 200, map[string]any{
		"status":      "ok",
		"message":     "配置已成功保存并热重载",
		"queue_stats": analysis.GetGlobalQueueManager().Stats(),
	})
}

// systemTestLLM handles POST /api/v1/system/settings/test-llm
func (s *Server) systemTestLLM(w http.ResponseWriter, r *http.Request) {
	var req TestLLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, fmt.Errorf("invalid json body: %w", err))
		return
	}

	// Resolve masked or empty keys from database
	if req.APIBase == "" || req.APIKey == "" || strings.Contains(req.APIKey, "***") {
		var dbBase, dbKey, dbModel string
		_ = s.Repo.DB.QueryRow(r.Context(), `
			SELECT 
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='ai.llm_api_base'), ''),
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='ai.llm_api_key'), ''),
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='ai.llm_model'), '')
		`).Scan(&dbBase, &dbKey, &dbModel)

		if req.APIBase == "" {
			req.APIBase = dbBase
		}
		if req.APIKey == "" || strings.Contains(req.APIKey, "***") {
			if decrypted, decryptErr := secrets.Decrypt(dbKey, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
				req.APIKey = decrypted
			}
		}
		if req.Model == "" {
			req.Model = dbModel
		}
	}

	if req.APIBase == "" {
		writeJSON(w, 400, TestLLMResponse{
			Status: "error",
			Error:  "未提供 API Base URL",
		})
		return
	}

	// Prepare lightweight test payload
	endpoint := strings.TrimRight(req.APIBase, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint = endpoint + "/chat/completions"
	}
	url := endpoint

	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, respond with pong"},
		},
		"max_tokens":  20,
		"temperature": 0.1,
	}
	payloadBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		writeJSON(w, 200, TestLLMResponse{
			Status: "error",
			Error:  fmt.Sprintf("构建请求失败: %v", err),
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	modelKey := strings.TrimSpace(req.APIBase) + "::" + strings.TrimSpace(req.Model)
	queue := analysis.GetGlobalQueueManager()
	// Connectivity tests share the same pacing path as analysis calls.
	var minIntervalMS, backoffSeconds int
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE((value #>> '{}')::int, 0) FROM system_configs WHERE key = 'ai.min_request_interval_ms'`).Scan(&minIntervalMS)
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE((value #>> '{}')::int, 0) FROM system_configs WHERE key = 'ai.rate_limit_backoff_seconds'`).Scan(&backoffSeconds)
	queue.ConfigureModel(modelKey, time.Duration(minIntervalMS)*time.Millisecond, time.Duration(backoffSeconds)*time.Second)
	release, err := queue.Acquire(r.Context(), modelKey)
	if err != nil {
		writeJSON(w, 200, TestLLMResponse{
			Status: "error",
			Error:  fmt.Sprintf("模型并发队列获取超时: %v", err),
		})
		return
	}
	defer release()
	if err := queue.WaitForRequest(r.Context(), modelKey); err != nil {
		writeJSON(w, 200, TestLLMResponse{Status: "error", Model: req.Model, Error: fmt.Sprintf("模型请求节奏等待被取消: %v", err)})
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(httpReq)
	latency := time.Since(startTime).Milliseconds()

	if err != nil {
		writeJSON(w, 200, TestLLMResponse{
			Status:    "error",
			LatencyMs: latency,
			Model:     req.Model,
			Error:     fmt.Sprintf("网络连接超时或拒绝: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			queue.NotifyRateLimit(modelKey, parseRetryAfterHeader(resp.Header.Get("Retry-After")))
		}
		writeJSON(w, 200, TestLLMResponse{
			Status:    "error",
			LatencyMs: latency,
			Model:     req.Model,
			Error:     fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes)),
		})
		return
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
	}
	_ = json.Unmarshal(bodyBytes, &chatResp)

	reply := ""
	if len(chatResp.Choices) > 0 {
		reply = strings.TrimSpace(chatResp.Choices[0].Message.Content)
	}
	if reply == "" {
		reply = "连通正常 (响应已解析)"
	}
	resModel := chatResp.Model
	if resModel == "" {
		resModel = req.Model
	}

	writeJSON(w, 200, TestLLMResponse{
		Status:    "ok",
		LatencyMs: latency,
		Model:     resModel,
		Reply:     reply,
	})
}

func parseRetryAfterHeader(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
}
