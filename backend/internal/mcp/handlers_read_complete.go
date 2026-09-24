package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (h *HandlerRegistry) resolveGroupKey(ctx context.Context, key string) (id, platformID string, err error) {
	err = h.DB.QueryRow(ctx, `SELECT id::text, platform_group_id FROM "groups" WHERE id::text=$1 OR (platform='qq' AND platform_group_id=$1) ORDER BY CASE WHEN id::text=$1 THEN 0 ELSE 1 END LIMIT 1`, key).Scan(&id, &platformID)
	return id, platformID, err
}

func (h *HandlerRegistry) handlePersonDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		QQ               string   `json:"qq"`
		Sections         []string `json:"sections"`
		ProfileLimit     int      `json:"profile_limit"`
		ProfileOffset    int      `json:"profile_offset"`
		MembershipLimit  int      `json:"membership_limit"`
		MembershipOffset int      `json:"membership_offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	personID, err := resolvePersonID(ctx, h.DB, input.QQ)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	sections := map[string]bool{}
	for _, s := range input.Sections {
		sections[s] = true
	}
	include := func(s string) bool { return len(sections) == 0 || sections["all"] || sections[s] }
	var id, name string
	var firstSeen, lastSeen *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT id::text, display_name, first_seen_at, last_seen_at FROM persons WHERE id=$1`, personID).Scan(&id, &name, &firstSeen, &lastSeen); err != nil {
		return errorResult(err.Error()), nil
	}
	result := map[string]any{"id": id, "qq": input.QQ, "display_name": name, "first_seen_at": firstSeen, "last_seen_at": lastSeen}
	if include("profiles") {
		limit, offset := pagingDefaults(input.ProfileLimit, input.ProfileOffset, 200)
		var total int
		_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM person_profiles WHERE person_id=$1`, personID).Scan(&total)
		rows, err := h.DB.Query(ctx, `SELECT id::text,COALESCE(nickname,''),COALESCE(avatar_uri,''),COALESCE(card_or_remark,''),COALESCE(source,''),valid_from,valid_to,COALESCE(sex,''),age,COALESCE(area,''),COALESCE(signature,''),version_number,observation_count FROM person_profiles WHERE person_id=$1 ORDER BY valid_from DESC LIMIT $2 OFFSET $3`, personID, limit, offset)
		if err == nil {
			defer rows.Close()
			items := []map[string]any{}
			for rows.Next() {
				var pid, nick, av, card, source, sex, area, sig string
				var validFrom, validTo *time.Time
				var age, ver *int
				var obs int64
				if rows.Scan(&pid, &nick, &av, &card, &source, &validFrom, &validTo, &sex, &age, &area, &sig, &ver, &obs) == nil {
					items = append(items, map[string]any{"id": pid, "nickname": nick, "avatar_uri": av, "card_or_remark": card, "source": source, "valid_from": validFrom, "valid_to": validTo, "sex": sex, "age": age, "area": area, "signature": sig, "version_number": ver, "observation_count": obs})
				}
			}
			result["profiles"] = items
			result["profiles_page"] = pagedResult(items, limit, offset, total)
		}
	}
	if include("memberships") {
		limit, offset := pagingDefaults(input.MembershipLimit, input.MembershipOffset, 200)
		var total int
		_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM group_memberships WHERE person_id=$1`, personID).Scan(&total)
		rows, err := h.DB.Query(ctx, `SELECT g.id::text,g.platform_group_id,g.group_name,gm.role,gm.card,gm.valid_from,gm.valid_to FROM group_memberships gm JOIN "groups" g ON g.id=gm.group_id WHERE gm.person_id=$1 ORDER BY gm.valid_from DESC LIMIT $2 OFFSET $3`, personID, limit, offset)
		if err == nil {
			defer rows.Close()
			items := []map[string]any{}
			for rows.Next() {
				var gid, gpid, gname, role, card string
				var from, to *time.Time
				if rows.Scan(&gid, &gpid, &gname, &role, &card, &from, &to) == nil {
					items = append(items, map[string]any{"group_id": gpid, "group_uuid": gid, "group_name": gname, "role": role, "card": card, "valid_from": from, "valid_to": to})
				}
			}
			result["memberships"] = items
			result["memberships_page"] = pagedResult(items, limit, offset, total)
		}
	}
	if include("stats") {
		var profiles, members, messages, relations, contents int
		_ = h.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM person_profiles WHERE person_id=$1),(SELECT count(*) FROM group_memberships WHERE person_id=$1),(SELECT count(*) FROM messages WHERE sender_id=$1),(SELECT count(*) FROM relation_events WHERE actor_person_id=$1 OR target_person_id=$1),(SELECT count(*) FROM contents WHERE author_id=$1)`, personID).Scan(&profiles, &members, &messages, &relations, &contents)
		result["stats"] = map[string]int{"profiles": profiles, "memberships": members, "messages": messages, "relation_events": relations, "contents": contents}
	}
	return jsonResult(result)
}

func (h *HandlerRegistry) handlePersonTimeline(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		QQ string `json:"qq"`

		EventType string `json:"event_type"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	personID, err := resolvePersonID(ctx, h.DB, input.QQ)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)
	where := ""
	var params []any
	idx := 2
	if input.EventType != "" {
		where = " WHERE event_type=$2"
		params = append(params, input.EventType)
		idx++
	}
	countQuery := `SELECT count(*) FROM (SELECT re.action_type AS event_type FROM relation_events re WHERE re.actor_person_id=$1 OR re.target_person_id=$1 UNION ALL SELECT 'sent_message' FROM messages m WHERE m.sender_id=$1 UNION ALL SELECT 'profile_change' FROM person_profiles pp WHERE pp.person_id=$1 UNION ALL SELECT 'group_membership' FROM group_memberships gm WHERE gm.person_id=$1) t` + where
	var total int
	if err := h.DB.QueryRow(ctx, countQuery, append([]any{personID}, params...)...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT id,event_type,occurred_at,details FROM (SELECT re.id::text,re.action_type AS event_type,re.occurred_at,jsonb_build_object('context_type',re.context_type,'actor_id',re.actor_person_id::text,'target_person_id',re.target_person_id::text,'evidence_ids',re.evidence_ids) AS details FROM relation_events re WHERE re.actor_person_id=$1 OR re.target_person_id=$1 UNION ALL SELECT m.id::text,'sent_message',m.sent_at,jsonb_build_object('conversation_id',m.conversation_id::text,'raw_text',LEFT(m.raw_text,200),'source_message_id',m.source_message_id) FROM messages m WHERE m.sender_id=$1 UNION ALL SELECT ('profile:'||pp.id::text),'profile_change',pp.valid_from,jsonb_build_object('nickname',pp.nickname,'source',pp.source,'avatar_uri',LEFT(pp.avatar_uri,80)) FROM person_profiles pp WHERE pp.person_id=$1 UNION ALL SELECT ('membership:'||gm.id::text),'group_membership',gm.valid_from,jsonb_build_object('group_id',gm.group_id::text,'role',gm.role) FROM group_memberships gm WHERE gm.person_id=$1) events` + where + ` ORDER BY occurred_at DESC NULLS LAST LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params2 := append([]any{personID}, params...)
	params2 = append(params2, limit, offset)
	rows, err := h.DB.Query(ctx, query, params2...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, etype string
		var at *time.Time
		var details []byte
		if rows.Scan(&id, &etype, &at, &details) == nil {
			var d any
			_ = json.Unmarshal(details, &d)
			items = append(items, map[string]any{"id": id, "event_type": etype, "occurred_at": at, "details": d})
		}
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleGroupDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		GroupID        string `json:"group_id"`
		IncludeMembers bool   `json:"include_members"`
		MemberLimit    int    `json:"member_limit"`
		MemberOffset   int    `json:"member_offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	id, platformID, err := h.resolveGroupKey(ctx, input.GroupID)
	if err != nil {
		return errorResult("group not found"), nil
	}
	var name string
	var first *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT group_name,first_seen_at FROM "groups" WHERE id=$1`, id).Scan(&name, &first); err != nil {
		return errorResult(err.Error()), nil
	}
	var members, messages int
	_ = h.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM group_memberships WHERE group_id=$1),(SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.conversation_type='group' AND c.platform_conversation_id=$2)`, id, platformID).Scan(&members, &messages)
	result := map[string]any{"id": id, "group_id": platformID, "group_name": name, "first_seen_at": first, "member_count": members, "message_count": messages}
	if input.IncludeMembers {
		mlimit, moffset := pagingDefaults(input.MemberLimit, input.MemberOffset, 200)
		rows, err := h.DB.Query(ctx, `SELECT p.id::text,p.display_name,COALESCE(pi.platform_user_id,''),gm.role,gm.card,gm.valid_from FROM group_memberships gm JOIN persons p ON p.id=gm.person_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE gm.group_id=$1 ORDER BY CASE gm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,p.display_name LIMIT $2 OFFSET $3`, id, mlimit, moffset)
		if err == nil {
			defer rows.Close()
			items := []map[string]any{}
			for rows.Next() {
				var pid, pname, pqq, role, card string
				var from *time.Time
				if rows.Scan(&pid, &pname, &pqq, &role, &card, &from) == nil {
					items = append(items, map[string]any{"id": pid, "display_name": pname, "qq": pqq, "role": role, "card": card, "valid_from": from})
				}
			}
			result["members"] = pagedResult(items, mlimit, moffset, members)
		}
	}
	return jsonResult(result)
}

func (h *HandlerRegistry) handleConversations(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Type string `json:"type"`

		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)
	where := "WHERE ($1='' OR c.conversation_type=$1) AND ($2='' OR c.platform_conversation_id ILIKE '%'||$2||'%' OR COALESCE(g.group_name,'') ILIKE '%'||$2||'%' OR COALESCE(p.display_name,'') ILIKE '%'||$2||'%')"
	var total int
	if err := h.DB.QueryRow(ctx, `SELECT count(*) FROM conversations c LEFT JOIN "groups" g ON c.conversation_type='group' AND g.platform_group_id=c.platform_conversation_id LEFT JOIN person_identifiers pi ON c.conversation_type='private' AND pi.platform='qq' AND pi.platform_user_id=c.platform_conversation_id LEFT JOIN persons p ON p.id=pi.person_id `+where, input.Type, input.Query).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT c.id::text,c.conversation_type,c.platform_conversation_id,COALESCE(NULLIF(c.name,''),COALESCE(g.group_name,p.display_name,'')),(SELECT count(*) FROM messages m WHERE m.conversation_id=c.id),(SELECT max(sent_at) FROM messages m WHERE m.conversation_id=c.id) FROM conversations c LEFT JOIN "groups" g ON c.conversation_type='group' AND g.platform_group_id=c.platform_conversation_id LEFT JOIN person_identifiers pi ON c.conversation_type='private' AND pi.platform='qq' AND pi.platform_user_id=c.platform_conversation_id LEFT JOIN persons p ON p.id=pi.person_id ` + where + ` ORDER BY 6 DESC NULLS LAST LIMIT $3 OFFSET $4`
	rows, err := h.DB.Query(ctx, query, input.Type, input.Query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type conv struct {
		ID, Type, PlatformID, Name string
		MessageCount               int
		LastSentAt                 *time.Time
	}
	items := []conv{}
	for rows.Next() {
		var it conv
		if rows.Scan(&it.ID, &it.Type, &it.PlatformID, &it.Name, &it.MessageCount, &it.LastSentAt) == nil {
			items = append(items, it)
		}
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleConversationContext(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ConversationID string `json:"conversation_id"`
		MemberLimit    int    `json:"member_limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var id, ctype, platformID, title string
	if err := h.DB.QueryRow(ctx, `SELECT c.id::text,c.conversation_type,c.platform_conversation_id,COALESCE(NULLIF(c.name,''),c.platform_conversation_id) FROM conversations c WHERE c.id::text=$1 OR c.platform_conversation_id=$1 LIMIT 1`, input.ConversationID).Scan(&id, &ctype, &platformID, &title); err != nil {
		return errorResult("conversation not found"), nil
	}
	var msgCount int64
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id=$1`, id).Scan(&msgCount)
	result := map[string]any{"id": id, "conversation_type": ctype, "platform_conversation_id": platformID, "title": title, "message_count": msgCount}
	if ctype == "group" {
		limit := input.MemberLimit
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		rows, err := h.DB.Query(ctx, `SELECT p.id::text,COALESCE(pi.platform_user_id,''),p.display_name,count(m.id) FROM group_memberships gm JOIN persons p ON p.id=gm.person_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' LEFT JOIN messages m ON m.sender_id=gm.person_id AND m.conversation_id=$2 WHERE gm.group_id=(SELECT id FROM "groups" WHERE platform='qq' AND platform_group_id=$1 LIMIT 1) GROUP BY p.id,pi.platform_user_id,p.display_name ORDER BY count(m.id) DESC LIMIT $3`, platformID, id, limit)
		if err == nil {
			defer rows.Close()
			members := []map[string]any{}
			for rows.Next() {
				var pid, qq, name string
				var c int64
				if rows.Scan(&pid, &qq, &name, &c) == nil {
					members = append(members, map[string]any{"id": pid, "qq": qq, "name": name, "message_count": c})
				}
			}
			result["active_members"] = members
		}
	}
	return jsonResult(result)
}

func (h *HandlerRegistry) handleMessageDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var id, source, senderQQ, senderName, text, ctype, platformID, convID, rawID, replyTo, replyText string
	var sentAt *time.Time
	var segments []byte
	err := h.DB.QueryRow(ctx, `SELECT m.id::text,COALESCE(m.source_message_id,''),COALESCE(pi.platform_user_id,''),COALESCE(p.display_name,''),COALESCE(m.raw_text,''),COALESCE(m.message_segments,'[]'::jsonb),m.sent_at,COALESCE(c.conversation_type,''),COALESCE(c.platform_conversation_id,''),COALESCE(c.id::text,''),COALESCE(m.raw_record_id::text,''),COALESCE(m.reply_to_message_id,''),COALESCE(rm.raw_text,'') FROM messages m LEFT JOIN persons p ON p.id=m.sender_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' LEFT JOIN conversations c ON c.id=m.conversation_id LEFT JOIN messages rm ON (rm.source_message_id=m.reply_to_message_id OR rm.id::text=m.reply_to_message_id) AND rm.conversation_id=m.conversation_id WHERE m.id::text=$1 OR m.source_message_id=$1 LIMIT 1`, input.MessageID).Scan(&id, &source, &senderQQ, &senderName, &text, &segments, &sentAt, &ctype, &platformID, &convID, &rawID, &replyTo, &replyText)
	if err != nil {
		return errorResult("message not found"), nil
	}
	var seg any
	_ = json.Unmarshal(segments, &seg)
	return jsonResult(map[string]any{"id": id, "source_message_id": source, "sender_qq": senderQQ, "sender_name": senderName, "text": text, "segments": seg, "sent_at": sentAt, "conversation_type": ctype, "conversation_id": platformID, "conversation_uuid": convID, "raw_record_id": rawID, "reply_to_message_id": replyTo, "reply_text": replyText})
}

func (h *HandlerRegistry) handleContentDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ContentID string `json:"content_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var id, platform, platformID, body, contextType, authorID, authorQQ, authorName, rawID, parentID string
	var publishedAt *time.Time
	var metadata []byte
	err := h.DB.QueryRow(ctx, `SELECT c.id::text,c.platform,c.platform_content_id,c.body,c.context_type,c.published_at,COALESCE(c.author_id::text,''),COALESCE(pi.platform_user_id,''),COALESCE(p.display_name,''),COALESCE(c.raw_record_id::text,''),COALESCE(c.parent_content_id::text,''),c.metadata FROM contents c LEFT JOIN persons p ON p.id=c.author_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE c.id::text=$1 OR c.platform_content_id=$1 LIMIT 1`, input.ContentID).Scan(&id, &platform, &platformID, &body, &contextType, &publishedAt, &authorID, &authorQQ, &authorName, &rawID, &parentID, &metadata)
	if err != nil {
		return errorResult("content not found"), nil
	}
	var meta any
	_ = json.Unmarshal(metadata, &meta)
	var likes, comments int
	_ = h.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM relation_events WHERE target_object_id=$1 AND action_type='liked'),(SELECT count(*) FROM contents WHERE parent_content_id=$1)`, id).Scan(&likes, &comments)
	return jsonResult(map[string]any{"id": id, "platform": platform, "content_id": platformID, "body": body, "context_type": contextType, "published_at": publishedAt, "author_id": authorID, "author_qq": authorQQ, "author_name": authorName, "raw_record_id": rawID, "parent_content_id": parentID, "metadata": meta, "likes_count": likes, "comments_count": comments})
}

func (h *HandlerRegistry) handleContentLikes(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ContentID string `json:"content_id"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)
	var total int
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM relation_events WHERE action_type='liked' AND target_object_id=$1`, input.ContentID).Scan(&total)
	rows, err := h.DB.Query(ctx, `SELECT re.id::text,re.actor_person_id::text,COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,''),re.occurred_at,re.evidence_ids FROM relation_events re LEFT JOIN persons p ON p.id=re.actor_person_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE re.action_type='liked' AND re.target_object_id=$1 ORDER BY re.occurred_at DESC LIMIT $2 OFFSET $3`, input.ContentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, pid, name, qq string
		var at *time.Time
		var ev []string
		if rows.Scan(&id, &pid, &name, &qq, &at, &ev) == nil {
			items = append(items, map[string]any{"id": id, "person_id": pid, "name": name, "qq": qq, "occurred_at": at, "evidence_ids": ev})
		}
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}
