package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// relationship returns a comprehensive view of how targetQQ relates to
// the person identified by {id}. It aggregates common groups, direct chat,
// QZone interactions, discovery paths, time distribution and trend.
func (s *Server) relationship(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	targetQQ := strings.TrimSpace(r.URL.Query().Get("target_qq"))
	if targetQQ == "" {
		writeJSON(w, 400, map[string]string{"error": "target_qq is required"})
		return
	}

	// Resolve target person
	var targetID string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1`, targetQQ).Scan(&targetID); err != nil {
		writeJSON(w, 404, map[string]string{"error": "target person not found"})
		return
	}

	result := map[string]any{}

	// 1. Common groups
	commonGroups := []map[string]any{}
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT DISTINCT ON (g.id) g.id::text, g.platform_group_id, g.group_name,
			gm_this.role AS this_role, gm_this.card AS this_card,
			gm_target.role AS target_role, gm_target.card AS target_card
		FROM group_memberships gm_this
		JOIN group_memberships gm_target ON gm_target.group_id = gm_this.group_id
		JOIN groups g ON g.id = gm_this.group_id
		WHERE gm_this.person_id = $1 AND gm_target.person_id = $2
		  AND (gm_this.valid_to IS NULL OR gm_this.valid_to > now())
		  AND (gm_target.valid_to IS NULL OR gm_target.valid_to > now())
		ORDER BY g.id`, personID, targetID)
	if err == nil {
		for rows.Next() {
			var gid, platformID, groupName, thisRole, thisCard, targetRole, targetCard string
			if err := rows.Scan(&gid, &platformID, &groupName, &thisRole, &thisCard, &targetRole, &targetCard); err == nil {
				commonGroups = append(commonGroups, map[string]any{
					"id": gid, "group_id": platformID, "group_name": groupName,
					"this_role": thisRole, "this_card": thisCard,
					"target_role": targetRole, "target_card": targetCard,
				})
			}
		}
		rows.Close()
	}
	result["common_groups"] = commonGroups
	result["common_group_count"] = len(commonGroups)

	// 2. Direct chat (private messages between them)
	var chatStats map[string]any
	var chatCount int64
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*), COALESCE(min(m.sent_at)::text, ''), COALESCE(max(m.sent_at)::text, '')
		FROM messages m
		WHERE m.conversation_type = 'private'
		  AND ((m.sender_id = $1 AND m.conversation_id IN (SELECT id FROM conversations WHERE conversation_type='private' AND platform_conversation_id=$2))
		    OR (m.sender_id = $2 AND m.conversation_id IN (SELECT id FROM conversations WHERE conversation_type='private' AND platform_conversation_id=$1)))`,
		personID, targetID).Scan(&chatCount, &chatStats, &chatStats)
	// Simpler approach: count messages where sender is one and conversation involves the other
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*), COALESCE(min(m.sent_at)::text,''), COALESCE(max(m.sent_at)::text,'')
		FROM messages m
		JOIN conversations c ON c.id=m.conversation_id
		WHERE c.conversation_type='private'
		  AND ((m.sender_id=$1 AND c.platform_conversation_id=$2)
		    OR (m.sender_id=$2 AND c.platform_conversation_id=$1))`,
		personID, targetID).Scan(&chatCount, &chatStats, &chatStats)
	result["direct_chat_count"] = chatCount

	// 3. QZone interactions (likes, comments on each other's posts)
	var likeCount, commentCount int64
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE re.action_type='liked' AND re.actor_person_id=$1
		  AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2)`,
		personID, targetID).Scan(&likeCount)
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE re.action_type='commented' AND re.actor_person_id=$1
		  AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2)`,
		personID, targetID).Scan(&commentCount)

	// Reverse direction (target -> this person)
	var reverseLikeCount, reverseCommentCount int64
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE re.action_type='liked' AND re.actor_person_id=$2
		  AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1)`,
		personID, targetID).Scan(&reverseLikeCount)
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE re.action_type='commented' AND re.actor_person_id=$2
		  AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1)`,
		personID, targetID).Scan(&reverseCommentCount)

	result["qzone_interactions"] = map[string]any{
		"this_liked_target_posts":    likeCount,
		"this_commented_target_posts": commentCount,
		"target_liked_this_posts":    reverseLikeCount,
		"target_commented_this_posts": reverseCommentCount,
	}

	// 4. Visits
	var visitCount int64
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE re.action_type='visited' AND re.actor_person_id=$1 AND re.target_person_id=$2`,
		personID, targetID).Scan(&visitCount)
	result["visit_count"] = visitCount

	// 5. All interaction events between these two people
	var totalInteractions int64
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE (re.actor_person_id=$1 AND re.target_person_id=$2)
		   OR (re.actor_person_id=$2 AND re.target_person_id=$1)
		   OR (re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2))
		   OR (re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1))`,
		personID, targetID).Scan(&totalInteractions)
	result["total_interactions"] = totalInteractions

	// 6. First and last interaction
	var firstAt, lastAt interface{}
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT min(re.occurred_at), max(re.occurred_at) FROM relation_events re
		WHERE (re.actor_person_id=$1 AND re.target_person_id=$2)
		   OR (re.actor_person_id=$2 AND re.target_person_id=$1)
		   OR (re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2))
		   OR (re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1))`,
		personID, targetID).Scan(&firstAt, &lastAt)
	result["first_interaction"] = firstAt
	result["last_interaction"] = lastAt

	// 7. Time distribution (by month)
	timeDist := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), `
		SELECT to_char(date_trunc('month', re.occurred_at), 'YYYY-MM') AS month, count(*) AS count
		FROM relation_events re
		WHERE (re.actor_person_id=$1 AND re.target_person_id=$2)
		   OR (re.actor_person_id=$2 AND re.target_person_id=$1)
		   OR (re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2))
		   OR (re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1))
		GROUP BY 1 ORDER BY 1`, personID, targetID)
	if err == nil {
		for rows.Next() {
			var month string
			var count int64
			if err := rows.Scan(&month, &count); err == nil {
				timeDist = append(timeDist, map[string]any{"month": month, "count": count})
			}
		}
		rows.Close()
	}
	result["time_distribution"] = timeDist

	// 8. Trend: recent 30 days vs previous 30 days
	var recent30, prev30 int64
	now := time.Now()
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE ((re.actor_person_id=$1 AND re.target_person_id=$2)
		   OR (re.actor_person_id=$2 AND re.target_person_id=$1)
		   OR (re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2))
		   OR (re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1)))
		  AND re.occurred_at >= $3`, personID, targetID, now.AddDate(0, 0, -30)).Scan(&recent30)
	s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM relation_events re
		WHERE ((re.actor_person_id=$1 AND re.target_person_id=$2)
		   OR (re.actor_person_id=$2 AND re.target_person_id=$1)
		   OR (re.actor_person_id=$1 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$2))
		   OR (re.actor_person_id=$2 AND re.target_object_id IN (SELECT id FROM contents WHERE author_id=$1)))
		  AND re.occurred_at >= $3 AND re.occurred_at < $4`, personID, targetID, now.AddDate(0, 0, -60), now.AddDate(0, 0, -30)).Scan(&prev30)

	trend := "stable"
	if recent30 == 0 && prev30 == 0 {
		trend = "none"
	} else if recent30 > prev30*2 {
		trend = "increasing"
	} else if prev30 > recent30*2 {
		trend = "decreasing"
	} else if recent30 > 0 && prev30 == 0 {
		trend = "new"
	}
	result["trend"] = trend
	result["recent_30_days"] = recent30
	result["previous_30_days"] = prev30

	// 9. Common contacts
	commonContacts := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), `
		SELECT DISTINCT p.id::text, p.display_name, pi.platform_user_id
		FROM relation_events re1
		JOIN relation_events re2 ON re2.target_person_id = re1.target_person_id
		                            AND re2.actor_person_id != re1.actor_person_id
		JOIN persons p ON p.id = re1.target_person_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform='qq'
		WHERE re1.actor_person_id IN ($1::uuid, $2::uuid)
		  AND re2.actor_person_id IN ($1::uuid, $2::uuid)
		  AND re1.actor_person_id != re2.actor_person_id
		  AND re1.target_person_id NOT IN ($1::uuid, $2::uuid)
		LIMIT 50`, personID, targetID)
	if err == nil {
		for rows.Next() {
			var id, name, qq string
			if err := rows.Scan(&id, &name, &qq); err == nil {
				commonContacts = append(commonContacts, map[string]any{"id": id, "name": name, "qq": qq})
			}
		}
		rows.Close()
	}
	result["common_contacts"] = commonContacts
	result["common_contact_count"] = len(commonContacts)

	writeJSON(w, 200, map[string]any{"data": result})
}

// timeline returns all events for a person in chronological order.
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	limit, offset := pageParams(r)
	if limit > 500 {
		limit = 500
	}
	eventType := r.URL.Query().Get("type")

	// Build UNION of all event sources for this person
	query := `
		SELECT id, event_type, occurred_at, details FROM (
			SELECT re.id::text, re.action_type AS event_type, re.occurred_at,
			       jsonb_build_object('context_type', re.context_type, 'actor_id', re.actor_person_id::text,
			                          'target_person_id', re.target_person_id::text, 'evidence_ids', re.evidence_ids) AS details
			FROM relation_events re
			WHERE re.actor_person_id = $1 OR re.target_person_id = $1
			UNION ALL
			SELECT m.id::text, 'sent_message' AS event_type, m.sent_at AS occurred_at,
			       jsonb_build_object('conversation_id', m.conversation_id::text, 'raw_text', LEFT(m.raw_text, 200), 'source_message_id', m.source_message_id) AS details
			FROM messages m WHERE m.sender_id = $1
			UNION ALL
			SELECT ('profile:' || pp.id::text) AS id, 'profile_change' AS event_type, pp.valid_at AS occurred_at,
			       jsonb_build_object('nickname', pp.nickname, 'source', pp.source, 'avatar_uri', LEFT(pp.avatar_uri, 80)) AS details
			FROM (SELECT id, person_id, nickname, source, avatar_uri, valid_from AS valid_at FROM person_profiles) pp
			WHERE pp.person_id = $1
			UNION ALL
			SELECT ('membership:' || gm.id::text) AS id, 'group_membership' AS event_type, gm.valid_from AS occurred_at,
			       jsonb_build_object('group_id', gm.group_id::text, 'role', gm.role) AS details
			FROM group_memberships gm WHERE gm.person_id = $1
		) events`
	args := []interface{}{personID}
	argIdx := 2
	if eventType != "" {
		query += " WHERE event_type = $" + strconv.Itoa(argIdx)
		args = append(args, eventType)
		argIdx++
	}
	query += " ORDER BY occurred_at DESC NULLS LAST LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.Repo.DB.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, etype string
		var occurredAt interface{}
		var details []byte
		if err := rows.Scan(&id, &etype, &occurredAt, &details); err != nil {
			writeError(w, 500, err)
			return
		}
		var detailValue any
		_ = json.Unmarshal(details, &detailValue)
		data = append(data, map[string]any{"id": id, "event_type": etype, "occurred_at": occurredAt, "details": detailValue})
	}

	// Count total
	var total int64
	countQuery := `SELECT count(*) FROM (
		SELECT 1 FROM relation_events re WHERE re.actor_person_id=$1 OR re.target_person_id=$1
		UNION ALL SELECT 1 FROM messages m WHERE m.sender_id=$1
		UNION ALL SELECT 1 FROM person_profiles pp WHERE pp.person_id=$1
		UNION ALL SELECT 1 FROM group_memberships gm WHERE gm.person_id=$1
	) t`
	if eventType != "" {
		countQuery = `SELECT count(*) FROM (
			SELECT re.action_type AS et, 1 FROM relation_events re WHERE (re.actor_person_id=$1 OR re.target_person_id=$1) AND re.action_type=$2
			UNION ALL SELECT 'sent_message', 1 FROM messages m WHERE m.sender_id=$1 AND 'sent_message'=$2
		) t`
		s.Repo.DB.QueryRow(r.Context(), countQuery, personID, eventType).Scan(&total)
	} else {
		s.Repo.DB.QueryRow(r.Context(), countQuery, personID).Scan(&total)
	}

	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}

// contentLikes returns the like list for a specific content item.
func (s *Server) contentLikes(w http.ResponseWriter, r *http.Request) {
	contentID := chi.URLParam(r, "id")
	limit, offset := pageParams(r)
	var total int64
	s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM relation_events re WHERE re.action_type='liked' AND re.target_object_id=$1`, contentID).Scan(&total)
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT re.id::text, re.actor_person_id::text, p.display_name,
		       pi.platform_user_id, re.occurred_at, re.evidence_ids
		FROM relation_events re
		LEFT JOIN persons p ON p.id=re.actor_person_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE re.action_type='liked' AND re.target_object_id=$1
		ORDER BY re.occurred_at DESC LIMIT $2 OFFSET $3`, contentID, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, actorID, name, qq string
		var occurredAt interface{}
		var evidence []string
		if err := rows.Scan(&id, &actorID, &name, &qq, &occurredAt, &evidence); err != nil {
			writeError(w, 500, err)
			return
		}
		avatar := ""
		_ = s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
			(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
			(SELECT '/api/v1/media/avatars/person/'||pi.platform_user_id FROM person_identifiers pi WHERE pi.person_id=p.id AND pi.platform='qq' AND pi.platform_user_id<>'' LIMIT 1),
			'')
			FROM persons p WHERE p.id=$1`, actorID).Scan(&avatar)
		data = append(data, map[string]any{"id": id, "person_id": actorID, "name": name, "qq": qq, "avatar_uri": avatar, "occurred_at": occurredAt, "evidence_ids": evidence})
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}
