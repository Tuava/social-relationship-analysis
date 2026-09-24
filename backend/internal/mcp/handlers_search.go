package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SearchArgs struct {
	EntityType      string `json:"entity_type"`
	Keyword         string `json:"keyword"`
	InGroup         string `json:"in_group"`
	ActiveSince     string `json:"active_since"`
	ActiveBefore    string `json:"active_before"`
	MinInteractions int    `json:"min_interactions"`
	SortBy          string `json:"sort_by"`
	Limit           int    `json:"limit"`
	Offset          int    `json:"offset"`
}

type PersonSearchResult struct {
	QQ               string     `json:"qq"`
	DisplayName      string     `json:"display_name"`
	GroupCount       int        `json:"group_count"`
	InteractionCount int        `json:"interaction_count"`
	LastSeenAt       *time.Time `json:"last_seen_at"`
}

type GroupSearchResult struct {
	GroupID     string `json:"group_id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

type ContentSearchResult struct {
	ID            string     `json:"id"`
	AuthorQQ      string     `json:"author_qq"`
	Body          string     `json:"body"`
	PublishedAt   *time.Time `json:"published_at"`
	LikesCount    int        `json:"likes_count"`
	CommentsCount int        `json:"comments_count"`
}

func (h *HandlerRegistry) handleSearch(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input SearchArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 30
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	switch input.EntityType {
	case "person":
		return h.searchPersons(ctx, input, limit, offset)
	case "group":
		return h.searchGroups(ctx, input, limit, offset)
	case "content":
		return h.searchContents(ctx, input, limit, offset)
	default:
		return errorResult("invalid entity_type: must be person, group, or content"), nil
	}
}

func (h *HandlerRegistry) searchPersons(ctx context.Context, input SearchArgs, limit, offset int) (*ToolCallResult, error) {
	var query strings.Builder
	var params []interface{}
	paramIdx := 1

	query.WriteString(`
		SELECT COALESCE(pi.platform_user_id, '') as qq, 
		       COALESCE(p.display_name, '') as display_name,
		       (SELECT COUNT(*) FROM group_memberships gm2 WHERE gm2.person_id = p.id) as group_count,
		       (SELECT COUNT(*) FROM relation_events re WHERE re.actor_person_id = p.id OR re.target_person_id = p.id) as interaction_count,
		       p.last_seen_at
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
	`)

	if input.InGroup != "" {
		query.WriteString(fmt.Sprintf(` JOIN group_memberships gm ON gm.person_id = p.id AND gm.group_id = (SELECT id FROM "groups" WHERE platform_group_id = $%d LIMIT 1) `, paramIdx))
		params = append(params, input.InGroup)
		paramIdx++
	}

	query.WriteString(" WHERE 1=1 ")

	if input.Keyword != "" {
		keyword := "%" + input.Keyword + "%"
		query.WriteString(fmt.Sprintf(` AND (pi.platform_user_id ILIKE $%d OR p.display_name ILIKE $%d) `, paramIdx, paramIdx))
		params = append(params, keyword)
		paramIdx++
	}

	if input.ActiveSince != "" {
		t, err := time.Parse(time.RFC3339, input.ActiveSince)
		if err == nil {
			query.WriteString(fmt.Sprintf(` AND p.last_seen_at >= $%d `, paramIdx))
			params = append(params, t)
			paramIdx++
		}
	}

	if input.MinInteractions > 0 {
		query.WriteString(fmt.Sprintf(` AND (SELECT COUNT(*) FROM relation_events re WHERE re.actor_person_id = p.id OR re.target_person_id = p.id) >= $%d `, paramIdx))
		params = append(params, input.MinInteractions)
		paramIdx++
	}

	switch input.SortBy {
	case "recent_activity":
		query.WriteString(" ORDER BY p.last_seen_at DESC NULLS LAST ")
	case "interaction_count":
		query.WriteString(" ORDER BY interaction_count DESC ")
	case "group_count":
		query.WriteString(" ORDER BY group_count DESC ")
	default:
		query.WriteString(" ORDER BY p.last_seen_at DESC NULLS LAST ")
	}

	query.WriteString(fmt.Sprintf(` LIMIT $%d OFFSET $%d`, paramIdx, paramIdx+1))
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query.String(), params...)
	if err != nil {
		return errorResult(fmt.Sprintf("db query error: %v", err)), nil
	}
	defer rows.Close()

	var results []PersonSearchResult
	for rows.Next() {
		var r PersonSearchResult
		if err := rows.Scan(&r.QQ, &r.DisplayName, &r.GroupCount, &r.InteractionCount, &r.LastSeenAt); err != nil {
			return errorResult(fmt.Sprintf("db scan error: %v", err)), nil
		}
		results = append(results, r)
	}

	return jsonResult(results)
}

func (h *HandlerRegistry) searchGroups(ctx context.Context, input SearchArgs, limit, offset int) (*ToolCallResult, error) {
	var query strings.Builder
	var params []interface{}
	paramIdx := 1

	query.WriteString(`
		SELECT platform_group_id, COALESCE(group_name, ''),
		       (SELECT COUNT(*) FROM group_memberships gm WHERE gm.group_id = g.id) as member_count
		FROM "groups" g
		WHERE 1=1
	`)

	if input.Keyword != "" {
		keyword := "%" + input.Keyword + "%"
		query.WriteString(fmt.Sprintf(` AND (g.group_name ILIKE $%d OR g.platform_group_id ILIKE $%d) `, paramIdx, paramIdx))
		params = append(params, keyword)
		paramIdx++
	}

	query.WriteString(" ORDER BY member_count DESC ")

	query.WriteString(fmt.Sprintf(` LIMIT $%d OFFSET $%d`, paramIdx, paramIdx+1))
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query.String(), params...)
	if err != nil {
		return errorResult(fmt.Sprintf("db query error: %v", err)), nil
	}
	defer rows.Close()

	var results []GroupSearchResult
	for rows.Next() {
		var r GroupSearchResult
		if err := rows.Scan(&r.GroupID, &r.Name, &r.MemberCount); err != nil {
			return errorResult(fmt.Sprintf("db scan error: %v", err)), nil
		}
		results = append(results, r)
	}

	return jsonResult(results)
}

func (h *HandlerRegistry) searchContents(ctx context.Context, input SearchArgs, limit, offset int) (*ToolCallResult, error) {
	var query strings.Builder
	var params []interface{}
	paramIdx := 1

	query.WriteString(`
		SELECT c.id, COALESCE(pi.platform_user_id, ''), c.body, c.published_at,
		       (SELECT COUNT(*) FROM relation_events re WHERE re.target_object_id = c.id AND re.action_type = 'liked') as likes_count,
		       (SELECT COUNT(*) FROM contents child WHERE child.parent_content_id = c.id) as comments_count
		FROM contents c
		LEFT JOIN person_identifiers pi ON pi.person_id = c.author_id AND pi.platform = 'qq'
		WHERE 1=1
	`)

	if input.Keyword != "" {
		keyword := "%" + input.Keyword + "%"
		query.WriteString(fmt.Sprintf(` AND c.body ILIKE $%d `, paramIdx))
		params = append(params, keyword)
		paramIdx++
	}

	if input.ActiveSince != "" {
		t, err := time.Parse(time.RFC3339, input.ActiveSince)
		if err == nil {
			query.WriteString(fmt.Sprintf(` AND c.published_at >= $%d `, paramIdx))
			params = append(params, t)
			paramIdx++
		}
	}
	if input.ActiveBefore != "" {
		t, err := time.Parse(time.RFC3339, input.ActiveBefore)
		if err == nil {
			query.WriteString(fmt.Sprintf(` AND c.published_at <= $%d `, paramIdx))
			params = append(params, t)
			paramIdx++
		}
	}

	query.WriteString(" ORDER BY c.published_at DESC NULLS LAST ")

	query.WriteString(fmt.Sprintf(` LIMIT $%d OFFSET $%d`, paramIdx, paramIdx+1))
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query.String(), params...)
	if err != nil {
		return errorResult(fmt.Sprintf("db query error: %v", err)), nil
	}
	defer rows.Close()

	var results []ContentSearchResult
	for rows.Next() {
		var r ContentSearchResult
		var body *string
		if err := rows.Scan(&r.ID, &r.AuthorQQ, &body, &r.PublishedAt, &r.LikesCount, &r.CommentsCount); err != nil {
			return errorResult(fmt.Sprintf("db scan error: %v", err)), nil
		}
		if body != nil {
			s := *body
			runes := []rune(s)
			if len(runes) > 200 {
				r.Body = string(runes[:200]) + "..."
			} else {
				r.Body = s
			}
		}
		results = append(results, r)
	}

	return jsonResult(results)
}

type DiscoverConnectionsArgs struct {
	TargetQQ         string   `json:"target_qq"`
	GroupID          string   `json:"group_id"`
	InteractionTypes []string `json:"interaction_types"`
	Direction        string   `json:"direction"`
	MinWeight        int      `json:"min_weight"`
	TimeRangeStart   string   `json:"time_range_start"`
	TimeRangeEnd     string   `json:"time_range_end"`
	Limit            int      `json:"limit"`
}

type ConnectionItem struct {
	QQ                string     `json:"qq"`
	DisplayName       string     `json:"display_name"`
	TotalInteractions int        `json:"total_interactions"`
	InteractionTypes  []string   `json:"interaction_types"`
	FirstInteraction  *time.Time `json:"first_interaction"`
	LastInteraction   *time.Time `json:"last_interaction"`
}

type DiscoverConnectionsResult struct {
	TargetQQ         string           `json:"target_qq"`
	TotalConnections int              `json:"total_connections"`
	Connections      []ConnectionItem `json:"connections"`
}

func (h *HandlerRegistry) handleDiscoverConnections(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input DiscoverConnectionsArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	personID, err := resolvePersonID(ctx, h.DB, input.TargetQQ)
	if err != nil {
		return errorResult(fmt.Sprintf("could not resolve target QQ: %v", err)), nil
	}

	minWeight := input.MinWeight
	if minWeight <= 0 {
		minWeight = 1
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	var subquery strings.Builder
	var params []interface{}
	paramIdx := 1

	params = append(params, personID)
	paramIdx++
	var groupUUID string
	if input.GroupID != "" {
		if err := h.DB.QueryRow(ctx, `SELECT id::text FROM "groups" WHERE platform='qq' AND platform_group_id=$1`, input.GroupID).Scan(&groupUUID); err != nil {
			return errorResult(fmt.Sprintf("group not found: %s", input.GroupID)), nil
		}
		params = append(params, groupUUID)
		paramIdx++
	}

	subquery.WriteString(`
		SELECT CASE 
			WHEN re.actor_person_id = $1 THEN re.target_person_id
			ELSE re.actor_person_id 
		END as counterparty_id,
		re.action_type, re.occurred_at
		FROM relation_events re
		WHERE re.actor_person_id IS NOT NULL AND re.target_person_id IS NOT NULL
	`)
	if groupUUID != "" {
		subquery.WriteString(` AND re.context_type='group' AND (re.target_object_id=$2::uuid OR re.target_object_id IN (SELECT c.id FROM conversations c WHERE c.conversation_type='group' AND c.platform_conversation_id=(SELECT platform_group_id FROM "groups" WHERE id=$2::uuid))) `)
	}

	if input.Direction == "incoming" {
		subquery.WriteString(` AND re.target_person_id = $1 `)
	} else if input.Direction == "outgoing" {
		subquery.WriteString(` AND re.actor_person_id = $1 `)
	} else {
		subquery.WriteString(` AND (re.actor_person_id = $1 OR re.target_person_id = $1) `)
	}

	if len(input.InteractionTypes) > 0 {
		subquery.WriteString(fmt.Sprintf(` AND re.action_type = ANY($%d) `, paramIdx))
		params = append(params, input.InteractionTypes)
		paramIdx++
	}

	if input.TimeRangeStart != "" {
		t, err := time.Parse(time.RFC3339, input.TimeRangeStart)
		if err == nil {
			subquery.WriteString(fmt.Sprintf(` AND re.occurred_at >= $%d `, paramIdx))
			params = append(params, t)
			paramIdx++
		}
	}

	if input.TimeRangeEnd != "" {
		t, err := time.Parse(time.RFC3339, input.TimeRangeEnd)
		if err == nil {
			subquery.WriteString(fmt.Sprintf(` AND re.occurred_at <= $%d `, paramIdx))
			params = append(params, t)
			paramIdx++
		}
	}

	query := fmt.Sprintf(`
		SELECT 
		  sub.counterparty_id,
		  COALESCE(pi.platform_user_id, '') as qq,
		  COALESCE(p.display_name, '') as name,
		  COUNT(*) as total_weight,
		  array_agg(DISTINCT sub.action_type) as interaction_types,
		  MIN(sub.occurred_at) as first_interaction,
		  MAX(sub.occurred_at) as last_interaction
		FROM ( %s ) sub
		JOIN persons p ON p.id = sub.counterparty_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE sub.counterparty_id <> $1
		GROUP BY sub.counterparty_id, pi.platform_user_id, p.display_name
		HAVING COUNT(*) >= $%d
		ORDER BY total_weight DESC
		LIMIT $%d
	`, subquery.String(), paramIdx, paramIdx+1)

	params = append(params, minWeight, limit)

	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return errorResult(fmt.Sprintf("db query error: %v", err)), nil
	}
	defer rows.Close()

	var connections []ConnectionItem
	for rows.Next() {
		var c ConnectionItem
		var counterpartyID string
		if err := rows.Scan(&counterpartyID, &c.QQ, &c.DisplayName, &c.TotalInteractions, &c.InteractionTypes, &c.FirstInteraction, &c.LastInteraction); err != nil {
			return errorResult(fmt.Sprintf("db scan error: %v", err)), nil
		}
		connections = append(connections, c)
	}

	res := DiscoverConnectionsResult{
		TargetQQ:         input.TargetQQ,
		TotalConnections: len(connections),
		Connections:      connections,
	}

	return jsonResult(res)
}
