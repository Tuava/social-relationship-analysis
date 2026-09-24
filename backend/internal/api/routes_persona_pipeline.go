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

// listPersonaPipelines handles GET /api/v1/pipelines/batch-persona
func (s *Server) listPersonaPipelines(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	offset := 0
	if oStr := r.URL.Query().Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT 
			id::text, title, status, scope_type, COALESCE(scope_target, ''),
			total_targets, completed_targets, failed_targets, concurrency,
			auto_retry, max_retries, retry_backoff_seconds, queue_wait_seconds, COALESCE(error_message, ''), created_at, updated_at
		FROM batch_persona_pipelines
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	type PipelineSummary struct {
		ID                  string    `json:"id"`
		Title               string    `json:"title"`
		Status              string    `json:"status"`
		ScopeType           string    `json:"scope_type"`
		ScopeTarget         string    `json:"scope_target"`
		TotalTargets        int       `json:"total_targets"`
		CompletedTargets    int       `json:"completed_targets"`
		FailedTargets       int       `json:"failed_targets"`
		Concurrency         int       `json:"concurrency"`
		AutoRetry           bool      `json:"auto_retry"`
		MaxRetries          int       `json:"max_retries"`
		RetryBackoffSeconds int       `json:"retry_backoff_seconds"`
		QueueWaitSeconds    int       `json:"queue_wait_seconds"`
		ErrorMessage        string    `json:"error_message"`
		CreatedAt           time.Time `json:"created_at"`
		UpdatedAt           time.Time `json:"updated_at"`
	}

	list := make([]PipelineSummary, 0)
	for rows.Next() {
		var p PipelineSummary
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Status, &p.ScopeType, &p.ScopeTarget,
			&p.TotalTargets, &p.CompletedTargets, &p.FailedTargets, &p.Concurrency,
			&p.AutoRetry, &p.MaxRetries, &p.RetryBackoffSeconds, &p.QueueWaitSeconds, &p.ErrorMessage, &p.CreatedAt, &p.UpdatedAt,
		); err == nil {
			list = append(list, p)
		}
	}

	var total int
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM batch_persona_pipelines`).Scan(&total)

	writeJSON(w, http.StatusOK, map[string]any{
		"data":   list,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// createPersonaPipeline handles POST /api/v1/pipelines/batch-persona
func (s *Server) createPersonaPipeline(w http.ResponseWriter, r *http.Request) {
	var in analysis.PipelineCreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if s.PipelineMgr == nil {
		s.PipelineMgr = analysis.NewPipelineManager(s.Repo.DB)
	}

	pipelineID, err := s.PipelineMgr.CreatePipeline(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"pipeline_id": pipelineID,
		"status":      "queued",
		"message":     "批量画像研判流水线已在后台成功调度启动",
	})
}

// getPersonaPipeline handles GET /api/v1/pipelines/batch-persona/{id}
func (s *Server) getPersonaPipeline(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pipeline id"})
		return
	}
	itemLimit := 50
	if raw := r.URL.Query().Get("item_limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			itemLimit = parsed
		}
	}
	if itemLimit > 200 {
		itemLimit = 200
	}
	itemOffset := 0
	if raw := r.URL.Query().Get("item_offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			itemOffset = parsed
		}
	}

	var p struct {
		ID                  string          `json:"id"`
		Title               string          `json:"title"`
		Status              string          `json:"status"`
		ScopeType           string          `json:"scope_type"`
		ScopeTarget         string          `json:"scope_target"`
		TotalTargets        int             `json:"total_targets"`
		CompletedTargets    int             `json:"completed_targets"`
		FailedTargets       int             `json:"failed_targets"`
		Concurrency         int             `json:"concurrency"`
		AutoRetry           bool            `json:"auto_retry"`
		MaxRetries          int             `json:"max_retries"`
		RetryBackoffSeconds int             `json:"retry_backoff_seconds"`
		QueueWaitSeconds    int             `json:"queue_wait_seconds"`
		SummaryData         json.RawMessage `json:"summary_data"`
		ErrorMessage        string          `json:"error_message"`
		CreatedAt           time.Time       `json:"created_at"`
		UpdatedAt           time.Time       `json:"updated_at"`
	}

	var rawSummary []byte
	err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT 
			id::text, title, status, scope_type, COALESCE(scope_target, ''),
			total_targets, completed_targets, failed_targets, concurrency,
			auto_retry, max_retries, retry_backoff_seconds, queue_wait_seconds, summary_data, COALESCE(error_message, ''), created_at, updated_at
		FROM batch_persona_pipelines
		WHERE id = $1::uuid
	`, idStr).Scan(
		&p.ID, &p.Title, &p.Status, &p.ScopeType, &p.ScopeTarget,
		&p.TotalTargets, &p.CompletedTargets, &p.FailedTargets, &p.Concurrency,
		&p.AutoRetry, &p.MaxRetries, &p.RetryBackoffSeconds, &p.QueueWaitSeconds, &rawSummary, &p.ErrorMessage, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
		return
	}
	if len(rawSummary) > 0 {
		p.SummaryData = rawSummary
	}

	// Fetch item details
	itemRows, err := s.Repo.DB.Query(r.Context(), `
		SELECT 
			bpi.id::text, bpi.person_id::text, bpi.status, bpi.attempts, 
			COALESCE(bpi.error_message, ''), bpi.started_at, bpi.finished_at,
			COALESCE(p.display_name, ''), COALESCE(pi.platform_user_id, '')
		FROM batch_pipeline_items bpi
		INNER JOIN persons p ON p.id = bpi.person_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE bpi.pipeline_id = $1::uuid
		ORDER BY bpi.id ASC
		LIMIT $2 OFFSET $3
	`, idStr, itemLimit, itemOffset)

	type PipelineItemDetail struct {
		ID           string     `json:"id"`
		PersonID     string     `json:"person_id"`
		DisplayName  string     `json:"display_name"`
		QQ           string     `json:"qq"`
		Status       string     `json:"status"`
		Attempts     int        `json:"attempts"`
		ErrorMessage string     `json:"error_message"`
		StartedAt    *time.Time `json:"started_at"`
		FinishedAt   *time.Time `json:"finished_at"`
	}

	items := make([]PipelineItemDetail, 0)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var it PipelineItemDetail
			if err := itemRows.Scan(
				&it.ID, &it.PersonID, &it.Status, &it.Attempts,
				&it.ErrorMessage, &it.StartedAt, &it.FinishedAt,
				&it.DisplayName, &it.QQ,
			); err == nil {
				items = append(items, it)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":         p,
		"items":        items,
		"items_limit":  itemLimit,
		"items_offset": itemOffset,
	})
}

// cancelPersonaPipeline handles POST /api/v1/pipelines/batch-persona/{id}/cancel
func (s *Server) cancelPersonaPipeline(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pipeline id"})
		return
	}

	if s.PipelineMgr == nil {
		s.PipelineMgr = analysis.NewPipelineManager(s.Repo.DB)
	}

	cancelled := s.PipelineMgr.CancelPipeline(idStr)
	writeJSON(w, http.StatusOK, map[string]any{
		"cancelled": cancelled,
		"message":   "流水线任务已中止",
	})
}

func (s *Server) retryFailedPersonaPipeline(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pipeline id"})
		return
	}
	if s.PipelineMgr == nil {
		s.PipelineMgr = analysis.NewPipelineManager(s.Repo.DB)
	}
	if err := s.PipelineMgr.RetryFailedPipeline(r.Context(), idStr); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipeline_id": idStr, "status": "queued", "message": "失败目标已重新排队"})
}

// getPersonaPipelineSummary handles GET /api/v1/pipelines/batch-persona/{id}/summary
func (s *Server) getPersonaPipelineSummary(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pipeline id"})
		return
	}

	var rawSummary []byte
	err := s.Repo.DB.QueryRow(r.Context(), `
		SELECT summary_data 
		FROM batch_persona_pipelines 
		WHERE id = $1::uuid
	`, idStr).Scan(&rawSummary)
	if err != nil || len(rawSummary) == 0 {
		if s.PipelineMgr == nil {
			s.PipelineMgr = analysis.NewPipelineManager(s.Repo.DB)
		}
		summary, cErr := s.PipelineMgr.CompileTaskSummary(r.Context(), idStr)
		if cErr != nil {
			writeError(w, http.StatusInternalServerError, cErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": summary})
		return
	}

	var summaryData map[string]any
	_ = json.Unmarshal(rawSummary, &summaryData)
	writeJSON(w, http.StatusOK, map[string]any{"data": summaryData})
}
