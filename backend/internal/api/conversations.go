package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

type ConversationItem struct {
	ID                     string           `json:"id"`
	ConversationType       string           `json:"conversation_type"`
	PlatformConversationID string           `json:"platform_conversation_id"`
	Title                  string           `json:"title"`
	AvatarURI              string           `json:"avatar_uri"`
	MessageCount           int64            `json:"message_count"`
	LastSentAt             *time.Time       `json:"last_sent_at"`
	LastMessage            *LastMessageInfo `json:"last_message,omitempty"`
}

type LastMessageInfo struct {
	ID         string     `json:"id"`
	Text       string     `json:"text"`
	SenderName string     `json:"sender_name"`
	SenderQQ   string     `json:"sender_qq"`
	SentAt     *time.Time `json:"sent_at"`
}

type ConversationContextInfo struct {
	Conversation ConversationItem `json:"conversation"`
	ActiveRank   []ActiveMember   `json:"active_rank"`
	MediaItems   []MediaItem      `json:"media_items"`
	TotalMedia   int64            `json:"total_media"`
	TotalMembers int64            `json:"total_members"`
}

type ActiveMember struct {
	PersonID     string     `json:"person_id"`
	QQ           string     `json:"qq"`
	DisplayName  string     `json:"display_name"`
	AvatarURI    string     `json:"avatar_uri"`
	MessageCount int64      `json:"message_count"`
	LastActiveAt *time.Time `json:"last_active_at"`
	Role         string     `json:"role,omitempty"`
}

type MediaItem struct {
	ID          string     `json:"id"`
	ReferenceID string     `json:"reference_id"`
	MessageID   string     `json:"message_id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	AssetURL    string     `json:"asset_url"`
	MimeType    string     `json:"mime_type"`
	Filename    string     `json:"filename"`
	SentAt      *time.Time `json:"sent_at"`
	SenderName  string     `json:"sender_name"`
}

func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	convType := strings.TrimSpace(r.URL.Query().Get("type"))

	countSQL := `
		SELECT count(*)
		FROM conversations c
		LEFT JOIN "groups" g ON c.conversation_type = 'group' AND g.platform_group_id = c.platform_conversation_id
		LEFT JOIN person_identifiers pi ON c.conversation_type = 'private' AND pi.platform = 'qq' AND pi.platform_user_id = c.platform_conversation_id
		LEFT JOIN persons p ON p.id = pi.person_id
		WHERE ($1 = '' OR c.conversation_type = $1)
		  AND ($2 = '' OR c.platform_conversation_id ILIKE '%' || $2 || '%' OR g.group_name ILIKE '%' || $2 || '%' OR p.display_name ILIKE '%' || $2 || '%')`

	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), countSQL, convType, query).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}

	querySQL := `
		WITH conv_stats AS (
			SELECT 
				m.conversation_id,
				count(*) as total_messages,
				max(m.sent_at) as max_sent_at
			FROM messages m
			GROUP BY m.conversation_id
		)
		SELECT 
			c.id::text,
			c.conversation_type,
			c.platform_conversation_id,
			COALESCE(NULLIF(c.name, ''), CASE WHEN c.conversation_type = 'group' THEN COALESCE(NULLIF(g.group_name,''), '群 ' || c.platform_conversation_id) ELSE COALESCE(NULLIF(p.display_name,''), 'QQ ' || c.platform_conversation_id) END) as title,
			CASE 
				WHEN c.conversation_type = 'group' THEN 'https://p.qlogo.cn/gh/' || c.platform_conversation_id || '/' || c.platform_conversation_id || '/640/'
				ELSE '/api/v1/media/avatars/person/' || c.platform_conversation_id
			END as avatar_uri,
			COALESCE(cs.total_messages, 0) as message_count,
			cs.max_sent_at,
			latest.id::text,
			COALESCE(latest.raw_text, ''),
			COALESCE(lp.display_name, ''),
			COALESCE(lpi.platform_user_id, ''),
			latest.sent_at
		FROM conversations c
		LEFT JOIN "groups" g ON c.conversation_type = 'group' AND g.platform_group_id = c.platform_conversation_id
		LEFT JOIN person_identifiers pi ON c.conversation_type = 'private' AND pi.platform = 'qq' AND pi.platform_user_id = c.platform_conversation_id
		LEFT JOIN persons p ON p.id = pi.person_id
		LEFT JOIN conv_stats cs ON cs.conversation_id = c.id
		LEFT JOIN LATERAL (
			SELECT m2.id, m2.raw_text, m2.sender_id, m2.sent_at
			FROM messages m2
			WHERE m2.conversation_id = c.id
			ORDER BY m2.sent_at DESC NULLS LAST, m2.created_at DESC
			LIMIT 1
		) latest ON true
		LEFT JOIN persons lp ON lp.id = latest.sender_id
		LEFT JOIN person_identifiers lpi ON lpi.person_id = latest.sender_id AND lpi.platform = 'qq'
		WHERE ($1 = '' OR c.conversation_type = $1)
		  AND ($2 = '' OR c.platform_conversation_id ILIKE '%' || $2 || '%' OR g.group_name ILIKE '%' || $2 || '%' OR p.display_name ILIKE '%' || $2 || '%')
		ORDER BY cs.max_sent_at DESC NULLS LAST, cs.total_messages DESC NULLS LAST
		LIMIT $3 OFFSET $4`

	rows, err := s.Repo.DB.Query(r.Context(), querySQL, convType, query, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()

	items := make([]ConversationItem, 0, limit)
	for rows.Next() {
		var item ConversationItem
		var lastMsgID, lastText, lastSenderName, lastSenderQQ *string
		var lastSentAt *time.Time
		if err := rows.Scan(
			&item.ID,
			&item.ConversationType,
			&item.PlatformConversationID,
			&item.Title,
			&item.AvatarURI,
			&item.MessageCount,
			&item.LastSentAt,
			&lastMsgID,
			&lastText,
			&lastSenderName,
			&lastSenderQQ,
			&lastSentAt,
		); err != nil {
			writeError(w, 500, err)
			return
		}
		item.Title = domain.CleanDisplayText(item.Title)
		if lastMsgID != nil && *lastMsgID != "" {
			item.LastMessage = &LastMessageInfo{
				ID:         *lastMsgID,
				Text:       domain.CleanDisplayText(*lastText),
				SenderName: domain.CleanDisplayText(*lastSenderName),
				SenderQQ:   *lastSenderQQ,
				SentAt:     lastSentAt,
			}
		}
		items = append(items, item)
	}

	writeJSON(w, 200, map[string]any{
		"data":   items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) conversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	querySQL := `
		SELECT 
			c.id::text,
			c.conversation_type,
			c.platform_conversation_id,
			COALESCE(NULLIF(c.name, ''), CASE WHEN c.conversation_type = 'group' THEN COALESCE(NULLIF(g.group_name,''), '群 ' || c.platform_conversation_id) ELSE COALESCE(NULLIF(p.display_name,''), 'QQ ' || c.platform_conversation_id) END) as title,
			CASE 
				WHEN c.conversation_type = 'group' THEN 'https://p.qlogo.cn/gh/' || c.platform_conversation_id || '/' || c.platform_conversation_id || '/640/'
				ELSE '/api/v1/media/avatars/person/' || c.platform_conversation_id
			END as avatar_uri,
			(SELECT count(*) FROM messages m WHERE m.conversation_id = c.id) as message_count,
			(SELECT max(sent_at) FROM messages m WHERE m.conversation_id = c.id) as max_sent_at
		FROM conversations c
		LEFT JOIN "groups" g ON c.conversation_type = 'group' AND g.platform_group_id = c.platform_conversation_id
		LEFT JOIN person_identifiers pi ON c.conversation_type = 'private' AND pi.platform = 'qq' AND pi.platform_user_id = c.platform_conversation_id
		LEFT JOIN persons p ON p.id = pi.person_id
		WHERE c.id::text = $1 OR c.platform_conversation_id = $1
		LIMIT 1`

	var item ConversationItem
	if err := s.Repo.DB.QueryRow(r.Context(), querySQL, id).Scan(
		&item.ID,
		&item.ConversationType,
		&item.PlatformConversationID,
		&item.Title,
		&item.AvatarURI,
		&item.MessageCount,
		&item.LastSentAt,
	); err != nil {
		writeError(w, 404, fmt.Errorf("conversation not found"))
		return
	}
	item.Title = domain.CleanDisplayText(item.Title)
	writeJSON(w, 200, map[string]any{"data": item})
}

func (s *Server) conversationMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit, offset := pageParams(r)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	beforeTime := strings.TrimSpace(r.URL.Query().Get("before"))
	around := strings.TrimSpace(r.URL.Query().Get("around"))
	if around != "" && beforeTime == "" {
		var targetSentAt *time.Time
		_ = s.Repo.DB.QueryRow(r.Context(), `SELECT sent_at + interval '20 seconds' FROM messages WHERE id::text=$1 OR source_message_id=$1 LIMIT 1`, around).Scan(&targetSentAt)
		if targetSentAt != nil {
			beforeTime = targetSentAt.Format(time.RFC3339Nano)
		}
	}

	// Resolve conversation UUID
	var convID, convPlatformID, convType string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text, platform_conversation_id, conversation_type FROM conversations WHERE id::text=$1 OR platform_conversation_id=$1 LIMIT 1`, id).Scan(&convID, &convPlatformID, &convType); err != nil {
		writeError(w, 404, fmt.Errorf("conversation not found"))
		return
	}

	// Total count for current filter
	var total int64
	countSQL := `
		SELECT count(*)
		FROM messages m
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE m.conversation_id = $1::uuid
		  AND ($2 = '' OR m.raw_text ILIKE '%' || $2 || '%' OR p.display_name ILIKE '%' || $2 || '%' OR pi.platform_user_id = $2)
		  AND ($3 = '' OR m.sent_at < $3::timestamptz)`

	if err := s.Repo.DB.QueryRow(r.Context(), countSQL, convID, q, beforeTime).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}

	querySQL := `
		SELECT 
			m.id::text,
			COALESCE(m.source_message_id, ''),
			COALESCE(NULLIF(p.display_name, ''), NULLIF(na.name, ''), CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN 'QQ ' || pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN 'QQ ' || na.qq_uin ELSE '我' END) as sender_name,
			COALESCE(pi.platform_user_id, na.qq_uin, ''),
			COALESCE(
				(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
				(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
				CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN '/api/v1/media/avatars/person/'||na.qq_uin ELSE '' END,
				''
			) as sender_avatar,
			COALESCE(m.raw_text, ''),
			COALESCE(m.message_segments, '[]'::jsonb),
			m.sent_at,
			COALESCE(m.reply_to_message_id, ''),
			COALESCE(NULLIF(rp.display_name, ''), CASE WHEN rpi.platform_user_id IS NOT NULL AND rpi.platform_user_id<>'' THEN 'QQ ' || rpi.platform_user_id ELSE '' END, ''),
			COALESCE(rm.raw_text, ''),
			COALESCE(m.is_recalled, false),
			m.raw_record_id::text,
			COALESCE(media.items, '[]'::jsonb)
		FROM messages m
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		LEFT JOIN napcat_accounts na ON na.id = m.source_account_id
		LEFT JOIN messages rm ON (rm.source_message_id = m.reply_to_message_id OR rm.id::text = m.reply_to_message_id) AND rm.conversation_id = m.conversation_id
		LEFT JOIN persons rp ON rp.id = rm.sender_id
		LEFT JOIN person_identifiers rpi ON rpi.person_id = rp.id AND rpi.platform = 'qq'
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(item ORDER BY position) AS items FROM (
				SELECT cm.segment_index AS position, jsonb_build_object(
					'reference_id', mr.id::text,
					'kind', mr.media_kind,
					'status', mr.status,
					'asset_url', CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/' || mr.asset_id::text ELSE '' END,
					'mime_type', COALESCE(ma.mime_type, ''),
					'filename', COALESCE(ma.original_filename, mr.original_filename, '')
				) AS item
				FROM message_media cm
				JOIN media_references mr ON mr.id = cm.media_reference_id
				LEFT JOIN media_assets ma ON ma.id = mr.asset_id
				WHERE cm.message_id = m.id
				ORDER BY cm.segment_index
			) preview
		) media ON true
		WHERE m.conversation_id = $1::uuid
		  AND ($2 = '' OR m.raw_text ILIKE '%' || $2 || '%' OR p.display_name ILIKE '%' || $2 || '%' OR pi.platform_user_id = $2 OR na.name ILIKE '%' || $2 || '%' OR na.qq_uin = $2)
		  AND ($3 = '' OR m.sent_at < $3::timestamptz)
		ORDER BY m.sent_at DESC NULLS LAST, m.created_at DESC
		LIMIT $4 OFFSET $5`

	rows, err := s.Repo.DB.Query(r.Context(), querySQL, convID, q, beforeTime, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()

	data := []map[string]any{}
	for rows.Next() {
		var id, sourceID, senderName, senderQQ, senderAvatar, text, replyTo, replySender, replyText, rawID string
		var isRecalled bool
		var segmentsRaw, mediaRaw []byte
		var sentAt interface{}
		if err := rows.Scan(&id, &sourceID, &senderName, &senderQQ, &senderAvatar, &text, &segmentsRaw, &sentAt, &replyTo, &replySender, &replyText, &isRecalled, &rawID, &mediaRaw); err != nil {
			writeError(w, 500, err)
			return
		}
		var segmentsValue any
		_ = json.Unmarshal(segmentsRaw, &segmentsValue)
		var mediaValue any
		_ = json.Unmarshal(mediaRaw, &mediaValue)

		data = append(data, map[string]any{
			"id":                id,
			"source_message_id": sourceID,
			"sender":            domain.CleanDisplayText(senderName),
			"sender_qq":         senderQQ,
			"sender_avatar":     senderAvatar,
			"text":              domain.CleanDisplayText(text),
			"segments":          segmentsValue,
			"sent_at":           sentAt,
			"reply_to":          replyTo,
			"reply_sender":      domain.CleanDisplayText(replySender),
			"reply_text":        domain.CleanDisplayText(replyText),
			"is_recalled":       isRecalled,
			"raw_record_id":     rawID,
			"media":             mediaValue,
		})
	}

	writeJSON(w, 200, map[string]any{
		"data":            data,
		"total":           total,
		"limit":           limit,
		"offset":          offset,
		"conversation_id": convID,
	})
}

func (s *Server) conversationContext(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 1. Conversation summary
	var conv ConversationItem
	queryConvSQL := `
		SELECT 
			c.id::text,
			c.conversation_type,
			c.platform_conversation_id,
			COALESCE(NULLIF(c.name, ''), CASE WHEN c.conversation_type = 'group' THEN COALESCE(NULLIF(g.group_name,''), '群 ' || c.platform_conversation_id) ELSE COALESCE(NULLIF(p.display_name,''), 'QQ ' || c.platform_conversation_id) END) as title,
			CASE 
				WHEN c.conversation_type = 'group' THEN 'https://p.qlogo.cn/gh/' || c.platform_conversation_id || '/' || c.platform_conversation_id || '/640/'
				ELSE '/api/v1/media/avatars/person/' || c.platform_conversation_id
			END as avatar_uri,
			(SELECT count(*) FROM messages m WHERE m.conversation_id = c.id) as message_count,
			(SELECT max(m.sent_at) FROM messages m WHERE m.conversation_id = c.id) as last_sent_at
		FROM conversations c
		LEFT JOIN "groups" g ON g.platform_group_id = c.platform_conversation_id AND c.conversation_type = 'group'
		LEFT JOIN person_identifiers pi ON pi.platform_user_id = c.platform_conversation_id AND pi.platform = 'qq' AND c.conversation_type = 'private'
		LEFT JOIN persons p ON p.id = pi.person_id
		WHERE c.id::text = $1 OR c.platform_conversation_id = $1
		LIMIT 1`

	if err := s.Repo.DB.QueryRow(r.Context(), queryConvSQL, id).Scan(
		&conv.ID,
		&conv.ConversationType,
		&conv.PlatformConversationID,
		&conv.Title,
		&conv.AvatarURI,
		&conv.MessageCount,
		&conv.LastSentAt,
	); err != nil {
		writeError(w, 404, fmt.Errorf("conversation not found: %w", err))
		return
	}
	conv.Title = domain.CleanDisplayText(conv.Title)

	// 2. Active member ranking (Top 20 most active chatters in this session)
	activeRank := []ActiveMember{}
	rankSQL := `
		SELECT 
			COALESCE(p.id::text, ''),
			COALESCE(pi.platform_user_id, na.qq_uin, ''),
			COALESCE(NULLIF(p.display_name, ''), NULLIF(na.name, ''), CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN 'QQ ' || pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN 'QQ ' || na.qq_uin ELSE '未知成员' END) as display_name,
			COALESCE(
				(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
				(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
				CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN '/api/v1/media/avatars/person/'||na.qq_uin ELSE '' END,
				''
			) as avatar_uri,
			count(m.id) as message_count,
			max(m.sent_at) as last_active_at,
			COALESCE(gm.role, '') as role
		FROM messages m
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		LEFT JOIN napcat_accounts na ON na.id = m.source_account_id
		LEFT JOIN "groups" g ON g.platform_group_id = $2
		LEFT JOIN group_memberships gm ON gm.group_id = g.id AND gm.person_id = p.id
		WHERE m.conversation_id = $1::uuid
		GROUP BY p.id, pi.platform_user_id, na.qq_uin, p.display_name, na.name, gm.role
		ORDER BY message_count DESC
		LIMIT 20`

	rankRows, err := s.Repo.DB.Query(r.Context(), rankSQL, conv.ID, conv.PlatformConversationID)
	if err == nil {
		for rankRows.Next() {
			var member ActiveMember
			if scanErr := rankRows.Scan(&member.PersonID, &member.QQ, &member.DisplayName, &member.AvatarURI, &member.MessageCount, &member.LastActiveAt, &member.Role); scanErr == nil {
				member.DisplayName = domain.CleanDisplayText(member.DisplayName)
				activeRank = append(activeRank, member)
			}
		}
		rankRows.Close()
	}

	// 3. Shared media gallery (Images/files shared in this conversation)
	mediaList := []MediaItem{}
	mediaSQL := `
		SELECT 
			mr.id::text,
			mr.id::text,
			m.id::text,
			mr.media_kind,
			mr.status,
			CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/' || mr.asset_id::text ELSE '' END as asset_url,
			COALESCE(ma.mime_type, ''),
			COALESCE(ma.original_filename, mr.original_filename, ''),
			m.sent_at,
			COALESCE(p.display_name, '')
		FROM message_media mm
		JOIN messages m ON m.id = mm.message_id
		JOIN media_references mr ON mr.id = mm.media_reference_id
		LEFT JOIN media_assets ma ON ma.id = mr.asset_id
		LEFT JOIN persons p ON p.id = m.sender_id
		WHERE m.conversation_id = $1::uuid
		ORDER BY m.sent_at DESC NULLS LAST
		LIMIT 60`

	mediaRows, err := s.Repo.DB.Query(r.Context(), mediaSQL, conv.ID)
	if err == nil {
		for mediaRows.Next() {
			var item MediaItem
			if scanErr := mediaRows.Scan(&item.ID, &item.ReferenceID, &item.MessageID, &item.Kind, &item.Status, &item.AssetURL, &item.MimeType, &item.Filename, &item.SentAt, &item.SenderName); scanErr == nil {
				item.SenderName = domain.CleanDisplayText(item.SenderName)
				mediaList = append(mediaList, item)
			}
		}
		mediaRows.Close()
	}

	var totalMedia, totalMembers int64
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM message_media mm JOIN messages m ON m.id=mm.message_id WHERE m.conversation_id=$1::uuid`, conv.ID).Scan(&totalMedia)
	if conv.ConversationType == "group" {
		_ = s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM group_memberships gm JOIN "groups" g ON g.id=gm.group_id WHERE g.platform_group_id=$1`, conv.PlatformConversationID).Scan(&totalMembers)
	}

	writeJSON(w, 200, map[string]any{
		"data": ConversationContextInfo{
			Conversation: conv,
			ActiveRank:   activeRank,
			MediaItems:   mediaList,
			TotalMedia:   totalMedia,
			TotalMembers: totalMembers,
		},
	})
}

type SendMessageRequest struct {
	Text      string   `json:"text"`
	ReplyTo   string   `json:"reply_to"`
	AtQQ      string   `json:"at_qq"`
	FaceID    string   `json:"face_id"`
	Images    []string `json:"images"`
	AccountID string   `json:"account_id"`
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, fmt.Errorf("invalid json body"))
		return
	}

	var convID, platformID, convType string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text, platform_conversation_id, conversation_type FROM conversations WHERE id::text=$1 OR platform_conversation_id=$1 LIMIT 1`, id).Scan(&convID, &platformID, &convType); err != nil {
		writeError(w, 404, fmt.Errorf("conversation not found"))
		return
	}

	client, accountID, ok := s.enabledAccount(w, r, req.AccountID)
	if !ok {
		return
	}

	// Resolve account QQ UIN and find/create person record
	var accountQQ, accountName string
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT qq_uin, name FROM napcat_accounts WHERE id=$1::uuid`, accountID).Scan(&accountQQ, &accountName)

	var senderPersonID *string
	if accountQQ != "" {
		var pid string
		err := s.Repo.DB.QueryRow(r.Context(), `SELECT person_id::text FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, accountQQ).Scan(&pid)
		if err == nil && pid != "" {
			senderPersonID = &pid
		} else {
			displayName := accountName
			if displayName == "" {
				displayName = "QQ " + accountQQ
			}
			if err := s.Repo.DB.QueryRow(r.Context(), `INSERT INTO persons(display_name) VALUES($1) RETURNING id::text`, displayName).Scan(&pid); err == nil && pid != "" {
				_, _ = s.Repo.DB.Exec(r.Context(), `INSERT INTO person_identifiers(person_id, platform, platform_user_id, source_account_id) VALUES($1::uuid, 'qq', $2, $3::uuid) ON CONFLICT(platform, platform_user_id) DO NOTHING`, pid, accountQQ, accountID)
				senderPersonID = &pid
			}
		}
	}

	segments := make([]map[string]any, 0)
	if req.ReplyTo != "" {
		segments = append(segments, map[string]any{"type": "reply", "data": map[string]any{"id": req.ReplyTo}})
	}
	if req.AtQQ != "" {
		segments = append(segments, map[string]any{"type": "at", "data": map[string]any{"qq": req.AtQQ}})
	}
	if req.FaceID != "" {
		segments = append(segments, map[string]any{"type": "face", "data": map[string]any{"id": req.FaceID}})
	}
	if req.Text != "" {
		segments = append(segments, map[string]any{"type": "text", "data": map[string]any{"text": req.Text}})
	}
	for _, img := range req.Images {
		if strings.TrimSpace(img) != "" {
			segments = append(segments, map[string]any{"type": "image", "data": map[string]any{"file": img}})
		}
	}

	if len(segments) == 0 {
		writeError(w, 400, fmt.Errorf("message content cannot be empty"))
		return
	}

	sendParams := map[string]any{
		"message_type": convType,
		"message":      segments,
	}
	if convType == "group" {
		sendParams["group_id"] = platformID
	} else {
		sendParams["user_id"] = platformID
	}

	raw, err := client.Call(r.Context(), "/send_msg", sendParams)
	if err != nil {
		writeError(w, 502, fmt.Errorf("NapCat 发送失败: %w", err))
		return
	}

	rawID, _ := s.Repo.SaveRaw(r.Context(), accountID, "napcat_http", "send_msg:"+platformID, raw)

	// Persist sent message in DB
	var respData struct {
		MessageID any `json:"message_id"`
	}
	_ = json.Unmarshal(raw, &respData)
	sourceMsgID := fmt.Sprint(respData.MessageID)

	segmentsJSON, _ := json.Marshal(segments)
	now := time.Now()
	_, _ = s.Repo.DB.Exec(r.Context(), `
		INSERT INTO messages(source_account_id, source_message_id, conversation_id, sender_id, raw_text, message_segments, sent_at, reply_to_message_id, raw_record_id)
		VALUES($1::uuid, $2, $3::uuid, $4::uuid, $5, $6::jsonb, $7, $8, NULLIF($9,'')::uuid)
		ON CONFLICT(source_account_id, source_message_id) DO UPDATE SET sender_id=EXCLUDED.sender_id, raw_text=EXCLUDED.raw_text, message_segments=EXCLUDED.message_segments`,
		accountID, sourceMsgID, convID, senderPersonID, req.Text, segmentsJSON, now, req.ReplyTo, rawID,
	)

	writeJSON(w, 200, map[string]any{
		"status":        "ok",
		"message_id":    sourceMsgID,
		"raw_record_id": rawID,
		"data":          raw,
	})
}

func (s *Server) recallMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var sourceMsgID string
	var accountID string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE(source_message_id,''), COALESCE(source_account_id::text,'') FROM messages WHERE id=$1`, id).Scan(&sourceMsgID, &accountID); err != nil || sourceMsgID == "" {
		writeError(w, 404, fmt.Errorf("message not found or missing source message id"))
		return
	}
	client, _, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.Call(r.Context(), "/delete_msg", map[string]any{"message_id": sourceMsgID})
	isSuccess := err == nil
	if err != nil && (strings.Contains(err.Error(), `"result": 0`) || strings.Contains(err.Error(), `"result":0`)) {
		isSuccess = true
	}
	if !isSuccess {
		writeError(w, 502, fmt.Errorf("撤回失败: %w", err))
		return
	}

	// Update message as recalled in database (preserving original content for forensics/chat log audit)
	_, _ = s.Repo.DB.Exec(r.Context(), `UPDATE messages SET is_recalled=true WHERE id=$1`, id)

	writeJSON(w, 200, map[string]any{"status": "ok", "message": "消息已成功撤回", "data": raw})
}

func (s *Server) reactMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		EmojiID int  `json:"emoji_id"`
		Set     bool `json:"set"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	var sourceMsgID string
	var accountID string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE(source_message_id,''), COALESCE(source_account_id::text,'') FROM messages WHERE id=$1`, id).Scan(&sourceMsgID, &accountID); err != nil || sourceMsgID == "" {
		writeError(w, 404, fmt.Errorf("message not found or missing source message id"))
		return
	}
	client, _, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.Call(r.Context(), "/set_msg_emoji_like", map[string]any{
		"message_id": sourceMsgID,
		"emoji_id":   req.EmojiID,
		"set":        true,
	})
	if err != nil {
		writeError(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "data": raw})
}

func (s *Server) essenceMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var sourceMsgID string
	var accountID string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT COALESCE(source_message_id,''), COALESCE(source_account_id::text,'') FROM messages WHERE id=$1`, id).Scan(&sourceMsgID, &accountID); err != nil || sourceMsgID == "" {
		writeError(w, 404, fmt.Errorf("message not found or missing source message id"))
		return
	}
	client, _, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.Call(r.Context(), "/set_essence_msg", map[string]any{"message_id": sourceMsgID})
	if err != nil {
		writeError(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "data": raw})
}

