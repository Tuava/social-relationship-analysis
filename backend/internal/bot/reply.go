package bot

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
)

var cqImageRegex = regexp.MustCompile(`\[CQ:image,[^\]]*\]`)
var cqURLParamRegex = regexp.MustCompile(`url=([^,\]]+)`)
var cqFileParamRegex = regexp.MustCompile(`file=([^,\]]+)`)

// fetchQuotedContent calls NapCat /get_msg (or queries local database) to retrieve
// the full content of a quoted (replied-to) message. It extracts image URLs/files and text
// so the LLM can reason about what the user is referring to.
func (s *Service) fetchQuotedContent(ctx context.Context, accountID, msgID string) string {
	if msgID == "" || s.DB == nil {
		return ""
	}

	acc, err := s.accountForSend(ctx, accountID)
	if err != nil || acc.HTTPURL == "" {
		return ""
	}

	client := napcat.NewHTTPClient(acc.HTTPURL, acc.HTTPToken)

	fetchCtx, cancel := context.WithTimeout(ctx, 4_000_000_000) // 4 seconds
	defer cancel()

	var midParam any = msgID
	if intVal, err := strconv.ParseInt(msgID, 10, 64); err == nil {
		midParam = intVal
	}

	raw, err := client.Call(fetchCtx, "/get_msg", map[string]any{"message_id": midParam})

	// Unified response struct: handles both unwrapped data from client.Call and wrapped {"data": ...}
	type MsgPayload struct {
		Message    json.RawMessage `json:"message"`
		RawMessage string          `json:"raw_message"`
		Sender     struct {
			UserID   any    `json:"user_id"`
			Nickname string `json:"nickname"`
			Card     string `json:"card"`
		} `json:"sender"`
	}

	var payload MsgPayload
	if err == nil && len(raw) > 0 {
		// client.Call returns parsed.Data directly
		_ = json.Unmarshal(raw, &payload)
		// If payload.Message was empty, try unmarshaling as wrapped
		if len(payload.Message) == 0 && payload.RawMessage == "" {
			var wrapped struct {
				Data MsgPayload `json:"data"`
			}
			_ = json.Unmarshal(raw, &wrapped)
			payload = wrapped.Data
		}
	}

	// Fallback to local Postgres messages table if NapCat /get_msg failed or returned empty
	if len(payload.Message) == 0 && payload.RawMessage == "" {
		var rawText string
		var segJSON []byte
		err := s.DB.QueryRow(ctx, `SELECT COALESCE(raw_text, ''), COALESCE(message_segments::text, '[]') FROM messages WHERE source_message_id = $1 LIMIT 1`, msgID).Scan(&rawText, &segJSON)
		if err == nil {
			payload.RawMessage = rawText
			payload.Message = json.RawMessage(segJSON)
		}
	}

	// Parse the segments of the quoted message
	var segments []struct {
		Type string `json:"type"`
		Data struct {
			Text    string `json:"text"`
			URL     string `json:"url"`
			File    string `json:"file"`
			Summary string `json:"summary"` // mface: emoji description
		} `json:"data"`
	}
	_ = json.Unmarshal(payload.Message, &segments)

	var textParts []string
	var imgItems []string

	for _, seg := range segments {
		switch seg.Type {
		case "text":
			if t := strings.TrimSpace(seg.Data.Text); t != "" {
				textParts = append(textParts, t)
			}
		case "image", "mface", "marketface":
			target := strings.TrimSpace(seg.Data.URL)
			if target == "" {
				target = strings.TrimSpace(seg.Data.File)
			}
			if target != "" {
				target = html.UnescapeString(target)
				target = strings.ReplaceAll(target, "&amp;", "&")
				// Try resolving local path or fresh URL via /get_image if file is available
				if seg.Data.File != "" {
					imgRes, err := client.Call(fetchCtx, "/get_image", map[string]any{"file": seg.Data.File})
					if err == nil {
						type ImgPayload struct {
							File string `json:"file"`
							URL  string `json:"url"`
						}
						var imgPayload ImgPayload
						_ = json.Unmarshal(imgRes, &imgPayload)
						if imgPayload.File == "" && imgPayload.URL == "" {
							var wrapped struct {
								Data ImgPayload `json:"data"`
							}
							_ = json.Unmarshal(imgRes, &wrapped)
							imgPayload = wrapped.Data
						}

						if imgPayload.File != "" {
							imgItems = append(imgItems, imgPayload.File)
						} else if imgPayload.URL != "" {
							imgItems = append(imgItems, imgPayload.URL)
						} else {
							imgItems = append(imgItems, target)
						}
					} else {
						imgItems = append(imgItems, target)
					}
				} else {
					imgItems = append(imgItems, target)
				}
			} else if seg.Data.Summary != "" {
				textParts = append(textParts, "[表情: "+seg.Data.Summary+"]")
			}
		case "face":
			textParts = append(textParts, "[QQ表情]")
		}
	}

	// Parse CQ codes in raw_message if segments were empty or had no images
	if len(imgItems) == 0 && payload.RawMessage != "" {
		rawMsg := payload.RawMessage
		for _, m := range cqImageRegex.FindAllString(rawMsg, -1) {
			if u := cqURLParamRegex.FindStringSubmatch(m); len(u) > 1 {
				cleanU := html.UnescapeString(u[1])
				cleanU = strings.ReplaceAll(cleanU, "&amp;", "&")
				imgItems = append(imgItems, cleanU)
			} else if f := cqFileParamRegex.FindStringSubmatch(m); len(f) > 1 {
				cleanF := html.UnescapeString(f[1])
				imgRes, err := client.Call(fetchCtx, "/get_image", map[string]any{"file": cleanF})
				if err == nil {
					type ImgPayload struct {
						File string `json:"file"`
						URL  string `json:"url"`
					}
					var imgPayload ImgPayload
					_ = json.Unmarshal(imgRes, &imgPayload)
					if imgPayload.File == "" && imgPayload.URL == "" {
						var wrapped struct {
							Data ImgPayload `json:"data"`
						}
						_ = json.Unmarshal(imgRes, &wrapped)
						imgPayload = wrapped.Data
					}

					if imgPayload.File != "" {
						imgItems = append(imgItems, imgPayload.File)
					} else if imgPayload.URL != "" {
						imgItems = append(imgItems, imgPayload.URL)
					} else {
						imgItems = append(imgItems, cleanF)
					}
				} else {
					imgItems = append(imgItems, cleanF)
				}
			}
		}
	}

	if len(textParts) == 0 && len(imgItems) == 0 {
		raw := strings.TrimSpace(payload.RawMessage)
		if raw == "" {
			return ""
		}
		raw = formatCQImages(raw)
		return "[被引用消息内容: " + raw + "]"
	}

	var b strings.Builder
	b.WriteString("[被引用消息内容: ")
	if len(textParts) > 0 {
		b.WriteString("\"")
		b.WriteString(strings.Join(textParts, " "))
		b.WriteString("\"")
	}
	for _, it := range imgItems {
		if b.Len() > len("[被引用消息内容: ") {
			b.WriteString(" ")
		}
		b.WriteString("附带图片URL: ")
		b.WriteString(it)
	}
	b.WriteString("]")
	return b.String()
}
