package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ─────────────────────────────────────────────────────────────────────
// sra_content_comments — QZone comment tree for a single content item.
// ─────────────────────────────────────────────────────────────────────

func (h *HandlerRegistry) handleContentComments(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ContentID      string `json:"content_id"`
		Limit          int    `json:"limit"`
		Offset         int    `json:"offset"`
		IncludeReplies *bool  `json:"include_replies"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.ContentID = strings.TrimSpace(input.ContentID)
	if input.ContentID == "" {
		return errorResult("content_id is required"), nil
	}
	includeReplies := input.IncludeReplies == nil || *input.IncludeReplies
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)

	var contentUUID string
	if err := h.DB.QueryRow(ctx, `SELECT id::text FROM contents WHERE id::text=$1 OR platform_content_id=$1 LIMIT 1`, input.ContentID).Scan(&contentUUID); err != nil {
		return errorResult("content not found: " + input.ContentID), nil
	}

	repliesFilter := ""
	if !includeReplies {
		repliesFilter = " AND c.reply_to_content_id IS NULL"
	}
	var total int
	if err := h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM contents c WHERE c.parent_content_id=$1::uuid`+repliesFilter, contentUUID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count comments: %w", err)
	}

	rows, err := h.DB.Query(ctx, `
		SELECT c.id::text, COALESCE(c.platform_content_id,''), COALESCE(pi.platform_user_id,''),
		       COALESCE(p.display_name,''), c.body, c.published_at,
		       COALESCE(c.reply_to_content_id::text,''), COALESCE(rpi.platform_user_id,''),
		       COALESCE(rp.display_name,''), COALESCE(c.raw_record_id::text,'')
		FROM contents c
		LEFT JOIN persons p ON p.id=c.author_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		LEFT JOIN contents rc ON rc.id=c.reply_to_content_id
		LEFT JOIN persons rp ON rp.id=rc.author_id
		LEFT JOIN person_identifiers rpi ON rpi.person_id=rp.id AND rpi.platform='qq'
		WHERE c.parent_content_id=$1::uuid`+repliesFilter+`
		ORDER BY c.published_at ASC NULLS LAST, c.updated_at ASC
		LIMIT $2 OFFSET $3`,
		contentUUID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, platformID, authorQQ, authorName, body, replyToID, replyToQQ, replyToName, rawID string
		var publishedAt *time.Time
		if err := rows.Scan(&id, &platformID, &authorQQ, &authorName, &body, &publishedAt, &replyToID, &replyToQQ, &replyToName, &rawID); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                  id,
			"content_id":          platformID,
			"author_qq":           authorQQ,
			"author_name":         authorName,
			"body":                body,
			"published_at":        publishedAt,
			"reply_to_content_id": replyToID,
			"reply_to_qq":         replyToQQ,
			"reply_to_name":       replyToName,
			"raw_record_id":       rawID,
		})
	}
	return jsonResult(map[string]any{
		"parent_content_id": contentUUID,
		"include_replies":   includeReplies,
		"comments":          pagedResult(items, limit, offset, total),
	})
}

// ─────────────────────────────────────────────────────────────────────
// sra_visitor_stream — aggregated visit events between persons.
// ─────────────────────────────────────────────────────────────────────

func (h *HandlerRegistry) handleVisitorStream(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TargetQQ       string `json:"target_qq"`
		ActorQQ        string `json:"actor_qq"`
		TimeRangeStart string `json:"time_range_start"`
		TimeRangeEnd   string `json:"time_range_end"`
		Limit          int    `json:"limit"`
		Offset         int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.TargetQQ = strings.TrimSpace(input.TargetQQ)
	input.ActorQQ = strings.TrimSpace(input.ActorQQ)
	if input.TargetQQ == "" && input.ActorQQ == "" {
		return errorResult("at least one of target_qq or actor_qq is required"), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)

	var where []string
	var params []any
	idx := 1
	if input.TargetQQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, input.TargetQQ)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		where = append(where, fmt.Sprintf("re.target_person_id=$%d::uuid", idx))
		params = append(params, personID)
		idx++
	}
	if input.ActorQQ != "" {
		personID, err := resolvePersonID(ctx, h.DB, input.ActorQQ)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		where = append(where, fmt.Sprintf("re.actor_person_id=$%d::uuid", idx))
		params = append(params, personID)
		idx++
	}
	if strings.TrimSpace(input.TimeRangeStart) != "" {
		where = append(where, fmt.Sprintf("re.occurred_at >= $%d::timestamptz", idx))
		params = append(params, input.TimeRangeStart)
		idx++
	}
	if strings.TrimSpace(input.TimeRangeEnd) != "" {
		where = append(where, fmt.Sprintf("re.occurred_at <= $%d::timestamptz", idx))
		params = append(params, input.TimeRangeEnd)
		idx++
	}
	whereSQL := " WHERE re.action_type='visited'"
	if len(where) > 0 {
		whereSQL += " AND " + strings.Join(where, " AND ")
	}

	countParams := append([]any(nil), params...)
	var total int
	if err := h.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM (SELECT 1 FROM relation_events re`+whereSQL+` GROUP BY re.actor_person_id, re.target_person_id) v`,
		countParams...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count visitor stream: %w", err)
	}

	query := `
		SELECT COALESCE(apia.platform_user_id,''), COALESCE(pa.display_name,''),
		       COALESCE(apit.platform_user_id,''), COALESCE(pt.display_name,''),
		       MIN(re.occurred_at), MAX(re.occurred_at), COUNT(*)
		FROM relation_events re
		LEFT JOIN persons pa ON pa.id=re.actor_person_id
		LEFT JOIN person_identifiers apia ON apia.person_id=pa.id AND apia.platform='qq'
		LEFT JOIN persons pt ON pt.id=re.target_person_id
		LEFT JOIN person_identifiers apit ON apit.person_id=pt.id AND apit.platform='qq'` +
		whereSQL + `
		GROUP BY re.actor_person_id, re.target_person_id, pa.display_name, apia.platform_user_id,
		         pt.display_name, apit.platform_user_id
		ORDER BY COUNT(*) DESC
		LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)

	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query visitor stream: %w", err)
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var visitorQQ, visitorName, targetQQ, targetName string
		var firstSeen, lastSeen *time.Time
		var count int
		if err := rows.Scan(&visitorQQ, &visitorName, &targetQQ, &targetName, &firstSeen, &lastSeen, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"visitor_qq":   visitorQQ,
			"visitor_name": visitorName,
			"target_qq":    targetQQ,
			"target_name":  targetName,
			"first_seen":   firstSeen,
			"last_seen":    lastSeen,
			"visit_count":  count,
		})
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

// ─────────────────────────────────────────────────────────────────────
// sra_build_evidence_pack — assemble a question-scoped, token-bounded
// evidence pack from messages, contents and relation events.
// ─────────────────────────────────────────────────────────────────────

type mediaMeta struct {
	Sha256   string `json:"sha256"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

type evidencePackEvent struct {
	EventID     string      `json:"event_id"`
	ActionType  string      `json:"action_type"`
	Body        string      `json:"body"`
	OccurredAt  *time.Time  `json:"occurred_at"`
	RawRecordID string      `json:"raw_record_id"`
	Media       []mediaMeta `json:"media,omitempty"`
}

func estimateEventTokens(body string) int {
	return utf8.RuneCountInString(body)/4 + 40
}

// snippetBody truncates body to limit runes (0 = unlimited) and appends an
// ellipsis when truncated.
func snippetBody(body string, limit int) (string, bool) {
	if limit <= 0 {
		return body, false
	}
	runes := []rune(body)
	if len(runes) <= limit {
		return body, false
	}
	return string(runes[:limit]) + "…", true
}

func (h *HandlerRegistry) handleBuildEvidencePack(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		SubjectQQ        string `json:"subject_qq"`
		Question         string `json:"question"`
		TokenBudget      int    `json:"token_budget"`
		RedactionPolicy  string `json:"redaction_policy"`
		IncludeMediaMeta bool   `json:"include_media_meta"`
		Persist          bool   `json:"persist"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.SubjectQQ = strings.TrimSpace(input.SubjectQQ)
	input.Question = strings.TrimSpace(input.Question)
	if input.SubjectQQ == "" {
		return errorResult("subject_qq is required"), nil
	}
	budget := input.TokenBudget
	if budget <= 0 {
		budget = 30000
	}
	if budget > 200000 {
		budget = 200000
	}
	policy := strings.TrimSpace(input.RedactionPolicy)
	if policy == "" {
		policy = "local-only"
	}
	snippetLimit := 400
	if policy == "full" {
		snippetLimit = 0
	}

	personID, err := resolvePersonID(ctx, h.DB, input.SubjectQQ)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	const fetchPage = 500
	seen := map[string]bool{}
	events := []evidencePackEvent{}
	usedTokens := 0
	scanned := 0
	budgetExceeded := false

	for page := 0; !budgetExceeded; page++ {
		rows, err := h.DB.Query(ctx, `
			SELECT event_id::text, action_type, body, occurred_at, COALESCE(raw_record_id::text,'')
			FROM (
				SELECT m.id AS event_id, 'sent_message'::text AS action_type, m.raw_text AS body,
				       m.sent_at AS occurred_at, m.raw_record_id
				FROM messages m WHERE m.sender_id=$1::uuid
				UNION ALL
				SELECT c.id, CASE WHEN c.parent_content_id IS NOT NULL THEN 'commented'::text
				                  ELSE 'published'::text END, c.body, c.published_at, c.raw_record_id
				FROM contents c WHERE c.author_id=$1::uuid
				UNION ALL
				SELECT re.id, re.action_type, ''::text, re.occurred_at, re.raw_record_id
				FROM relation_events re
				WHERE re.actor_person_id=$1::uuid OR re.target_person_id=$1::uuid
			) e
			ORDER BY e.occurred_at DESC NULLS LAST
			LIMIT $2 OFFSET $3`,
			personID, fetchPage, page*fetchPage)
		if err != nil {
			return nil, fmt.Errorf("query evidence events: %w", err)
		}

		fetched := 0
		for rows.Next() {
			fetched++
			scanned++
			var id, actionType, body, rawID string
			var occurredAt *time.Time
			if err := rows.Scan(&id, &actionType, &body, &occurredAt, &rawID); err != nil {
				rows.Close()
				return nil, err
			}
			if seen[id] {
				continue
			}
			snippet, _ := snippetBody(body, snippetLimit)
			est := estimateEventTokens(snippet)
			if usedTokens+est > budget {
				budgetExceeded = true
				break
			}
			seen[id] = true
			usedTokens += est
			events = append(events, evidencePackEvent{
				EventID:     id,
				ActionType:  actionType,
				Body:        snippet,
				OccurredAt:  occurredAt,
				RawRecordID: rawID,
			})
		}
		rows.Close()
		if fetched < fetchPage {
			break
		}
	}

	if input.IncludeMediaMeta && len(events) > 0 {
		var contentIDs, messageIDs []string
		for _, ev := range events {
			switch ev.ActionType {
			case "published", "commented":
				contentIDs = append(contentIDs, ev.EventID)
			case "sent_message":
				messageIDs = append(messageIDs, ev.EventID)
			}
		}
		mediaByOwner := map[string][]mediaMeta{}
		loadMedia := func(query string, ids []string) error {
			if len(ids) == 0 {
				return nil
			}
			rows, err := h.DB.Query(ctx, query, ids)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var ownerID, sha256, mime string
				var size int64
				if err := rows.Scan(&ownerID, &sha256, &mime, &size); err != nil {
					return err
				}
				mediaByOwner[ownerID] = append(mediaByOwner[ownerID], mediaMeta{Sha256: sha256, MimeType: mime, Size: size})
			}
			return rows.Err()
		}
		if err := loadMedia(`
			SELECT COALESCE(mr.content_id::text,''), COALESCE(a.sha256,''), COALESCE(a.mime_type,''), COALESCE(a.size,0)
			FROM media_references mr JOIN media_assets a ON a.id=mr.asset_id
			WHERE mr.content_id=ANY($1::uuid[]) AND mr.status='completed'`, contentIDs); err != nil {
			return nil, fmt.Errorf("query content media: %w", err)
		}
		if err := loadMedia(`
			SELECT COALESCE(mr.message_id::text,''), COALESCE(a.sha256,''), COALESCE(a.mime_type,''), COALESCE(a.size,0)
			FROM media_references mr JOIN media_assets a ON a.id=mr.asset_id
			WHERE mr.message_id=ANY($1::uuid[]) AND mr.status='completed'`, messageIDs); err != nil {
			return nil, fmt.Errorf("query message media: %w", err)
		}
		for i := range events {
			if media, ok := mediaByOwner[events[i].EventID]; ok {
				events[i].Media = media
			}
		}
	}

	result := map[string]any{
		"subject_qq":       input.SubjectQQ,
		"person_id":        personID,
		"question":         input.Question,
		"redaction_policy": policy,
		"token_budget":     budget,
		"estimated_tokens": usedTokens,
		"scanned_events":   scanned,
		"deduplicated":     scanned - len(seen),
		"event_count":      len(events),
		"events":           events,
		"persisted":        false,
	}

	var first, last *time.Time
	for _, ev := range events {
		if ev.OccurredAt == nil {
			continue
		}
		if first == nil || ev.OccurredAt.Before(*first) {
			first = ev.OccurredAt
		}
		if last == nil || ev.OccurredAt.After(*last) {
			last = ev.OccurredAt
		}
	}
	result["time_range"] = map[string]any{"first": first, "last": last}

	if input.Persist {
		if len(events) == 0 {
			return jsonResult(result)
		}
		scope := map[string]any{
			"subject_qq":         input.SubjectQQ,
			"question":           input.Question,
			"include_media_meta": input.IncludeMediaMeta,
			"built_by":           "sra_build_evidence_pack",
		}
		scopeJSON, _ := json.Marshal(scope)
		eventIDs := make([]string, 0, len(events))
		for _, ev := range events {
			eventIDs = append(eventIDs, ev.EventID)
		}
		var packID string
		if err := h.DB.QueryRow(ctx, `
			INSERT INTO evidence_packs(subject_id, scope, event_ids, redaction_policy, token_budget)
			VALUES($1, $2::jsonb, $3::uuid[], $4, $5)
			RETURNING id::text`,
			personID, string(scopeJSON), eventIDs, policy, budget).Scan(&packID); err != nil {
			if err != pgx.ErrNoRows {
				return errorResult(fmt.Sprintf("failed to persist evidence pack: %v", err)), nil
			}
		}
		result["pack_id"] = packID
		result["persisted"] = true
	}

	return jsonResult(result)
}

// ─────────────────────────────────────────────────────────────────────
// sra_hypothesis_check — heuristic supporting/contradicting evidence scan.
// Methodological voices: self (本人自述), other (他人转述), behavior (行为),
// inference (已存推断). Outputs are hypotheses, not facts.
// ─────────────────────────────────────────────────────────────────────

type hypothesisCandidate struct {
	EventID       string     `json:"event_id"`
	Voice         string     `json:"voice"`
	SourceType    string     `json:"source_type"`
	Body          string     `json:"body"`
	OccurredAt    *time.Time `json:"occurred_at"`
	RawRecordID   string     `json:"raw_record_id"`
	Contradiction bool       `json:"contradiction"`
	Weight        int        `json:"-"`
}

func evidenceDirection(supporting, contradicting int) string {
	switch {
	case supporting == 0 && contradicting == 0:
		return "证据不足"
	case supporting > contradicting:
		return "支持为主"
	case contradicting > supporting:
		return "反证为主"
	default:
		return "证据不足"
	}
}

var negationPatterns = []string{
	"不是", "没有", "并非", "从未", "否认", "没去过", "不喜欢", "不认为", "不是的",
	"no ", "not ", "never ", "doesn't", "don't",
}

// negationLikePatterns wraps each bare negation token with wildcards so the
// ILIKE ANY(...) tests work as substring containment. Bare tokens would
// require an exact full-string match and would never fire.
func negationLikePatterns() []string {
	out := make([]string, 0, len(negationPatterns))
	for _, p := range negationPatterns {
		out = append(out, "%"+p+"%")
	}
	return out
}

func (h *HandlerRegistry) handleHypothesisCheck(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		SubjectQQ     string `json:"subject_qq"`
		Claim         string `json:"claim"`
		AttributeType string `json:"attribute_type"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.SubjectQQ = strings.TrimSpace(input.SubjectQQ)
	input.Claim = strings.TrimSpace(input.Claim)
	if input.SubjectQQ == "" || input.Claim == "" {
		return errorResult("subject_qq and claim are required"), nil
	}
	personID, err := resolvePersonID(ctx, h.DB, input.SubjectQQ)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	pattern := "%" + input.Claim + "%"
	qqPattern := "%" + input.SubjectQQ + "%"
	negations := negationLikePatterns()

	var candidates []hypothesisCandidate
	supporting, contradicting, behaviorCount, inferenceCount := 0, 0, 0, 0
	var firstSeen, lastSeen *time.Time

	scanRows := func(rows pgx.Rows, voice, sourceType string, weight int, contradiction bool) {
		defer rows.Close()
		for rows.Next() {
			var id, body, rawID string
			var at *time.Time
			if sourceType == "relation_event" {
				var actionType string
				if err := rows.Scan(&id, &actionType, &at, &rawID); err != nil {
					continue
				}
				body = "[" + actionType + "]"
			} else {
				if err := rows.Scan(&id, &body, &at, &rawID); err != nil {
					continue
				}
			}
			if voice == "behavior" {
				behaviorCount++
			} else if voice == "inference" {
				inferenceCount++
			} else if contradiction {
				contradicting++
			} else {
				supporting++
			}
			if at != nil {
				if firstSeen == nil || at.Before(*firstSeen) {
					firstSeen = at
				}
				if lastSeen == nil || at.After(*lastSeen) {
					lastSeen = at
				}
			}
			snippet, _ := snippetBody(body, 400)
			candidates = append(candidates, hypothesisCandidate{
				EventID:       id,
				Voice:         voice,
				SourceType:    sourceType,
				Body:          snippet,
				OccurredAt:    at,
				RawRecordID:   rawID,
				Contradiction: contradiction,
				Weight:        weight,
			})
		}
	}

	// 1. Self-reported messages (支持) and negated versions (反证).
	rows, err := h.DB.Query(ctx, `
		SELECT m.id::text, m.raw_text, m.sent_at, COALESCE(m.raw_record_id::text,'')
		FROM messages m
		WHERE m.sender_id=$1::uuid AND m.raw_text ILIKE $2
		  AND NOT (m.raw_text ILIKE ANY($3::text[]))
		ORDER BY m.sent_at DESC NULLS LAST LIMIT 100`,
		personID, pattern, negations)
	if err == nil {
		scanRows(rows, "self", "message", 10, false)
	}
	rows, err = h.DB.Query(ctx, `
		SELECT m.id::text, m.raw_text, m.sent_at, COALESCE(m.raw_record_id::text,'')
		FROM messages m
		WHERE m.sender_id=$1::uuid AND m.raw_text ILIKE $2
		  AND m.raw_text ILIKE ANY($3::text[])
		ORDER BY m.sent_at DESC NULLS LAST LIMIT 100`,
		personID, pattern, negations)
	if err == nil {
		scanRows(rows, "self", "message", 10, true)
	}

	// 2. Self-reported QZone contents.
	rows, err = h.DB.Query(ctx, `
		SELECT c.id::text, c.body, c.published_at, COALESCE(c.raw_record_id::text,'')
		FROM contents c
		WHERE c.author_id=$1::uuid AND c.body ILIKE $2
		  AND NOT (c.body ILIKE ANY($3::text[]))
		ORDER BY c.published_at DESC NULLS LAST LIMIT 100`,
		personID, pattern, negations)
	if err == nil {
		scanRows(rows, "self", "content", 10, false)
	}
	rows, err = h.DB.Query(ctx, `
		SELECT c.id::text, c.body, c.published_at, COALESCE(c.raw_record_id::text,'')
		FROM contents c
		WHERE c.author_id=$1::uuid AND c.body ILIKE $2
		  AND c.body ILIKE ANY($3::text[])
		ORDER BY c.published_at DESC NULLS LAST LIMIT 100`,
		personID, pattern, negations)
	if err == nil {
		scanRows(rows, "self", "content", 10, true)
	}

	// 3. Other people mentioning the subject QQ while discussing the claim.
	rows, err = h.DB.Query(ctx, `
		SELECT m.id::text, m.raw_text, m.sent_at, COALESCE(m.raw_record_id::text,'')
		FROM messages m
		WHERE m.raw_text ILIKE $1 AND m.raw_text ILIKE $2
		  AND NOT (m.raw_text ILIKE ANY($3::text[]))
		ORDER BY m.sent_at DESC NULLS LAST LIMIT 100`,
		pattern, qqPattern, negations)
	if err == nil {
		scanRows(rows, "other", "message", 5, false)
	}
	rows, err = h.DB.Query(ctx, `
		SELECT m.id::text, m.raw_text, m.sent_at, COALESCE(m.raw_record_id::text,'')
		FROM messages m
		WHERE m.raw_text ILIKE $1 AND m.raw_text ILIKE $2
		  AND m.raw_text ILIKE ANY($3::text[])
		ORDER BY m.sent_at DESC NULLS LAST LIMIT 100`,
		pattern, qqPattern, negations)
	if err == nil {
		scanRows(rows, "other", "message", 5, true)
	}

	// 4. Behavioral evidence: interactions involving the subject.
	rows, err = h.DB.Query(ctx, `
		SELECT re.id::text, re.action_type, re.occurred_at, COALESCE(re.raw_record_id::text,'')
		FROM relation_events re
		WHERE (re.actor_person_id=$1::uuid OR re.target_person_id=$1::uuid)
		  AND re.action_type IN ('liked','commented','replied_to','visited','mentioned','published')
		ORDER BY re.occurred_at DESC NULLS LAST LIMIT 100`,
		personID)
	if err == nil {
		scanRows(rows, "behavior", "relation_event", 4, false)
	}

	// 5. Prior stored inferences (hypotheses only) when scoping by attribute.
	if strings.TrimSpace(input.AttributeType) != "" {
		rows, err = h.DB.Query(ctx, `
			SELECT i.id::text, COALESCE(i.value::text,''), i.created_at, ''::text
			FROM inferences i
			WHERE i.subject_id=$1 AND i.attribute_type=$2 AND i.review_status != 'rejected'
			ORDER BY i.created_at DESC NULLS LAST LIMIT 100`,
			personID, strings.TrimSpace(input.AttributeType))
		if err == nil {
			scanRows(rows, "inference", "inference", 2, false)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Weight != candidates[j].Weight {
			return candidates[i].Weight > candidates[j].Weight
		}
		if candidates[i].OccurredAt != nil && candidates[j].OccurredAt != nil {
			return candidates[i].OccurredAt.After(*candidates[j].OccurredAt)
		}
		return candidates[i].OccurredAt != nil
	})
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}

	return jsonResult(map[string]any{
		"subject_qq":          input.SubjectQQ,
		"claim":               input.Claim,
		"attribute_type":      input.AttributeType,
		"supporting_count":    supporting,
		"contradicting_count": contradicting,
		"behavior_count":      behaviorCount,
		"inference_count":     inferenceCount,
		"first_seen":          firstSeen,
		"last_seen":           lastSeen,
		"direction":           evidenceDirection(supporting, contradicting),
		"methodology":         "heuristic keyword + negation scan; voices: self=本人自述, other=他人转述, behavior=行为, inference=已存推断. 仅假设核查，不构成事实。",
		"evidence":            candidates,
	})
}

// ─────────────────────────────────────────────────────────────────────
// sra_trace_claim — inference → relation_events → raw_records chain.
// ─────────────────────────────────────────────────────────────────────

func (h *HandlerRegistry) handleTraceClaim(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		InferenceID string `json:"inference_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.InferenceID = strings.TrimSpace(input.InferenceID)
	if input.InferenceID == "" {
		return errorResult("inference_id is required"), nil
	}

	var subjectID, attributeType, methodVersion, reviewStatus, value string
	var inferredEventIDs []string
	var confidence float64
	var createdAt *time.Time
	if err := h.DB.QueryRow(ctx, `
		SELECT subject_id, attribute_type, COALESCE(value::text,''), confidence, method_version,
		       evidence_event_ids, review_status, created_at
		FROM inferences WHERE id=$1`, input.InferenceID).Scan(
		&subjectID, &attributeType, &value, &confidence, &methodVersion, &inferredEventIDs, &reviewStatus, &createdAt); err != nil {
		if err == pgx.ErrNoRows {
			return errorResult("inference not found: " + input.InferenceID), nil
		}
		return nil, fmt.Errorf("load inference: %w", err)
	}

	found := map[string]bool{}
	rawIDs := []string{}
	events := []map[string]any{}
	if len(inferredEventIDs) > 0 {
		rows, err := h.DB.Query(ctx, `
			SELECT id::text, action_type, context_type, occurred_at,
			       COALESCE(actor_person_id::text,''), COALESCE(target_person_id::text,''),
			       COALESCE(target_object_id::text,''), COALESCE(raw_record_id::text,''), evidence_ids
			FROM relation_events WHERE id=ANY($1::uuid[])`, inferredEventIDs)
		if err != nil {
			return nil, fmt.Errorf("query evidence events: %w", err)
		}
		for rows.Next() {
			var id, actionType, contextType, actorID, targetID, targetObject, rawID string
			var occurredAt *time.Time
			var evidenceIDs []string
			if err := rows.Scan(&id, &actionType, &contextType, &occurredAt, &actorID, &targetID, &targetObject, &rawID, &evidenceIDs); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = true
			if rawID != "" {
				rawIDs = append(rawIDs, rawID)
			}
			events = append(events, map[string]any{
				"id":                  id,
				"action_type":         actionType,
				"context_type":        contextType,
				"occurred_at":         occurredAt,
				"actor_person_id":     actorID,
				"target_person_id":    targetID,
				"target_object_id":    targetObject,
				"raw_record_id":       rawID,
				"nested_evidence_ids": evidenceIDs,
			})
		}
		rows.Close()
	}

	// Evidence ids that do not resolve to relation events may still be raw
	// record ids; resolve those directly so legitimately referenced raw
	// evidence is never reported as missing.
	missing := []string{}
	directRawIDs := []string{}
	for _, id := range inferredEventIDs {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		rows, err := h.DB.Query(ctx, `SELECT id::text FROM raw_records WHERE id=ANY($1::uuid[])`, missing)
		if err != nil {
			return nil, fmt.Errorf("resolve raw evidence: %w", err)
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				found[id] = true
				directRawIDs = append(directRawIDs, id)
			}
		}
		rows.Close()
		recomputed := []string{}
		for _, id := range missing {
			if !found[id] {
				recomputed = append(recomputed, id)
			}
		}
		missing = recomputed
	}

	rawIDs = append(rawIDs, directRawIDs...)
	records := []map[string]any{}
	if len(rawIDs) > 0 {
		rows, err := h.DB.Query(ctx, `
			SELECT id::text, source, endpoint_or_event_type, payload, payload_hash, collected_at
			FROM raw_records WHERE id=ANY($1::uuid[])`, rawIDs)
		if err != nil {
			return nil, fmt.Errorf("query raw records: %w", err)
		}
		for rows.Next() {
			var id, source, endpoint, hash string
			var payload []byte
			var collectedAt *time.Time
			if err := rows.Scan(&id, &source, &endpoint, &payload, &hash, &collectedAt); err != nil {
				rows.Close()
				return nil, err
			}
			var parsed any
			_ = json.Unmarshal(payload, &parsed)
			records = append(records, map[string]any{
				"id":                     id,
				"source":                 source,
				"endpoint_or_event_type": endpoint,
				"payload":                parsed,
				"payload_hash":           hash,
				"collected_at":           collectedAt,
			})
		}
		rows.Close()
	}

	return jsonResult(map[string]any{
		"inference": map[string]any{
			"id":                 input.InferenceID,
			"subject_id":         subjectID,
			"attribute_type":     attributeType,
			"value":              value,
			"confidence":         confidence,
			"method_version":     methodVersion,
			"review_status":      reviewStatus,
			"created_at":         createdAt,
			"evidence_event_ids": inferredEventIDs,
		},
		"trace_complete":    len(missing) == 0,
		"missing_event_ids": missing,
		"events":            events,
		"raw_records":       records,
	})
}
