package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) groupMembers(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	limit, offset := pageParams(r)
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM group_memberships WHERE group_id=$1`, groupID).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT p.id::text,p.display_name,COALESCE(pi.platform_user_id,''),gm.role,gm.valid_from,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri<>'' ORDER BY pp.valid_from DESC LIMIT 1),'')
		FROM group_memberships gm JOIN persons p ON p.id=gm.person_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE gm.group_id=$1 ORDER BY CASE gm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,p.display_name LIMIT $2 OFFSET $3`, groupID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, qq, role, avatar string
		var at any
		if err := rows.Scan(&id, &name, &qq, &role, &at, &avatar); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, map[string]any{"id": id, "display_name": name, "qq": qq, "role": role, "valid_from": at, "avatar_uri": avatar})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) groupMessages(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	var platformID string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT platform_group_id FROM groups WHERE id=$1`, chi.URLParam(r, "id")).Scan(&platformID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.conversation_type='group' AND c.platform_conversation_id=$1`, platformID).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT m.id::text,COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,''),m.raw_text,m.sent_at,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri<>'' ORDER BY pp.valid_from DESC LIMIT 1),''),COALESCE(media.items,'[]'::jsonb),COALESCE(media.total,0)
		FROM messages m JOIN conversations c ON c.id=m.conversation_id LEFT JOIN persons p ON p.id=m.sender_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		LEFT JOIN LATERAL (SELECT count(*) AS total,jsonb_agg(item ORDER BY position) AS items FROM (
			SELECT mm.segment_index AS position,jsonb_build_object('reference_id',mr.id::text,'kind',mr.media_kind,'status',mr.status,
				'asset_url',CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/'||mr.asset_id::text ELSE '' END,
				'mime_type',COALESCE(ma.mime_type,''),'filename',COALESCE(ma.original_filename,mr.original_filename,'')) AS item
			FROM message_media mm JOIN media_references mr ON mr.id=mm.media_reference_id LEFT JOIN media_assets ma ON ma.id=mr.asset_id
			WHERE mm.message_id=m.id ORDER BY mm.segment_index LIMIT 4) preview) media ON true
		WHERE c.conversation_type='group' AND c.platform_conversation_id=$1 ORDER BY m.sent_at DESC NULLS LAST LIMIT $2 OFFSET $3`, platformID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sender, senderQQ, text, avatar string
		var sentAt any
		var media []byte
		var mediaCount int
		if err := rows.Scan(&id, &sender, &senderQQ, &text, &sentAt, &avatar, &media, &mediaCount); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var mediaValue any
		_ = json.Unmarshal(media, &mediaValue)
		items = append(items, map[string]any{"id": id, "sender": sender, "sender_qq": senderQQ, "sender_avatar": avatar, "text": text, "sent_at": sentAt, "media_preview": mediaValue, "media_count": mediaCount})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": total, "limit": limit, "offset": offset})
}
