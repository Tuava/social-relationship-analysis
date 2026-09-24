package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

// handleMediaOCRSearch handles semantic and keyword OCR searches across media assets.
func (h *HandlerRegistry) handleMediaOCRSearch(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}
	if input.Query == "" {
		return errorResult("query cannot be empty"), nil
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 200 {
		input.Limit = 200
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	rows, err := h.DB.Query(ctx, `
		SELECT ma.id::text, ma.mime_type, ma.size, ma.object_path, ''::text,
		       ma.phash, COALESCE(ma.ocr_text, ''), COALESCE(ma.transcript_text, ''),
		       ma.visual_tags, ma.ocr_status, ma.created_at
		FROM media_assets ma
		WHERE ma.ocr_text ILIKE '%' || $1 || '%' OR ma.transcript_text ILIKE '%' || $1 || '%'
		ORDER BY ma.created_at DESC
		LIMIT $2 OFFSET $3
	`, input.Query, input.Limit, input.Offset)
	if err != nil {
		return errorResult("query ocr assets: " + err.Error()), nil
	}
	defer rows.Close()

	type assetItem struct {
		ID             string          `json:"id"`
		MimeType       string          `json:"mime_type"`
		FileSize       int64           `json:"file_size"`
		FilePath       string          `json:"file_path"`
		ThumbnailPath  string          `json:"thumbnail_path"`
		PHash          *string         `json:"phash"`
		OCRText        string          `json:"ocr_text"`
		TranscriptText string          `json:"transcript_text"`
		VisualTags     json.RawMessage `json:"visual_tags"`
		OCRStatus      string          `json:"ocr_status"`
		CreatedAt      string          `json:"created_at"`
	}

	var results []assetItem
	for rows.Next() {
		var item assetItem
		var phash *string
		var crTime interface{}
		if err := rows.Scan(&item.ID, &item.MimeType, &item.FileSize, &item.FilePath, &item.ThumbnailPath,
			&phash, &item.OCRText, &item.TranscriptText, &item.VisualTags, &item.OCRStatus, &crTime); err != nil {
			return errorResult("scan ocr asset: " + err.Error()), nil
		}
		item.PHash = phash
		item.CreatedAt = fmt.Sprintf("%v", crTime)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return errorResult("read ocr assets: " + err.Error()), nil
	}

	return jsonResult(map[string]interface{}{
		"query":   input.Query,
		"results": results,
		"total":   len(results),
	})
}

// handleImageDiffusionGraph traces image diffusion across conversations and persons.
func (h *HandlerRegistry) handleImageDiffusionGraph(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		MediaAssetID string `json:"media_asset_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}
	if input.MediaAssetID == "" {
		return errorResult("media_asset_id cannot be empty"), nil
	}

	diffGraph, err := analysis.TraceImageDiffusion(ctx, h.DB, input.MediaAssetID)
	if err != nil {
		return errorResult("trace image diffusion: " + err.Error()), nil
	}

	return jsonResult(diffGraph)
}

// handleDialogueThreads queries disentangled conversation threads.
func (h *HandlerRegistry) handleDialogueThreads(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ConversationID string `json:"conversation_id"`
		GroupID        string `json:"group_id"`
		Limit          int    `json:"limit"`
		Offset         int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}

	convID := input.ConversationID
	if convID == "" && input.GroupID != "" {
		_ = h.DB.QueryRow(ctx, `SELECT id::text FROM conversations WHERE platform_conversation_id = $1 LIMIT 1`, input.GroupID).Scan(&convID)
	}
	if convID == "" {
		return errorResult("conversation_id or group_id is required"), nil
	}

	rows, err := h.DB.Query(ctx, `
		SELECT dt.id::text, dt.title, dt.summary, dt.stance, dt.topic_category,
		       dt.key_entities, dt.started_at, dt.ended_at, dt.message_count, dt.participant_count
		FROM dialogue_threads dt
		WHERE dt.conversation_id = $1::uuid
		ORDER BY dt.started_at DESC
		LIMIT $2 OFFSET $3
	`, convID, input.Limit, input.Offset)
	if err != nil {
		return errorResult("query dialogue threads: " + err.Error()), nil
	}
	defer rows.Close()

	type threadItem struct {
		ID               string          `json:"id"`
		Title            string          `json:"title"`
		Summary          string          `json:"summary"`
		Stance           string          `json:"stance"`
		TopicCategory    string          `json:"topic_category"`
		KeyEntities      json.RawMessage `json:"key_entities"`
		StartedAt        *string         `json:"started_at"`
		EndedAt          *string         `json:"ended_at"`
		MessageCount     int             `json:"message_count"`
		ParticipantCount int             `json:"participant_count"`
	}

	var threads []threadItem
	for rows.Next() {
		var item threadItem
		var sAt, eAt interface{}
		if err := rows.Scan(&item.ID, &item.Title, &item.Summary, &item.Stance, &item.TopicCategory,
			&item.KeyEntities, &sAt, &eAt, &item.MessageCount, &item.ParticipantCount); err == nil {
			if sAt != nil {
				stStr := fmt.Sprintf("%v", sAt)
				item.StartedAt = &stStr
			}
			if eAt != nil {
				etStr := fmt.Sprintf("%v", eAt)
				item.EndedAt = &etStr
			}
			threads = append(threads, item)
		}
	}

	return jsonResult(map[string]interface{}{
		"conversation_id": convID,
		"threads":         threads,
		"total":           len(threads),
	})
}

// handleThreadDetail fetches detailed message trees of a thread.
func (h *HandlerRegistry) handleThreadDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}
	if input.ThreadID == "" {
		return errorResult("thread_id cannot be empty"), nil
	}

	var thread struct {
		ID            string          `json:"id"`
		Title         string          `json:"title"`
		Summary       string          `json:"summary"`
		Stance        string          `json:"stance"`
		TopicCategory string          `json:"topic_category"`
		KeyEntities   json.RawMessage `json:"key_entities"`
	}
	err := h.DB.QueryRow(ctx, `
		SELECT id::text, title, summary, stance, topic_category, key_entities
		FROM dialogue_threads
		WHERE id = $1::uuid
	`, input.ThreadID).Scan(&thread.ID, &thread.Title, &thread.Summary, &thread.Stance, &thread.TopicCategory, &thread.KeyEntities)
	if err != nil {
		return errorResult("thread not found: " + err.Error()), nil
	}

	rows, err := h.DB.Query(ctx, `
		SELECT tm.message_id::text, tm.sequence_order, tm.reply_to_message_id::text,
		       COALESCE(m.text, ''), COALESCE(p.display_name, pi.platform_user_id, '未知'),
		       pi.platform_user_id, m.sent_at
		FROM thread_messages tm
		JOIN messages m ON m.id = tm.message_id
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE tm.thread_id = $1::uuid
		ORDER BY tm.sequence_order ASC
	`, input.ThreadID)
	if err != nil {
		return errorResult("query thread messages: " + err.Error()), nil
	}
	defer rows.Close()

	type msgItem struct {
		MessageID  string  `json:"message_id"`
		Sequence   int     `json:"sequence"`
		ReplyToID  *string `json:"reply_to_id"`
		Text       string  `json:"text"`
		SenderName string  `json:"sender_name"`
		SenderQQ   *string `json:"sender_qq"`
		SentAt     string  `json:"sent_at"`
	}

	var messages []msgItem
	for rows.Next() {
		var item msgItem
		var replyID *string
		var senderQQ *string
		var sentAt interface{}
		if err := rows.Scan(&item.MessageID, &item.Sequence, &replyID, &item.Text, &item.SenderName, &senderQQ, &sentAt); err == nil {
			item.ReplyToID = replyID
			item.SenderQQ = senderQQ
			item.SentAt = fmt.Sprintf("%v", sentAt)
			messages = append(messages, item)
		}
	}

	return jsonResult(map[string]interface{}{
		"thread":   thread,
		"messages": messages,
		"total":    len(messages),
	})
}

// handlePersonPersona computes or retrieves persona profile and linguistic fingerprint.
func (h *HandlerRegistry) handlePersonPersona(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		PersonID       string `json:"person_id"`
		QQ             string `json:"qq"`
		Force          bool   `json:"force"`
		ForceRecompute bool   `json:"force_recompute"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}

	target := input.PersonID
	if target == "" {
		target = input.QQ
	}
	if target == "" {
		return errorResult("person_id or qq is required"), nil
	}

	force := input.Force || input.ForceRecompute

	if !force {
		var valJSON []byte
		err := h.DB.QueryRow(ctx, `
			SELECT i.value
			FROM inferences i
			LEFT JOIN persons p ON p.id::text = i.subject_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE (i.subject_id = $1 OR (p.id IS NOT NULL AND (p.id::text = $1 OR pi.platform_user_id = $1)))
			  AND i.attribute_type = 'persona_profile'
			ORDER BY i.created_at DESC
			LIMIT 1
		`, target).Scan(&valJSON)
		if err == nil && len(valJSON) > 0 {
			var cached analysis.PersonaProfile
			if err := json.Unmarshal(valJSON, &cached); err == nil {
				return jsonResult(cached)
			}
		}
	}

	profile, err := analysis.AnalyzePersonPersona(ctx, h.DB, target)
	if err != nil {
		return errorResult("analyze persona: " + err.Error()), nil
	}

	return jsonResult(profile)
}

// handleFeedDynamics computes or retrieves QZone feed temporal dynamics and circle structure.
func (h *HandlerRegistry) handleFeedDynamics(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		PersonID       string `json:"person_id"`
		QQ             string `json:"qq"`
		Force          bool   `json:"force"`
		ForceRecompute bool   `json:"force_recompute"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}

	target := input.PersonID
	if target == "" {
		target = input.QQ
	}
	if target == "" {
		return errorResult("person_id or qq is required"), nil
	}

	force := input.Force || input.ForceRecompute

	if !force {
		var valJSON []byte
		err := h.DB.QueryRow(ctx, `
			SELECT i.value
			FROM inferences i
			LEFT JOIN persons p ON p.id::text = i.subject_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE (i.subject_id = $1 OR (p.id IS NOT NULL AND (p.id::text = $1 OR pi.platform_user_id = $1)))
			  AND i.attribute_type = 'feed_dynamics'
			ORDER BY i.created_at DESC
			LIMIT 1
		`, target).Scan(&valJSON)
		if err == nil && len(valJSON) > 0 {
			var cached analysis.FeedDynamicsResult
			if err := json.Unmarshal(valJSON, &cached); err == nil {
				return jsonResult(cached)
			}
		}
	}

	dynamics, err := analysis.AnalyzeFeedDynamics(ctx, h.DB, target)
	if err != nil {
		return errorResult("analyze feed dynamics: " + err.Error()), nil
	}

	return jsonResult(dynamics)
}

// handleTriggerVisionAnalysis executes AI multimodal vision analysis on a media asset and persists OCR & scene intelligence.
func (h *HandlerRegistry) handleTriggerVisionAnalysis(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		MediaAssetID string `json:"media_asset_id"`
		URL          string `json:"url"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}

	// Accept either a direct URL (from chat messages) or a stored media asset UUID.
	target := input.MediaAssetID
	if target == "" {
		target = input.URL
	}
	if target == "" {
		return errorResult("either url or media_asset_id is required"), nil
	}

	analyzer := analysis.NewVisionAnalyzer(h.DB, "")
	result, err := analyzer.AnalyzeMediaAsset(ctx, target)
	if err != nil {
		return errorResult("vision analysis failed: " + err.Error()), nil
	}

	return jsonResult(result)
}

// handleTriggerDialogueDisentanglement triggers AI dialogue disentanglement on a group conversation and commits threads.
func (h *HandlerRegistry) handleTriggerDialogueDisentanglement(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		ConversationID string `json:"conversation_id"`
		GroupID        string `json:"group_id"`
		Limit          int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid args: " + err.Error()), nil
	}

	convID := input.ConversationID
	if convID == "" && input.GroupID != "" {
		_ = h.DB.QueryRow(ctx, `SELECT id::text FROM conversations WHERE platform_conversation_id = $1 LIMIT 1`, input.GroupID).Scan(&convID)
	}
	if convID == "" {
		return errorResult("conversation_id or group_id is required"), nil
	}

	if input.Limit <= 0 {
		input.Limit = 60
	}

	threads, err := analysis.DisentangleConversation(ctx, h.DB, convID, input.Limit)
	if err != nil {
		return errorResult("dialogue disentanglement failed: " + err.Error()), nil
	}

	return jsonResult(map[string]any{
		"conversation_id": convID,
		"threads_created": len(threads),
		"threads":         threads,
	})
}
