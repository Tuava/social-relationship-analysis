package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

var mediaKinds = map[string]bool{
	"avatar": true, "image": true, "sticker": true,
	"audio": true, "video": true, "file": true,
}

func (s *Server) mediaDownloads(w http.ResponseWriter, r *http.Request) {
	var paused bool
	var concurrency int
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT paused,concurrency FROM media_download_settings WHERE singleton=true`).Scan(&paused, &concurrency); err != nil {
		writeError(w, 500, err)
		return
	}
	var assetCount, assetBytes int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*),COALESCE(sum(size),0) FROM media_assets`).Scan(&assetCount, &assetBytes); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT p.media_kind,p.enabled,p.priority,
        count(r.id),count(r.id) FILTER(WHERE r.status='pending'),count(r.id) FILTER(WHERE r.status='downloading'),
        count(r.id) FILTER(WHERE r.status='completed'),count(r.id) FILTER(WHERE r.status='failed'),
        COALESCE((SELECT sum(a.size) FROM media_assets a WHERE a.id IN (
            SELECT DISTINCT mr.asset_id FROM media_references mr WHERE mr.media_kind=p.media_kind AND mr.status='completed' AND mr.asset_id IS NOT NULL
        )),0)
        FROM media_download_policies p
        LEFT JOIN media_references r ON r.media_kind=p.media_kind
        GROUP BY p.media_kind,p.enabled,p.priority ORDER BY p.priority DESC`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	policies := []map[string]any{}
	for rows.Next() {
		var kind string
		var enabled bool
		var priority int
		var total, pending, downloading, completed, failed, bytes int64
		if err := rows.Scan(&kind, &enabled, &priority, &total, &pending, &downloading, &completed, &failed, &bytes); err != nil {
			writeError(w, 500, err)
			return
		}
		policies = append(policies, map[string]any{"kind": kind, "enabled": enabled, "priority": priority, "total": total, "pending": pending, "downloading": downloading, "completed": completed, "failed": failed, "bytes": bytes})
	}
	writeJSON(w, 200, map[string]any{"paused": paused, "concurrency": concurrency, "asset_count": assetCount, "asset_bytes": assetBytes, "policies": policies})
}

func (s *Server) updateMediaDownloads(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paused      *bool           `json:"paused"`
		Concurrency *int            `json:"concurrency"`
		Policies    map[string]bool `json:"policies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Concurrency != nil && (*in.Concurrency < 1 || *in.Concurrency > 6) {
		writeJSON(w, 400, map[string]string{"error": "concurrency must be between 1 and 6"})
		return
	}
	tx, err := s.Repo.DB.Begin(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback(r.Context())
	if in.Paused != nil {
		_, err = tx.Exec(r.Context(), `UPDATE media_download_settings SET paused=$1,updated_at=now() WHERE singleton=true`, *in.Paused)
	}
	if err == nil && in.Concurrency != nil {
		_, err = tx.Exec(r.Context(), `UPDATE media_download_settings SET concurrency=$1,updated_at=now() WHERE singleton=true`, *in.Concurrency)
	}
	for kind, enabled := range in.Policies {
		if err != nil {
			break
		}
		if !mediaKinds[kind] {
			err = &mediaPolicyError{kind: kind}
			break
		}
		if kind == "avatar" && !enabled {
			err = &mediaPolicyError{kind: "avatar cannot be disabled"}
			break
		}
		_, err = tx.Exec(r.Context(), `UPDATE media_download_policies SET enabled=$2,updated_at=now() WHERE media_kind=$1`, kind, enabled)
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, err)
		return
	}
	s.mediaDownloads(w, r)
}

type mediaPolicyError struct{ kind string }

func (e *mediaPolicyError) Error() string { return "unsupported media kind: " + e.kind }

func (s *Server) mediaReferences(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	kind, status, query := r.URL.Query().Get("kind"), r.URL.Query().Get("status"), strings.TrimSpace(r.URL.Query().Get("q"))
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	depth := -1
	if rawDepth := strings.TrimSpace(r.URL.Query().Get("depth")); rawDepth != "" {
		value, err := strconv.Atoi(rawDepth)
		if err != nil || value < 0 || value > 99 {
			writeJSON(w, 400, map[string]string{"error": "depth must be between 0 and 99"})
			return
		}
		depth = value
	}
	if kind != "" && !mediaKinds[kind] {
		writeJSON(w, 400, map[string]string{"error": "unsupported media kind"})
		return
	}
	filter := ` FROM media_references r
        LEFT JOIN media_assets a ON a.id=r.asset_id
		LEFT JOIN media_download_policies dp ON dp.media_kind=r.media_kind
        LEFT JOIN persons rp ON rp.id=r.person_id
        LEFT JOIN person_identifiers rpi ON rpi.person_id=rp.id AND rpi.platform='qq'
        LEFT JOIN messages m ON m.id=r.message_id
        LEFT JOIN persons sp ON sp.id=m.sender_id
        LEFT JOIN person_identifiers spi ON spi.person_id=sp.id AND spi.platform='qq'
        LEFT JOIN conversations c ON c.id=m.conversation_id
        WHERE ($1='' OR r.media_kind=$1) AND ($2='' OR r.status=$2)
          AND ($3='' OR r.original_filename ILIKE '%'||$3||'%' OR r.source_ref ILIKE '%'||$3||'%'
            OR rp.display_name ILIKE '%'||$3||'%' OR rpi.platform_user_id=$3
            OR sp.display_name ILIKE '%'||$3||'%' OR spi.platform_user_id=$3
            OR c.platform_conversation_id=$3)
          AND ($4::integer < 0 OR r.relation_depth=$4)
          AND ($5='' OR r.priority_reason=$5)`
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*)`+filter, kind, status, query, depth, reason).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT r.id::text,r.media_kind,r.status,r.original_filename,r.source_ref,
        COALESCE(r.last_error,''),r.attempt_count,r.download_selected,r.created_at,r.completed_at,
		r.relation_depth,r.priority_reason,r.priority_boost,COALESCE(dp.priority,0),
        COALESCE(a.id::text,''),COALESCE(a.mime_type,''),COALESCE(a.size,0),
        COALESCE(rp.display_name,sp.display_name,''),COALESCE(rpi.platform_user_id,spi.platform_user_id,''),
        COALESCE(m.id::text,''),COALESCE(m.source_message_id,''),m.sent_at,
        COALESCE(c.conversation_type,''),COALESCE(c.platform_conversation_id,'')`+filter+`
        ORDER BY CASE r.status WHEN 'downloading' THEN 0 WHEN 'pending' THEN 1 WHEN 'failed' THEN 2 ELSE 3 END,
		  r.download_selected DESC,COALESCE(dp.priority,0) DESC,r.relation_depth ASC,r.priority_boost DESC,
          CASE WHEN r.status IN ('downloading','pending') THEN r.created_at END ASC,r.created_at DESC
        LIMIT $6 OFFSET $7`, kind, status, query, depth, reason, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, mediaKind, itemStatus, filename, sourceRef, lastError, assetID, mimeType, personName, qq string
		var messageID, sourceMessageID, conversationType, conversationID string
		var attempts, relationDepth, priorityBoost, typePriority int
		var priorityReason string
		var selected bool
		var createdAt, completedAt, sentAt any
		var size int64
		if err := rows.Scan(&id, &mediaKind, &itemStatus, &filename, &sourceRef, &lastError, &attempts, &selected, &createdAt, &completedAt, &relationDepth, &priorityReason, &priorityBoost, &typePriority, &assetID, &mimeType, &size, &personName, &qq, &messageID, &sourceMessageID, &sentAt, &conversationType, &conversationID); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"id": id, "kind": mediaKind, "status": itemStatus, "filename": filename, "source_ref": sourceRef, "error": lastError, "attempts": attempts, "selected": selected, "relation_depth": relationDepth, "priority_reason": priorityReason, "priority_boost": priorityBoost, "type_priority": typePriority, "created_at": createdAt, "completed_at": completedAt, "asset_id": assetID, "asset_url": assetURL(assetID), "mime_type": mimeType, "size": size, "person_name": personName, "qq": qq, "message_id": messageID, "source_message_id": sourceMessageID, "sent_at": sentAt, "conversation_type": conversationType, "conversation_id": conversationID})
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) selectMediaDownload(w http.ResponseWriter, r *http.Request) {
	command, err := s.Repo.DB.Exec(r.Context(), `UPDATE media_references SET download_selected=true,status=CASE WHEN status='completed' THEN status ELSE 'pending' END,attempt_count=CASE WHEN status='completed' THEN attempt_count ELSE 0 END,next_attempt_at=now(),last_error=CASE WHEN status='completed' THEN last_error ELSE NULL END WHERE id=$1`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if command.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "media reference not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) retryMediaDownloads(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Kind != "" && !mediaKinds[in.Kind] {
		writeJSON(w, 400, map[string]string{"error": "unsupported media kind"})
		return
	}
	command, err := s.Repo.DB.Exec(r.Context(), `UPDATE media_references SET status='pending',attempt_count=0,last_error=NULL,next_attempt_at=now() WHERE status='failed' AND ($1='' OR media_kind=$1)`, in.Kind)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"retried": command.RowsAffected()})
}

func assetURL(id string) string {
	if strings.TrimSpace(id) == "" {
		return ""
	}
	return "/api/v1/media/assets/" + id
}

func parseLimit(value string, fallback int) int {
	n, _ := strconv.Atoi(value)
	if n <= 0 {
		return fallback
	}
	return n
}
