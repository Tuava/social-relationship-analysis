package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

type chatMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Thought          string     `json:"thought,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatCompletionRequest struct {
	Model       string              `json:"model"`
	Messages    []chatMessage       `json:"messages"`
	Tools       []LLMToolDefinition `json:"tools,omitempty"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// generateLLMReply executes the LLM reasoning loop with full Tool-Calling capability.
func (s *Service) generateLLMReply(ctx context.Context, inst Instance, msg message) (string, error) {
	reply, _, _, err := s.generateLLMReplyWithTools(ctx, inst, msg, "")
	return reply, err
}

func (s *Service) generateLLMReplyWithTools(ctx context.Context, inst Instance, msg message, auditID string) (string, []string, []ThoughtStep, error) {
	apiBase := strings.TrimSpace(inst.LLMAPIBase)
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	apiBase = strings.TrimRight(apiBase, "/")
	if !strings.HasSuffix(apiBase, "/chat/completions") {
		apiBase = apiBase + "/chat/completions"
	}

	model := inst.LLMModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	temp := inst.LLMTemperature
	if temp <= 0 {
		temp = 0.7
	}
	maxTokens := inst.LLMMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	if maxTokens > 8192 {
		maxTokens = 8192
	}

	systemPrompt := inst.Persona
	if systemPrompt == "" {
		systemPrompt = "你是一只住在 QQ 群里的小猫咪，名字叫'老吴'。你说话简短、俏皮、粘人，喜欢在句尾加'老吴~'或'喵'。你拥有查阅社交情报数据库的能力。"
	}

	timeStr := time.Now().Format("2006-01-02 15:04:05")
	var contextInfo string
	if msg.Private {
		contextInfo = fmt.Sprintf("【当前上下文】当前时间: %s, 场景: 私聊, 发送者昵称: %s, 发送者QQ: %s", timeStr, msg.Nickname, msg.UserID)
	} else {
		contextInfo = fmt.Sprintf("【当前上下文】当前时间: %s, 场景: 群聊(群号:%s), 发送者昵称: %s, 发送者QQ: %s。用户说‘这个群/群里’时，关系查询必须限定 group_id=%s，不得使用全库结果回答。", timeStr, msg.GroupID, msg.Nickname, msg.UserID, msg.GroupID)
	}

	contextPrompt := inst.ContextPrompt
	if contextPrompt == "" {
		contextPrompt = "请以符合你小猫咪人设的活泼口吻进行自然语言纯文本回答（不要输出Markdown加粗符号**）。\n【工具调用指引】\n1. 外部实时资讯/天气/时事/常识：主动调用 web_search 联网搜索；\n2. 图片识别与截屏分析：当群消息中包含 [附带图片URL: ...] 或用户要求识别/看图时，主动调用 sra_trigger_vision_analysis 解析图中内容与场景意图；\n3. 人物画像与性格分析：当用户要求研判特定人/QQ时，主动调用 sra_person_persona 或 sra_feed_dynamics；\n4. 群聊话题与争论总结：当用户询问群里在聊什么或发生了什么，调用 sra_trigger_dialogue_disentanglement 或 sra_dialogue_threads；\n5. 社交图谱与查人查记录：调用 sra_search, sra_person_dossier, sra_ego_network 或 sra_message_history。\n若工具未查到结果，请以萌态口吻自然回复说明。"
	}

	// Mandatory vision routing — always injected regardless of user-configured context_prompt.
	// Without this, the LLM tends to say "I can't see the image" instead of calling the tool.
	visionInstruction := "\n\n【图片处理规则 - 必须遵守】当消息中包含「[附带图片URL:」或「[被引用消息内容:」且内含图片URL/文件时，你必须立即调用 sra_trigger_vision_analysis 工具（传入 url 参数）来识别图片内容，然后再基于识别结果回答用户。禁止在未调用工具的情况下说\"我看不到图片\"或要求用户重新发送。"
	systemContent := systemPrompt + "\n\n" + contextInfo + "\n" + contextPrompt + visionInstruction

	// Build per-user/context conversation space key
	var sessionKey string
	var userPrompt string
	if msg.Private {
		sessionKey = fmt.Sprintf("p:%s:%s", inst.ID, msg.UserID)
		userPrompt = msg.Text
	} else {
		// Group-level shared context across all members in the group
		sessionKey = fmt.Sprintf("g:%s:%s", inst.ID, msg.GroupID)
		senderInfo := strings.TrimSpace(msg.Nickname)
		if senderInfo == "" {
			senderInfo = msg.UserID
		}
		userPrompt = fmt.Sprintf("[群成员 %s (QQ: %s)]: %s", senderInfo, msg.UserID, msg.Text)
	}

	var history []chatMessage
	if s.Memory != nil {
		history = s.Memory.GetHistory(sessionKey)
	}

	messages := make([]chatMessage, 0, len(history)+2)
	messages = append(messages, chatMessage{Role: "system", Content: systemContent})
	messages = append(messages, history...)
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})

	tools := s.botTools()
	toolsUsed := make([]string, 0, 4)
	thoughtTrace := make([]ThoughtStep, 0, 8)

	httpClient := &http.Client{Timeout: 120 * time.Second}

	// Multi-turn tool calling loop (configurable, default 10 hops with automatic synthesis)
	maxHops := inst.MaxToolHops
	if maxHops <= 0 {
		maxHops = 10
	}
	if maxHops > 25 {
		maxHops = 25
	}
	for hop := 0; hop < maxHops; hop++ {
		currentTools := tools
		// On the final hop, strip tool definitions and prompt the model to synthesize findings
		if hop == maxHops-1 {
			currentTools = nil
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "（系统提示：已收集充足线索，请结合上述全部已查询到的情报数据，用符合你人设的口吻直接整理输出最终分析结论与完整回答，无需再调用工具）",
			})
		}

		var respBytes []byte
		var lastStatus int
		currentModel := model
		for attempt := 0; attempt < 4; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return "", nil, thoughtTrace, ctx.Err()
				case <-time.After(time.Duration(400*(1<<attempt)) * time.Millisecond):
				}
				// Retry the configured model only. Switching models on provider
				// overload makes behavior and cost non-deterministic.
			}

			reqBody := chatCompletionRequest{
				Model:       currentModel,
				Messages:    messages,
				Tools:       currentTools,
				Temperature: temp,
				MaxTokens:   maxTokens,
			}
			bodyBytes, err := json.Marshal(reqBody)
			if err != nil {
				return "", nil, thoughtTrace, err
			}

			modelKey := strings.TrimSpace(apiBase) + "::" + strings.TrimSpace(currentModel)
			release, err := analysis.GetGlobalQueueManager().Acquire(ctx, modelKey)
			if err != nil {
				return "", nil, thoughtTrace, fmt.Errorf("llm queue acquire: %w", err)
			}

			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase, bytes.NewReader(bodyBytes))
			if err != nil {
				release()
				return "", nil, thoughtTrace, fmt.Errorf("create llm request: %w", err)
			}
			httpReq.Header.Set("Content-Type", "application/json")
			if inst.LLMAPIKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+inst.LLMAPIKey)
			}

			resp, err := httpClient.Do(httpReq)
			release()
			if err != nil {
				continue
			}

			lastStatus = resp.StatusCode
			respBytes, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			if lastStatus == http.StatusOK {
				break
			}
			if lastStatus != http.StatusTooManyRequests && lastStatus < 500 {
				break
			}
		}

		if lastStatus != http.StatusOK {
			return "", nil, thoughtTrace, fmt.Errorf("llm api error (status %d): %s", lastStatus, string(respBytes))
		}

		var chatResp chatCompletionResponse
		if err := json.Unmarshal(respBytes, &chatResp); err != nil {
			return "", nil, thoughtTrace, fmt.Errorf("unmarshal llm response: %w", err)
		}
		if chatResp.Error != nil {
			return "", nil, thoughtTrace, fmt.Errorf("llm returned error: %s", chatResp.Error.Message)
		}
		if len(chatResp.Choices) == 0 {
			return "", nil, thoughtTrace, fmt.Errorf("llm returned empty choices")
		}

		choice := chatResp.Choices[0]
		assistantMsg := choice.Message

		// Record model reasoning / thought if present
		thoughtText := strings.TrimSpace(assistantMsg.ReasoningContent)
		if thoughtText == "" {
			thoughtText = strings.TrimSpace(assistantMsg.Thought)
		}
		if thoughtText != "" {
			thoughtTrace = append(thoughtTrace, ThoughtStep{
				Hop:     hop + 1,
				Type:    "thinking",
				Thought: thoughtText,
				Time:    time.Now().Format("15:04:05"),
			})
			s.updateAuditThought(ctx, auditID, toolsUsed, thoughtTrace)
		}

		if len(assistantMsg.ToolCalls) == 0 || hop == maxHops-1 {
			// Final natural language response generated
			finalReply := strings.TrimSpace(assistantMsg.Content)
			if finalReply != "" && s.Memory != nil {
				s.Memory.Append(sessionKey, userPrompt, finalReply)
			}
			return finalReply, toolsUsed, thoughtTrace, nil
		}

		// Tool calls detected: append assistant message and execute tools
		messages = append(messages, assistantMsg)
		for _, tc := range assistantMsg.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Function.Name)

			callStep := ThoughtStep{
				Hop:       hop + 1,
				Type:      "tool_call",
				Tool:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Thought:   thoughtText,
				Time:      time.Now().Format("15:04:05"),
			}
			thoughtTrace = append(thoughtTrace, callStep)
			s.updateAuditThought(ctx, auditID, toolsUsed, thoughtTrace)

			toolStart := time.Now()
			toolResult, err := s.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				toolResult = fmt.Sprintf(`{"error": "%s"}`, err.Error())
			}
			toolLatency := int(time.Since(toolStart).Milliseconds())

			preview := toolResult
			if len(preview) > 600 {
				preview = preview[:600] + "... (已截断)"
			}
			resStep := ThoughtStep{
				Hop:           hop + 1,
				Type:          "tool_result",
				Tool:          tc.Function.Name,
				ResultPreview: preview,
				LatencyMs:     toolLatency,
				Time:          time.Now().Format("15:04:05"),
			}
			thoughtTrace = append(thoughtTrace, resStep)
			s.updateAuditThought(ctx, auditID, toolsUsed, thoughtTrace)

			messages = append(messages, chatMessage{
				Role:       "tool",
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    toolResult,
			})
		}
	}

	return "喵呜……老吴看了一大堆线索，不过脑子快转冒烟啦，下次记得问具体一点喵~", toolsUsed, thoughtTrace, nil
}
