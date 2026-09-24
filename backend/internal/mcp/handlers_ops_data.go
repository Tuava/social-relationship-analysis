package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (h *HandlerRegistry) handleMediaStats(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var paused bool
	var concurrency int
	if err := h.DB.QueryRow(ctx, `SELECT paused,concurrency FROM media_download_settings WHERE singleton=true`).Scan(&paused, &concurrency); err != nil {
		return nil, fmt.Errorf("read media settings: %w", err)
	}
	var assetCount, assetBytes int64
	if err := h.DB.QueryRow(ctx, `SELECT count(*),COALESCE(sum(size),0) FROM media_assets`).Scan(&assetCount, &assetBytes); err != nil {
		return nil, fmt.Errorf("read media assets: %w", err)
	}
	rows, err := h.DB.Query(ctx, `
		SELECT p.media_kind,p.enabled,p.priority,
		       count(r.id),count(r.id) FILTER(WHERE r.status='pending'),
		       count(r.id) FILTER(WHERE r.status='downloading'),
		       count(r.id) FILTER(WHERE r.status='completed'),
		       count(r.id) FILTER(WHERE r.status='failed'),
		       COALESCE((SELECT sum(a.size) FROM media_assets a WHERE a.id IN (
		           SELECT DISTINCT mr.asset_id FROM media_references mr WHERE mr.media_kind=p.media_kind AND mr.status='completed' AND mr.asset_id IS NOT NULL
		       )),0)
		FROM media_download_policies p
		LEFT JOIN media_references r ON r.media_kind=p.media_kind
		GROUP BY p.media_kind,p.enabled,p.priority ORDER BY p.priority DESC`)
	if err != nil {
		return nil, fmt.Errorf("read media policies: %w", err)
	}
	defer rows.Close()
	type policyItem struct {
		Kind        string `json:"kind"`
		Enabled     bool   `json:"enabled"`
		Priority    int    `json:"priority"`
		Total       int64  `json:"total"`
		Pending     int64  `json:"pending"`
		Downloading int64  `json:"downloading"`
		Completed   int64  `json:"completed"`
		Failed      int64  `json:"failed"`
		Bytes       int64  `json:"bytes"`
	}
	policies := make([]policyItem, 0)
	for rows.Next() {
		var item policyItem
		if err := rows.Scan(&item.Kind, &item.Enabled, &item.Priority, &item.Total, &item.Pending,
			&item.Downloading, &item.Completed, &item.Failed, &item.Bytes); err != nil {
			return nil, err
		}
		policies = append(policies, item)
	}
	return jsonResult(map[string]any{
		"paused": paused, "concurrency": concurrency,
		"asset_count": assetCount, "asset_bytes": assetBytes, "policies": policies,
	})
}

type mediaReferencesArgs struct {
	Kind               string `json:"kind"`
	Status             string `json:"status"`
	Query              string `json:"query"`
	IncludeRecognition bool   `json:"include_recognition"`
	Limit              int    `json:"limit"`
	Offset             int    `json:"offset"`
}

func (h *HandlerRegistry) handleMediaReferences(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input mediaReferencesArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	var where []string
	var params []any
	idx := 1
	if input.Kind != "" {
		where = append(where, fmt.Sprintf("r.media_kind=$%d", idx))
		params = append(params, input.Kind)
		idx++
	}
	if input.Status != "" {
		where = append(where, fmt.Sprintf("r.status=$%d", idx))
		params = append(params, input.Status)
		idx++
	}
	if input.Query != "" {
		where = append(where, fmt.Sprintf("(r.original_filename ILIKE '%%' || $%d || '%%' OR r.source_ref ILIKE '%%' || $%d || '%%' OR COALESCE(rpi.platform_user_id,'')=$%d OR COALESCE(rp.display_name,'') ILIKE '%%' || $%d || '%%')", idx, idx, idx, idx))
		params = append(params, input.Query)
		idx++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	countSQL := `SELECT COUNT(*) FROM media_references r
		LEFT JOIN persons rp ON rp.id=r.person_id
		LEFT JOIN person_identifiers rpi ON rpi.person_id=rp.id AND rpi.platform='qq'` + whereSQL
	var total int
	if err := h.DB.QueryRow(ctx, countSQL, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count media references: %w", err)
	}

	query := `SELECT r.id::text,r.media_kind,r.status,COALESCE(r.original_filename,''),COALESCE(r.source_ref,''),
		COALESCE(r.relation_depth,99),COALESCE(r.priority_reason,''),COALESCE(r.priority_boost,0),
		COALESCE(r.asset_id::text,''),COALESCE(a.mime_type,''),COALESCE(a.size,0),
		COALESCE(rpi.platform_user_id,''),COALESCE(rp.display_name,''),COALESCE(r.last_error,''),
		a.ocr_text,a.transcript_text,a.processed_at,r.created_at
		FROM media_references r
		LEFT JOIN media_assets a ON a.id=r.asset_id
		LEFT JOIN persons rp ON rp.id=r.person_id
		LEFT JOIN person_identifiers rpi ON rpi.person_id=rp.id AND rpi.platform='qq'` + whereSQL +
		` ORDER BY CASE r.status WHEN 'downloading' THEN 0 WHEN 'pending' THEN 1 WHEN 'failed' THEN 2 ELSE 3 END,
			r.created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query media references: %w", err)
	}
	defer rows.Close()

	type item struct {
		ID             string     `json:"id"`
		Kind           string     `json:"kind"`
		Status         string     `json:"status"`
		Filename       string     `json:"filename"`
		SourceRef      string     `json:"source_ref"`
		RelationDepth  int        `json:"relation_depth"`
		PriorityReason string     `json:"priority_reason"`
		PriorityBoost  int        `json:"priority_boost"`
		AssetID        string     `json:"asset_id"`
		MimeType       string     `json:"mime_type"`
		Size           int64      `json:"size"`
		QQ             string     `json:"qq"`
		PersonName     string     `json:"person_name"`
		LastError      string     `json:"last_error"`
		OcrText        *string    `json:"ocr_text,omitempty"`
		TranscriptText *string    `json:"transcript_text,omitempty"`
		ProcessedAt    *time.Time `json:"processed_at,omitempty"`
		CreatedAt      *time.Time `json:"created_at"`
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		var ocrText, transcriptText *string
		var processedAt *time.Time
		if err := rows.Scan(&it.ID, &it.Kind, &it.Status, &it.Filename, &it.SourceRef, &it.RelationDepth,
			&it.PriorityReason, &it.PriorityBoost, &it.AssetID, &it.MimeType, &it.Size, &it.QQ,
			&it.PersonName, &it.LastError, &ocrText, &transcriptText, &processedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		if input.IncludeRecognition {
			it.OcrText = ocrText
			it.TranscriptText = transcriptText
			it.ProcessedAt = processedAt
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleAccounts(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT a.id::text,a.name,a.qq_uin,a.http_url,a.ws_url,a.enabled,a.status,a.last_connected_at,
		       COALESCE(q.status,''),COALESCE(q.last_event_at, a.last_connected_at)
		FROM napcat_accounts a
		LEFT JOIN qzone_connections q ON q.account_id=a.id
		ORDER BY a.created_at`)
	if err != nil {
		return nil, fmt.Errorf("read accounts: %w", err)
	}
	defer rows.Close()
	type accountItem struct {
		ID              string     `json:"id"`
		Name            string     `json:"name"`
		QQ              string     `json:"qq"`
		HTTPURL         string     `json:"http_url"`
		WSURL           string     `json:"ws_url"`
		Enabled         bool       `json:"enabled"`
		Status          string     `json:"status"`
		LastConnectedAt *time.Time `json:"last_connected_at"`
		QZoneStatus     string     `json:"qzone_status"`
		LastEventAt     *time.Time `json:"last_event_at"`
	}
	items := make([]accountItem, 0)
	for rows.Next() {
		var it accountItem
		if err := rows.Scan(&it.ID, &it.Name, &it.QQ, &it.HTTPURL, &it.WSURL, &it.Enabled, &it.Status,
			&it.LastConnectedAt, &it.QZoneStatus, &it.LastEventAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(map[string]any{"accounts": items, "count": len(items)})
}

type collectionRunsArgs struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

func (h *HandlerRegistry) handleCollectionRuns(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input collectionRunsArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	var where []string
	var params []any
	idx := 1
	if input.Type != "" {
		where = append(where, fmt.Sprintf("type=$%d", idx))
		params = append(params, input.Type)
		idx++
	}
	if input.Status != "" {
		where = append(where, fmt.Sprintf("status=$%d", idx))
		params = append(params, input.Status)
		idx++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	countSQL := "SELECT COUNT(*) FROM collection_runs" + whereSQL
	var total int
	if err := h.DB.QueryRow(ctx, countSQL, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count collection runs: %w", err)
	}
	query := `SELECT id::text,type,status,progress,COALESCE(error,''),COALESCE(account_id::text,''),started_at,ended_at,created_at
		FROM collection_runs` + whereSQL + ` ORDER BY started_at DESC NULLS LAST, created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query collection runs: %w", err)
	}
	defer rows.Close()
	type item struct {
		ID        string     `json:"id"`
		Type      string     `json:"type"`
		Status    string     `json:"status"`
		Progress  int        `json:"progress"`
		Error     string     `json:"error"`
		AccountID string     `json:"account_id"`
		StartedAt *time.Time `json:"started_at"`
		EndedAt   *time.Time `json:"ended_at"`
		CreatedAt *time.Time `json:"created_at"`
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Type, &it.Status, &it.Progress, &it.Error, &it.AccountID,
			&it.StartedAt, &it.EndedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

type collectionCandidatesArgs struct {
	RunID  string `json:"run_id"`
	State  string `json:"state"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

func (h *HandlerRegistry) handleCollectionCandidates(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input collectionCandidatesArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	runID := input.RunID
	if runID == "" {
		if err := h.DB.QueryRow(ctx, `SELECT id FROM collection_runs ORDER BY started_at DESC NULLS LAST, created_at DESC LIMIT 1`).Scan(&runID); err != nil {
			return errorResult("no collection run found"), nil
		}
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	where := "run_id=$1"
	params := []any{runID}
	idx := 2
	if input.State != "" {
		where += fmt.Sprintf(" AND state=$%d", idx)
		params = append(params, input.State)
		idx++
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM collection_candidates WHERE "+where, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count candidates: %w", err)
	}
	query := `SELECT entity_type,entity_id,state,depth,COALESCE(priority,0),discovery_count,COALESCE(name,''),COALESCE(avatar_url,''),last_discovered_at
		FROM collection_candidates WHERE ` + where + ` ORDER BY priority DESC, depth, last_discovered_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query candidates: %w", err)
	}
	defer rows.Close()
	type item struct {
		EntityType string     `json:"entity_type"`
		EntityID   string     `json:"entity_id"`
		State      string     `json:"state"`
		Depth      int        `json:"depth"`
		Priority   float64    `json:"priority"`
		Discovery  int        `json:"discovery_count"`
		Name       string     `json:"name"`
		AvatarURL  string     `json:"avatar_url"`
		LastSeen   *time.Time `json:"last_discovered_at"`
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.EntityType, &it.EntityID, &it.State, &it.Depth, &it.Priority, &it.Discovery,
			&it.Name, &it.AvatarURL, &it.LastSeen); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(map[string]any{"run_id": runID, "total": total, "limit": limit, "offset": offset, "candidates": items})
}

func (h *HandlerRegistry) handleCapabilities(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Query    string `json:"query"`
		ReadOnly *bool  `json:"read_only"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	all := h.Capabilities
	if len(all) == 0 {
		return jsonResult(map[string]any{"total": 0, "capabilities": []any{}, "note": "NapCat capability catalog is not loaded"})
	}
	filtered := make([]map[string]any, 0, limit)
	total := 0
	for _, cap := range all {
		if input.Query != "" && !strings.Contains(strings.ToLower(cap.Endpoint+" "+cap.Summary+" "+cap.Tag), strings.ToLower(input.Query)) {
			continue
		}
		if input.ReadOnly != nil && cap.ReadOnly != *input.ReadOnly {
			continue
		}
		total++
		if total > offset && len(filtered) < limit {
			filtered = append(filtered, map[string]any{
				"endpoint": cap.Endpoint, "method": cap.Method, "summary": cap.Summary,
				"tag": cap.Tag, "read_only": cap.ReadOnly,
				"requires_confirmation": cap.RequiresConfirmation, "implemented": cap.Implemented,
				"requires_admin": cap.RequiresAdmin,
			})
		}
	}
	return jsonResult(map[string]any{
		"total": total, "limit": limit, "offset": offset,
		"capabilities": filtered, "has_more": offset+len(filtered) < total,
	})
}
