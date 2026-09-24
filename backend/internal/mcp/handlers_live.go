package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

func (h *HandlerRegistry) handleNapcatRead(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		AccountID  string          `json:"account_id"`
		Endpoint   string          `json:"endpoint"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.Endpoint = "/" + strings.TrimPrefix(strings.TrimSpace(input.Endpoint), "/")
	if input.Endpoint == "/" {
		return errorResult("endpoint is required"), nil
	}
	var capability *struct {
		Endpoint string
		ReadOnly bool
	}
	for _, cap := range h.Capabilities {
		if cap.Endpoint == input.Endpoint {
			capability = &struct {
				Endpoint string
				ReadOnly bool
			}{Endpoint: cap.Endpoint, ReadOnly: cap.ReadOnly}
			break
		}
	}
	if capability == nil {
		return errorResult("endpoint is not in the NapCat capability catalog"), nil
	}
	if !capability.ReadOnly {
		return errorResult("endpoint is not read-only; use sra_operation_preview instead"), nil
	}
	accountID := strings.TrimSpace(input.AccountID)
	if accountID == "" {
		if err := h.DB.QueryRow(ctx, `SELECT id::text FROM napcat_accounts WHERE enabled=true AND http_url<>'' ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
			return errorResult("no enabled NapCat HTTP account is available"), nil
		}
	}
	var httpURL, httpToken string
	if err := h.DB.QueryRow(ctx, `SELECT http_url,http_token FROM napcat_accounts WHERE id=$1`, accountID).Scan(&httpURL, &httpToken); err != nil {
		return errorResult("NapCat account not found"), nil
	}
	if h.Repo != nil {
		var decryptErr error
		httpToken, decryptErr = h.Repo.DecryptCredential(httpToken)
		if decryptErr != nil {
			return errorResult("NapCat credential unavailable: " + decryptErr.Error()), nil
		}
	}
	if httpURL == "" {
		return errorResult("NapCat account has no HTTP URL"), nil
	}
	params := any(nil)
	if len(input.Parameters) > 0 {
		if err := json.Unmarshal(input.Parameters, &params); err != nil {
			return errorResult("parameters must be valid JSON: " + err.Error()), nil
		}
	}
	client := napcat.NewHTTPClient(httpURL, httpToken)
	response, err := client.Call(ctx, input.Endpoint, params)
	if err != nil {
		return errorResult(fmt.Sprintf("NapCat read failed: %v", err)), nil
	}
	rawID := ""
	if id, saveErr := h.Repo.SaveRaw(ctx, accountID, "napcat_http", strings.TrimPrefix(input.Endpoint, "/")+":mcp", response); saveErr == nil {
		rawID = id
	}
	var value any
	if err := json.Unmarshal(response, &value); err != nil {
		return errorResult("NapCat returned non-JSON payload"), nil
	}
	return jsonResult(map[string]any{
		"account_id": accountID, "endpoint": input.Endpoint, "raw_record_id": rawID, "data": value,
	})
}

func (h *HandlerRegistry) handleOperationPreview(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		AccountID  string          `json:"account_id"`
		Endpoint   string          `json:"endpoint"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.Endpoint = "/" + strings.TrimPrefix(strings.TrimSpace(input.Endpoint), "/")
	if input.AccountID == "" || input.Endpoint == "/" {
		return errorResult("account_id and endpoint are required"), nil
	}
	var cap *struct {
		Endpoint string
		ReadOnly bool
	}
	for _, item := range h.Capabilities {
		if item.Endpoint == input.Endpoint {
			cap = &struct {
				Endpoint string
				ReadOnly bool
			}{Endpoint: item.Endpoint, ReadOnly: item.ReadOnly}
			break
		}
	}
	if cap == nil {
		return errorResult("endpoint is not in the NapCat capability catalog"), nil
	}
	if cap.ReadOnly {
		return errorResult("read-only endpoint does not require the operation center"), nil
	}
	params := any(nil)
	if len(input.Parameters) > 0 {
		if err := json.Unmarshal(input.Parameters, &params); err != nil {
			return errorResult("parameters must be valid JSON: " + err.Error()), nil
		}
	}
	hash, err := persistence.PreviewHash(input.Endpoint, params)
	if err != nil {
		return errorResult("failed to hash operation preview: " + err.Error()), nil
	}
	op, err := h.Repo.CreateOperation(ctx, input.AccountID, input.Endpoint, params, hash, "")
	if err != nil {
		return errorResult("failed to create operation preview: " + err.Error()), nil
	}
	return jsonResult(map[string]any{
		"operation": op, "status": "pending_confirmation", "note": "operation requires human confirmation before execution",
	})
}
