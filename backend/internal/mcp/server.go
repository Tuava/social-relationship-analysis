package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type Server struct {
	db       *pgxpool.Pool
	repo     *persistence.Repository
	handlers *HandlerRegistry
	logger   *slog.Logger
	in       io.Reader
	out      io.Writer
}

func NewServer(db *pgxpool.Pool, repo *persistence.Repository, logger *slog.Logger) *Server {
	return NewServerWithPolicy(db, repo, logger, normalizedRoutingPolicy(analysis.RoutingPolicy{}))
}

func NewServerWithPolicy(db *pgxpool.Pool, repo *persistence.Repository, logger *slog.Logger, policy analysis.RoutingPolicy) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	policy = normalizedRoutingPolicy(policy)
	return &Server{
		db:       db,
		repo:     repo,
		handlers: &HandlerRegistry{DB: db, Repo: repo, Policy: policy},
		logger:   logger,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

func (s *Server) SetSQLDatabase(pool *pgxpool.Pool) { s.handlers.SQLDB = pool }

func (s *Server) SetIO(in io.Reader, out io.Writer) {
	s.in = in
	s.out = out
}

func (s *Server) SetCapabilities(caps []domain.Capability) {
	s.handlers.Capabilities = caps
}

func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("Starting Social Relationship Analysis MCP Server", "latestProtocolVersion", SupportedProtocolVersions[0], "supportedVersions", SupportedProtocolVersions)
	scanner := bufio.NewScanner(s.in)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		s.handleRequest(ctx, &req)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		s.logger.Error("Scanner read error", "error", err)
		return err
	}
	return nil
}

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	switch req.Method {
	// MCP 2026-07-28 standard discovery method
	case "server/discover":
		s.sendResult(req.ID, DiscoverResult{
			SupportedVersions: SupportedProtocolVersions,
			Capabilities: ServerCapabilities{
				Tools:       &ToolsCapability{ListChanged: false},
				Resources:   &ResourcesCapability{Subscribe: false, ListChanged: false},
				Prompts:     &PromptsCapability{ListChanged: false},
				Completions: &CompletionsCapability{},
			},
			Instructions: serverInstructions(),
			Meta: map[string]any{
				"io.modelcontextprotocol/serverInfo": ImplementationInfo{
					Name:    "social-relationship-analysis-mcp",
					Version: "1.0.0",
				},
			},
		})

	// Legacy & backward-compatible handshake
	case "initialize":
		var params InitializeParams
		_ = json.Unmarshal(req.Params, &params)
		negotiatedVersion := NegotiateProtocolVersion(params.ProtocolVersion)

		s.sendResult(req.ID, InitializeResult{
			ProtocolVersion: negotiatedVersion,
			Capabilities: ServerCapabilities{
				Tools:       &ToolsCapability{ListChanged: false},
				Resources:   &ResourcesCapability{Subscribe: false, ListChanged: false},
				Prompts:     &PromptsCapability{ListChanged: false},
				Completions: &CompletionsCapability{},
			},
			ServerInfo: ImplementationInfo{
				Name:    "social-relationship-analysis-mcp",
				Version: "1.0.0",
			},
			Instructions: serverInstructions(),
		})

	case "notifications/initialized":
		s.logger.Info("MCP client initialized notification received")

	case "ping":
		s.sendResult(req.ID, map[string]any{})

	case "tools/list":
		s.sendResult(req.ID, map[string]any{
			"resultType": "complete",
			"ttlMs":      60000,
			"tools":      AllTools(),
		})

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		result, err := s.handlers.CallTool(ctx, params.Name, params.Arguments)
		if err != nil {
			// Tool execution failures are results marked isError, not
			// JSON-RPC transport errors, so clients can read the structured
			// error body ({ok, code, message}) instead of losing it.
			s.sendResult(req.ID, errorResult(err.Error()))
			return
		}
		s.sendResult(req.ID, result)

	case "resources/list":
		s.sendResult(req.ID, map[string]any{
			"resources": AllResources(),
		})

	case "resources/templates/list":
		s.sendResult(req.ID, map[string]any{
			"resourceTemplates": AllResourceTemplates(),
		})

	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		result, err := ReadResource(ctx, s.db, params.URI)
		if err != nil {
			s.sendErrorWithCode(req.ID, -32603, err.Error(), deriveErrorCode(err.Error()))
			return
		}
		s.sendResult(req.ID, result)

	case "prompts/list":
		s.sendResult(req.ID, map[string]any{
			"prompts": AllPrompts(),
		})

	case "prompts/get":
		var params GetPromptParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		result, err := GetPrompt(params.Name, params.Arguments)
		if err != nil {
			s.sendError(req.ID, -32603, "Prompt error: "+err.Error())
			return
		}
		s.sendResult(req.ID, result)

	case "completion/complete":
		var params CompleteParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		values := completeValues(params)
		if values == nil {
			values = []string{}
		}
		s.sendResult(req.ID, CompleteResult{
			Completion: CompletionDetails{
				Values:  values,
				Total:   len(values),
				HasMore: false,
			},
		})

	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *Server) sendResult(id any, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(resp)
}

func (s *Server) sendError(id any, code int, message string) {
	s.sendErrorWithCode(id, code, message, "")
}

// sendErrorWithCode sends a JSON-RPC error whose Data carries a stable
// machine-readable code alongside the human message.
func (s *Server) sendErrorWithCode(id any, code int, message, errorCode string) {
	var data any
	if errorCode != "" {
		data = map[string]any{"code": errorCode}
	}
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.writeJSON(resp)
}

// serverInstructions is returned by both server/discover and initialize. The
// first 512 characters are self-contained so clients make good tool-selection
// decisions even when the rest is truncated.
func serverInstructions() string {
	return `This server is the Social Relationship Analysis (SRA) OSINT platform for QQ: persons, groups, QZone contents, messages and relation events, every record traceable to raw evidence. Most tools are read-only; collection and side-effect operations require human confirmation and never execute implicitly.

Workflow:
1. sra_resolve a QQ/nickname/group first to normalize identifiers.
2. Prefer semantic tools (sra_person_dossier, sra_ego_network, sra_find_path, sra_activity_timeline, sra_interaction_stream, sra_content_feed, sra_content_comments, sra_visitor_stream) over raw sra_sql_query.
3. For claims about a person use sra_hypothesis_check and sra_build_evidence_pack, then verify quotes via sra_raw_evidence / sra_evidence_details. Inferences are hypotheses, not facts: report confidence, supporting/contradicting evidence and review status.
4. sra_collection_orchestrate and sra_operation_preview only create requests; a human confirms them in the operation center. Never claim an operation already executed.
5. Pagination uses offset; per-page caps are documented on each tool. Request only the sections you need to keep responses token-efficient.

中文要点：一切结论必须能回溯到原始证据；自述不等于事实；推断不写入事实表；副作用操作必须人工确认。`
}
func (s *Server) writeJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("Failed to marshal JSON-RPC response", "error", err)
		return
	}
	data = append(data, '\n')
	_, _ = s.out.Write(data)
}
