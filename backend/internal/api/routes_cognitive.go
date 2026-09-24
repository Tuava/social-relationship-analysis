package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

// mediaOCRSearch handles GET /api/v1/media/ocr-search?q=keyword&limit=50
func (s *Server) mediaOCRSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 200 {
			limit = val
		}
	}

	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT id::text, sha256, mime_type, size, object_path,
		       COALESCE(ocr_text, ''), COALESCE(transcript_text, ''),
		       COALESCE(visual_tags, '[]'::jsonb), processed_at, created_at
		FROM media_assets
		WHERE ocr_text ILIKE '%' || $1 || '%'
		   OR transcript_text ILIKE '%' || $1 || '%'
		ORDER BY processed_at DESC NULLS LAST, created_at DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query media by ocr: " + err.Error()})
		return
	}
	defer rows.Close()

	type ocrMediaItem struct {
		ID             string          `json:"id"`
		SHA256         string          `json:"sha256"`
		MIMEType       string          `json:"mime_type"`
		Size           int64           `json:"size"`
		ObjectPath     string          `json:"object_path"`
		OCRText        string          `json:"ocr_text"`
		TranscriptText string          `json:"transcript_text"`
		VisualTags     json.RawMessage `json:"visual_tags"`
		ProcessedAt    *time.Time      `json:"processed_at"`
		CreatedAt      time.Time       `json:"created_at"`
	}

	var items []ocrMediaItem
	for rows.Next() {
		var item ocrMediaItem
		if err := rows.Scan(&item.ID, &item.SHA256, &item.MIMEType, &item.Size, &item.ObjectPath,
			&item.OCRText, &item.TranscriptText, &item.VisualTags, &item.ProcessedAt, &item.CreatedAt); err == nil {
			items = append(items, item)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query": query,
		"total": len(items),
		"data":  items,
	})
}

// mediaDiffusion handles GET /api/v1/media/{id}/diffusion
func (s *Server) mediaDiffusion(w http.ResponseWriter, r *http.Request) {
	assetID := chi.URLParam(r, "id")
	if assetID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media asset id is required"})
		return
	}

	result, err := analysis.TraceImageDiffusion(r.Context(), s.Repo.DB, assetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// analyzeMediaAsset handles POST /api/v1/media/{id}/analyze
func (s *Server) analyzeMediaAsset(w http.ResponseWriter, r *http.Request) {
	assetID := chi.URLParam(r, "id")
	if assetID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media asset id is required"})
		return
	}

	analyzer := analysis.NewVisionAnalyzer(s.Repo.DB, s.ObjectRoot)
	result, err := analyzer.AnalyzeMediaAsset(r.Context(), assetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// conversationThreads handles GET /api/v1/conversations/{id}/threads
func (s *Server) conversationThreads(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")
	if convID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conversation id is required"})
		return
	}

	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT id::text, conversation_id::text, title, topic_category, stance, summary,
		       participant_count, message_count, started_at, ended_at, key_entities, created_at
		FROM dialogue_threads
		WHERE conversation_id = $1::uuid
		ORDER BY started_at DESC
	`, convID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query threads: " + err.Error()})
		return
	}
	defer rows.Close()

	var threads []analysis.DialogueThread
	for rows.Next() {
		var th analysis.DialogueThread
		var keyEnt json.RawMessage
		if err := rows.Scan(&th.ID, &th.ConversationID, &th.Title, &th.TopicCategory, &th.Stance, &th.Summary,
			&th.ParticipantCount, &th.MessageCount, &th.StartedAt, &th.EndedAt, &keyEnt, &th.CreatedAt); err == nil {
			_ = json.Unmarshal(keyEnt, &th.KeyEntities)
			threads = append(threads, th)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"total":           len(threads),
		"data":            threads,
	})
}

// disentangleConversation handles POST /api/v1/conversations/{id}/disentangle
func (s *Server) disentangleConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")
	if convID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conversation id is required"})
		return
	}

	limit := 80
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 200 {
			limit = val
		}
	}

	threads, err := analysis.DisentangleConversation(r.Context(), s.Repo.DB, convID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disentangle failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"total":           len(threads),
		"data":            threads,
	})
}

// threadDetail handles GET /api/v1/threads/{id}
func (s *Server) threadDetail(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	if threadID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "thread id is required"})
		return
	}

	var th analysis.DialogueThread
	var keyEnt json.RawMessage
	err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT id::text, conversation_id::text, title, topic_category, stance, summary,
		       participant_count, message_count, started_at, ended_at, key_entities, created_at
		FROM dialogue_threads
		WHERE id = $1::uuid
	`, threadID).Scan(&th.ID, &th.ConversationID, &th.Title, &th.TopicCategory, &th.Stance, &th.Summary,
		&th.ParticipantCount, &th.MessageCount, &th.StartedAt, &th.EndedAt, &keyEnt, &th.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread not found"})
		return
	}
	_ = json.Unmarshal(keyEnt, &th.KeyEntities)

	// Fetch thread messages
	msgRows, err := s.Repo.DB.Query(r.Context(), `
		SELECT tm.message_id::text, tm.sequence_index, tm.reply_to_message_id::text,
		       COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, ''),
		       COALESCE(m.raw_text, ''), m.sent_at
		FROM thread_messages tm
		JOIN messages m ON m.id = tm.message_id
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE tm.thread_id = $1::uuid
		ORDER BY tm.sequence_index ASC, m.sent_at ASC
	`, threadID)
	if err == nil {
		defer msgRows.Close()
		for msgRows.Next() {
			var d analysis.ThreadMessageDetail
			var replyTo *string
			var rStr string
			if err := msgRows.Scan(&d.MessageID, &d.SequenceIndex, &rStr, &d.SenderQQ, &d.SenderName, &d.Text, &d.SentAt); err == nil {
				if rStr != "" {
					replyTo = &rStr
				}
				d.ReplyToMessageID = replyTo
				th.Messages = append(th.Messages, d)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": th})
}

// personPersona handles GET /api/v1/persons/{id}/persona
func (s *Server) personPersona(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	if personID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person id is required"})
		return
	}

	// 1. Resolve canonical UUID and QQ identifier
	var realUUID, qq, displayName string
	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT p.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id::text = $1 OR pi.platform_user_id = $1
		LIMIT 1
	`, personID).Scan(&realUUID, &qq, &displayName)
	if realUUID == "" {
		realUUID = personID
	}

	// 2. Fetch existing cached inference from database
	var valBytes []byte
	var conf float64
	var createdAt time.Time
	var inferenceID string
	var evidenceEventIDs []string
	err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT id::text, value, confidence, created_at, COALESCE(evidence_event_ids::text[], ARRAY[]::text[])
		FROM inferences
		WHERE (subject_id = $1 OR subject_id = $2) AND attribute_type = 'persona_profile'
		ORDER BY created_at DESC
		LIMIT 1
	`, realUUID, qq).Scan(&inferenceID, &valBytes, &conf, &createdAt, &evidenceEventIDs)
	if err == nil {
		var profile analysis.PersonaProfile
		if err := json.Unmarshal(valBytes, &profile); err == nil {
			profile.InferenceID = inferenceID
			profile.PersonID = realUUID
			profile.QQ = qq
			profile.DisplayName = displayName
			profile.EvidenceMessageIDs = evidenceEventIDs
			profile.EvidenceCount = len(evidenceEventIDs)
			profile.AnalyzedAt = createdAt
			writeJSON(w, http.StatusOK, map[string]any{"data": profile})
			return
		}
	}

	// 3. Not analyzed yet: return null rather than blocking with LLM
	writeJSON(w, http.StatusOK, map[string]any{"data": nil})
}

// analyzePersonPersona handles POST /api/v1/persons/{id}/analyze-persona
func (s *Server) analyzePersonPersona(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	if personID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person id is required"})
		return
	}

	profile, err := analysis.AnalyzePersonPersona(r.Context(), s.Repo.DB, personID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "analyze persona: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// personFeedDynamics handles GET /api/v1/persons/{id}/feed-dynamics
func (s *Server) personFeedDynamics(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	if personID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person id is required"})
		return
	}

	// 1. Resolve canonical UUID and QQ identifier
	var realUUID, qq, displayName string
	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT p.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id::text = $1 OR pi.platform_user_id = $1
		LIMIT 1
	`, personID).Scan(&realUUID, &qq, &displayName)
	if realUUID == "" {
		realUUID = personID
	}

	// 2. Fetch existing cached inference
	var valBytes []byte
	var createdAt time.Time
	var inferenceID string
	err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT id::text, value, created_at
		FROM inferences
		WHERE (subject_id = $1 OR subject_id = $2) AND attribute_type = 'feed_dynamics'
		ORDER BY created_at DESC
		LIMIT 1
	`, realUUID, qq).Scan(&inferenceID, &valBytes, &createdAt)
	if err == nil {
		var res analysis.FeedDynamicsResult
		if err := json.Unmarshal(valBytes, &res); err == nil {
			res.InferenceID = inferenceID
			res.PersonID = realUUID
			res.QQ = qq
			res.DisplayName = displayName
			res.AnalyzedAt = createdAt
			writeJSON(w, http.StatusOK, map[string]any{"data": res})
			return
		}
	}

	// 3. Not analyzed yet: return null
	writeJSON(w, http.StatusOK, map[string]any{"data": nil})
}

// analyzePersonFeedDynamics handles POST /api/v1/persons/{id}/analyze-feeds
func (s *Server) analyzePersonFeedDynamics(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	if personID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person id is required"})
		return
	}

	res, err := analysis.AnalyzeFeedDynamics(r.Context(), s.Repo.DB, personID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "analyze feed dynamics: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}
