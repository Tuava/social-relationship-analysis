package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

func (h *HandlerRegistry) handleSearchPersons(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if input.Limit <= 0 || input.Limit > 100 {
		input.Limit = 20
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return errorResult("query cannot be empty"), nil
	}

	rows, err := h.DB.Query(ctx, `
		SELECT 
			p.id, 
			p.display_name, 
			COALESCE(pi.platform_user_id, ''),
			p.last_seen_at,
			(SELECT COUNT(*) FROM group_memberships gm WHERE gm.person_id = p.id) as group_count
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE pi.platform_user_id ILIKE $1 
		   OR p.display_name ILIKE $1
		ORDER BY 
			CASE WHEN pi.platform_user_id = $2 THEN 0 ELSE 1 END,
			p.last_seen_at DESC
		LIMIT $3`,
		"%"+query+"%", query, input.Limit,
	)
	if err != nil {
		return errorResult("database error: " + err.Error()), nil
	}
	defer rows.Close()

	type PersonSummary struct {
		ID          string    `json:"id"`
		QQ          string    `json:"qq,omitempty"`
		DisplayName string    `json:"display_name"`
		GroupCount  int       `json:"group_count"`
		LastSeenAt  time.Time `json:"last_seen_at"`
	}

	var results []PersonSummary
	for rows.Next() {
		var ps PersonSummary
		if err := rows.Scan(&ps.ID, &ps.DisplayName, &ps.QQ, &ps.LastSeenAt, &ps.GroupCount); err == nil {
			results = append(results, ps)
		}
	}

	return jsonResult(map[string]any{
		"total":   len(results),
		"results": results,
	})
}

func (h *HandlerRegistry) handleGetPersonProfile(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		QQ       string `json:"qq"`
		PersonID string `json:"person_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}

	var personID, displayName, qq string
	var firstSeenAt, lastSeenAt time.Time

	if input.QQ != "" {
		err := h.DB.QueryRow(ctx, `
			SELECT p.id, p.display_name, pi.platform_user_id, p.first_seen_at, p.last_seen_at
			FROM persons p
			JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE pi.platform_user_id = $1 LIMIT 1`, input.QQ).
			Scan(&personID, &displayName, &qq, &firstSeenAt, &lastSeenAt)
		if err != nil {
			return errorResult("person not found for QQ: " + input.QQ), nil
		}
	} else if input.PersonID != "" {
		err := h.DB.QueryRow(ctx, `
			SELECT p.id, p.display_name, COALESCE(pi.platform_user_id, ''), p.first_seen_at, p.last_seen_at
			FROM persons p
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE p.id = $1 LIMIT 1`, input.PersonID).
			Scan(&personID, &displayName, &qq, &firstSeenAt, &lastSeenAt)
		if err != nil {
			return errorResult("person not found for ID: " + input.PersonID), nil
		}
	} else {
		return errorResult("either qq or person_id must be provided"), nil
	}

	// 1. 获取空间快照 (Profile Observations)
	var rawSnapshot []byte
	var snapshotTime *time.Time
	_ = h.DB.QueryRow(ctx, `
		SELECT payload, observed_at FROM profile_observations 
		WHERE person_id = $1
		ORDER BY observed_at DESC LIMIT 1`, personID).Scan(&rawSnapshot, &snapshotTime)

	var snapshotData map[string]any
	if len(rawSnapshot) > 0 {
		_ = json.Unmarshal(rawSnapshot, &snapshotData)
	}

	// 2. 获取加入的群组
	groupRows, _ := h.DB.Query(ctx, `
		SELECT g.platform_group_id, g.name, COALESCE(gm.card, ''), gm.role, gm.valid_from
		FROM group_memberships gm
		JOIN "groups" g ON g.id = gm.group_id
		WHERE gm.person_id = $1
		ORDER BY gm.valid_from DESC LIMIT 50`, personID)

	type GroupMembership struct {
		GroupID   string     `json:"group_id"`
		Name      string     `json:"name"`
		Card      string     `json:"card,omitempty"`
		Role      string     `json:"role"`
		ValidFrom *time.Time `json:"valid_from,omitempty"`
	}
	var groups []GroupMembership
	if groupRows != nil {
		for groupRows.Next() {
			var gm GroupMembership
			if err := groupRows.Scan(&gm.GroupID, &gm.Name, &gm.Card, &gm.Role, &gm.ValidFrom); err == nil {
				groups = append(groups, gm)
			}
		}
		groupRows.Close()
	}

	// 3. 统计互动关系数据 (点赞、被点赞、评论等)
	var likesGiven, likesReceived, commentsGiven, postsCount int
	_ = h.DB.QueryRow(ctx, `
		SELECT 
			COUNT(CASE WHEN action_type = 'like' AND actor_person_id = $1 THEN 1 END),
			COUNT(CASE WHEN action_type = 'like' AND target_person_id = $1 THEN 1 END),
			COUNT(CASE WHEN action_type = 'comment' AND actor_person_id = $1 THEN 1 END)
		FROM relation_events 
		WHERE actor_person_id = $1 OR target_person_id = $1`, personID).Scan(&likesGiven, &likesReceived, &commentsGiven)

	_ = h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM contents WHERE author_id = $1`, personID).Scan(&postsCount)

	return jsonResult(map[string]any{
		"person_id":     personID,
		"qq":            qq,
		"display_name":  displayName,
		"first_seen_at": firstSeenAt,
		"last_seen_at":  lastSeenAt,
		"stats": map[string]int{
			"groups_count":   len(groups),
			"posts_count":    postsCount,
			"likes_given":    likesGiven,
			"likes_received": likesReceived,
			"comments_given": commentsGiven,
		},
		"groups":           groups,
		"profile_snapshot": snapshotData,
	})
}

func (h *HandlerRegistry) handleAnalyzeEgoNetwork(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TargetQQ string `json:"target_qq"`
		Depth    int    `json:"depth"`
		MaxNodes int    `json:"max_nodes"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if input.Depth <= 0 || input.Depth > 4 {
		input.Depth = 1
	}
	if input.MaxNodes <= 0 || input.MaxNodes > 500 {
		input.MaxNodes = 100
	}
	targetQQ := strings.TrimSpace(input.TargetQQ)
	if targetQQ == "" {
		return errorResult("target_qq is required"), nil
	}

	builder := analysis.EgoBuilder{DB: h.DB}
	graph, err := builder.BuildWithOptions(ctx, targetQQ, input.Depth, analysis.BuildOptions{
		MaxNodes: input.MaxNodes,
	})
	if err != nil {
		return errorResult("failed to build ego network: " + err.Error()), nil
	}

	type NodeSummary struct {
		Key      string `json:"key"`
		Type     string `json:"type"`
		Label    string `json:"label"`
		Metadata any    `json:"metadata,omitempty"`
	}
	type EdgeSummary struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		RelationType string `json:"relation_type"`
		Weight       int    `json:"weight"`
	}

	nodes := make([]NodeSummary, 0, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodes = append(nodes, NodeSummary{
			Key:      n.Key,
			Type:     n.Type,
			Label:    n.Label,
			Metadata: n.Metadata,
		})
	}

	edges := make([]EdgeSummary, 0, len(graph.Edges))
	for _, e := range graph.Edges {
		edges = append(edges, EdgeSummary{
			Source:       e.Source,
			Target:       e.Target,
			RelationType: e.RelationType,
			Weight:       e.Weight,
		})
	}

	return jsonResult(map[string]any{
		"target_qq":       targetQQ,
		"requested_depth": input.Depth,
		"total_nodes":     len(nodes),
		"total_edges":     len(edges),
		"nodes":           nodes,
		"edges":           edges,
	})
}

func (h *HandlerRegistry) handleQueryRelationship(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		SourceQQ string `json:"source_qq"`
		TargetQQ string `json:"target_qq"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	sourceQQ := strings.TrimSpace(input.SourceQQ)
	targetQQ := strings.TrimSpace(input.TargetQQ)
	if sourceQQ == "" || targetQQ == "" {
		return errorResult("both source_qq and target_qq are required"), nil
	}

	var p1, p2 string
	_ = h.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, sourceQQ).Scan(&p1)
	_ = h.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, targetQQ).Scan(&p2)

	if p1 == "" || p2 == "" {
		return errorResult("one or both QQ accounts not found in database"), nil
	}

	// 1. 查找共同群聊 (Common Groups)
	commonGroupRows, _ := h.DB.Query(ctx, `
		SELECT g.platform_group_id, g.name
		FROM group_memberships gm1
		JOIN group_memberships gm2 ON gm1.group_id = gm2.group_id
		JOIN "groups" g ON g.id = gm1.group_id
		WHERE gm1.person_id = $1 AND gm2.person_id = $2`, p1, p2)

	type GroupItem struct {
		GroupID string `json:"group_id"`
		Name    string `json:"name"`
	}
	var commonGroups []GroupItem
	if commonGroupRows != nil {
		for commonGroupRows.Next() {
			var g GroupItem
			if err := commonGroupRows.Scan(&g.GroupID, &g.Name); err == nil {
				commonGroups = append(commonGroups, g)
			}
		}
		commonGroupRows.Close()
	}

	// 2. 查找点赞与互动事件
	type InteractionSummary struct {
		ActionType string    `json:"action_type"`
		Direction  string    `json:"direction"`
		OccurredAt time.Time `json:"occurred_at"`
		ContentID  string    `json:"content_id,omitempty"`
	}
	var interactions []InteractionSummary
	eventRows, _ := h.DB.Query(ctx, `
		SELECT action_type, actor_person_id, occurred_at, COALESCE(target_object_id, '')
		FROM relation_events
		WHERE (actor_person_id = $1 AND target_person_id = $2)
		   OR (actor_person_id = $2 AND target_person_id = $1)
		ORDER BY occurred_at DESC LIMIT 50`, p1, p2)

	if eventRows != nil {
		for eventRows.Next() {
			var action, actor, targetObj string
			var t time.Time
			if err := eventRows.Scan(&action, &actor, &t, &targetObj); err == nil {
				dir := fmt.Sprintf("%s -> %s", sourceQQ, targetQQ)
				if actor == p2 {
					dir = fmt.Sprintf("%s -> %s", targetQQ, sourceQQ)
				}
				interactions = append(interactions, InteractionSummary{
					ActionType: action,
					Direction:  dir,
					OccurredAt: t,
					ContentID:  targetObj,
				})
			}
		}
		eventRows.Close()
	}

	return jsonResult(map[string]any{
		"source_qq":          sourceQQ,
		"target_qq":          targetQQ,
		"common_groups":      commonGroups,
		"total_interactions": len(interactions),
		"interactions":       interactions,
	})
}

func (h *HandlerRegistry) handleSearchGroups(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Query   string `json:"query"`
		GroupID string `json:"group_id"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if input.Limit <= 0 || input.Limit > 100 {
		input.Limit = 20
	}

	type GroupResult struct {
		GroupID     string `json:"group_id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
	}

	var results []GroupResult
	if input.GroupID != "" {
		var gr GroupResult
		err := h.DB.QueryRow(ctx, `
			SELECT g.platform_group_id, g.name, (SELECT COUNT(*) FROM group_memberships gm WHERE gm.group_id = g.id)
			FROM "groups" g WHERE g.platform_group_id = $1`, input.GroupID).
			Scan(&gr.GroupID, &gr.Name, &gr.MemberCount)
		if err == nil {
			results = append(results, gr)
		}
	} else {
		rows, err := h.DB.Query(ctx, `
			SELECT g.platform_group_id, g.name, (SELECT COUNT(*) FROM group_memberships gm WHERE gm.group_id = g.id)
			FROM "groups" g 
			WHERE g.name ILIKE $1 OR g.platform_group_id ILIKE $1
			ORDER BY g.updated_at DESC LIMIT $2`, "%"+input.Query+"%", input.Limit)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var gr GroupResult
				if err := rows.Scan(&gr.GroupID, &gr.Name, &gr.MemberCount); err == nil {
					results = append(results, gr)
				}
			}
		}
	}

	return jsonResult(map[string]any{
		"total":  len(results),
		"groups": results,
	})
}

func (h *HandlerRegistry) handleSearchContents(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		AuthorQQ string `json:"author_qq"`
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if input.Limit <= 0 || input.Limit > 100 {
		input.Limit = 20
	}

	type ContentItem struct {
		ID          string    `json:"id"`
		AuthorQQ    string    `json:"author_qq"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
	}

	var items []ContentItem
	var rows pgx.Rows
	var err error

	if input.AuthorQQ != "" {
		rows, err = h.DB.Query(ctx, `
			SELECT c.id, COALESCE(pi.platform_user_id, ''), c.body, COALESCE(c.published_at, c.updated_at)
			FROM contents c
			JOIN person_identifiers pi ON pi.person_id = c.author_id AND pi.platform = 'qq'
			WHERE pi.platform_user_id = $1 AND ($2 = '' OR c.body ILIKE $3)
			ORDER BY COALESCE(c.published_at, c.updated_at) DESC LIMIT $4`, input.AuthorQQ, input.Query, "%"+input.Query+"%", input.Limit)
	} else {
		rows, err = h.DB.Query(ctx, `
			SELECT c.id, COALESCE(pi.platform_user_id, ''), c.body, COALESCE(c.published_at, c.updated_at)
			FROM contents c
			LEFT JOIN person_identifiers pi ON pi.person_id = c.author_id AND pi.platform = 'qq'
			WHERE ($1 = '' OR c.body ILIKE $2)
			ORDER BY COALESCE(c.published_at, c.updated_at) DESC LIMIT $3`, input.Query, "%"+input.Query+"%", input.Limit)
	}

	if err != nil {
		return errorResult("database query failed: " + err.Error()), nil
	}
	defer rows.Close()

	for rows.Next() {
		var ci ContentItem
		if err := rows.Scan(&ci.ID, &ci.AuthorQQ, &ci.Body, &ci.PublishedAt); err == nil {
			items = append(items, ci)
		}
	}

	return jsonResult(map[string]any{
		"total":    len(items),
		"contents": items,
	})
}

func (h *HandlerRegistry) handleTriggerCollection(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TargetQQ string `json:"target_qq"`
		MaxDepth int    `json:"max_depth"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	targetQQ := strings.TrimSpace(input.TargetQQ)
	if targetQQ == "" {
		return errorResult("target_qq is required"), nil
	}
	if input.MaxDepth <= 0 || input.MaxDepth > 4 {
		input.MaxDepth = 2
	}
	if input.Mode == "" {
		input.Mode = "expand_people"
	}

	scope := domain.CollectionScope{
		MaxDepth: &input.MaxDepth,
		Entries: []domain.CollectionEntry{
			{
				Type:     "qq",
				ID:       targetQQ,
				Depth:    0,
				Mode:     input.Mode,
				Metadata: map[string]any{"source": "mcp"},
			},
		},
	}

	run, err := h.Repo.CreateCollectionRunWithScope(ctx, nil, "qzone", scope)
	if err != nil {
		return errorResult("failed to create collection run: " + err.Error()), nil
	}

	return jsonResult(map[string]any{
		"status":    "created",
		"run_id":    run.ID,
		"target_qq": targetQQ,
		"max_depth": input.MaxDepth,
		"mode":      input.Mode,
		"message":   "Collection run enqueued successfully. Monitor progress via sra_get_collection_status.",
	})
}

func (h *HandlerRegistry) handleGetCollectionStatus(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	_ = json.Unmarshal(args, &input)

	var runID, collectorType, status string
	var progress int
	var startedAt, endedAt *time.Time
	var rawConfig []byte

	var err error
	if input.RunID != "" {
		err = h.DB.QueryRow(ctx, `
			SELECT id, type, status, progress, started_at, ended_at, config
			FROM collection_runs WHERE id = $1 LIMIT 1`, input.RunID).
			Scan(&runID, &collectorType, &status, &progress, &startedAt, &endedAt, &rawConfig)
	} else {
		err = h.DB.QueryRow(ctx, `
			SELECT id, type, status, progress, started_at, ended_at, config
			FROM collection_runs ORDER BY created_at DESC LIMIT 1`).
			Scan(&runID, &collectorType, &status, &progress, &startedAt, &endedAt, &rawConfig)
	}

	if err != nil {
		return errorResult("no collection run found"), nil
	}

	var candidateCount, completedCandidateCount int
	_ = h.DB.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			COUNT(CASE WHEN state = 'expanded' THEN 1 END)
		FROM collection_candidates WHERE run_id = $1`, runID).Scan(&candidateCount, &completedCandidateCount)

	return jsonResult(map[string]any{
		"run_id":                      runID,
		"collector_type":              collectorType,
		"status":                      status,
		"progress":                    progress,
		"started_at":                  startedAt,
		"ended_at":                    endedAt,
		"total_candidates_discovered": candidateCount,
		"candidates_processed":        completedCandidateCount,
	})
}

func (h *HandlerRegistry) handleGetRawEvidence(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		EvidenceID string `json:"evidence_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if input.EvidenceID == "" {
		return errorResult("evidence_id is required"), nil
	}

	var source, eventType, hash string
	var rawPayload []byte
	var collectedAt time.Time

	err := h.DB.QueryRow(ctx, `
		SELECT source, endpoint_or_event_type, payload, payload_hash, collected_at
		FROM raw_records WHERE id = $1 LIMIT 1`, input.EvidenceID).
		Scan(&source, &eventType, &rawPayload, &hash, &collectedAt)
	if err != nil {
		return errorResult("raw record not found for id: " + input.EvidenceID), nil
	}

	var payload any
	if len(rawPayload) > 0 {
		_ = json.Unmarshal(rawPayload, &payload)
	}

	return jsonResult(map[string]any{
		"evidence_id":  input.EvidenceID,
		"source":       source,
		"event_type":   eventType,
		"payload_hash": hash,
		"collected_at": collectedAt,
		"raw_payload":  payload,
	})
}
