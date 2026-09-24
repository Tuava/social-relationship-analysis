package analysis

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

type LLMConfig struct {
	APIBase            string
	APIKey             string
	Model              string
	MaxTokens          int
	RequestTimeout     time.Duration
	InternalAttempts   int
	JSONMode           bool
	ThinkingMode       string
	MinRequestInterval time.Duration
	RateLimitBackoff   time.Duration
}

// GetActiveLLMConfig resolves the analysis model from the dedicated analysis
// configuration only. It must never borrow credentials or a model from the
// chat bot: that would silently change the evidence-analysis model.
func GetActiveLLMConfig(ctx context.Context, pool *pgxpool.Pool) LLMConfig {
	cfg := LLMConfig{
		MaxTokens: 4096,
	}

	// 1. Query dynamic system_configs table first if available
	if pool != nil {
		rows, err := pool.Query(ctx, `
			SELECT key, value::text 
			FROM system_configs 
			WHERE key IN ('ai.llm_api_base', 'ai.llm_api_key', 'ai.llm_model', 'ai.max_concurrency', 'ai.llm_max_tokens', 'ai.llm_request_timeout_seconds', 'ai.llm_internal_attempts', 'ai.persona_thinking_mode', 'ai.min_request_interval_ms', 'ai.rate_limit_backoff_seconds')
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var k, rawVal string
				if err := rows.Scan(&k, &rawVal); err == nil {
					// unquote json string if needed
					val := strings.Trim(strings.TrimSpace(rawVal), "\"")
					if val != "" {
						if k == "ai.llm_api_base" {
							cfg.APIBase = val
						} else if k == "ai.llm_api_key" {
							if decrypted, decryptErr := secrets.Decrypt(val, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
								cfg.APIKey = decrypted
							}
						} else if k == "ai.llm_model" {
							cfg.Model = val
						} else if k == "ai.max_concurrency" {
							var maxC int
							if json.Unmarshal([]byte(rawVal), &maxC) == nil && maxC > 0 {
								GetGlobalQueueManager().SetGlobalMaxConcurrency(maxC)
							}
						} else if k == "ai.llm_max_tokens" {
							var maxTokens int
							if json.Unmarshal([]byte(rawVal), &maxTokens) == nil && maxTokens > 0 {
								cfg.MaxTokens = maxTokens
							}
						} else if k == "ai.llm_request_timeout_seconds" {
							var seconds int
							if json.Unmarshal([]byte(rawVal), &seconds) == nil && seconds > 0 {
								cfg.RequestTimeout = time.Duration(seconds) * time.Second
							}
						} else if k == "ai.llm_internal_attempts" {
							var attempts int
							if json.Unmarshal([]byte(rawVal), &attempts) == nil && attempts > 0 {
								cfg.InternalAttempts = attempts
							}
						} else if k == "ai.persona_thinking_mode" {
							cfg.ThinkingMode = val
						} else if k == "ai.min_request_interval_ms" {
							var milliseconds int
							if json.Unmarshal([]byte(rawVal), &milliseconds) == nil && milliseconds >= 0 {
								cfg.MinRequestInterval = time.Duration(milliseconds) * time.Millisecond
							}
						} else if k == "ai.rate_limit_backoff_seconds" {
							var seconds int
							if json.Unmarshal([]byte(rawVal), &seconds) == nil && seconds >= 0 {
								cfg.RateLimitBackoff = time.Duration(seconds) * time.Second
							}
						}
					}
				}
			}
		}
	}

	return cfg
}

type VisionContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *ImageURLParam `json:"image_url,omitempty"`
}

type ImageURLParam struct {
	URL string `json:"url"`
}

type ChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type ChatRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
	Thinking *struct {
		Type string `json:"type"`
	} `json:"thinking,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// CallLLM makes an OpenAI-compatible HTTP POST request to the configured LLM endpoint.
func CallLLM(ctx context.Context, cfg LLMConfig, messages []ChatMessage, temperature float64) (string, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIBase) == "" || strings.TrimSpace(cfg.Model) == "" {
		return "", fmt.Errorf("analysis LLM is not fully configured: api_base, api_key, and model are all required")
	}

	apiEndpoint := strings.TrimRight(cfg.APIBase, "/")
	if !strings.HasSuffix(apiEndpoint, "/chat/completions") {
		apiEndpoint = apiEndpoint + "/chat/completions"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 120 * time.Second
	}
	maxAttempts := cfg.InternalAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	// Acquire per-model concurrency slot with FIFO queuing
	modelKey := strings.TrimSpace(cfg.APIBase) + "::" + strings.TrimSpace(cfg.Model)
	queue := GetGlobalQueueManager()
	queue.ConfigureModel(modelKey, cfg.MinRequestInterval, cfg.RateLimitBackoff)
	release, err := queue.AcquireWithTimeout(ctx, modelKey, llmQueueTimeout(ctx))
	if err != nil {
		return "", fmt.Errorf("llm queue acquire: %w", err)
	}
	defer release()

	reqBody := ChatRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}
	if cfg.JSONMode {
		reqBody.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	if cfg.ThinkingMode != "" {
		reqBody.Thinking = &struct {
			Type string `json:"type"`
		}{Type: cfg.ThinkingMode}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	var lastErr error
	var respBytes []byte

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := queue.WaitForRequest(ctx, modelKey); err != nil {
			return "", fmt.Errorf("llm request pacing cancelled: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("create http request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

		client := &http.Client{Timeout: requestTimeout}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http request failed: %w", err)
			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(time.Duration(attempt*600) * time.Millisecond):
					continue
				}
			}
			continue
		}

		respBytes, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response body: %w", err)
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			cooldown := parseRetryAfter(resp.Header.Get("Retry-After"))
			cooldown = queue.NotifyRateLimit(modelKey, cooldown)
			lastErr = fmt.Errorf("LLM API returned HTTP %d: %s (model cooldown %s)", resp.StatusCode, string(respBytes), formatCooldown(cooldown))
			if attempt < maxAttempts {
				continue
			}
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("LLM API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
		} else {
			lastErr = nil
			break
		}
	}

	if lastErr != nil {
		return "", lastErr
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal chat response: %w (raw: %s)", err, string(respBytes))
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM API returned empty choices")
	}

	content := chatResp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" && strings.TrimSpace(chatResp.Choices[0].Message.ReasoningContent) != "" {
		content = chatResp.Choices[0].Message.ReasoningContent
	}

	return content, nil
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
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

func formatCooldown(duration time.Duration) string {
	if duration <= 0 {
		return "未设置"
	}
	return duration.Round(time.Second).String()
}

// EncodeImageToBase64 reads only validated images inside configured media roots.
// The legacy MIME argument is deliberately ignored: derive it from decoded bytes.
func EncodeImageToBase64(filePath string, _ string) (string, error) {
	img, err := readVisionImage(filePath, "")
	if err != nil {
		return "", fmt.Errorf("read vision image: %w", err)
	}
	return visionImageDataURL(img.data, img.mimeType), nil
}

// Only pass a validatedVisionImage snapshot here, not arbitrary file contents.
func visionImageDataURL(data []byte, mimeType string) string {
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

var (
	jsonBlockRegex    = regexp.MustCompile(`(?s)\x60\x60\x60(?:json)?\s*(\{.*?\}|\[.*?\])\s*\x60\x60\x60`)
	jsonRawBraces     = regexp.MustCompile(`(?s)(\{.*\}|\[.*\])`)
	trailingCommaJSON = regexp.MustCompile(`,\s*([\}\]])`)
)

// CleanJSONResponse extracts and repairs JSON strings from messy or truncated LLM responses.
func CleanJSONResponse(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "{}"
	}

	// 1. Try finding markdown fenced json: ```json { ... } ``` or ``` { ... } ```
	if matches := jsonBlockRegex.FindStringSubmatch(text); len(matches) > 1 {
		text = matches[1]
	} else {
		// 2. Extract outermost JSON object {...} or array [...]
		firstBrace := strings.Index(text, "{")
		lastBrace := strings.LastIndex(text, "}")
		firstBracket := strings.Index(text, "[")
		lastBracket := strings.LastIndex(text, "]")

		if firstBrace != -1 && lastBrace > firstBrace && (firstBracket == -1 || firstBrace < firstBracket) {
			text = text[firstBrace : lastBrace+1]
		} else if firstBracket != -1 && lastBracket > firstBracket {
			text = text[firstBracket : lastBracket+1]
		} else if firstBrace != -1 {
			// Incomplete/truncated starting with '{'
			text = text[firstBrace:]
		} else if firstBracket != -1 {
			// Incomplete/truncated starting with '['
			text = text[firstBracket:]
		}
	}

	text = strings.TrimSpace(text)
	// If text does not start with JSON container, return empty JSON object
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return "{}"
	}

	// 3. Normalize JSON syntax issues (single-quoted keys, mixed quotes in arrays)
	text = NormalizeJSONSyntax(text)

	// 4. Remove common JSON syntax errors from weaker models (trailing commas: ",}" or ",]")
	text = trailingCommaJSON.ReplaceAllString(text, "$1")

	// 5. Repair unescaped quotes inside JSON strings
	text = RepairUnescapedQuotes(text)

	// 6. Repair dangling comma-separated string literals for a single key
	text = RepairDanglingMultipleStrings(text)

	// 4. Auto-close truncated brackets/braces if unclosed
	var stack []rune
	inString := false
	escape := false

	for _, r := range text {
		if escape {
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if r == '{' || r == '[' {
			stack = append(stack, r)
		} else if r == '}' {
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		} else if r == ']' {
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if inString {
		text += `"`
	}

	// Pop unclosed brackets in reverse
	for i := len(stack) - 1; i >= 0; i-- {
		b := stack[i]
		// clean any trailing comma right before closing
		text = strings.TrimRight(text, ", \n\r\t")
		if b == '{' {
			text += "}"
		} else if b == '[' {
			text += "]"
		}
	}

	return strings.TrimSpace(text)
}

// RepairUnescapedQuotes repairs illegal unescaped double quotes inside JSON string values.
// E.g. "key": "abc "def" ghi" -> "key": "abc 'def' ghi"
func RepairUnescapedQuotes(text string) string {
	runes := []rune(text)
	n := len(runes)
	var out []rune

	lookaheadAfter := func(idx int) rune {
		j := idx + 1
		for j < n && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
			j++
		}
		if j >= n {
			return 0
		}
		return runes[j]
	}

	inString := false
	escape := false

	for i := 0; i < n; i++ {
		r := runes[i]

		if escape {
			out = append(out, r)
			escape = false
			continue
		}

		if r == '\\' {
			out = append(out, r)
			escape = true
			continue
		}

		if !inString {
			if r == '"' {
				inString = true
				out = append(out, r)
			} else {
				out = append(out, r)
			}
		} else {
			// Inside string
			if r == '"' {
				nextC := lookaheadAfter(i)
				if nextC == ':' || nextC == ',' || nextC == '}' || nextC == ']' || nextC == 0 {
					// Valid JSON closing quote
					inString = false
					out = append(out, r)
				} else {
					// Unescaped inner quote -> replace with single quote
					out = append(out, '\'')
				}
			} else {
				out = append(out, r)
			}
		}
	}

	return string(out)
}

var quotedStringRegex = regexp.MustCompile(`"([^"\\]*(?:\\.[^"\\]*)*)"`)

// RepairDanglingMultipleStrings detects and merges comma-separated string literals within a single JSON key:
// e.g., `"verbatim_evidence": "val1", "val2", "val3"` -> `"verbatim_evidence": "val1；val2；val3"`
func RepairDanglingMultipleStrings(text string) string {
	lines := strings.Split(text, "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if colonIdx := strings.Index(trimmed, "\":"); colonIdx != -1 {
			valuePart := strings.TrimSpace(trimmed[colonIdx+2:])
			matches := quotedStringRegex.FindAllStringSubmatch(valuePart, -1)
			if len(matches) > 1 {
				firstQuoteEnd := strings.Index(valuePart[1:], "\"")
				if firstQuoteEnd != -1 {
					restOfValue := valuePart[firstQuoteEnd+2:]
					if !strings.Contains(restOfValue, ":") {
						var vals []string
						for _, m := range matches {
							vals = append(vals, m[1])
						}
						joined := strings.Join(vals, "；")
						hasTrailingComma := strings.HasSuffix(trimmed, ",")
						newVal := fmt.Sprintf("\"%s\"", joined)
						if hasTrailingComma {
							newVal += ","
						}
						prefix := line[:strings.Index(line, "\":")+2]
						lines[idx] = prefix + " " + newVal
					}
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

var singleOrMixedQuoteKeyRegex = regexp.MustCompile(`(?m)(^|[,\{\s])['"]([a-zA-Z0-9_]+)['"]\s*:`)

// NormalizeJSONSyntax fixes common LLM malformed JSON patterns:
// 1. Single or mixed quoted keys: `'key":` or `'key':` -> `"key":`
// 2. Single or mixed quoted array items with annotations: `'word' (comment),` -> `"word (comment)",`
func NormalizeJSONSyntax(text string) string {
	// 1. Fix object keys with single quotes or mixed quotes: 'key': or 'key": or "key': -> "key":
	text = singleOrMixedQuoteKeyRegex.ReplaceAllString(text, `$1"$2":`)

	// 2. Fix array items or lines with single quotes or mixed quote annotations
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "{" || trimmed == "}" || trimmed == "[" || trimmed == "]" || trimmed == "}," || trimmed == "]," {
			continue
		}

		// If line does NOT have a colon ':' (meaning it's likely an array element or array line)
		if !strings.Contains(trimmed, ":") {
			hasComma := strings.HasSuffix(trimmed, ",")
			content := strings.TrimSuffix(trimmed, ",")
			content = strings.TrimSpace(content)

			if strings.HasPrefix(content, "'") || strings.HasPrefix(content, "\"") {
				inner := content
				inner = strings.TrimPrefix(inner, "'")
				inner = strings.TrimPrefix(inner, "\"")
				// Remove interior quotes if they were enclosing slang terms: 'foo' (bar) -> foo (bar)
				inner = strings.ReplaceAll(inner, "'", "")
				inner = strings.ReplaceAll(inner, "\"", "")
				inner = strings.TrimSpace(inner)

				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				newLine := indent + fmt.Sprintf("\"%s\"", inner)
				if hasComma {
					newLine += ","
				}
				lines[i] = newLine
			}
		}
	}
	return strings.Join(lines, "\n")
}
