package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

var atQQRegex = regexp.MustCompile(`@(\d{5,12})`)

// planRoutes handles POST /api/v1/analysis/routes
func (s *Server) planRoutes(w http.ResponseWriter, r *http.Request) {
	var req analysis.RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	req.SourceQQ = strings.TrimSpace(req.SourceQQ)
	req.TargetQQ = strings.TrimSpace(req.TargetQQ)
	if req.SourceQQ == "" || req.TargetQQ == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "both source_qq and target_qq are required"})
		return
	}

	router := analysis.Router{DB: s.Repo.DB, Policy: s.RoutingPolicy}
	result, err := router.PlanRoutes(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// relationshipDeep handles GET /api/v1/persons/{id}/relationship-deep
func (s *Server) relationshipDeep(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	targetQQ := strings.TrimSpace(r.URL.Query().Get("target_qq"))
	if targetQQ == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_qq query parameter is required"})
		return
	}

	var targetID, targetName, thisName, thisQQ string
	if err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT p.id, COALESCE(p.display_name, ''), COALESCE(pi.platform_user_id, '')
		FROM persons p
		JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE pi.platform_user_id = $1 LIMIT 1`, targetQQ).Scan(&targetID, &targetName, &targetQQ); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "target person not found"})
		return
	}

	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT COALESCE(p.display_name, ''), COALESCE(pi.platform_user_id, '')
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id = $1 LIMIT 1`, personID).Scan(&thisName, &thisQQ)

	result := map[string]any{
		"this_person": map[string]string{
			"id":   personID,
			"name": thisName,
			"qq":   thisQQ,
		},
		"target_person": map[string]string{
			"id":   targetID,
			"name": targetName,
			"qq":   targetQQ,
		},
	}

	// 1. Asymmetric Directional Flow (A -> B vs B -> A)
	var thisLikes, thisComments, thisMessages int64
	var targetLikes, targetComments, targetMessages int64

	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT 
			COUNT(CASE WHEN re.action_type='liked' AND re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2) THEN 1 END),
			COUNT(CASE WHEN re.action_type='commented' AND re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2) THEN 1 END),
			COUNT(CASE WHEN re.action_type='liked' AND re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1) THEN 1 END),
			COUNT(CASE WHEN re.action_type='commented' AND re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1) THEN 1 END)
		FROM relation_events re
		WHERE (re.actor_person_id=$1 OR re.actor_person_id=$2)`,
		personID, targetID).Scan(&thisLikes, &thisComments, &targetLikes, &targetComments)

	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT 
			COUNT(CASE WHEN m.sender_id=$1 AND c.platform_conversation_id=$3 THEN 1 END),
			COUNT(CASE WHEN m.sender_id=$2 AND c.platform_conversation_id=$4 THEN 1 END)
		FROM messages m
		JOIN conversations c ON c.id=m.conversation_id
		WHERE c.conversation_type='private' AND ((m.sender_id=$1 AND c.platform_conversation_id=$3) OR (m.sender_id=$2 AND c.platform_conversation_id=$4))`,
		personID, targetID, targetQQ, thisQQ).Scan(&thisMessages, &targetMessages)

	totalThisInitiatives := thisLikes + thisComments + thisMessages
	totalTargetInitiatives := targetLikes + targetComments + targetMessages
	totalAll := totalThisInitiatives + totalTargetInitiatives

	var thisRatio, targetRatio float64
	if totalAll > 0 {
		thisRatio = math.Round((float64(totalThisInitiatives)/float64(totalAll))*1000) / 10
		targetRatio = math.Round((float64(totalTargetInitiatives)/float64(totalAll))*1000) / 10
	} else {
		thisRatio = 50.0
		targetRatio = 50.0
	}

	powerDynamics := "balanced"
	if thisRatio >= 80 {
		powerDynamics = "strongly_this_dominant"
	} else if thisRatio >= 65 {
		powerDynamics = "moderately_this_dominant"
	} else if targetRatio >= 80 {
		powerDynamics = "strongly_target_dominant"
	} else if targetRatio >= 65 {
		powerDynamics = "moderately_target_dominant"
	}

	result["directionality"] = map[string]any{
		"this_to_target": map[string]int64{
			"likes":    thisLikes,
			"comments": thisComments,
			"messages": thisMessages,
			"total":    totalThisInitiatives,
		},
		"target_to_this": map[string]int64{
			"likes":    targetLikes,
			"comments": targetComments,
			"messages": targetMessages,
			"total":    totalTargetInitiatives,
		},
		"this_ratio_pct":   thisRatio,
		"target_ratio_pct": targetRatio,
		"power_dynamics":   powerDynamics,
	}

	// 2. Fast Response Latency Analysis ("特别关心" 秒赞秒评检测)
	var fastResponseCount int64
	var avgLatencyMinutes float64
	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT 
			COUNT(CASE WHEN EXTRACT(EPOCH FROM (re.occurred_at - c.published_at))/60 <= 5 THEN 1 END),
			COALESCE(AVG(EXTRACT(EPOCH FROM (re.occurred_at - c.published_at))/60), 0)
		FROM relation_events re
		JOIN contents c ON c.id = re.target_object_id
		WHERE re.actor_person_id = $1 AND c.author_id = $2
		  AND re.occurred_at >= c.published_at`, personID, targetID).Scan(&fastResponseCount, &avgLatencyMinutes)

	specialAttentionSuspected := fastResponseCount >= 3 || (fastResponseCount >= 1 && totalThisInitiatives <= 3)
	result["response_latency"] = map[string]any{
		"fast_responses_under_5min":   fastResponseCount,
		"avg_latency_minutes":         math.Round(avgLatencyMinutes*10) / 10,
		"special_attention_suspected": specialAttentionSuspected,
	}

	// 3. 24-hour Circadian Rhythm Alignment
	thisHours := make([]int, 24)
	targetHours := make([]int, 24)

	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT EXTRACT(HOUR FROM occurred_at)::int as hr, COUNT(*) 
		FROM relation_events WHERE actor_person_id = $1 GROUP BY hr`, personID)
	if err == nil {
		for rows.Next() {
			var hr, cnt int
			if err := rows.Scan(&hr, &cnt); err == nil && hr >= 0 && hr < 24 {
				thisHours[hr] = cnt
			}
		}
		rows.Close()
	}

	tRows, err := s.Repo.DB.Query(r.Context(), `
		SELECT EXTRACT(HOUR FROM occurred_at)::int as hr, COUNT(*) 
		FROM relation_events WHERE actor_person_id = $1 GROUP BY hr`, targetID)
	if err == nil {
		for tRows.Next() {
			var hr, cnt int
			if err := tRows.Scan(&hr, &cnt); err == nil && hr >= 0 && hr < 24 {
				targetHours[hr] = cnt
			}
		}
		tRows.Close()
	}

	similarity := cosineSimilarity(thisHours, targetHours)
	result["circadian_rhythm"] = map[string]any{
		"this_hours":           thisHours,
		"target_hours":         targetHours,
		"similarity_score_pct": math.Round(similarity*1000) / 10,
	}

	// 4. Intermediary Triads (Top 3 common key contacts)
	type TriadContact struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		QQ          string `json:"qq"`
		WeightThis  int    `json:"weight_this"`
		WeightTgt   int    `json:"weight_target"`
		TotalWeight int    `json:"total_weight"`
	}
	var triads []TriadContact
	triadRows, err := s.Repo.DB.Query(r.Context(), `
		SELECT 
			p.id::text, p.display_name, COALESCE(pi.platform_user_id, ''),
			(SELECT COUNT(*) FROM relation_events re WHERE (re.actor_person_id=p.id AND re.target_person_id=$1) OR (re.actor_person_id=$1 AND re.target_person_id=p.id)) as w_this,
			(SELECT COUNT(*) FROM relation_events re WHERE (re.actor_person_id=p.id AND re.target_person_id=$2) OR (re.actor_person_id=$2 AND re.target_person_id=p.id)) as w_tgt
		FROM persons p
		JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id != $1 AND p.id != $2
		  AND EXISTS (SELECT 1 FROM relation_events re WHERE (re.actor_person_id=p.id AND re.target_person_id=$1) OR (re.actor_person_id=$1 AND re.target_person_id=p.id))
		  AND EXISTS (SELECT 1 FROM relation_events re WHERE (re.actor_person_id=p.id AND re.target_person_id=$2) OR (re.actor_person_id=$2 AND re.target_person_id=p.id))
		ORDER BY (w_this + w_tgt) DESC
		LIMIT 5`, personID, targetID)
	if err == nil {
		for triadRows.Next() {
			var tc TriadContact
			if err := triadRows.Scan(&tc.ID, &tc.Name, &tc.QQ, &tc.WeightThis, &tc.WeightTgt); err == nil {
				tc.TotalWeight = tc.WeightThis + tc.WeightTgt
				triads = append(triads, tc)
			}
		}
		triadRows.Close()
	}
	result["intermediary_triads"] = triads

	// 5. Overall Qualitative Relationship Classification
	relCategory := "casual_acquaintance"
	if totalAll >= 30 || (thisMessages+targetMessages > 10) {
		relCategory = "intimate_frequent"
	} else if specialAttentionSuspected && totalThisInitiatives > totalTargetInitiatives*3 {
		relCategory = "unidirectional_focal"
	} else if len(triads) > 0 && totalAll == 0 {
		relCategory = "indirect_connected"
	} else if totalAll == 0 {
		relCategory = "dormant"
	}
	result["category"] = relCategory

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func cosineSimilarity(a, b []int) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		magA += float64(a[i] * a[i])
		magB += float64(b[i] * b[i])
	}
	if magA == 0 || magB == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

// evidenceDetails handles POST /api/v1/analysis/evidence-details
func (s *Server) evidenceDetails(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EvidenceIDs  []string `json:"evidence_ids"`
		SourceQQ     string   `json:"source_qq"`
		TargetQQ     string   `json:"target_qq"`
		RelationType string   `json:"relation_type"`
		Limit        int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request: " + err.Error()})
		return
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 100
	}

	items := make([]EvidenceDetailItem, 0)

	// Case 1: Query directly by raw_record IDs (step.evidence_ids)
	if len(req.EvidenceIDs) > 0 {
		rows, err := s.Repo.DB.Query(r.Context(), `
			SELECT 
				rr.id::text, rr.source, rr.endpoint_or_event_type, rr.collected_at::text, rr.payload,
				COALESCE((SELECT re.target_object_id::text FROM relation_events re WHERE rr.id = ANY(re.evidence_ids) LIMIT 1), '') as target_obj_id
			FROM raw_records rr
			WHERE rr.id = ANY($1::uuid[])
			ORDER BY rr.collected_at DESC
			LIMIT $2`, req.EvidenceIDs, req.Limit)
		if err == nil {
			for rows.Next() {
				var id, source, endpoint, collectedAt, targetObjID string
				var payload map[string]any
				if err := rows.Scan(&id, &source, &endpoint, &collectedAt, &payload, &targetObjID); err == nil {
					item := parseRawRecordToDetailItem(id, source, endpoint, collectedAt, payload)
					if targetObjID != "" {
						item.ContentKey = targetObjID
					}
					items = append(items, item)
				}
			}
			rows.Close()
		}
	} else if req.SourceQQ != "" && req.TargetQQ != "" {
		// Case 2: Query relation_events between Source and Target, then unnest evidence_ids
		rows, err := s.Repo.DB.Query(r.Context(), `
			WITH p1 AS (SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1),
			     p2 AS (SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$2 LIMIT 1),
			     matched_events AS (
			         SELECT DISTINCT unnest(evidence_ids) as ev_id, COALESCE(re.target_object_id::text, '') as target_obj_id
			         FROM relation_events re
			         WHERE (
			             (re.actor_person_id = (SELECT person_id FROM p1) AND (re.target_person_id = (SELECT person_id FROM p2) OR re.target_object_id IN (SELECT id FROM contents WHERE author_id = (SELECT person_id FROM p2))))
			             OR
			             (re.actor_person_id = (SELECT person_id FROM p2) AND (re.target_person_id = (SELECT person_id FROM p1) OR re.target_object_id IN (SELECT id FROM contents WHERE author_id = (SELECT person_id FROM p1))))
			         )
			         AND ($3 = '' OR re.action_type = $3)
			         LIMIT 150
			     )
			SELECT 
				rr.id::text, rr.source, rr.endpoint_or_event_type, rr.collected_at::text, rr.payload,
				COALESCE(me.target_obj_id, '')
			FROM raw_records rr
			JOIN matched_events me ON me.ev_id = rr.id
			ORDER BY rr.collected_at DESC
			LIMIT $4`, req.SourceQQ, req.TargetQQ, req.RelationType, req.Limit)
		if err == nil {
			for rows.Next() {
				var id, source, endpoint, collectedAt, targetObjID string
				var payload map[string]any
				if err := rows.Scan(&id, &source, &endpoint, &collectedAt, &payload, &targetObjID); err == nil {
					item := parseRawRecordToDetailItem(id, source, endpoint, collectedAt, payload)
					if targetObjID != "" {
						item.ContentKey = targetObjID
					}
					items = append(items, item)
				}
			}
			rows.Close()
		}
	}

	type CommonGroupInfo struct {
		ID              string `json:"id"`
		PlatformGroupID string `json:"platform_group_id"`
		Name            string `json:"name"`
		AvatarURI       string `json:"avatar_uri"`
		Role1           string `json:"role_1"`
		Card1           string `json:"card_1"`
		Role2           string `json:"role_2"`
		Card2           string `json:"card_2"`
		MemberCount     int    `json:"member_count"`
		IsCurrent       bool   `json:"is_current"`
		StatusLabel     string `json:"status_label"`
	}

	commonGroups := make([]CommonGroupInfo, 0)
	if req.SourceQQ != "" && req.TargetQQ != "" {
		gRows, err := s.Repo.DB.Query(r.Context(), `
			WITH p1 AS (
				SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1
				UNION ALL
				SELECT id as person_id FROM persons WHERE id::text=$1
				LIMIT 1
			),
			p2 AS (
				SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$2
				UNION ALL
				SELECT id as person_id FROM persons WHERE id::text=$2
				LIMIT 1
			)
			SELECT 
				g.id::text,
				COALESCE(g.platform_group_id, ''),
				COALESCE(g.group_name, '未命名群聊'),
				COALESCE('/api/v1/media/avatars/group/' || g.platform_group_id, ''),
				COALESCE(gm1.role, 'member'),
				COALESCE(gm1.card, ''),
				COALESCE(gm2.role, 'member'),
				COALESCE(gm2.card, ''),
				(SELECT COUNT(*) FROM group_memberships gm WHERE gm.group_id = g.id)::int,
				(gm1.valid_to IS NULL AND gm2.valid_to IS NULL) as is_current
			FROM group_memberships gm1
			JOIN group_memberships gm2 ON gm1.group_id = gm2.group_id
			JOIN groups g ON g.id = gm1.group_id
			WHERE gm1.person_id = (SELECT person_id FROM p1)
			  AND gm2.person_id = (SELECT person_id FROM p2)
			ORDER BY is_current DESC, 9 DESC`, req.SourceQQ, req.TargetQQ)
		if err == nil {
			for gRows.Next() {
				var g CommonGroupInfo
				if err := gRows.Scan(&g.ID, &g.PlatformGroupID, &g.Name, &g.AvatarURI, &g.Role1, &g.Card1, &g.Role2, &g.Card2, &g.MemberCount, &g.IsCurrent); err == nil {
					if g.IsCurrent {
						g.StatusLabel = "当前在群"
					} else {
						g.StatusLabel = "曾同在群"
					}
					commonGroups = append(commonGroups, g)

					pTitle := fmt.Sprintf("群成员信息 (共 %d 人 · %s)", g.MemberCount, g.StatusLabel)
					if g.Card1 != "" || g.Card2 != "" {
						pTitle = fmt.Sprintf("名片: %s / %s (%s)", g.Card1, g.Card2, g.StatusLabel)
					}

					actorAvatar := ""
					if req.SourceQQ != "" {
						actorAvatar = fmt.Sprintf("/api/v1/media/avatars/person/%s", req.SourceQQ)
					}
					targetAvatar := ""
					if req.TargetQQ != "" {
						targetAvatar = fmt.Sprintf("/api/v1/media/avatars/person/%s", req.TargetQQ)
					}

					actionLabel := "共同在群"
					if !g.IsCurrent {
						actionLabel = "曾同在群"
					}

					items = append(items, EvidenceDetailItem{
						ID:            "common_group_" + g.ID,
						ActionType:    "common_group",
						ActionLabel:   actionLabel,
						ContextType:   "group_membership",
						ActorQQ:       req.SourceQQ,
						TargetQQ:      req.TargetQQ,
						ActorAvatar:   actorAvatar,
						TargetAvatar:  targetAvatar,
						ContextName:   g.Name,
						ContentText:   fmt.Sprintf("双方共同加入群聊「%s」(群号: %s · %s)", g.Name, g.PlatformGroupID, g.StatusLabel),
						ParentTitle:   pTitle,
						ParentSnippet: fmt.Sprintf("同在群关联 · 该群共采集 %d 名成员", g.MemberCount),
					})
				}
			}
			gRows.Close()
		}
	}

	// 3. Enrich reply contexts, resolve @QQ names, and fetch QZone feed snapshots
	enrichedItems := s.enrichEvidenceItems(r.Context(), items)

	writeJSON(w, http.StatusOK, map[string]any{
		"data":          enrichedItems,
		"total":         len(enrichedItems),
		"common_groups": commonGroups,
	})
}

type QZoneFeedSnapshot struct {
	ContentID    string   `json:"content_id"`
	Tid          string   `json:"tid"`
	AuthorQQ     string   `json:"author_qq"`
	AuthorName   string   `json:"author_name"`
	AuthorAvatar string   `json:"author_avatar"`
	Body         string   `json:"body"`
	PublishedAt  string   `json:"published_at"`
	Images       []string `json:"images,omitempty"`
}

type EvidenceDetailItem struct {
	ID            string             `json:"id"`
	ActionType    string             `json:"action_type"`
	ActionLabel   string             `json:"action_label"`
	ContextType   string             `json:"context_type"`
	ActorName     string             `json:"actor_name"`
	ActorQQ       string             `json:"actor_qq"`
	ActorAvatar   string             `json:"actor_avatar"`
	TargetName    string             `json:"target_name"`
	TargetQQ      string             `json:"target_qq"`
	TargetAvatar  string             `json:"target_avatar"`
	ContentText   string             `json:"content_text"`
	ParentTitle   string             `json:"parent_title"`
	ParentSnippet string             `json:"parent_snippet"`
	ContextName   string             `json:"context_name"`
	OccurredAt    string             `json:"occurred_at"`
	RawSnippet    string             `json:"raw_snippet,omitempty"`
	FeedSnapshot  *QZoneFeedSnapshot `json:"feed_snapshot,omitempty"`
	ContentKey    string             `json:"-"`
	ReplyMsgID    string             `json:"-"`
	MentionedQQ   string             `json:"-"`
	GroupID       string             `json:"-"`
}

func (s *Server) enrichEvidenceItems(ctx context.Context, items []EvidenceDetailItem) []EvidenceDetailItem {
	if len(items) == 0 {
		return items
	}

	// 1. Collect all replyMsgIDs and mentioned QQs
	replyIDs := make([]string, 0)
	qqs := make(map[string]bool)

	for _, it := range items {
		if it.ActorQQ != "" {
			qqs[it.ActorQQ] = true
		}
		if it.TargetQQ != "" {
			qqs[it.TargetQQ] = true
		}
		if it.MentionedQQ != "" {
			qqs[it.MentionedQQ] = true
		}
		if it.ReplyMsgID != "" {
			replyIDs = append(replyIDs, it.ReplyMsgID)
		}
		// Extract any @123456 from ContentText
		for _, match := range atQQRegex.FindAllStringSubmatch(it.ContentText, -1) {
			if len(match) > 1 && match[1] != "" {
				qqs[match[1]] = true
			}
		}
	}

	// 2. Query nicknames for all QQs from person_identifiers AND accounts
	qqList := make([]string, 0, len(qqs))
	for q := range qqs {
		qqList = append(qqList, q)
	}

	qqToName := make(map[string]string)
	if len(qqList) > 0 {
		rows, err := s.Repo.DB.Query(ctx, `
			SELECT pi.platform_user_id, COALESCE(p.display_name, '')
			FROM person_identifiers pi
			JOIN persons p ON p.id = pi.person_id
			WHERE pi.platform = 'qq' AND pi.platform_user_id = ANY($1::text[])`, qqList)
		if err == nil {
			for rows.Next() {
				var q, name string
				if err := rows.Scan(&q, &name); err == nil && name != "" {
					qqToName[q] = name
				}
			}
			rows.Close()
		}

		// Also check logged-in / bot accounts
		accRows, err := s.Repo.DB.Query(ctx, `
			SELECT qq_uin, COALESCE(name, '账号')
			FROM accounts
			WHERE qq_uin = ANY($1::text[])`, qqList)
		if err == nil {
			for accRows.Next() {
				var q, name string
				if err := accRows.Scan(&q, &name); err == nil && name != "" {
					if _, exists := qqToName[q]; !exists || qqToName[q] == "" {
						qqToName[q] = name
					}
				}
			}
			accRows.Close()
		}
	}

	// 3. Query replied messages for all ReplyMsgIDs
	replyMsgMap := make(map[string]struct {
		SenderName string
		SenderQQ   string
		RawText    string
	})
	if len(replyIDs) > 0 {
		rows, err := s.Repo.DB.Query(ctx, `
			SELECT 
				m.source_message_id,
				COALESCE(p.display_name, '成员'),
				COALESCE(pi.platform_user_id, ''),
				COALESCE(m.raw_text, '')
			FROM messages m
			LEFT JOIN persons p ON p.id = m.sender_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE m.source_message_id = ANY($1::text[])`, replyIDs)
		if err == nil {
			for rows.Next() {
				var msgID, senderName, senderQQ, rawText string
				if err := rows.Scan(&msgID, &senderName, &senderQQ, &rawText); err == nil {
					replyMsgMap[msgID] = struct {
						SenderName string
						SenderQQ   string
						RawText    string
					}{
						SenderName: senderName,
						SenderQQ:   senderQQ,
						RawText:    rawText,
					}
				}
			}
			rows.Close()
		}
	}

	// 4. Query QZone feeds for all ContentKeys
	contentKeys := make([]string, 0)
	for _, it := range items {
		if it.ContentKey != "" {
			contentKeys = append(contentKeys, it.ContentKey)
			if strings.Contains(it.ContentKey, "/mood/") {
				parts := strings.Split(it.ContentKey, "/mood/")
				if len(parts) > 1 && parts[1] != "" {
					contentKeys = append(contentKeys, parts[1])
				}
			}
		}
	}

	feedMap := make(map[string]QZoneFeedSnapshot)
	if len(contentKeys) > 0 {
		cRows, err := s.Repo.DB.Query(ctx, `
			SELECT 
				c.id::text,
				COALESCE(c.platform_content_id, ''),
				COALESCE(p.display_name, '空间用户'),
				COALESCE(pi.platform_user_id, ''),
				COALESCE(c.body, ''),
				COALESCE(to_char(c.published_at, 'YYYY-MM-DD"T"HH24:MI:SS"+08:00"'), '')
			FROM contents c
			LEFT JOIN persons p ON p.id = c.author_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE c.id::text = ANY($1::text[])
			   OR c.platform_content_id = ANY($1::text[])`, contentKeys)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn("load QZone feed snapshots failed", "error", err)
			}
		} else {
			for cRows.Next() {
				var cid, pid, authorName, authorQQ, body, pubAt string
				if err := cRows.Scan(&cid, &pid, &authorName, &authorQQ, &body, &pubAt); err == nil {
					snap := QZoneFeedSnapshot{
						ContentID:    cid,
						Tid:          pid,
						AuthorQQ:     authorQQ,
						AuthorName:   authorName,
						AuthorAvatar: "/api/v1/media/avatars/person/" + authorQQ,
						Body:         cleanCQCodeString(body),
						PublishedAt:  pubAt,
					}
					if cid != "" {
						feedMap[cid] = snap
					}
					if pid != "" {
						feedMap[pid] = snap
					}
				} else {
					if s.Logger != nil {
						s.Logger.Warn("scan QZone feed snapshot failed", "error", err)
					}
				}
			}
			cRows.Close()
		}
	}

	// 5. Enrich items
	for i := range items {
		// Fill in actor/target names if empty
		if (items[i].ActorName == "未知人员" || items[i].ActorName == "") && items[i].ActorQQ != "" {
			if name, ok := qqToName[items[i].ActorQQ]; ok {
				items[i].ActorName = name
			}
		}
		if items[i].TargetName == "" && items[i].TargetQQ != "" {
			if name, ok := qqToName[items[i].TargetQQ]; ok {
				items[i].TargetName = name
			}
		}

		// Attach feed snapshot if available
		if items[i].ContentKey != "" {
			snap, hasSnap := feedMap[items[i].ContentKey]
			if !hasSnap && strings.Contains(items[i].ContentKey, "/mood/") {
				parts := strings.Split(items[i].ContentKey, "/mood/")
				if len(parts) > 1 {
					snap, hasSnap = feedMap[parts[1]]
				}
			}
			if hasSnap {
				items[i].FeedSnapshot = &snap
				if items[i].TargetName == "" && snap.AuthorName != "" {
					items[i].TargetName = snap.AuthorName
				}
				if items[i].TargetQQ == "" && snap.AuthorQQ != "" {
					items[i].TargetQQ = snap.AuthorQQ
				}
				if items[i].TargetAvatar == "" && snap.AuthorQQ != "" {
					items[i].TargetAvatar = snap.AuthorAvatar
				}
				items[i].ParentTitle = fmt.Sprintf("QQ空间说说 · %s (%s)", snap.AuthorName, snap.AuthorQQ)
				items[i].ParentSnippet = snap.Body
			}
		}

		// Replace @QQ with @Nickname(QQ) in contentText
		for q, name := range qqToName {
			targetAt := "@" + q
			if strings.Contains(items[i].ContentText, targetAt) {
				items[i].ContentText = strings.ReplaceAll(items[i].ContentText, targetAt, fmt.Sprintf("@%s(%s)", name, q))
			}
		}

		// Case A: Explicit Reply Quote
		if items[i].ReplyMsgID != "" {
			if replyInfo, ok := replyMsgMap[items[i].ReplyMsgID]; ok && replyInfo.RawText != "" {
				displayName := replyInfo.SenderName
				if replyInfo.SenderQQ != "" {
					displayName = fmt.Sprintf("%s (%s)", replyInfo.SenderName, replyInfo.SenderQQ)
				}
				items[i].ParentTitle = fmt.Sprintf("引用回复 %s 的发言", displayName)
				items[i].ParentSnippet = cleanCQCodeString(replyInfo.RawText)
			}
		} else if items[i].MentionedQQ != "" && items[i].GroupID != "" && items[i].OccurredAt != "" && items[i].ActionType == "message" {
			// Case B: Explicit @Mention - ONLY query the preceding message from THIS EXACT mentioned user in this group
			var prevSenderName, prevSenderQQ, prevRawText string
			err := s.Repo.DB.QueryRow(ctx, `
				SELECT 
					COALESCE(p.display_name, '成员'),
					COALESCE(pi.platform_user_id, $2),
					COALESCE(m.raw_text, '')
				FROM messages m
				JOIN conversations c ON c.id = m.conversation_id
				LEFT JOIN persons p ON p.id = m.sender_id
				LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
				WHERE c.platform_conversation_id = $1
				  AND pi.platform_user_id = $2
				  AND m.sent_at <= $3::timestamptz
				  AND m.sent_at >= $3::timestamptz - interval '15 minutes'
				ORDER BY m.sent_at DESC
				LIMIT 1`, items[i].GroupID, items[i].MentionedQQ, items[i].OccurredAt).Scan(&prevSenderName, &prevSenderQQ, &prevRawText)
			if err == nil && prevRawText != "" {
				prevDisplayName := prevSenderName
				if name, ok := qqToName[items[i].MentionedQQ]; ok && name != "" {
					prevDisplayName = name
				}
				if prevSenderQQ != "" {
					prevDisplayName = fmt.Sprintf("%s (%s)", prevDisplayName, prevSenderQQ)
				}
				items[i].ParentTitle = fmt.Sprintf("响应 @%s 的前文发言", prevDisplayName)
				items[i].ParentSnippet = cleanCQCodeString(prevRawText)
			} else {
				// No preceding message found from this specific mentioned user
				items[i].ParentSnippet = ""
			}
		} else if items[i].FeedSnapshot == nil {
			// Case C: No explicit reply and no @mention and no feed -> clear ParentSnippet
			items[i].ParentSnippet = ""
		}
	}

	return items
}

func parseRawRecordToDetailItem(id, source, endpoint, collectedAt string, payload map[string]any) EvidenceDetailItem {
	actionType := "interaction"
	actionLabel := "交互记录"
	actorName := "未知人员"
	actorQQ := ""
	targetName := ""
	targetQQ := ""
	contentText := ""
	parentTitle := ""
	parentSnippet := ""
	contextName := ""
	replyMsgID := ""
	mentionedQQ := ""
	groupID := ""
	occurredAt := collectedAt

	// 1. Group Message / DM
	if postType, _ := payload["post_type"].(string); postType == "message" || strings.Contains(endpoint, "msg") {
		actionType = "sent_message"
		actionLabel = "发送消息"
		if rawMsg, ok := payload["raw_message"].(string); ok && rawMsg != "" {
			if idx := strings.Index(rawMsg, "[CQ:reply,id="); idx != -1 {
				end := strings.Index(rawMsg[idx:], "]")
				if end != -1 {
					replyMsgID = rawMsg[idx+13 : idx+end]
					actionType = "replied_to"
					actionLabel = "引用回复"
				}
			}
			if idx := strings.Index(rawMsg, "[CQ:at,qq="); idx != -1 {
				end := strings.Index(rawMsg[idx:], "]")
				if end != -1 {
					mentionedQQ = rawMsg[idx+10 : idx+end]
					targetQQ = mentionedQQ
					if actionType != "replied_to" {
						actionType = "mentioned"
						actionLabel = "@提及"
					}
				}
			}
			contentText = cleanCQCodeString(rawMsg)
		}
		if sender, ok := payload["sender"].(map[string]any); ok {
			if n, ok := sender["nickname"].(string); ok && n != "" {
				actorName = n
			}
			if u := sender["user_id"]; u != nil {
				actorQQ = fmtNumberOrString(u)
			}
		} else if u := payload["user_id"]; u != nil {
			actorQQ = fmtNumberOrString(u)
		}
		if gName, ok := payload["group_name"].(string); ok && gName != "" {
			contextName = gName
			parentTitle = fmt.Sprintf("群聊: %s", gName)
		}
		if gID := payload["group_id"]; gID != nil {
			groupID = fmtNumberOrString(gID)
			if contextName == "" {
				contextName = fmt.Sprintf("群 %s", groupID)
				parentTitle = contextName
			}
		} else {
			if contextName == "" {
				contextName = "私聊消息"
				parentTitle = "私聊消息"
			}
		}
		if t, ok := payload["time"].(float64); ok && t > 0 {
			occurredAt = time.Unix(int64(t), 0).Format("2006-01-02T15:04:05+08:00")
		}
	} else if strings.Contains(endpoint, "comment") || payload["commentid"] != nil || payload["is_reply"] != nil {
		// 2. QZone Comment / Reply
		actionType = "commented"
		actionLabel = "说说评论"
		if payload["is_reply"] == true {
			actionType = "feed_reply"
			actionLabel = "说说回复"
		}
		if c, ok := payload["content"].(string); ok {
			contentText = strings.TrimSpace(cleanCQCodeString(c))
		}
		if n, ok := payload["name"].(string); ok && n != "" {
			actorName = n
		}
		if u := payload["uin"]; u != nil {
			actorQQ = fmtNumberOrString(u)
		}
		if authorQQ, ok := payload["_post_author_qq"].(string); ok && authorQQ != "" {
			targetQQ = authorQQ
			parentTitle = fmt.Sprintf("QQ %s 的说说", authorQQ)
		}
		if postSnippet, ok := payload["_post_snippet"].(string); ok && postSnippet != "" {
			parentSnippet = postSnippet
		}
		if t, ok := payload["createtime"].(float64); ok && t > 0 {
			occurredAt = time.Unix(int64(t), 0).Format("2006-01-02T15:04:05+08:00")
		}
	} else if strings.Contains(endpoint, "like") || payload["likeCurkey"] != nil {
		// 3. QZone Like
		actionType = "liked"
		actionLabel = "空间点赞"
		if n, ok := payload["name"].(string); ok && n != "" {
			actorName = n
		}
		if u := payload["uin"]; u != nil {
			actorQQ = fmtNumberOrString(u)
		}
		contentText = "赞了该条空间说说"
		if authorQQ, ok := payload["_post_author_qq"].(string); ok && authorQQ != "" {
			targetQQ = authorQQ
			parentTitle = fmt.Sprintf("QQ %s 的说说", authorQQ)
		}
		if t, ok := payload["createtime"].(float64); ok && t > 0 {
			occurredAt = time.Unix(int64(t), 0).Format("2006-01-02T15:04:05+08:00")
		}
	} else if strings.Contains(endpoint, "feeds") || strings.Contains(endpoint, "emotion") {
		// 4. QZone Feed Publish
		actionType = "published"
		actionLabel = "发布说说"
		if msg, ok := payload["msg"].(string); ok {
			contentText = strings.TrimSpace(cleanCQCodeString(msg))
		} else if content, ok := payload["content"].(string); ok {
			contentText = strings.TrimSpace(cleanCQCodeString(content))
		}
		if n, ok := payload["name"].(string); ok && n != "" {
			actorName = n
		}
		if u := payload["uin"]; u != nil {
			actorQQ = fmtNumberOrString(u)
		}
		if t, ok := payload["createtime"].(float64); ok && t > 0 {
			occurredAt = time.Unix(int64(t), 0).Format("2006-01-02T15:04:05+08:00")
		}
	} else {
		// Fallback
		if text, ok := payload["text"].(string); ok {
			contentText = text
		} else if body, ok := payload["body"].(string); ok {
			contentText = body
		}
	}

	var contentKey string
	if k, _ := payload["_post_content_id"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["_post_tid"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["likeCurkey"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["curkey"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["unikey"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["tid"].(string); k != "" {
		contentKey = k
	} else if k, _ := payload["cellid"].(string); k != "" {
		contentKey = k
	}

	if targetQQ == "" {
		if u, _ := payload["_post_author_qq"].(string); u != "" {
			targetQQ = u
		} else if u, _ := payload["ownerUin"].(string); u != "" {
			targetQQ = u
		} else if contentKey != "" && strings.Contains(contentKey, "qzone.qq.com/") {
			parts := strings.Split(contentKey, "qzone.qq.com/")
			if len(parts) > 1 {
				sub := strings.Split(parts[1], "/")
				if len(sub) > 0 && sub[0] != "" {
					targetQQ = sub[0]
				}
			}
		}
	}

	actorAvatar := ""
	if actorQQ != "" {
		actorAvatar = fmt.Sprintf("/api/v1/media/avatars/person/%s", actorQQ)
	}
	targetAvatar := ""
	if targetQQ != "" {
		targetAvatar = fmt.Sprintf("/api/v1/media/avatars/person/%s", targetQQ)
	}

	rawJSON, _ := json.Marshal(payload)

	return EvidenceDetailItem{
		ID:            id,
		ActionType:    actionType,
		ActionLabel:   actionLabel,
		ContextType:   source,
		ActorName:     actorName,
		ActorQQ:       actorQQ,
		ActorAvatar:   actorAvatar,
		TargetName:    targetName,
		TargetQQ:      targetQQ,
		TargetAvatar:  targetAvatar,
		ContentText:   contentText,
		ParentTitle:   parentTitle,
		ParentSnippet: parentSnippet,
		ContextName:   contextName,
		ContentKey:    contentKey,
		ReplyMsgID:    replyMsgID,
		GroupID:       groupID,
		OccurredAt:    occurredAt,
		RawSnippet:    string(rawJSON),
	}
}

func fmtNumberOrString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func cleanCQCodeString(s string) string {
	if !strings.Contains(s, "[CQ:") {
		return strings.TrimSpace(s)
	}
	res := s
	// Replace [CQ:at,qq=123] with @123
	for {
		start := strings.Index(res, "[CQ:at,qq=")
		if start == -1 {
			break
		}
		end := strings.Index(res[start:], "]")
		if end == -1 {
			break
		}
		qq := res[start+10 : start+end]
		res = res[:start] + "@" + qq + " " + res[start+end+1:]
	}
	// Remove reply tags
	for {
		start := strings.Index(res, "[CQ:reply,id=")
		if start == -1 {
			break
		}
		end := strings.Index(res[start:], "]")
		if end == -1 {
			break
		}
		res = res[:start] + res[start+end+1:]
	}
	// Remove emotion / image tags to clean readable form
	for {
		start := strings.Index(res, "[CQ:image,")
		if start == -1 {
			break
		}
		end := strings.Index(res[start:], "]")
		if end == -1 {
			break
		}
		res = res[:start] + "[图片]" + res[start+end+1:]
	}
	return strings.TrimSpace(res)
}
