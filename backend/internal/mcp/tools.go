package mcp

import "strings"

// BaseTools returns the raw catalog of OSINT intelligence tools supported by
// the Social Relationship Analysis MCP server.
// Organized into 7 tiers: Reconnaissance, Profiling, Network Intelligence, Temporal/Behavioral, Content Intelligence, Operations, Power Tools.
func BaseTools() []Tool {
	return []Tool{
		// ═══════════════════════════════════════════════════════════════════
		// Tier 1: Reconnaissance & Discovery
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_search",
			Description: "Unified multi-entity faceted search across persons, groups, and contents. Supports filtering by activity recency, group co-membership, interaction count thresholds, and time ranges. Use this as the primary discovery tool.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"entity_type":      {Type: "string", Enum: []string{"person", "group", "content"}, Description: "What to search for"},
					"keyword":          {Type: "string", Description: "Fuzzy match on name/QQ/nickname/body text"},
					"in_group":         {Type: "string", Description: "Filter persons who are members of this QQ group number"},
					"active_since":     {Type: "string", Description: "ISO date string. Only return entities with activity after this date"},
					"active_before":    {Type: "string", Description: "ISO date string. Only return entities with activity before this date"},
					"min_interactions": {Type: "integer", Description: "Minimum total interaction event count (likes+comments+messages)"},
					"sort_by":          {Type: "string", Enum: []string{"relevance", "recent_activity", "interaction_count", "group_count"}, Default: "relevance", Description: "Sort order for results"},
					"limit":            {Type: "integer", Default: 30, Description: "Maximum results to return (max 100)"},
					"offset":           {Type: "integer", Default: 0, Description: "Pagination offset"},
				},
				Required: []string{"entity_type"},
			},
		},
		{
			Name:        "sra_discover_connections",
			Description: "Find persons connected to a target through specific interaction channels. Discover who likes their posts most, who comments most, who shares the most groups, who messages them. Returns a ranked connection list with interaction breakdown and directionality.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"target_qq":         {Type: "string", Description: "Target QQ number to find connections for"},
					"group_id":          {Type: "string", Description: "Optional QQ group number. Restricts relationship counts to this group."},
					"interaction_types": {Type: "array", Description: "Filter by specific interaction types: liked, commented, replied_to, sent_message, member_of, mentioned, friend_visible. Omit for all types."},
					"direction":         {Type: "string", Enum: []string{"incoming", "outgoing", "both"}, Default: "both", Description: "incoming=who interacts WITH target, outgoing=who target interacts WITH"},
					"min_weight":        {Type: "integer", Default: 1, Description: "Minimum interaction count to include a connection"},
					"time_range_start":  {Type: "string", Description: "ISO date. Only count interactions after this date"},
					"time_range_end":    {Type: "string", Description: "ISO date. Only count interactions before this date"},
					"limit":             {Type: "integer", Default: 50, Description: "Maximum connections to return"},
				},
				Required: []string{"target_qq"},
			},
		},

		// ═══════════════════════════════════════════════════════════════════
		// Tier 2: Deep Profiling
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_person_dossier",
			Description: "Comprehensive intelligence dossier for a target person. Includes: identity (QQ, nicknames, avatars), behavioral fingerprint (active hours distribution, posting frequency, peak activity), social footprint (groups, top interactors, interaction summary), collection coverage map (what data has been collected vs gaps), and profile evolution timeline (nickname/avatar/signature changes). Use 'sections' parameter to request only needed parts and save tokens.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq":       {Type: "string", Description: "Target QQ number"},
					"sections": {Type: "array", Description: "Which dossier sections to include: identity, behavioral, social, coverage, evolution, all. Default: all. Use specific sections to reduce response size."},
				},
				Required: []string{"qq"},
			},
		},
		{
			Name:        "sra_group_intel",
			Description: "Group intelligence report: member roster with activity ranking, key contributors (most messages/interactions), and cross-group overlap analysis (which members also appear in other specified groups).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"group_id":               {Type: "string", Description: "QQ group number"},
					"include_roster":         {Type: "boolean", Default: true, Description: "Include full member roster"},
					"compare_with_groups":    {Type: "array", Description: "Other QQ group numbers to compute member overlap with"},
					"top_contributors_limit": {Type: "integer", Default: 20, Description: "Number of top contributors to return"},
				},
				Required: []string{"group_id"},
			},
		},
		{
			Name:        "sra_profile_evolution",
			Description: "Track identity changes over time: nickname history, avatar changes, signature changes. Uses temporal profile snapshots to detect identity shifts, account transfers, or behavioral pivots.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq":                    {Type: "string", Description: "Target QQ number"},
					"include_raw_snapshots": {Type: "boolean", Default: false, Description: "Include full raw profile observation payloads (verbose, use sparingly)"},
				},
				Required: []string{"qq"},
			},
		},

		// ═══════════════════════════════════════════════════════════════════
		// Tier 3: Network Intelligence
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_ego_network",
			Description: "Build ego network with pre-computed graph analytics: degree centrality, betweenness centrality (bridge detection), community clusters, and interaction-weighted rankings. Returns computed metrics so the LLM does not need to run graph algorithms. Use 'summary' format for token-efficient overview, 'full' for complete topology.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"target_qq":       {Type: "string", Description: "Target QQ number to center the ego network on"},
					"group_id":        {Type: "string", Description: "Optional QQ group number. Restricts graph evidence to this group."},
					"depth":           {Type: "integer", Default: 1, Description: "BFS depth (1=direct contacts, 2=secondary, 3=extended). Max 3."},
					"relation_filter": {Type: "array", Description: "Only include edges of these types: liked, commented, replied_to, sent_message, member_of, mentioned, friend_visible, published. Omit for all."},
					"max_nodes":       {Type: "integer", Description: "Optional node cap for this response. Omit for all nodes within the requested depth; this is not a collection limit."},
					"include_metrics": {Type: "boolean", Default: true, Description: "Pre-compute centrality and community metrics (recommended)"},
					"format":          {Type: "string", Enum: []string{"full", "summary"}, Default: "summary", Description: "summary=metrics+top nodes only (token-efficient), full=complete node/edge list"},
				},
				Required: []string{"target_qq"},
			},
		},
		{
			Name:        "sra_find_path",
			Description: "Find shortest connection path(s) between two persons through intermediary nodes. Returns the chain of persons and the relationship type at each hop. Essential for investigating 'how are these two people connected?' and 'who is the bridge between them?'",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"source_qq":       {Type: "string", Description: "Starting person QQ number"},
					"target_qq":       {Type: "string", Description: "Destination person QQ number"},
					"max_hops":        {Type: "integer", Default: 4, Description: "Maximum path length requested by the caller. Higher values may be slower."},
					"max_paths":       {Type: "integer", Default: 3, Description: "Return top N shortest paths"},
					"relation_filter": {Type: "array", Description: "Only traverse these relation types. Omit for all."},
				},
				Required: []string{"source_qq", "target_qq"},
			},
		},
		{
			Name:        "sra_mutual_analysis",
			Description: "Deep mutual/cross analysis between 2-5 persons: shared group memberships, mutual friends (persons both interact with), interaction history with directionality scoring (who initiates more), and relationship strength score.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq_list":  {Type: "array", Description: "2-5 QQ numbers to cross-analyze"},
					"sections": {Type: "array", Description: "Sections to include: shared_groups, mutual_friends, interactions, strength_score, all. Default: all."},
				},
				Required: []string{"qq_list"},
			},
		},

		// ═══════════════════════════════════════════════════════════════════
		// Tier 4: Temporal & Behavioral Analysis
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_activity_timeline",
			Description: "Generate activity heatmap and temporal patterns for a person or group. Reveals: hourly activity distribution (sleep pattern inference), daily/weekly rhythms, activity trend over time, and burst detection. Provides text insight summarizing patterns.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq":               {Type: "string", Description: "Target person QQ (mutually exclusive with group_id)"},
					"group_id":         {Type: "string", Description: "Target QQ group number (mutually exclusive with qq)"},
					"granularity":      {Type: "string", Enum: []string{"hourly", "daily", "weekly", "monthly"}, Default: "hourly", Description: "Time bucket granularity"},
					"time_range_start": {Type: "string", Description: "ISO date. Only count events after this date"},
					"time_range_end":   {Type: "string", Description: "ISO date. Only count events before this date"},
					"action_types":     {Type: "array", Description: "Filter by action types (e.g., only count messages)"},
				},
			},
		},
		{
			Name:        "sra_interaction_stream",
			Description: "Chronological stream of interaction events with full context. Each event shows: who did what to whom, on which post/message, when. Supports filtering by actor, target, action type, and time range. Includes content preview for context.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"actor_qq":                {Type: "string", Description: "Filter by the person who performed the action"},
					"target_qq":               {Type: "string", Description: "Filter by the person who received the action"},
					"action_types":            {Type: "array", Description: "Filter by action types: liked, commented, replied_to, sent_message, mentioned, published, member_of, friend_visible"},
					"time_range_start":        {Type: "string", Description: "ISO date start"},
					"time_range_end":          {Type: "string", Description: "ISO date end"},
					"include_content_preview": {Type: "boolean", Default: true, Description: "Include a preview of the target content body"},
					"limit":                   {Type: "integer", Default: 50, Description: "Maximum events to return (max 200)"},
					"offset":                  {Type: "integer", Default: 0, Description: "Pagination offset"},
				},
			},
		},
		{
			Name:        "sra_content_feed",
			Description: "Rich content feed with engagement metrics. Each post includes: body text, publish time, like count, comment count, and optionally the reply thread. Supports filtering by author, time range, minimum engagement, keyword, and sorting by popularity.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"author_qq":        {Type: "string", Description: "Filter by author QQ number"},
					"keyword":          {Type: "string", Description: "Full-text search in post body"},
					"time_range_start": {Type: "string", Description: "ISO date start"},
					"time_range_end":   {Type: "string", Description: "ISO date end"},
					"min_engagement":   {Type: "integer", Description: "Minimum total likes+comments on the post"},
					"sort_by":          {Type: "string", Enum: []string{"recent", "most_liked", "most_commented", "most_engaged"}, Default: "recent", Description: "Sort order"},
					"include_replies":  {Type: "boolean", Default: false, Description: "Include comment/reply thread under each post"},
					"limit":            {Type: "integer", Default: 20, Description: "Maximum posts to return"},
					"offset":           {Type: "integer", Default: 0, Description: "Pagination offset"},
				},
			},
		},

		// ═══════════════════════════════════════════════════════════════════
		// Tier 5: Collection & Operations
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_collection_orchestrate",
			Description: "Trigger a targeted collection run with fine-grained module control. Specify exactly which data sources to collect (feeds, likes, comments, visits, friends, profile, media) and which to skip. Supports multiple seed targets in one run.",
			Annotations: &ToolAnnotations{DestructiveHint: boolPtr(true), ReadOnlyHint: boolPtr(false), IdempotentHint: boolPtr(false)},
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"targets":   {Type: "array", Description: "Array of seed targets, each with 'type' (qq/group) and 'id' (the number)"},
					"modules":   {Type: "array", Description: "Which collection modules to run: basic_profile, qzone_feeds, qzone_likes, qzone_comments, qzone_visits, group_memberships, group_messages, private_messages, media, avatar. Omit for all modules."},
					"max_depth": {Type: "integer", Default: 1, Description: "Max recursive diffusion depth (1=target only, 2=target+contacts)"},
					"mode":      {Type: "string", Enum: []string{"expand_people", "full_collect", "profile_only"}, Default: "full_collect", Description: "Collection mode"},
				},
				Required: []string{"targets"},
			},
		},
		{
			Name:        "sra_collection_status",
			Description: "Query collection run status with per-module progress breakdown, candidate queue summary, and processing statistics.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"run_id": {Type: "string", Description: "Specific collection run UUID. Omit to query the latest run."},
				},
			},
		},
		{
			Name:        "sra_coverage_audit",
			Description: "Audit intelligence coverage for a list of persons. Shows which data sources have been successfully collected, which are partial, which have never been attempted. Identifies intelligence gaps and suggests high-value targets for next collection.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq_list":              {Type: "array", Description: "List of QQ numbers to audit coverage for"},
					"suggest_next_targets": {Type: "boolean", Default: true, Description: "Suggest which persons should be collected next for maximum intelligence gain"},
				},
				Required: []string{"qq_list"},
			},
		},
		{
			Name:        "sra_raw_evidence",
			Description: "Retrieve the raw network evidence record and original payload behind an intelligence event. Use for forensic verification and audit trails. For batch retrieval of multiple records use sra_evidence_details.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"evidence_id": {Type: "string", Description: "UUID of the raw record"},
				},
				Required: []string{"evidence_id"},
			},
		},

		// ═══════════════════════════════════════════════════════════════════
		// Tier 6: Power Tools
		// ═══════════════════════════════════════════════════════════════════
		{
			Name:        "sra_sql_query",
			Description: "Execute a read-only SQL query against the intelligence database for ad-hoc analysis. Use when no predefined tool fits your analytical question. Runs in a read-only transaction with 10-second timeout. Available tables: persons, person_identifiers, person_profiles, profile_observations, groups (use double quotes), group_memberships, contents, content_media, messages, relation_events (action_type: member_of/sent_message/commented/mentioned/liked/replied_to/published/friend_visible), collection_coverage, raw_records, media_assets, conversations, inferences, collection_runs, collection_candidates, collection_run_modules.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"sql":      {Type: "string", Description: "Read-only SQL query (SELECT only). Will be rejected if it contains INSERT/UPDATE/DELETE/DROP/ALTER/TRUNCATE."},
					"max_rows": {Type: "integer", Default: 50, Description: "Maximum rows to return (max 200)"},
				},
				Required: []string{"sql"},
			},
		},
		{
			Name:        "sra_system_overview",
			Description: "System-wide intelligence overview: total counts of all entities (persons, groups, contents, messages, events, media), latest collection run status, action type breakdown, and database health metrics.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "sra_resolve",
			Description: "Resolve a QQ number, nickname, or group identifier to canonical internal entity IDs. Returns compact person/group matches with exact-match priority. Use first when a prompt gives a name or QQ to normalize it before other tools.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"keyword":     {Type: "string", Description: "QQ number, nickname, or group ID/name to resolve."},
					"entity_type": {Type: "string", Enum: []string{"auto", "person", "group"}, Default: "auto", Description: "Limit resolution to a kind or infer automatically."},
					"limit":       {Type: "integer", Default: 20, Description: "Maximum matches to return."},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "sra_schema",
			Description: "Return a compact read-only schema of all public intelligence tables and their column types. Use before writing sra_sql_query or when deciding which semantic tool can answer a question.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "sra_message_history",
			Description: "Search and page through archived private or group messages by sender, group, conversation, keyword, or time range. Returns compact message records with source IDs, conversation identity, and media count.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"qq":                {Type: "string", Description: "Sender QQ number."},
					"group_id":          {Type: "string", Description: "Group QQ number. Implies group conversation type."},
					"conversation_id":   {Type: "string", Description: "Internal conversation UUID or platform conversation ID."},
					"conversation_type": {Type: "string", Enum: []string{"private", "group"}, Description: "Conversation kind."},
					"query":             {Type: "string", Description: "Keyword matched against text, sender name, or sender QQ."},
					"time_start":        {Type: "string", Description: "ISO timestamp lower bound."},
					"time_end":          {Type: "string", Description: "ISO timestamp upper bound."},
					"limit":             {Type: "integer", Default: 50, Description: "Maximum messages per page, up to 200."},
					"offset":            {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_relation_events",
			Description: "Query the normalized atomic relation event table with actor/target/action/time filters. Every returned event can be traced to a raw record ID for evidence verification.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"actor_qq":         {Type: "string", Description: "Actor QQ number."},
					"target_qq":        {Type: "string", Description: "Target person QQ number."},
					"action_types":     {Type: "array", Description: "Relation action types: member_of, sent_message, commented, mentioned, liked, replied_to, published, friend_visible, visited."},
					"context_type":     {Type: "string", Description: "Event context, for example group, qzone_post, qzone_comment, private."},
					"time_start":       {Type: "string", Description: "ISO timestamp lower bound."},
					"time_end":         {Type: "string", Description: "ISO timestamp upper bound."},
					"include_evidence": {Type: "boolean", Default: false, Description: "Include the evidence record count for each event."},
					"limit":            {Type: "integer", Default: 50, Description: "Maximum events per page, up to 200."},
					"offset":           {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_media_stats",
			Description: "Summarize media download policy, concurrency, archived asset count/bytes, and per-kind pending/completed/failed status.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "sra_media_references",
			Description: "Page through media references by kind, status, or filename/source/QQ keyword. Returns archive metadata without downloading or exposing raw URLs.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"kind":                {Type: "string", Enum: []string{"avatar", "image", "sticker", "audio", "video", "file"}, Description: "Media kind."},
					"status":              {Type: "string", Enum: []string{"pending", "downloading", "completed", "failed"}, Description: "Download status."},
					"query":               {Type: "string", Description: "Keyword in filename, source reference, QQ, or person name."},
					"include_recognition": {Type: "boolean", Default: false, Description: "Attach OCR text, transcript text, and recognition time from the archived asset (default false to keep responses small)."},
					"limit":               {Type: "integer", Default: 50, Description: "Maximum references per page, up to 200."},
					"offset":              {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_accounts",
			Description: "List connected NapCat and QZone source accounts with status and last-event timestamps. Secrets are never returned.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "sra_collection_runs",
			Description: "List collection runs by type and status with progress, errors, and lifecycle timestamps.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"type":   {Type: "string", Description: "Collection run type filter."},
					"status": {Type: "string", Description: "Collection run status filter."},
					"limit":  {Type: "integer", Default: 50, Description: "Maximum runs per page, up to 200."},
					"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_collection_candidates",
			Description: "Inspect the recursive candidate queue for a collection run, including entity, depth, priority, and discovery count.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"run_id": {Type: "string", Description: "Collection run UUID. Omit for the latest run."},
					"state":  {Type: "string", Description: "Candidate state filter."},
					"limit":  {Type: "integer", Default: 50, Description: "Maximum candidates per page, up to 200."},
					"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_capabilities",
			Description: "Search the loaded NapCat capability catalog. Use to discover available endpoints and their read-only/confirmation requirements before requesting collection or an operation.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"query":     {Type: "string", Description: "Keyword in endpoint, summary, or tag."},
					"read_only": {Type: "boolean", Description: "When true, return only read-only endpoints; when false, return only side-effect endpoints."},
					"limit":     {Type: "integer", Default: 100, Description: "Maximum capabilities to return, up to 500."},
					"offset":    {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_inferences",
			Description: "List saved inference claims with subject, attribute, confidence, review status, and referenced evidence events. Inferences are hypotheses, not fact rows.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"subject_id":     {Type: "string", Description: "Subject QQ or internal subject identifier."},
					"attribute_type": {Type: "string", Description: "Inference attribute type."},
					"review_status":  {Type: "string", Description: "Review status filter."},
					"min_confidence": {Type: "number", Description: "Minimum confidence score."},
					"limit":          {Type: "integer", Default: 50, Description: "Maximum results per page, up to 200."},
					"offset":         {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_ai_runs",
			Description: "List AI analysis runs and their lifecycle/model metadata without returning generated output payloads.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"task_type": {Type: "string", Description: "AI task type filter."},
					"status":    {Type: "string", Description: "Run status filter."},
					"limit":     {Type: "integer", Default: 50, Description: "Maximum results per page, up to 200."},
					"offset":    {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_evidence_packs",
			Description: "List controlled evidence packs: subject, scope, event IDs, redaction policy, and token budget.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"subject_id": {Type: "string", Description: "Subject filter."},
					"limit":      {Type: "integer", Default: 50, Description: "Maximum results per page, up to 200."},
					"offset":     {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_operations",
			Description: "List operation requests and confirmation state. Read-only; this tool never confirms or executes an operation.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"status": {Type: "string", Description: "Operation status filter."},
					"limit":  {Type: "integer", Default: 50, Description: "Maximum results per page, up to 200."},
					"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_research_workspaces",
			Description: "List saved research workspaces and draft/snapshot metadata without returning full workspace state.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"draft_only": {Type: "boolean", Default: false, Description: "Return only draft workspaces."},
					"limit":      {Type: "integer", Default: 50, Description: "Maximum results per page, up to 200."},
					"offset":     {Type: "integer", Default: 0, Description: "Pagination offset."},
				},
			},
		},
		{
			Name:        "sra_source_connections",
			Description: "List source connection status across NapCat and QZone transports with last-event and error state.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertyDef{},
			},
		},
		{
			Name:        "sra_propose_inference",
			Description: "Save a proposed evidence-backed inference for later human review. Writes to the inference layer only; it never modifies fact tables or raw records.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false)},
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"subject_id":         {Type: "string", Description: "Subject QQ number or internal subject identifier."},
					"attribute_type":     {Type: "string", Description: "Inference attribute, for example education, location, interest, or identity_link."},
					"value":              {Type: "object", Description: "JSON value describing the proposed conclusion."},
					"confidence":         {Type: "number", Description: "Confidence between 0 and 1."},
					"evidence_event_ids": {Type: "array", Description: "UUIDs of relation events or evidence records supporting this proposal."},
					"method_version":     {Type: "string", Description: "Version or prompt identifier used by the model."},
				},
				Required: []string{"subject_id", "attribute_type", "value", "confidence"},
			},
		},
		{
			Name:        "sra_person_detail",
			Description: "Return a person's full archived detail: identifiers, profile versions, group memberships, and aggregate counts. Use when the dossier summary is not enough. Prefer sra_person_dossier for the compact intelligence summary.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"qq":                {Type: "string", Description: "Target QQ number."},
				"sections":          {Type: "array", Description: "Sections to include: profiles, memberships, stats. Omit for all."},
				"profile_limit":     {Type: "integer", Default: 50, Description: "Profile observations per page, up to 200."},
				"profile_offset":    {Type: "integer", Default: 0, Description: "Profile observation pagination offset."},
				"membership_limit":  {Type: "integer", Default: 50, Description: "Membership observations per page, up to 200."},
				"membership_offset": {Type: "integer", Default: 0, Description: "Membership observation pagination offset."},
			}, Required: []string{"qq"}},
		},
		{
			Name:        "sra_person_timeline",
			Description: "Return a chronological timeline for a person across relations, messages, profile changes, and memberships.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"qq":         {Type: "string", Description: "Target QQ number."},
				"event_type": {Type: "string", Description: "Optional event type filter."},
				"limit":      {Type: "integer", Default: 50, Description: "Maximum events per page, up to 200."},
				"offset":     {Type: "integer", Default: 0, Description: "Pagination offset."},
			}, Required: []string{"qq"}},
		},
		{
			Name:        "sra_group_detail",
			Description: "Return a group's metadata, counts, and optional member roster. For activity rankings, contributors and cross-group overlap use sra_group_intel.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"group_id":        {Type: "string", Description: "Group UUID or QQ group number."},
				"include_members": {Type: "boolean", Default: false, Description: "Include member roster."},
				"member_limit":    {Type: "integer", Default: 50, Description: "Members per page, up to 200."},
				"member_offset":   {Type: "integer", Default: 0, Description: "Member pagination offset."},
			}, Required: []string{"group_id"}},
		},
		{
			Name:        "sra_conversations",
			Description: "List archived conversations with type, identity, message count, and last message time.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"type":   {Type: "string", Enum: []string{"private", "group"}, Description: "Conversation kind."},
				"query":  {Type: "string", Description: "Keyword in conversation ID, group name, or person name."},
				"limit":  {Type: "integer", Default: 50, Description: "Maximum conversations per page, up to 200."},
				"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
			}},
		},
		{
			Name:        "sra_conversation_context",
			Description: "Return conversation metadata and active member ranking for a group conversation.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"conversation_id": {Type: "string", Description: "Conversation UUID or platform conversation ID."},
				"member_limit":    {Type: "integer", Default: 50, Description: "Maximum active members to return."},
			}, Required: []string{"conversation_id"}},
		},
		{
			Name:        "sra_message_detail",
			Description: "Return a single message with sender, conversation, segments, reply target, and raw record ID.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"message_id": {Type: "string", Description: "Message UUID or source message ID."},
			}, Required: []string{"message_id"}},
		},
		{
			Name:        "sra_content_detail",
			Description: "Return a single QZone content item with author, metadata, media, and engagement counts.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"content_id": {Type: "string", Description: "Content UUID or platform content ID."},
			}, Required: []string{"content_id"}},
		},
		{
			Name:        "sra_content_likes",
			Description: "Return the like list for a single QZone content item.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"content_id": {Type: "string", Description: "Content UUID or platform content ID."},
				"limit":      {Type: "integer", Default: 50, Description: "Maximum likes per page, up to 200."},
				"offset":     {Type: "integer", Default: 0, Description: "Pagination offset."},
			}, Required: []string{"content_id"}},
		},
		{
			Name:        "sra_ego_networks",
			Description: "List saved ego network snapshots with target, depth, counts, truncation, and lifecycle state.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"limit":  {Type: "integer", Default: 50, Description: "Maximum networks per page, up to 100."},
				"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
			}},
		},
		{
			Name:        "sra_ego_network_detail",
			Description: "Return a saved ego network and optionally its stored nodes and edges.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"network_id":    {Type: "string", Description: "Saved ego network UUID."},
				"include_nodes": {Type: "boolean", Default: false, Description: "Include stored nodes."},
				"include_edges": {Type: "boolean", Default: false, Description: "Include stored edges."},
			}, Required: []string{"network_id"}},
		},
		{
			Name:        "sra_realtime_stats",
			Description: "Return ingestion freshness and latest timestamps for relations, messages, and raw records.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "sra_qzone_connections",
			Description: "List QZone source connections with status and last-event timestamps. Secrets are never returned.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{}},
		},
		{
			Name:        "sra_collection_modules",
			Description: "Return per-module progress, pages, records, errors, and timestamps for a collection run.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"run_id": {Type: "string", Description: "Collection run UUID. Omit for the latest run."},
			}},
		},
		{
			Name:        "sra_collection_events",
			Description: "Return the candidate discovery events for a collection run.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"run_id": {Type: "string", Description: "Collection run UUID. Omit for the latest run."},
				"limit":  {Type: "integer", Default: 50, Description: "Maximum events per page, up to 200."},
				"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
			}},
		},
		{
			Name:        "sra_ai_run_detail",
			Description: "Return a single AI run with its evidence pack and generated output.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"run_id": {Type: "string", Description: "AI run UUID."},
			}, Required: []string{"run_id"}},
		},
		{
			Name:        "sra_evidence_pack_detail",
			Description: "Return a single evidence pack with scope, event IDs, redaction policy, and token budget.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"pack_id": {Type: "string", Description: "Evidence pack UUID."},
			}, Required: []string{"pack_id"}},
		},
		{
			Name:        "sra_operation_audits",
			Description: "List operation audit records with request/response and execution status.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"limit":  {Type: "integer", Default: 50, Description: "Maximum audits per page, up to 100."},
				"offset": {Type: "integer", Default: 0, Description: "Pagination offset."},
			}},
		},
		{
			Name:        "sra_evidence_details",
			Description: "Retrieve multiple raw evidence records by UUID in one call. For a single raw record, sra_raw_evidence returns the same shape by id.",
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"evidence_ids": {Type: "array", Description: "Raw record UUIDs to fetch."},
				"limit":        {Type: "integer", Default: 50, Description: "Raw records per page, up to 200."},
				"offset":       {Type: "integer", Default: 0, Description: "Pagination offset."},
			}, Required: []string{"evidence_ids"}},
		},
		{
			Name:        "sra_napcat_read",
			Description: "Call a read-only NapCat endpoint from the loaded capability catalog through the selected account. Validates the endpoint is read-only, saves the raw response, and returns parsed JSON.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"account_id": {Type: "string", Description: "NapCat account UUID. Omit to use the first enabled HTTP account."},
				"endpoint":   {Type: "string", Description: "NapCat endpoint path, for example /get_group_list."},
				"parameters": {Type: "object", Description: "JSON object of query/body parameters."},
			}, Required: []string{"endpoint"}},
		},
		{
			Name:        "sra_content_comments",
			Description: "Read the QZone comment tree of a content item (main post or another comment). Comments are ordered by published time ascending so the thread reads as a timeline; replies are marked with reply_to_content_id and the replied author.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"content_id":      {Type: "string", Description: "Content UUID or platform content id of the post to read comments for."},
				"include_replies": {Type: "boolean", Default: true, Description: "Include replies to other comments (nested thread). Set false for top-level comments only."},
				"limit":           {Type: "integer", Default: 100, Description: "Maximum comments per page (max 200)."},
				"offset":          {Type: "integer", Default: 0, Description: "Pagination offset."},
			}, Required: []string{"content_id"}},
		},
		{
			Name:        "sra_visitor_stream",
			Description: "Aggregate QZone visit events between persons: who visited whom, when, and how often. Requires at least one of target_qq or actor_qq so the query always stays scoped to a person.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"target_qq":        {Type: "string", Description: "Person whose visitors should be listed."},
				"actor_qq":         {Type: "string", Description: "Person acting as visitor; lists whom they visited."},
				"time_range_start": {Type: "string", Description: "ISO date. Only count visits after this timestamp."},
				"time_range_end":   {Type: "string", Description: "ISO date. Only count visits before this timestamp."},
				"limit":            {Type: "integer", Default: 100, Description: "Maximum aggregated pairs per page (max 200)."},
				"offset":           {Type: "integer", Default: 0, Description: "Pagination offset."},
			}},
		},
		{
			Name:        "sra_build_evidence_pack",
			Description: "Assemble a question-scoped evidence pack for a subject: merges their messages, QZone contents and interaction events, deduplicates by event id, sorts newest-first, and truncates to a token budget. By default this only previews; set persist=true to save a row in evidence_packs for later audit.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"subject_qq":         {Type: "string", Description: "Target QQ number to build evidence around."},
				"question":           {Type: "string", Description: "The analysis question this pack is assembled for; stored in scope when persisted."},
				"token_budget":       {Type: "integer", Default: 30000, Description: "Approximate token cap for the returned event stream (max 200000)."},
				"redaction_policy":   {Type: "string", Enum: []string{"local-only", "full"}, Default: "local-only", Description: "local-only truncates every body snippet to 400 characters; full returns full bodies."},
				"include_media_meta": {Type: "boolean", Default: false, Description: "Attach sha256/mime/size summaries for media linked to included events (paths are never returned)."},
				"persist":            {Type: "boolean", Default: false, Description: "When true, save the assembled pack to evidence_packs and return its id."},
			}, Required: []string{"subject_qq"}},
		},
		{
			Name:        "sra_hypothesis_check",
			Description: "Heuristic supporting/contradicting evidence scan for a claim about a person. Distinguishes 本人自述 (self), 他人转述 (other), 行为 (behavior) and 已存推断 (inference) voices. Output is a hypothesis aid, never a fact determination: an AI should return to the raw evidence to verify.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"subject_qq":     {Type: "string", Description: "Target QQ number the claim is about."},
				"claim":          {Type: "string", Description: "Keyword of the claim to scan for (e.g. 学校名, 游戏名)."},
				"attribute_type": {Type: "string", Description: "Optional inference attribute scope (e.g. education, hobby) to also surface saved inferences."},
			}, Required: []string{"subject_qq", "claim"}},
		},
		{
			Name:        "sra_trace_claim",
			Description: "Trace a saved inference back to its supporting relation events and the original raw records. Each layer keeps its ids and timestamps; broken links are reported as missing_event_ids.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"inference_id": {Type: "string", Description: "UUID of the inference to trace."},
			}, Required: []string{"inference_id"}},
		},
		{
			Name:        "sra_operation_preview",
			Description: "Create a pending side-effect operation request. It does not execute anything; a human must confirm it in the operation center.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(false)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"account_id": {Type: "string", Description: "NapCat account UUID."},
				"endpoint":   {Type: "string", Description: "Side-effect NapCat endpoint path."},
				"parameters": {Type: "object", Description: "JSON object of query/body parameters."},
			}, Required: []string{"account_id", "endpoint"}},
		},
		{
			Name:        "sra_media_ocr_search",
			Description: "Search media assets, images, and chat screenshots by OCR-extracted text and semantic tags.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"query": {Type: "string", Description: "Keyword to search within image OCR text and tags."},
				"limit": {Type: "integer", Default: 30, Description: "Maximum images to return, up to 100."},
			}, Required: []string{"query"}},
		},
		{
			Name:        "sra_image_diffusion_graph",
			Description: "Trace the cross-group/person diffusion tree and propagation timeline of an image asset.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"asset_id": {Type: "string", Description: "Media asset UUID or SHA256."},
			}, Required: []string{"asset_id"}},
		},
		{
			Name:        "sra_dialogue_threads",
			Description: "Disentangle and retrieve coherent conversation threads and topics from interleaved group chat messages.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"conversation_id": {Type: "string", Description: "Conversation UUID."},
				"limit":           {Type: "integer", Default: 60, Description: "Maximum messages to analyze."},
			}, Required: []string{"conversation_id"}},
		},
		{
			Name:        "sra_thread_detail",
			Description: "Inspect a single dialogue thread's message chain, participants, and reply relationships.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"thread_id": {Type: "string", Description: "Dialogue thread UUID."},
			}, Required: []string{"thread_id"}},
		},
		{
			Name:        "sra_person_persona",
			Description: "Retrieve or generate a person's dynamic persona profile, linguistic fingerprint, and interest spectrum.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"qq":              {Type: "string", Description: "Target QQ number."},
				"person_id":       {Type: "string", Description: "Target person UUID (alternative to qq)."},
				"force_recompute": {Type: "boolean", Default: false, Description: "Whether to bypass cache and recompute persona fresh with LLM."},
			}},
		},
		{
			Name:        "sra_feed_dynamics",
			Description: "Analyze QZone feed sentiment flow, life milestones, and 3-Tier interaction hierarchy for a person.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"qq":              {Type: "string", Description: "Target QQ number."},
				"person_id":       {Type: "string", Description: "Target person UUID (alternative to qq)."},
				"force_recompute": {Type: "boolean", Default: false, Description: "Whether to bypass cache and recompute feed dynamics."},
			}},
		},
		{
			Name:        "sra_trigger_vision_analysis",
			Description: "Trigger AI vision analysis (OCR, scene parsing, social intent) on an image. Pass either a direct image URL (url) from a chat message, or a stored media asset UUID (media_asset_id). Use this whenever you receive a message containing '[附带图片URL:' or '[被引用消息内容:' with an image URL — extract the URL and pass it here. Results are persisted.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"url":            {Type: "string", Description: "Direct CDN URL of the image to analyze. Use this when you have a URL from a chat message or quoted message."},
				"media_asset_id": {Type: "string", Description: "Media asset UUID (alternative to url, for assets already stored in the database)."},
			}},
		},
		{
			Name:        "sra_trigger_dialogue_disentanglement",
			Description: "Trigger AI dialogue disentanglement on a group conversation, separating interleaved topics into structured threads with participant stances.",
			Annotations: &ToolAnnotations{ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false)},
			InputSchema: InputSchema{Type: "object", Properties: map[string]PropertyDef{
				"conversation_id": {Type: "string", Description: "Conversation UUID."},
				"group_id":        {Type: "string", Description: "Platform group ID / QQ group number (alternative to conversation_id)."},
			}},
		},
	}
}

// AllTools returns the full advertised tool catalog, decorated with titles,
// annotations, output schemas, and schema constraints.
func AllTools() []Tool {
	return decorateTools(BaseTools())
}

// writeToolAnnotations overrides the default read-only annotations for tools
// that have side effects or reach outside the local database.
var writeToolAnnotations = map[string]ToolAnnotations{
	"sra_collection_orchestrate": {ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"sra_operation_preview":      {ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"sra_propose_inference":      {ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"sra_napcat_read":            {ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
}

// Core analysis tools return JSON objects in structuredContent as well as a
// text representation for older MCP clients. The open object schema is
// intentional: each tool has a stable envelope but different evidence fields.
var coreOutputSchemas = map[string]any{
	"sra_search":              jsonObjectSchema(),
	"sra_person_dossier":      jsonObjectSchema(),
	"sra_ego_network":         jsonObjectSchema(),
	"sra_find_path":           jsonObjectSchema(),
	"sra_activity_timeline":   jsonObjectSchema(),
	"sra_interaction_stream":  jsonObjectSchema(),
	"sra_build_evidence_pack": jsonObjectSchema(),
	"sra_hypothesis_check":    jsonObjectSchema(),
	"sra_trace_claim":         jsonObjectSchema(),
	"sra_content_comments":    jsonObjectSchema(),
}

func jsonObjectSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}

// decorateTools applies titles, annotations, schema constraints and output
// schemas to the raw catalog. It is the single place presentation-level
// metadata lives, keeping the catalog itself read-only.
func decorateTools(tools []Tool) []Tool {
	qqProps := map[string]bool{"qq": true, "target_qq": true, "actor_qq": true, "source_qq": true}
	for i := range tools {
		t := &tools[i]

		if t.Title == "" {
			t.Title = deriveTitle(t.Name)
		}

		if t.InputSchema.AdditionalProperties == nil {
			t.InputSchema.AdditionalProperties = boolPtr(false)
		}

		if ann, ok := writeToolAnnotations[t.Name]; ok {
			t.Annotations = &ann
		} else if t.Annotations == nil {
			t.Annotations = &ToolAnnotations{
				ReadOnlyHint:    boolPtr(true),
				IdempotentHint:  boolPtr(true),
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			}
		} else {
			// Complete partial annotations so every tool advertises all four
			// hints consistently (some catalog entries predate the others).
			if t.Annotations.ReadOnlyHint == nil {
				t.Annotations.ReadOnlyHint = boolPtr(true)
			}
			if t.Annotations.DestructiveHint == nil {
				t.Annotations.DestructiveHint = boolPtr(false)
			}
			if t.Annotations.IdempotentHint == nil {
				t.Annotations.IdempotentHint = boolPtr(true)
			}
			if t.Annotations.OpenWorldHint == nil {
				t.Annotations.OpenWorldHint = boolPtr(false)
			}
		}

		for name, prop := range t.InputSchema.Properties {
			if qqProps[name] && prop.Type == "string" && prop.Pattern == "" {
				prop.Pattern = "^[1-9]\\d{4,11}$"
			}
			if (strings.HasSuffix(name, "limit") || strings.HasSuffix(name, "offset")) &&
				prop.Type == "integer" && prop.Minimum == nil {
				zero := float64(0)
				prop.Minimum = &zero
			}
			t.InputSchema.Properties[name] = prop
		}

		if schema, ok := coreOutputSchemas[t.Name]; ok && t.OutputSchema == nil {
			t.OutputSchema = schema
		}
	}
	return tools
}

// deriveTitle converts a snake_case tool name into a human-readable title:
// sra_build_evidence_pack -> "Build Evidence Pack".
func deriveTitle(name string) string {
	trimmed := strings.TrimPrefix(name, "sra_")
	parts := strings.Split(trimmed, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		switch p {
		case "sql":
			parts[i] = "SQL"
		case "qq":
			parts[i] = "QQ"
		case "ai":
			parts[i] = "AI"
		default:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// outObject and outProp build lightweight JSON Schema fragments for output
// declarations.
func outObject(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props}
}

func outProp(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}
