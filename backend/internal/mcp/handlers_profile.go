package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (h *HandlerRegistry) handlePersonDossier(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		QQ       string   `json:"qq"`
		Sections []string `json:"sections"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(fmt.Sprintf("failed to parse args: %v", err)), nil
	}
	if input.QQ == "" {
		return errorResult("qq is required"), nil
	}
	if len(input.Sections) == 0 {
		input.Sections = []string{"all"}
	}

	hasSection := func(s string) bool {
		for _, sec := range input.Sections {
			if sec == "all" || sec == s {
				return true
			}
		}
		return false
	}

	personID, err := resolvePersonID(ctx, h.DB, input.QQ)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to resolve person: %v", err)), nil
	}

	result := make(map[string]any)

	// Identity Section
	if hasSection("identity") {
		var identity struct {
			ID               string     `json:"id"`
			DisplayName      *string    `json:"display_name"`
			PlatformUserID   string     `json:"platform_user_id"`
			FirstSeenAt      *time.Time `json:"first_seen_at"`
			LastSeenAt       *time.Time `json:"last_seen_at"`
			ObservationCount int        `json:"observation_count"`
			LatestSnapshot   any        `json:"latest_snapshot,omitempty"`
		}

		err = h.DB.QueryRow(ctx, `
			SELECT p.id, p.display_name, pi.platform_user_id, p.first_seen_at, p.last_seen_at,
			  (SELECT COUNT(*) FROM profile_observations po WHERE po.person_id = p.id)
			FROM persons p
			JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE pi.platform_user_id = $1
		`, input.QQ).Scan(
			&identity.ID,
			&identity.DisplayName,
			&identity.PlatformUserID,
			&identity.FirstSeenAt,
			&identity.LastSeenAt,
			&identity.ObservationCount,
		)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to fetch identity: %v", err)), nil
		}

		var payload []byte
		err = h.DB.QueryRow(ctx, `
			SELECT payload FROM profile_observations
			WHERE person_id = $1
			ORDER BY observed_at DESC LIMIT 1
		`, personID).Scan(&payload)
		if err == nil && len(payload) > 0 {
			var parsed map[string]any
			if json.Unmarshal(payload, &parsed) == nil {
				identity.LatestSnapshot = parsed
			}
		}

		result["identity"] = identity
	}

	// Behavioral Section
	if hasSection("behavioral") {
		behavioral := make(map[string]any)

		// Heatmap
		rows, err := h.DB.Query(ctx, `
			SELECT EXTRACT(HOUR FROM occurred_at)::int as hour, COUNT(*) as count
			FROM relation_events
			WHERE actor_person_id = $1
			GROUP BY hour ORDER BY hour
		`, personID)
		if err == nil {
			heatmap := make(map[int]int)
			for rows.Next() {
				var hour, count int
				if rows.Scan(&hour, &count) == nil {
					heatmap[hour] = count
				}
			}
			rows.Close()
			behavioral["activity_hours_heatmap"] = heatmap
		}

		// Posting frequency
		var dailyAvg *float64
		h.DB.QueryRow(ctx, `
			SELECT COUNT(*)::float / GREATEST(1, EXTRACT(DAY FROM now() - MIN(occurred_at)))::float as daily_avg
			FROM relation_events
			WHERE actor_person_id = $1 AND action_type = 'published'
			  AND occurred_at >= now() - interval '90 days'
		`, personID).Scan(&dailyAvg)
		if dailyAvg != nil {
			behavioral["posting_frequency_daily_avg"] = *dailyAvg
		} else {
			behavioral["posting_frequency_daily_avg"] = 0
		}

		// Peak activity
		var peakDow *int
		var peakCount int
		h.DB.QueryRow(ctx, `
			SELECT EXTRACT(DOW FROM occurred_at)::int as dow, COUNT(*) as count
			FROM relation_events WHERE actor_person_id = $1
			GROUP BY dow ORDER BY count DESC LIMIT 1
		`, personID).Scan(&peakDow, &peakCount)

		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		if peakDow != nil && *peakDow >= 0 && *peakDow < 7 {
			behavioral["peak_activity_day"] = map[string]any{
				"day":   days[*peakDow],
				"count": peakCount,
			}
		}

		result["behavioral"] = behavioral
	}

	// Social Section
	if hasSection("social") {
		social := make(map[string]any)

		var summary struct {
			LikesGiven       int `json:"likes_given"`
			LikesReceived    int `json:"likes_received"`
			CommentsGiven    int `json:"comments_given"`
			CommentsReceived int `json:"comments_received"`
			MessagesSent     int `json:"messages_sent"`
			MessagesReceived int `json:"messages_received"`
		}
		err = h.DB.QueryRow(ctx, `
			SELECT
			  COUNT(CASE WHEN action_type='liked' AND actor_person_id=$1 THEN 1 END) as likes_given,
			  COUNT(CASE WHEN action_type='liked' AND target_person_id=$1 THEN 1 END) as likes_received,
			  COUNT(CASE WHEN action_type='commented' AND actor_person_id=$1 THEN 1 END) as comments_given,
			  COUNT(CASE WHEN action_type='commented' AND target_person_id=$1 THEN 1 END) as comments_received,
			  COUNT(CASE WHEN action_type='sent_message' AND actor_person_id=$1 THEN 1 END) as messages_sent,
			  COUNT(CASE WHEN action_type='sent_message' AND target_person_id=$1 THEN 1 END) as messages_received
			FROM relation_events
			WHERE actor_person_id=$1 OR target_person_id=$1
		`, personID).Scan(
			&summary.LikesGiven, &summary.LikesReceived,
			&summary.CommentsGiven, &summary.CommentsReceived,
			&summary.MessagesSent, &summary.MessagesReceived,
		)
		if err == nil {
			social["interaction_summary"] = summary
		}

		type interactor struct {
			PersonID       string   `json:"person_id"`
			PlatformUserID *string  `json:"platform_user_id"`
			DisplayName    *string  `json:"display_name"`
			Score          int      `json:"score"`
			Types          []string `json:"types"`
		}
		var interactors []interactor
		rows, err := h.DB.Query(ctx, `
			SELECT counterparty, pi.platform_user_id, p.display_name, COUNT(*) as score,
			  array_agg(DISTINCT action_type) as types
			FROM (
			  SELECT CASE WHEN actor_person_id=$1 THEN target_person_id ELSE actor_person_id END as counterparty,
				action_type
			  FROM relation_events
			  WHERE (actor_person_id=$1 OR target_person_id=$1)
				AND actor_person_id IS NOT NULL AND target_person_id IS NOT NULL
			) sub
			JOIN persons p ON p.id = sub.counterparty
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform='qq'
			GROUP BY counterparty, pi.platform_user_id, p.display_name
			ORDER BY score DESC LIMIT 10
		`, personID)
		if err == nil {
			for rows.Next() {
				var i interactor
				if rows.Scan(&i.PersonID, &i.PlatformUserID, &i.DisplayName, &i.Score, &i.Types) == nil {
					interactors = append(interactors, i)
				}
			}
			rows.Close()
			social["top_interactors"] = interactors
		}

		type groupInfo struct {
			PlatformGroupID string  `json:"platform_group_id"`
			GroupName       *string `json:"group_name"`
			Card            string  `json:"card"`
			Role            *string `json:"role"`
		}
		var groups []groupInfo
		rows, err = h.DB.Query(ctx, `
			SELECT g.platform_group_id, g.group_name, COALESCE(gm.card,''), gm.role
			FROM group_memberships gm
			JOIN "groups" g ON g.id = gm.group_id
			WHERE gm.person_id = $1
			ORDER BY gm.valid_from DESC LIMIT 20
		`, personID)
		if err == nil {
			for rows.Next() {
				var g groupInfo
				if rows.Scan(&g.PlatformGroupID, &g.GroupName, &g.Card, &g.Role) == nil {
					groups = append(groups, g)
				}
			}
			rows.Close()
			social["groups"] = groups
			social["group_count"] = len(groups)
		}

		result["social"] = social
	}

	// Coverage Section
	if hasSection("coverage") {
		coverage := make(map[string]any)

		allSources := []string{
			"avatar", "basic_profile", "group_memberships", "group_messages",
			"media", "private_messages", "qzone_comments", "qzone_feeds",
			"qzone_likes", "qzone_visits",
		}

		type covRecord struct {
			DataSource      string     `json:"data_source"`
			Status          *string    `json:"status"`
			ItemsCollected  int        `json:"items_collected"`
			LastCollectedAt *time.Time `json:"last_collected_at"`
		}
		var collected []covRecord
		var collectedCount int
		coverageIndex := make(map[string]int)

		rows, err := h.DB.Query(ctx, `
			SELECT data_source, status, items_collected, last_collected_at
			FROM collection_coverage
			WHERE person_id = $1
			ORDER BY data_source
		`, personID)
		if err == nil {
			for rows.Next() {
				var c covRecord
				if rows.Scan(&c.DataSource, &c.Status, &c.ItemsCollected, &c.LastCollectedAt) == nil {
					collected = append(collected, c)
					coverageIndex[c.DataSource] = len(collected) - 1
					if c.Status != nil && (*c.Status == "complete" || *c.Status == "partial" || *c.Status == "no_results") {
						collectedCount++
					}
				}
			}
			rows.Close()
		}

		// Existing normalized data is useful evidence, but it does not prove that
		// a source was fully scanned. Expose it as inferred instead of calling it
		// complete or hiding it as not_collected.
		observed := observedCoverageCounts(ctx, h.DB, personID)
		unverified := []string{}
		for _, source := range allSources {
			count := observed[source]
			if count <= 0 {
				continue
			}
			index, exists := coverageIndex[source]
			if exists {
				status := ""
				if collected[index].Status != nil {
					status = *collected[index].Status
				}
				if status != "not_collected" && status != "failed" {
					continue
				}
				inferred := "inferred"
				collected[index].Status = &inferred
				collected[index].ItemsCollected = count
			} else {
				inferred := "inferred"
				collected = append(collected, covRecord{DataSource: source, Status: &inferred, ItemsCollected: count})
				coverageIndex[source] = len(collected) - 1
			}
			unverified = append(unverified, source)
		}

		var missing []string
		for _, s := range allSources {
			index, ok := coverageIndex[s]
			if !ok || collected[index].Status == nil || (*collected[index].Status != "complete" && *collected[index].Status != "partial" && *collected[index].Status != "no_results" && *collected[index].Status != "inferred") {
				missing = append(missing, s)
			}
		}

		coverage["collected_sources"] = collected
		coverage["missing_sources"] = missing
		confirmed := []string{}
		for _, source := range allSources {
			if index, ok := coverageIndex[source]; ok && collected[index].Status != nil {
				status := *collected[index].Status
				if status == "complete" || status == "partial" || status == "no_results" {
					confirmed = append(confirmed, source)
				}
			}
		}
		coverage["confirmed_sources"] = confirmed
		coverage["unverified_sources"] = unverified
		coverage["confirmed_coverage_percentage"] = float64(collectedCount) / float64(len(allSources)) * 100.0
		coverage["coverage_percentage"] = float64(len(allSources)-len(missing)) / float64(len(allSources)) * 100.0

		result["coverage"] = coverage
	}

	// Evolution Section
	if hasSection("evolution") {
		evolution := make(map[string]any)

		type profileRecord struct {
			Nickname      *string    `json:"nickname"`
			AvatarURI     *string    `json:"avatar_uri"`
			Signature     *string    `json:"signature"`
			ValidFrom     *time.Time `json:"valid_from"`
			VersionNumber *int       `json:"version_number"`
		}
		var records []profileRecord

		rows, err := h.DB.Query(ctx, `
			SELECT nickname, avatar_uri, signature, valid_from, version_number
			FROM person_profiles
			WHERE person_id = $1
			ORDER BY valid_from ASC
		`, personID)
		if err == nil {
			for rows.Next() {
				var p profileRecord
				if rows.Scan(&p.Nickname, &p.AvatarURI, &p.Signature, &p.ValidFrom, &p.VersionNumber) == nil {
					records = append(records, p)
				}
			}
			rows.Close()
		}

		type change struct {
			From      *string    `json:"from"`
			To        *string    `json:"to"`
			ChangedAt *time.Time `json:"changed_at"`
		}
		var nicknameChanges, avatarChanges, signatureChanges []change

		var lastNick, lastAv, lastSig *string
		for i, r := range records {
			if i > 0 {
				if lastNick != nil && r.Nickname != nil && *lastNick != *r.Nickname {
					nicknameChanges = append(nicknameChanges, change{From: lastNick, To: r.Nickname, ChangedAt: r.ValidFrom})
				}
				if lastAv != nil && r.AvatarURI != nil && *lastAv != *r.AvatarURI {
					avatarChanges = append(avatarChanges, change{From: lastAv, To: r.AvatarURI, ChangedAt: r.ValidFrom})
				}
				if lastSig != nil && r.Signature != nil && *lastSig != *r.Signature {
					signatureChanges = append(signatureChanges, change{From: lastSig, To: r.Signature, ChangedAt: r.ValidFrom})
				}
			}
			if r.Nickname != nil {
				lastNick = r.Nickname
			}
			if r.AvatarURI != nil {
				lastAv = r.AvatarURI
			}
			if r.Signature != nil {
				lastSig = r.Signature
			}
		}

		evolution["nickname_changes"] = nicknameChanges
		evolution["avatar_changes"] = avatarChanges
		evolution["signature_changes"] = signatureChanges

		result["evolution"] = evolution
	}

	return jsonResult(result)
}

func (h *HandlerRegistry) handleGroupIntel(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		GroupID              string   `json:"group_id"`
		IncludeRoster        bool     `json:"include_roster"`
		CompareWithGroups    []string `json:"compare_with_groups"`
		TopContributorsLimit int      `json:"top_contributors_limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(fmt.Sprintf("failed to parse args: %v", err)), nil
	}
	if input.GroupID == "" {
		return errorResult("group_id is required"), nil
	}
	if input.TopContributorsLimit <= 0 {
		input.TopContributorsLimit = 20
	}
	if !strings.Contains(string(args), "include_roster") {
		input.IncludeRoster = true
	}

	var group struct {
		ID              string     `json:"id"`
		PlatformGroupID string     `json:"platform_group_id"`
		GroupName       *string    `json:"group_name"`
		FirstSeenAt     *time.Time `json:"first_seen_at"`
		MemberCount     int        `json:"member_count"`
	}

	err := h.DB.QueryRow(ctx, `
		SELECT id, platform_group_id, group_name, first_seen_at,
		  (SELECT COUNT(*) FROM group_memberships WHERE group_id = g.id)
		FROM "groups" g
		WHERE platform = 'qq' AND platform_group_id = $1
	`, input.GroupID).Scan(&group.ID, &group.PlatformGroupID, &group.GroupName, &group.FirstSeenAt, &group.MemberCount)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to find group: %v", err)), nil
	}

	result := map[string]any{
		"group": group,
	}

	if input.IncludeRoster {
		type member struct {
			PersonID       string     `json:"person_id"`
			PlatformUserID *string    `json:"platform_user_id"`
			DisplayName    *string    `json:"display_name"`
			Role           *string    `json:"role"`
			Card           *string    `json:"card"`
			ValidFrom      *time.Time `json:"valid_from"`
		}
		var roster []member
		rows, err := h.DB.Query(ctx, `
			SELECT gm.person_id, pi.platform_user_id, p.display_name, gm.role, gm.card, gm.valid_from
			FROM group_memberships gm
			JOIN persons p ON p.id = gm.person_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE gm.group_id = $1
			ORDER BY gm.valid_from ASC
		`, group.ID)
		if err == nil {
			for rows.Next() {
				var m member
				if rows.Scan(&m.PersonID, &m.PlatformUserID, &m.DisplayName, &m.Role, &m.Card, &m.ValidFrom) == nil {
					roster = append(roster, m)
				}
			}
			rows.Close()
			result["roster"] = roster
		}
	}

	type contributor struct {
		PersonID       string  `json:"person_id"`
		PlatformUserID *string `json:"platform_user_id"`
		DisplayName    *string `json:"display_name"`
		MessageCount   int     `json:"message_count"`
	}
	var contributors []contributor
	rows, err := h.DB.Query(ctx, `
		SELECT m.sender_id, pi.platform_user_id, p.display_name, COUNT(*) as count
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE c.platform_conversation_id = $1 AND c.conversation_type = 'group'
		GROUP BY m.sender_id, pi.platform_user_id, p.display_name
		ORDER BY count DESC LIMIT $2
	`, input.GroupID, input.TopContributorsLimit)
	if err == nil {
		for rows.Next() {
			var c contributor
			if rows.Scan(&c.PersonID, &c.PlatformUserID, &c.DisplayName, &c.MessageCount) == nil {
				contributors = append(contributors, c)
			}
		}
		rows.Close()
		result["top_contributors"] = contributors
	}

	if len(input.CompareWithGroups) > 0 {
		comparisons := make(map[string]any)
		for _, compGroupID := range input.CompareWithGroups {
			var compID string
			var compName *string
			err := h.DB.QueryRow(ctx, `SELECT id, group_name FROM "groups" WHERE platform='qq' AND platform_group_id=$1`, compGroupID).Scan(&compID, &compName)
			if err != nil {
				continue
			}

			var intersection []string
			rows, err := h.DB.Query(ctx, `
				SELECT p.id
				FROM group_memberships gm1
				JOIN group_memberships gm2 ON gm1.person_id = gm2.person_id
				JOIN persons p ON p.id = gm1.person_id
				WHERE gm1.group_id = $1 AND gm2.group_id = $2
			`, group.ID, compID)
			if err == nil {
				for rows.Next() {
					var pid string
					if rows.Scan(&pid) == nil {
						intersection = append(intersection, pid)
					}
				}
				rows.Close()
			}

			comparisons[compGroupID] = map[string]any{
				"group_name":         compName,
				"intersection_count": len(intersection),
				"intersection_ids":   intersection,
			}
		}
		result["comparisons"] = comparisons
	}

	return jsonResult(result)
}

func (h *HandlerRegistry) handleProfileEvolution(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		QQ                  string `json:"qq"`
		IncludeRawSnapshots bool   `json:"include_raw_snapshots"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(fmt.Sprintf("failed to parse args: %v", err)), nil
	}
	if input.QQ == "" {
		return errorResult("qq is required"), nil
	}

	personID, err := resolvePersonID(ctx, h.DB, input.QQ)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to resolve person: %v", err)), nil
	}

	type profileRecord struct {
		ID            string     `json:"id"`
		Nickname      *string    `json:"nickname"`
		AvatarURI     *string    `json:"avatar_uri"`
		Signature     *string    `json:"signature"`
		ValidFrom     *time.Time `json:"valid_from"`
		VersionNumber *int       `json:"version_number"`
	}
	var records []profileRecord

	rows, err := h.DB.Query(ctx, `
		SELECT id, nickname, avatar_uri, signature, valid_from, version_number
		FROM person_profiles
		WHERE person_id = $1
		ORDER BY valid_from ASC
	`, personID)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to query profiles: %v", err)), nil
	}
	for rows.Next() {
		var p profileRecord
		if rows.Scan(&p.ID, &p.Nickname, &p.AvatarURI, &p.Signature, &p.ValidFrom, &p.VersionNumber) == nil {
			records = append(records, p)
		}
	}
	rows.Close()

	type change struct {
		From      *string    `json:"from"`
		To        *string    `json:"to"`
		ChangedAt *time.Time `json:"changed_at"`
	}
	var nicknameChanges, avatarChanges, signatureChanges []change

	var lastNick, lastAv, lastSig *string
	for i, r := range records {
		if i > 0 {
			if lastNick != nil && r.Nickname != nil && *lastNick != *r.Nickname {
				nicknameChanges = append(nicknameChanges, change{From: lastNick, To: r.Nickname, ChangedAt: r.ValidFrom})
			}
			if lastAv != nil && r.AvatarURI != nil && *lastAv != *r.AvatarURI {
				avatarChanges = append(avatarChanges, change{From: lastAv, To: r.AvatarURI, ChangedAt: r.ValidFrom})
			}
			if lastSig != nil && r.Signature != nil && *lastSig != *r.Signature {
				signatureChanges = append(signatureChanges, change{From: lastSig, To: r.Signature, ChangedAt: r.ValidFrom})
			}
		}
		if r.Nickname != nil {
			lastNick = r.Nickname
		}
		if r.AvatarURI != nil {
			lastAv = r.AvatarURI
		}
		if r.Signature != nil {
			lastSig = r.Signature
		}
	}

	result := map[string]any{
		"total_versions":    len(records),
		"nickname_changes":  nicknameChanges,
		"avatar_changes":    avatarChanges,
		"signature_changes": signatureChanges,
	}

	if input.IncludeRawSnapshots {
		var snapshots []any
		rows, err := h.DB.Query(ctx, `
			SELECT payload
			FROM profile_observations
			WHERE person_id = $1
			ORDER BY observed_at ASC
		`, personID)
		if err == nil {
			for rows.Next() {
				var payload []byte
				if rows.Scan(&payload) == nil {
					var parsed map[string]any
					if json.Unmarshal(payload, &parsed) == nil {
						snapshots = append(snapshots, parsed)
					}
				}
			}
			rows.Close()
			result["raw_snapshots"] = snapshots
		}
	}

	return jsonResult(result)
}
