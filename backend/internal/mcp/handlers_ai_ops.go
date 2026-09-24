package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h *HandlerRegistry) handleInferences(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Subject       string  `json:"subject_id"`
		Review        string  `json:"review_status"`
		Attribute     string  `json:"attribute_type"`
		MinConfidence float64 `json:"min_confidence"`
		Limit         int     `json:"limit"`
		Offset        int     `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	var where []string
	var params []any
	idx := 1
	if input.Subject != "" {
		where = append(where, fmt.Sprintf("subject_id=$%d", idx))
		params = append(params, input.Subject)
		idx++
	}
	if input.Review != "" {
		where = append(where, fmt.Sprintf("review_status=$%d", idx))
		params = append(params, input.Review)
		idx++
	}
	if input.Attribute != "" {
		where = append(where, fmt.Sprintf("attribute_type=$%d", idx))
		params = append(params, input.Attribute)
		idx++
	}
	if input.MinConfidence > 0 {
		where = append(where, fmt.Sprintf("confidence >= $%d", idx))
		params = append(params, input.MinConfidence)
		idx++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM inferences"+whereSQL, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count inferences: %w", err)
	}
	query := `SELECT id::text,subject_id,attribute_type,value::text,confidence,method_version,evidence_event_ids,review_status,created_at
		FROM inferences` + whereSQL + ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("query inferences: %w", err)
	}
	defer rows.Close()
	type item struct {
		ID         string     `json:"id"`
		Subject    string     `json:"subject_id"`
		Attribute  string     `json:"attribute_type"`
		Value      string     `json:"value"`
		Confidence float64    `json:"confidence"`
		Method     string     `json:"method_version"`
		Evidence   []string   `json:"evidence_event_ids"`
		Review     string     `json:"review_status"`
		CreatedAt  *time.Time `json:"created_at"`
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Subject, &it.Attribute, &it.Value, &it.Confidence, &it.Method, &it.Evidence, &it.Review, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleAIRuns(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Status string `json:"status"`

		TaskType string `json:"task_type"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	var where []string
	var params []any
	idx := 1
	if input.Status != "" {
		where = append(where, fmt.Sprintf("status=$%d", idx))
		params = append(params, input.Status)
		idx++
	}
	if input.TaskType != "" {
		where = append(where, fmt.Sprintf("task_type=$%d", idx))
		params = append(params, input.TaskType)
		idx++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM ai_runs"+whereSQL, params...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT id::text,task_type,model_provider,model_name,status,evidence_pack_id::text,created_at,completed_at
		FROM ai_runs` + whereSQL + ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, TaskType, ModelProvider, ModelName, Status, EvidencePackID string
		CreatedAt, CompletedAt                                         *time.Time
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.TaskType, &it.ModelProvider, &it.ModelName, &it.Status, &it.EvidencePackID, &it.CreatedAt, &it.CompletedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleEvidencePacks(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Subject string `json:"subject"`
		Limit   int    `json:"limit"`
		Offset  int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	where := ""
	var params []any
	idx := 1
	if input.Subject != "" {
		where = " WHERE subject_id=$1"
		params = append(params, input.Subject)
		idx++
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM evidence_packs"+where, params...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT id::text,subject_id,scope::text,event_ids,redaction_policy,token_budget,created_at FROM evidence_packs` + where + ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, Subject, Scope, Redaction string
		EventIDs                      []string
		TokenBudget                   int
		CreatedAt                     *time.Time
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Subject, &it.Scope, &it.EventIDs, &it.Redaction, &it.TokenBudget, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleOperations(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	where := ""
	var params []any
	idx := 1
	if input.Status != "" {
		where = " WHERE status=$1"
		params = append(params, input.Status)
		idx++
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM operation_requests"+where, params...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT id::text,account_id::text,endpoint,parameters::text,preview_hash,status,created_by::text,confirmed_at,created_at FROM operation_requests` + where + ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, AccountID, Endpoint, Parameters, Hash, Status, CreatedBy string
		ConfirmedAt, CreatedAt                                       *time.Time
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.AccountID, &it.Endpoint, &it.Parameters, &it.Hash, &it.Status, &it.CreatedBy, &it.ConfirmedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleResearchWorkspaces(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		DraftOnly bool `json:"draft_only"`
		Limit     int  `json:"limit"`
		Offset    int  `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	where := ""
	var params []any
	idx := 1
	if input.DraftOnly {
		where = " WHERE is_draft=true"
	}
	var total int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM research_workspaces"+where, params...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT id::text,user_id::text,name,is_draft,version,created_at,updated_at FROM research_workspaces` + where + ` ORDER BY updated_at DESC LIMIT $` + fmt.Sprint(idx) + ` OFFSET $` + fmt.Sprint(idx+1)
	params = append(params, limit, offset)
	rows, err := h.DB.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, UserID, Name     string
		Draft                bool
		Version              int
		CreatedAt, UpdatedAt *time.Time
	}
	items := make([]item, 0, limit)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.UserID, &it.Name, &it.Draft, &it.Version, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleSourceConnections(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	rows, err := h.DB.Query(ctx, `SELECT sc.id::text,COALESCE(sc.account_id::text,''),sc.kind,sc.status,sc.last_event_at,COALESCE(sc.error,''),COALESCE(a.name,''),COALESCE(a.qq_uin,'')
		FROM source_connections sc LEFT JOIN napcat_accounts a ON a.id=sc.account_id ORDER BY sc.last_event_at DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, AccountID, Kind, Status, Error, AccountName, QQ string
		LastEventAt                                         *time.Time
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.AccountID, &it.Kind, &it.Status, &it.LastEventAt, &it.Error, &it.AccountName, &it.QQ); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return jsonResult(map[string]any{"connections": items, "count": len(items)})
}

func (h *HandlerRegistry) handleProposeInference(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		SubjectID        string          `json:"subject_id"`
		AttributeType    string          `json:"attribute_type"`
		Value            json.RawMessage `json:"value"`
		Confidence       float64         `json:"confidence"`
		EvidenceEventIDs []string        `json:"evidence_event_ids"`
		MethodVersion    string          `json:"method_version"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.AttributeType = strings.TrimSpace(input.AttributeType)
	if input.SubjectID == "" || input.AttributeType == "" {
		return errorResult("subject_id and attribute_type are required"), nil
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return errorResult("confidence must be between 0 and 1"), nil
	}
	if len(input.Value) == 0 {
		input.Value = json.RawMessage("null")
	}
	if input.MethodVersion == "" {
		input.MethodVersion = "mcp-proposed-v1"
	}
	reviewStatus := "proposed"

	var id string
	err := h.DB.QueryRow(ctx, `
		INSERT INTO inferences(subject_id, attribute_type, value, confidence, method_version, evidence_event_ids, review_status)
		VALUES($1, $2, $3::jsonb, $4, $5, COALESCE($6::uuid[], '{}'::uuid[]), $7)
		RETURNING id::text`,
		input.SubjectID,
		input.AttributeType,
		string(input.Value),
		input.Confidence,
		input.MethodVersion,
		input.EvidenceEventIDs,
		reviewStatus,
	).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errorResult("inference insertion returned no row"), nil
		}
		return errorResult(fmt.Sprintf("failed to save inference: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"id":                 id,
		"subject_id":         input.SubjectID,
		"attribute_type":     input.AttributeType,
		"confidence":         input.Confidence,
		"evidence_event_ids": input.EvidenceEventIDs,
		"method_version":     input.MethodVersion,
		"review_status":      reviewStatus,
		"status":             "proposed",
	})
}

func pagingDefaults(limit, offset, max int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > max {
		limit = max
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
