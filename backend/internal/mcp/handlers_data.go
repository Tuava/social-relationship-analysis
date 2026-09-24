package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type resolveArgs struct {
	Keyword    string `json:"keyword"`
	EntityType string `json:"entity_type"`
	Limit      int    `json:"limit"`
}

func (h *HandlerRegistry) handleResolve(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input resolveArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.Keyword = strings.TrimSpace(input.Keyword)
	if input.Keyword == "" {
		return errorResult("keyword is required"), nil
	}
	entityType := input.EntityType
	if entityType == "" {
		entityType = "auto"
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	matches := make([]map[string]any, 0, limit)
	matchKind := ""
	if entityType == "person" || entityType == "auto" {
		rows, err := h.DB.Query(ctx, `
			SELECT p.id::text, COALESCE(pi.platform_user_id,''), COALESCE(p.display_name,''),
			       p.first_seen_at, p.last_seen_at,
			       (SELECT COUNT(*) FROM relation_events re WHERE re.actor_person_id=p.id OR re.target_person_id=p.id)
			FROM persons p
			LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
			WHERE pi.platform_user_id=$1 OR p.display_name ILIKE '%'||$1||'%'
			ORDER BY CASE WHEN pi.platform_user_id=$1 THEN 0 ELSE 1 END, p.last_seen_at DESC NULLS LAST
			LIMIT $2`, input.Keyword, limit)
		if err != nil {
			return nil, fmt.Errorf("resolve persons: %w", err)
		}
		for rows.Next() {
			var id, qq, name string
			var firstSeen, lastSeen *time.Time
			var events int
			if err := rows.Scan(&id, &qq, &name, &firstSeen, &lastSeen, &events); err != nil {
				rows.Close()
				return nil, err
			}
			matches = append(matches, map[string]any{
				"kind": "person", "id": id, "qq": qq, "name": name,
				"first_seen_at": firstSeen, "last_seen_at": lastSeen, "relation_events": events,
			})
		}
		rows.Close()
		if len(matches) > 0 {
			matchKind = "person"
		}
	}

	if entityType == "group" || (entityType == "auto" && len(matches) == 0) {
		rows, err := h.DB.Query(ctx, `
			SELECT g.id::text, g.platform_group_id, COALESCE(g.group_name,''), g.first_seen_at,
			       (SELECT COUNT(*) FROM group_memberships gm WHERE gm.group_id=g.id)
			FROM "groups" g
			WHERE g.platform_group_id=$1 OR g.group_name ILIKE '%'||$1||'%'
			ORDER BY CASE WHEN g.platform_group_id=$1 THEN 0 ELSE 1 END, g.first_seen_at DESC
			LIMIT $2`, input.Keyword, limit)
		if err != nil {
			return nil, fmt.Errorf("resolve groups: %w", err)
		}
		for rows.Next() {
			var id, groupID, name string
			var firstSeen *time.Time
			var members int
			if err := rows.Scan(&id, &groupID, &name, &firstSeen, &members); err != nil {
				rows.Close()
				return nil, err
			}
			matches = append(matches, map[string]any{
				"kind": "group", "id": id, "group_id": groupID, "name": name,
				"first_seen_at": firstSeen, "member_count": members,
			})
		}
		rows.Close()
		if matchKind == "" && len(matches) > 0 {
			matchKind = "group"
		}
	}

	return jsonResult(map[string]any{
		"keyword":     input.Keyword,
		"match_kind":  matchKind,
		"match_count": len(matches),
		"matches":     matches,
	})
}

func (h *HandlerRegistry) handleSchema(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT table_name, column_name || ':' || data_type
		FROM information_schema.columns
		WHERE table_schema='public'
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	defer rows.Close()

	tables := make(map[string][]string)
	order := make([]string, 0)
	index := make(map[string]int)
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		if _, ok := index[table]; !ok {
			index[table] = len(order)
			order = append(order, table)
		}
		tables[table] = append(tables[table], column)
	}
	return jsonResult(map[string]any{
		"table_count": len(order),
		"tables":      tables,
	})
}

type messageHistoryArgs struct {
	QQ               string `json:"qq"`
	GroupID          string `json:"group_id"`
	ConversationID   string `json:"conversation_id"`
	ConversationType string `json:"conversation_type"`
	Query            string `json:"query"`
	TimeStart        string `json:"time_start"`
	TimeEnd          string `json:"time_end"`
	Limit            int    `json:"limit"`
	Offset           int    `json:"offset"`
}

func (h *HandlerRegistry) handleMessageHistory(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input messageHistoryArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	var where []string
	var params []any
	idx := 1
	add := func(clause string, value any) {
		where = append(where, fmt.Sprintf("%s $%d", clause, idx))
		params = append(params, value)
		idx++
	}

	if input.QQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, input.QQ)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		add("m.sender_id =", personID)
	}
	if input.GroupID != "" {
		add("c.conversation_type = 'group' AND c.platform_conversation_id =", input.GroupID)
	}
	if input.ConversationID != "" {
		add("c.id::text =", input.ConversationID)
	}
	if input.ConversationType != "" {
		add("c.conversation_type =", input.ConversationType)
	}
	if input.Query != "" {
		where = append(where, fmt.Sprintf("(m.raw_text ILIKE '%%' || $%d || '%%' OR p.display_name ILIKE '%%' || $%d || '%%' OR pi.platform_user_id = $%d)", idx, idx, idx))
		params = append(params, input.Query)
		idx++
	}
	if input.TimeStart != "" {
		add("m.sent_at >=", input.TimeStart)
	}
	if input.TimeEnd != "" {
		add("m.sent_at <=", input.TimeEnd)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	countSQL := `SELECT COUNT(*) FROM messages m
		LEFT JOIN persons p ON p.id=m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		LEFT JOIN conversations c ON c.id=m.conversation_id` + whereSQL
	var total int
	if err := h.DB.QueryRow(ctx, countSQL, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count messages: %w", err)
	}

	query := `SELECT m.id::text, COALESCE(m.source_message_id,''),
		COALESCE(pi.platform_user_id,''), COALESCE(p.display_name,''),
		COALESCE(m.raw_text,''), m.sent_at,
		COALESCE(c.conversation_type,''), COALESCE(c.platform_conversation_id,''),
		COALESCE(c.id::text,''), COALESCE(m.raw_record_id::text,''),
		(SELECT COUNT(*) FROM message_media mm WHERE mm.message_id=m.id)
		FROM messages m
		LEFT JOIN persons p ON p.id=m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		LEFT JOIN conversations c ON c.id=m.conversation_id` + whereSQL +
		` ORDER BY m.sent_at DESC NULLS LAST, m.id DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	type messageItem struct {
		ID               string     `json:"id"`
		SourceMessageID  string     `json:"source_message_id"`
		SenderQQ         string     `json:"sender_qq"`
		SenderName       string     `json:"sender_name"`
		Text             string     `json:"text"`
		SentAt           *time.Time `json:"sent_at"`
		ConversationType string     `json:"conversation_type"`
		ConversationID   string     `json:"conversation_id"`
		ConversationUUID string     `json:"conversation_uuid"`
		RawRecordID      string     `json:"raw_record_id"`
		MediaCount       int        `json:"media_count"`
	}
	items := make([]messageItem, 0, limit)
	for rows.Next() {
		var item messageItem
		if err := rows.Scan(&item.ID, &item.SourceMessageID, &item.SenderQQ, &item.SenderName,
			&item.Text, &item.SentAt, &item.ConversationType, &item.ConversationID,
			&item.ConversationUUID, &item.RawRecordID, &item.MediaCount); err != nil {
			return nil, err
		}
		if runes := []rune(item.Text); len(runes) > 500 {
			item.Text = string(runes[:500]) + "..."
		}
		items = append(items, item)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

type relationEventsArgs struct {
	ActorQQ         string   `json:"actor_qq"`
	TargetQQ        string   `json:"target_qq"`
	ActionTypes     []string `json:"action_types"`
	ContextType     string   `json:"context_type"`
	TimeStart       string   `json:"time_start"`
	TimeEnd         string   `json:"time_end"`
	IncludeEvidence bool     `json:"include_evidence"`
	Limit           int      `json:"limit"`
	Offset          int      `json:"offset"`
}

func (h *HandlerRegistry) handleRelationEvents(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input relationEventsArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	var where []string
	var params []any
	idx := 1
	add := func(clause string, value any) {
		where = append(where, fmt.Sprintf("%s $%d", clause, idx))
		params = append(params, value)
		idx++
	}
	if input.ActorQQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, input.ActorQQ)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		add("re.actor_person_id =", personID)
	}
	if input.TargetQQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, input.TargetQQ)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		add("re.target_person_id =", personID)
	}
	if len(input.ActionTypes) > 0 {
		where = append(where, fmt.Sprintf("re.action_type = ANY($%d::text[])", idx))
		params = append(params, input.ActionTypes)
		idx++
	}
	if input.ContextType != "" {
		add("re.context_type =", input.ContextType)
	}
	if input.TimeStart != "" {
		add("re.occurred_at >=", input.TimeStart)
	}
	if input.TimeEnd != "" {
		add("re.occurred_at <=", input.TimeEnd)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	countSQL := `SELECT COUNT(*) FROM relation_events re` + whereSQL
	var total int
	if err := h.DB.QueryRow(ctx, countSQL, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count relation events: %w", err)
	}

	evidenceSelect := "0"
	if input.IncludeEvidence {
		evidenceSelect = "COALESCE(array_length(re.evidence_ids,1),0)"
	}
	query := `SELECT re.id::text, re.action_type, re.context_type, re.occurred_at,
		COALESCE(ap.platform_user_id,''), COALESCE(aperson.display_name,''),
		COALESCE(tp.platform_user_id,''), COALESCE(tperson.display_name,''),
		COALESCE(re.target_object_id::text,''),
		CASE WHEN c.id IS NOT NULL THEN 'content' WHEN m.id IS NOT NULL THEN 'message' ELSE '' END,
		COALESCE(re.raw_record_id::text,''), ` + evidenceSelect + `
		FROM relation_events re
		LEFT JOIN persons aperson ON aperson.id=re.actor_person_id
		LEFT JOIN person_identifiers ap ON ap.person_id=re.actor_person_id AND ap.platform='qq'
		LEFT JOIN persons tperson ON tperson.id=re.target_person_id
		LEFT JOIN person_identifiers tp ON tp.person_id=re.target_person_id AND tp.platform='qq'
		LEFT JOIN contents c ON c.id=re.target_object_id
		LEFT JOIN messages m ON m.id=re.target_object_id` + whereSQL +
		` ORDER BY re.occurred_at DESC, re.id DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query relation events: %w", err)
	}
	defer rows.Close()

	type relationItem struct {
		ID            string     `json:"id"`
		ActionType    string     `json:"action_type"`
		ContextType   string     `json:"context_type"`
		OccurredAt    *time.Time `json:"occurred_at"`
		ActorQQ       string     `json:"actor_qq"`
		ActorName     string     `json:"actor_name"`
		TargetQQ      string     `json:"target_qq"`
		TargetName    string     `json:"target_name"`
		ObjectID      string     `json:"object_id"`
		ObjectKind    string     `json:"object_kind"`
		RawRecordID   string     `json:"raw_record_id"`
		EvidenceCount int        `json:"evidence_count"`
	}
	items := make([]relationItem, 0, limit)
	for rows.Next() {
		var item relationItem
		if err := rows.Scan(&item.ID, &item.ActionType, &item.ContextType, &item.OccurredAt,
			&item.ActorQQ, &item.ActorName, &item.TargetQQ, &item.TargetName,
			&item.ObjectID, &item.ObjectKind, &item.RawRecordID, &item.EvidenceCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}
