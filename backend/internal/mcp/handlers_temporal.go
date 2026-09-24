package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ActivityTimelineArgs struct {
	QQ             string   `json:"qq"`
	GroupID        string   `json:"group_id"`
	Granularity    string   `json:"granularity"`
	TimeRangeStart string   `json:"time_range_start"`
	TimeRangeEnd   string   `json:"time_range_end"`
	ActionTypes    []string `json:"action_types"`
}

type InteractionStreamArgs struct {
	ActorQQ               string   `json:"actor_qq"`
	TargetQQ              string   `json:"target_qq"`
	ActionTypes           []string `json:"action_types"`
	TimeRangeStart        string   `json:"time_range_start"`
	TimeRangeEnd          string   `json:"time_range_end"`
	IncludeContentPreview *bool    `json:"include_content_preview"`
	Limit                 int      `json:"limit"`
	Offset                int      `json:"offset"`
}

type ContentFeedArgs struct {
	AuthorQQ       string `json:"author_qq"`
	Keyword        string `json:"keyword"`
	TimeRangeStart string `json:"time_range_start"`
	TimeRangeEnd   string `json:"time_range_end"`
	MinEngagement  int    `json:"min_engagement"`
	SortBy         string `json:"sort_by"`
	IncludeReplies bool   `json:"include_replies"`
	Limit          int    `json:"limit"`
	Offset         int    `json:"offset"`
}

func (h *HandlerRegistry) handleActivityTimeline(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var a ActivityTimelineArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	granularity := a.Granularity
	if granularity == "" {
		granularity = "hourly"
	}

	var target string
	var query string
	params := []interface{}{}
	whereClauses := []string{}
	paramIdx := 1

	if a.QQ != "" {
		target = "qq:" + a.QQ
		personID, err := resolvePersonID(ctx, h.DB, a.QQ)
		if err != nil {
			return errorResult("person not found for qq: " + a.QQ), nil
		}
		whereClauses = append(whereClauses, fmt.Sprintf("actor_person_id = $%d", paramIdx))
		params = append(params, personID)
		paramIdx++

		timeField := "occurred_at"
		if a.TimeRangeStart != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("%s >= $%d", timeField, paramIdx))
			params = append(params, a.TimeRangeStart)
			paramIdx++
		}
		if a.TimeRangeEnd != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("%s <= $%d", timeField, paramIdx))
			params = append(params, a.TimeRangeEnd)
			paramIdx++
		}
		if len(a.ActionTypes) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("action_type = ANY($%d)", paramIdx))
			params = append(params, a.ActionTypes)
			paramIdx++
		}

		where := strings.Join(whereClauses, " AND ")
		var selectExpr string
		switch granularity {
		case "daily":
			selectExpr = fmt.Sprintf("%s::date::text", timeField)
		case "weekly":
			selectExpr = fmt.Sprintf("EXTRACT(DOW FROM %s)::int::text", timeField)
		case "monthly":
			selectExpr = fmt.Sprintf("to_char(%s, 'YYYY-MM')", timeField)
		case "hourly":
			fallthrough
		default:
			selectExpr = fmt.Sprintf("EXTRACT(HOUR FROM %s)::int::text", timeField)
		}

		query = fmt.Sprintf("SELECT %s as bucket, COUNT(*) as count FROM relation_events WHERE %s GROUP BY bucket ORDER BY bucket", selectExpr, where)

	} else if a.GroupID != "" {
		target = "group:" + a.GroupID
		whereClauses = append(whereClauses, fmt.Sprintf("cv.platform_conversation_id = $%d", paramIdx))
		whereClauses = append(whereClauses, "cv.conversation_type = 'group'")
		params = append(params, a.GroupID)
		paramIdx++

		timeField := "m.sent_at"
		if a.TimeRangeStart != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("%s >= $%d", timeField, paramIdx))
			params = append(params, a.TimeRangeStart)
			paramIdx++
		}
		if a.TimeRangeEnd != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("%s <= $%d", timeField, paramIdx))
			params = append(params, a.TimeRangeEnd)
			paramIdx++
		}

		where := strings.Join(whereClauses, " AND ")
		var selectExpr string
		switch granularity {
		case "daily":
			selectExpr = fmt.Sprintf("%s::date::text", timeField)
		case "weekly":
			selectExpr = fmt.Sprintf("EXTRACT(DOW FROM %s)::int::text", timeField)
		case "monthly":
			selectExpr = fmt.Sprintf("to_char(%s, 'YYYY-MM')", timeField)
		case "hourly":
			fallthrough
		default:
			selectExpr = fmt.Sprintf("EXTRACT(HOUR FROM %s)::int::text", timeField)
		}

		query = fmt.Sprintf("SELECT %s as bucket, COUNT(*) as count FROM messages m JOIN conversations cv ON cv.id = m.conversation_id WHERE %s GROUP BY bucket ORDER BY bucket", selectExpr, where)
	} else {
		return errorResult("must provide qq or group_id"), nil
	}

	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	type BucketResult struct {
		Bucket string `json:"bucket"`
		Count  int    `json:"count"`
	}

	var results []BucketResult
	var totalEvents int
	var peak BucketResult

	for rows.Next() {
		var b string
		var c int
		if err := rows.Scan(&b, &c); err != nil {
			return nil, err
		}

		if granularity == "weekly" {
			switch b {
			case "0":
				b = "Sunday"
			case "1":
				b = "Monday"
			case "2":
				b = "Tuesday"
			case "3":
				b = "Wednesday"
			case "4":
				b = "Thursday"
			case "5":
				b = "Friday"
			case "6":
				b = "Saturday"
			}
		}

		results = append(results, BucketResult{Bucket: b, Count: c})
		totalEvents += c
		if c > peak.Count {
			peak = BucketResult{Bucket: b, Count: c}
		}
	}

	insight := "Not enough data for insights."
	if len(results) > 0 {
		insight = fmt.Sprintf("Peak activity is in bucket %s with %d events. Total events analyzed: %d.", peak.Bucket, peak.Count, totalEvents)
		if granularity == "hourly" {
			insight += " Consider mapping hours to UTC+8 to find the most active time of day."
		}
	}

	return jsonResult(map[string]interface{}{
		"target":       target,
		"granularity":  granularity,
		"total_events": totalEvents,
		"peak":         peak,
		"distribution": results,
		"insight":      insight,
	})
}

func (h *HandlerRegistry) handleInteractionStream(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var a InteractionStreamArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	includePreview := true
	if a.IncludeContentPreview != nil {
		includePreview = *a.IncludeContentPreview
	}

	limit := a.Limit
	if limit == 0 {
		limit = 50
	}
	offset := a.Offset

	var queryBuilder strings.Builder
	queryBuilder.WriteString(`
SELECT 
  re.id, re.action_type, re.context_type, re.occurred_at,
  COALESCE(actor_pi.platform_user_id, '') as actor_qq,
  COALESCE(actor_p.display_name, '') as actor_name,
  COALESCE(target_pi.platform_user_id, '') as target_qq,
  COALESCE(target_p.display_name, '') as target_name`)

	if includePreview {
		queryBuilder.WriteString(`, COALESCE(LEFT(c.body, 200), LEFT(m.raw_text, 200), '') as content_preview `)
	} else {
		queryBuilder.WriteString(`, '' as content_preview `)
	}

	queryBuilder.WriteString(`
FROM relation_events re
LEFT JOIN persons actor_p ON actor_p.id = re.actor_person_id
LEFT JOIN person_identifiers actor_pi ON actor_pi.person_id = re.actor_person_id AND actor_pi.platform = 'qq'
LEFT JOIN persons target_p ON target_p.id = re.target_person_id
LEFT JOIN person_identifiers target_pi ON target_pi.person_id = re.target_person_id AND target_pi.platform = 'qq' `)

	if includePreview {
		queryBuilder.WriteString(`
LEFT JOIN contents c ON c.id = re.target_object_id
LEFT JOIN messages m ON m.id = re.target_object_id `)
	}

	queryBuilder.WriteString(` WHERE 1=1 `)

	params := []interface{}{}
	paramIdx := 1

	if a.ActorQQ != "" {
		actorPersonID, err := resolvePersonID(ctx, h.DB, a.ActorQQ)
		if err == nil {
			queryBuilder.WriteString(fmt.Sprintf(" AND re.actor_person_id = $%d", paramIdx))
			params = append(params, actorPersonID)
			paramIdx++
		}
	}

	if a.TargetQQ != "" {
		targetPersonID, err := resolvePersonID(ctx, h.DB, a.TargetQQ)
		if err == nil {
			queryBuilder.WriteString(fmt.Sprintf(" AND re.target_person_id = $%d", paramIdx))
			params = append(params, targetPersonID)
			paramIdx++
		}
	}

	if len(a.ActionTypes) > 0 {
		queryBuilder.WriteString(fmt.Sprintf(" AND re.action_type = ANY($%d)", paramIdx))
		params = append(params, a.ActionTypes)
		paramIdx++
	}

	if a.TimeRangeStart != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND re.occurred_at >= $%d", paramIdx))
		params = append(params, a.TimeRangeStart)
		paramIdx++
	}

	if a.TimeRangeEnd != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND re.occurred_at <= $%d", paramIdx))
		params = append(params, a.TimeRangeEnd)
		paramIdx++
	}

	queryBuilder.WriteString(fmt.Sprintf(" ORDER BY re.occurred_at DESC LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1))
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, queryBuilder.String(), params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query interaction stream: %w", err)
	}
	defer rows.Close()

	type ActorTarget struct {
		QQ   string `json:"qq"`
		Name string `json:"name"`
	}
	type EventItem struct {
		ID             string      `json:"id"`
		ActionType     string      `json:"action_type"`
		ContextType    string      `json:"context_type"`
		OccurredAt     time.Time   `json:"occurred_at"`
		Actor          ActorTarget `json:"actor"`
		Target         ActorTarget `json:"target"`
		ContentPreview string      `json:"content_preview"`
	}

	var events []EventItem
	for rows.Next() {
		var e EventItem
		var occurredAt *time.Time
		if err := rows.Scan(
			&e.ID, &e.ActionType, &e.ContextType, &occurredAt,
			&e.Actor.QQ, &e.Actor.Name,
			&e.Target.QQ, &e.Target.Name,
			&e.ContentPreview,
		); err != nil {
			return nil, err
		}
		if occurredAt != nil {
			e.OccurredAt = *occurredAt
		}
		events = append(events, e)
	}

	if events == nil {
		events = []EventItem{}
	}

	return jsonResult(map[string]interface{}{
		"total_in_page": len(events),
		"events":        events,
	})
}

func (h *HandlerRegistry) handleContentFeed(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var a ContentFeedArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}

	limit := a.Limit
	if limit == 0 {
		limit = 20
	}
	offset := a.Offset

	var queryBuilder strings.Builder
	queryBuilder.WriteString(`
SELECT 
  c.id, c.body, c.published_at, c.context_type,
  COALESCE(pi.platform_user_id, '') as author_qq,
  COALESCE(p.display_name, '') as author_name,
  COALESCE(likes.cnt, 0) as likes_count,
  COALESCE(comments.cnt, 0) as comments_count
FROM contents c
LEFT JOIN persons p ON p.id = c.author_id
LEFT JOIN person_identifiers pi ON pi.person_id = c.author_id AND pi.platform = 'qq'
LEFT JOIN LATERAL (
  SELECT COUNT(*) as cnt FROM relation_events WHERE target_object_id = c.id AND action_type = 'liked'
) likes ON true
LEFT JOIN LATERAL (
  SELECT COUNT(*) as cnt FROM contents child WHERE child.parent_content_id = c.id
) comments ON true
WHERE c.context_type = 'qzone_post' `)

	params := []interface{}{}
	paramIdx := 1

	if a.AuthorQQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, a.AuthorQQ)
		if err == nil {
			queryBuilder.WriteString(fmt.Sprintf(" AND c.author_id = $%d", paramIdx))
			params = append(params, personID)
			paramIdx++
		}
	}

	if a.Keyword != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND c.body ILIKE '%%' || $%d || '%%'", paramIdx))
		params = append(params, a.Keyword)
		paramIdx++
	}

	if a.TimeRangeStart != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND c.published_at >= $%d", paramIdx))
		params = append(params, a.TimeRangeStart)
		paramIdx++
	}

	if a.TimeRangeEnd != "" {
		queryBuilder.WriteString(fmt.Sprintf(" AND c.published_at <= $%d", paramIdx))
		params = append(params, a.TimeRangeEnd)
		paramIdx++
	}

	if a.MinEngagement > 0 {
		queryBuilder.WriteString(fmt.Sprintf(" AND (COALESCE(likes.cnt, 0) + COALESCE(comments.cnt, 0)) >= $%d", paramIdx))
		params = append(params, a.MinEngagement)
		paramIdx++
	}

	sortBy := "recent"
	if a.SortBy != "" {
		sortBy = a.SortBy
	}

	switch sortBy {
	case "most_liked":
		queryBuilder.WriteString(" ORDER BY likes.cnt DESC, c.published_at DESC")
	case "most_commented":
		queryBuilder.WriteString(" ORDER BY comments.cnt DESC, c.published_at DESC")
	case "most_engaged":
		queryBuilder.WriteString(" ORDER BY (COALESCE(likes.cnt, 0) + COALESCE(comments.cnt, 0)) DESC, c.published_at DESC")
	case "recent":
		fallthrough
	default:
		queryBuilder.WriteString(" ORDER BY c.published_at DESC")
	}

	queryBuilder.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1))
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, queryBuilder.String(), params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query content feed: %w", err)
	}
	defer rows.Close()

	type Author struct {
		QQ   string `json:"qq"`
		Name string `json:"name"`
	}

	type ReplyItem struct {
		ID          string    `json:"id"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
		Author      Author    `json:"author"`
	}

	type PostItem struct {
		ID            string      `json:"id"`
		Body          string      `json:"body"`
		PublishedAt   time.Time   `json:"published_at"`
		ContextType   string      `json:"context_type"`
		Author        Author      `json:"author"`
		LikesCount    int         `json:"likes_count"`
		CommentsCount int         `json:"comments_count"`
		ReplyThread   []ReplyItem `json:"reply_thread,omitempty"`
	}

	var posts []PostItem
	for rows.Next() {
		var p PostItem
		var pubAt *time.Time
		if err := rows.Scan(
			&p.ID, &p.Body, &pubAt, &p.ContextType,
			&p.Author.QQ, &p.Author.Name,
			&p.LikesCount, &p.CommentsCount,
		); err != nil {
			return nil, err
		}
		if pubAt != nil {
			p.PublishedAt = *pubAt
		}
		posts = append(posts, p)
	}

	if a.IncludeReplies && len(posts) > 0 {
		for i := range posts {
			childQuery := `
SELECT c.id, c.body, c.published_at,
       COALESCE(pi.platform_user_id, '') as author_qq,
       COALESCE(p.display_name, '') as author_name
FROM contents c
LEFT JOIN persons p ON p.id = c.author_id
LEFT JOIN person_identifiers pi ON pi.person_id = c.author_id AND pi.platform = 'qq'
WHERE c.parent_content_id = $1
ORDER BY c.published_at ASC`
			cRows, err := h.DB.Query(ctx, childQuery, posts[i].ID)
			if err != nil {
				continue
			}
			var replies []ReplyItem
			for cRows.Next() {
				var r ReplyItem
				var rPubAt *time.Time
				if err := cRows.Scan(&r.ID, &r.Body, &rPubAt, &r.Author.QQ, &r.Author.Name); err == nil {
					if rPubAt != nil {
						r.PublishedAt = *rPubAt
					}
					replies = append(replies, r)
				}
			}
			cRows.Close()
			posts[i].ReplyThread = replies
		}
	}

	if posts == nil {
		posts = []PostItem{}
	}

	return jsonResult(map[string]interface{}{
		"total_in_page": len(posts),
		"posts":         posts,
	})
}
