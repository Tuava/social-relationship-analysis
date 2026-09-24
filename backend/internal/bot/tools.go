package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/mcp"
)

type toolGroupScopeKey struct{}

func withToolGroupScope(ctx context.Context, groupID string) context.Context {
	if strings.TrimSpace(groupID) == "" {
		return ctx
	}
	return context.WithValue(ctx, toolGroupScopeKey{}, groupID)
}

func toolGroupScope(ctx context.Context) string {
	groupID, _ := ctx.Value(toolGroupScopeKey{}).(string)
	return groupID
}

// LLMToolDefinition defines an OpenAI-compatible function tool.
type LLMToolDefinition struct {
	Type     string          `json:"type"`
	Function LLMToolFunction `json:"function"`
}

type LLMToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// botTools returns all MCP intelligence tools available to the LLM.
func (s *Service) botTools() []LLMToolDefinition {
	// If SRA MCP Handler Registry is available, export MCP intelligence tools.
	mcpTools := mcp.BaseTools()
	allowedPrefixes := []string{
		"sra_search",
		"sra_discover_connections",
		"sra_person_dossier",
		"sra_group_intel",
		"sra_profile_evolution",
		"sra_ego_network",
		"sra_find_path",
		"sra_mutual_analysis",
		"sra_activity_timeline",
		"sra_interaction_stream",
		"sra_content_feed",
		"sra_message_history",
		"sra_person_detail",
		"sra_group_detail",
		"sra_coverage_audit",
		"sra_raw_evidence",
		"sra_system_overview",
		"sra_realtime_stats",
		"sra_person_persona",
		"sra_feed_dynamics",
		"sra_trigger_vision_analysis",
		"sra_trigger_dialogue_disentanglement",
		"sra_dialogue_threads",
		"sra_thread_detail",
		"sra_media_ocr_search",
		"sra_image_diffusion_graph",
		"sra_propose_inference",
	}

	isAllowed := func(name string) bool {
		for _, prefix := range allowedPrefixes {
			if name == prefix {
				return true
			}
		}
		return false
	}

	tools := make([]LLMToolDefinition, 0, len(mcpTools)+1)

	// 1. Live Web Search Tool
	tools = append(tools, LLMToolDefinition{
		Type: "function",
		Function: LLMToolFunction{
			Name:        "web_search",
			Description: "实时全网搜索工具。当用户询问外部实时资讯、今日天气、最新科技/时事新闻、百科常识、网络公开文章或社交图谱数据库之外的公开信息时，使用此工具进行实时联网检索。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "搜索关键词或查询语句",
					},
					"count": map[string]any{
						"type":        "integer",
						"description": "期望返回的搜索结果条数 (默认 5 条，最多 10 条)",
					},
				},
				"required": []string{"query"},
			},
		},
	})

	for _, t := range mcpTools {
		if !isAllowed(t.Name) {
			continue
		}
		var params map[string]any
		schemaBytes, _ := json.Marshal(t.InputSchema)
		_ = json.Unmarshal(schemaBytes, &params)
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, LLMToolDefinition{
			Type: "function",
			Function: LLMToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return tools
}

// executeTool runs the MCP tool chosen by the LLM and returns the string result.
func (s *Service) executeTool(ctx context.Context, name string, argumentsJSON string) (string, error) {
	// Normalize arguments in case LLM emitted empty string or invalid JSON
	args := strings.TrimSpace(argumentsJSON)
	if args == "" {
		args = "{}"
	}
	if groupID := toolGroupScope(ctx); groupID != "" && (name == "sra_discover_connections" || name == "sra_ego_network") {
		var scoped map[string]any
		if json.Unmarshal([]byte(args), &scoped) == nil {
			if _, supplied := scoped["group_id"]; !supplied {
				scoped["group_id"] = groupID
				if encoded, err := json.Marshal(scoped); err == nil {
					args = string(encoded)
				}
			}
		}
	}

	if name == "web_search" || name == "internet_search" {
		var p struct {
			Query string `json:"query"`
			Count int    `json:"count"`
		}
		_ = json.Unmarshal([]byte(args), &p)
		res, err := s.SearchWeb(ctx, p.Query, p.Count)
		if err != nil {
			return fmt.Sprintf(`{"error": "%s"}`, err.Error()), nil
		}
		outBytes, _ := json.Marshal(res)
		return string(outBytes), nil
	}

	if s.MCP == nil {
		return fmt.Sprintf(`{"error": "MCP registry not initialized"}`), nil
	}

	res, err := s.MCP.CallTool(ctx, name, json.RawMessage(args))
	if err != nil {
		return fmt.Sprintf(`{"error": "%s"}`, err.Error()), nil
	}
	if res == nil {
		return `{"result": "empty response"}`, nil
	}

	if len(res.Content) > 0 {
		var combined strings.Builder
		for i, item := range res.Content {
			if i > 0 {
				combined.WriteString("\n")
			}
			combined.WriteString(item.Text)
		}
		return combined.String(), nil
	}

	return `{"result": "ok"}`, nil
}
