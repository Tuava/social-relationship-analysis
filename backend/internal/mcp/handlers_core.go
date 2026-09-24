package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

// HandlerRegistry dispatches tool calls to the appropriate handler method.
type HandlerRegistry struct {
	DB           *pgxpool.Pool
	SQLDB        *pgxpool.Pool
	Repo         *persistence.Repository
	Policy       analysis.RoutingPolicy
	Capabilities []domain.Capability
}

// CallTool routes a tool invocation to the correct handler.
func (h *HandlerRegistry) CallTool(ctx context.Context, name string, args json.RawMessage) (*ToolCallResult, error) {
	switch name {
	// Tier 1: Reconnaissance
	case "sra_search":
		return h.handleSearch(ctx, args)
	case "sra_discover_connections":
		return h.handleDiscoverConnections(ctx, args)
	// Tier 2: Profiling
	case "sra_person_dossier":
		return h.handlePersonDossier(ctx, args)
	case "sra_group_intel":
		return h.handleGroupIntel(ctx, args)
	case "sra_profile_evolution":
		return h.handleProfileEvolution(ctx, args)
	// Tier 3: Network Intelligence
	case "sra_ego_network":
		return h.handleEgoNetwork(ctx, args)
	case "sra_find_path":
		return h.handleFindPath(ctx, args)
	case "sra_mutual_analysis":
		return h.handleMutualAnalysis(ctx, args)
	// Tier 4: Temporal & Behavioral
	case "sra_activity_timeline":
		return h.handleActivityTimeline(ctx, args)
	case "sra_interaction_stream":
		return h.handleInteractionStream(ctx, args)
	case "sra_content_feed":
		return h.handleContentFeed(ctx, args)
	// Tier 5: Operations
	case "sra_collection_orchestrate":
		return h.handleCollectionOrchestrate(ctx, args)
	case "sra_collection_status":
		return h.handleCollectionStatus(ctx, args)
	case "sra_coverage_audit":
		return h.handleCoverageAudit(ctx, args)
	case "sra_raw_evidence":
		return h.handleRawEvidence(ctx, args)
	// Tier 6: Power Tools
	case "sra_sql_query":
		return h.handleSQLQuery(ctx, args)
	case "sra_system_overview":
		return h.handleSystemOverview(ctx, args)
	case "sra_resolve":
		return h.handleResolve(ctx, args)
	case "sra_schema":
		return h.handleSchema(ctx, args)
	case "sra_message_history":
		return h.handleMessageHistory(ctx, args)
	case "sra_relation_events":
		return h.handleRelationEvents(ctx, args)
	case "sra_media_stats":
		return h.handleMediaStats(ctx, args)
	case "sra_media_references":
		return h.handleMediaReferences(ctx, args)
	case "sra_accounts":
		return h.handleAccounts(ctx, args)
	case "sra_collection_runs":
		return h.handleCollectionRuns(ctx, args)
	case "sra_collection_candidates":
		return h.handleCollectionCandidates(ctx, args)
	case "sra_capabilities":
		return h.handleCapabilities(ctx, args)
	case "sra_inferences":
		return h.handleInferences(ctx, args)
	case "sra_ai_runs":
		return h.handleAIRuns(ctx, args)
	case "sra_evidence_packs":
		return h.handleEvidencePacks(ctx, args)
	case "sra_operations":
		return h.handleOperations(ctx, args)
	case "sra_research_workspaces":
		return h.handleResearchWorkspaces(ctx, args)
	case "sra_source_connections":
		return h.handleSourceConnections(ctx, args)
	case "sra_propose_inference":
		return h.handleProposeInference(ctx, args)
	case "sra_person_detail":
		return h.handlePersonDetail(ctx, args)
	case "sra_person_timeline":
		return h.handlePersonTimeline(ctx, args)
	case "sra_group_detail":
		return h.handleGroupDetail(ctx, args)
	case "sra_conversations":
		return h.handleConversations(ctx, args)
	case "sra_conversation_context":
		return h.handleConversationContext(ctx, args)
	case "sra_message_detail":
		return h.handleMessageDetail(ctx, args)
	case "sra_content_detail":
		return h.handleContentDetail(ctx, args)
	case "sra_content_likes":
		return h.handleContentLikes(ctx, args)
	case "sra_ego_networks":
		return h.handleEgoNetworks(ctx, args)
	case "sra_ego_network_detail":
		return h.handleEgoNetworkDetail(ctx, args)
	case "sra_realtime_stats":
		return h.handleRealtimeStats(ctx, args)
	case "sra_qzone_connections":
		return h.handleQZoneConnections(ctx, args)
	case "sra_collection_modules":
		return h.handleCollectionModules(ctx, args)
	case "sra_collection_events":
		return h.handleCollectionEvents(ctx, args)
	case "sra_ai_run_detail":
		return h.handleAIRunDetail(ctx, args)
	case "sra_evidence_pack_detail":
		return h.handleEvidencePackDetail(ctx, args)
	case "sra_operation_audits":
		return h.handleOperationAudits(ctx, args)
	case "sra_evidence_details":
		return h.handleEvidenceDetails(ctx, args)
	case "sra_napcat_read":
		return h.handleNapcatRead(ctx, args)
	case "sra_operation_preview":
		return h.handleOperationPreview(ctx, args)
	case "sra_content_comments":
		return h.handleContentComments(ctx, args)
	case "sra_visitor_stream":
		return h.handleVisitorStream(ctx, args)
	case "sra_build_evidence_pack":
		return h.handleBuildEvidencePack(ctx, args)
	case "sra_hypothesis_check":
		return h.handleHypothesisCheck(ctx, args)
	case "sra_trace_claim":
		return h.handleTraceClaim(ctx, args)
	case "sra_media_ocr_search":
		return h.handleMediaOCRSearch(ctx, args)
	case "sra_image_diffusion_graph":
		return h.handleImageDiffusionGraph(ctx, args)
	case "sra_dialogue_threads":
		return h.handleDialogueThreads(ctx, args)
	case "sra_thread_detail":
		return h.handleThreadDetail(ctx, args)
	case "sra_person_persona":
		return h.handlePersonPersona(ctx, args)
	case "sra_feed_dynamics":
		return h.handleFeedDynamics(ctx, args)
	case "sra_trigger_vision_analysis":
		return h.handleTriggerVisionAnalysis(ctx, args)
	case "sra_trigger_dialogue_disentanglement":
		return h.handleTriggerDialogueDisentanglement(ctx, args)
	// Legacy aliases remain callable for existing MCP clients. They are not
	// advertised by tools/list; new clients should use the tiered tools above.
	case "sra_search_persons":
		return h.handleSearchPersons(ctx, args)
	case "sra_get_person_profile":
		return h.handleGetPersonProfile(ctx, args)
	case "sra_analyze_ego_network":
		return h.handleAnalyzeEgoNetwork(ctx, args)
	case "sra_query_relationship":
		return h.handleQueryRelationship(ctx, args)
	case "sra_search_groups":
		return h.handleSearchGroups(ctx, args)
	case "sra_search_contents":
		return h.handleSearchContents(ctx, args)
	case "sra_trigger_collection":
		return h.handleTriggerCollection(ctx, args)
	case "sra_get_collection_status":
		return h.handleGetCollectionStatus(ctx, args)
	case "sra_get_raw_evidence":
		return h.handleGetRawEvidence(ctx, args)
	default:
		return errorResult("unknown tool: " + name), nil
	}
}

// resolvePersonID resolves a QQ number to a person UUID.
func resolvePersonID(ctx context.Context, db *pgxpool.Pool, qq string) (string, error) {
	var personID string
	err := db.QueryRow(ctx,
		`SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`,
		qq).Scan(&personID)
	if err != nil {
		return "", fmt.Errorf("person not found for QQ %s: %w", qq, err)
	}
	return personID, nil
}

// jsonResult marshals any value into a ToolCallResult with JSON text content.
func jsonResult(v any) (*ToolCallResult, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return errorResult("failed to format JSON: " + err.Error()), nil
	}
	return &ToolCallResult{
		ResultType:        "complete",
		Content:           []ContentItem{{Type: "text", Text: string(bytes)}},
		StructuredContent: v,
	}, nil
}

func pagedResult(data any, limit, offset, total int) any {
	nextOffset := offset + limit
	hasMore := int64(nextOffset) < int64(total)
	var nextCursor any
	if hasMore {
		nextCursor = nextOffset
	}
	return map[string]any{
		"data":        data,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	}
}

func NormalizedRoutingPolicy(policy analysis.RoutingPolicy) analysis.RoutingPolicy {
	if policy.DefaultMaxHops <= 0 {
		policy.DefaultMaxHops = 4
	}
	if policy.DefaultMaxPaths <= 0 {
		policy.DefaultMaxPaths = 3
	}
	if policy.DefaultLargeGroupThreshold <= 0 {
		policy.DefaultLargeGroupThreshold = 100
	}
	return policy
}

func normalizedRoutingPolicy(policy analysis.RoutingPolicy) analysis.RoutingPolicy {
	return NormalizedRoutingPolicy(policy)
}

// errorResult creates an error ToolCallResult with a structured, machine-readable body.
func errorResult(msg string) *ToolCallResult {
	return errorResultCode(msg, deriveErrorCode(msg))
}

// errorResultCode creates an error ToolCallResult with an explicit error code.
func errorResultCode(msg, code string) *ToolCallResult {
	body, _ := json.Marshal(map[string]any{
		"ok":      false,
		"code":    code,
		"message": msg,
	})
	return &ToolCallResult{
		IsError:    true,
		ResultType: "complete",
		Content:    []ContentItem{{Type: "text", Text: string(body)}},
	}
}

// deriveErrorCode maps an error message to a stable machine-readable code.
//
// The code set is intentionally small and stable so MCP clients can branch on
// it without coupling to human-readable text: not_found, invalid_arguments,
// database_error, internal_error.
func deriveErrorCode(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"),
		strings.Contains(lower, "no record"),
		strings.Contains(lower, "no rows"),
		strings.Contains(lower, "missing_event"):
		return "not_found"
	case strings.Contains(lower, "invalid arguments"),
		strings.Contains(lower, "invalid args"),
		strings.Contains(lower, "required"),
		strings.Contains(lower, "cannot be empty"),
		strings.Contains(lower, "must be between"),
		strings.Contains(lower, "must provide"),
		strings.Contains(lower, "at least one"),
		strings.Contains(lower, "unknown tool"):
		return "invalid_arguments"
	case strings.Contains(lower, "database"),
		strings.Contains(lower, "sql"),
		strings.Contains(lower, "constraint"),
		strings.Contains(lower, "query failed"),
		strings.Contains(lower, "insertion returned no row"):
		return "database_error"
	default:
		return "internal_error"
	}
}

// clampInt clamps value between min and max.
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// defaultInt returns value if > 0, otherwise defaultVal.
func defaultInt(v, defaultVal int) int {
	if v <= 0 {
		return defaultVal
	}
	return v
}

// containsSection checks if sections list contains a specific section or "all".
func containsSection(sections []string, target string) bool {
	for _, s := range sections {
		if s == "all" || s == target {
			return true
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	return &value
}
