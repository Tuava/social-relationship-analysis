package mcp

import (
	"context"
	"encoding/json"
	"time"
)

func (h *HandlerRegistry) handleEgoNetworks(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	var total int
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM ego_networks`).Scan(&total)
	rows, err := h.DB.Query(ctx, `SELECT id::text,target_qq,depth,status,node_count,edge_count,truncated,processed_event_count,created_at,completed_at FROM ego_networks ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, TargetQQ, Status                   string
		Depth, NodeCount, EdgeCount, Processed int
		Truncated                              bool
		CreatedAt, CompletedAt                 *time.Time
	}
	items := []item{}
	for rows.Next() {
		var it item
		if rows.Scan(&it.ID, &it.TargetQQ, &it.Depth, &it.Status, &it.NodeCount, &it.EdgeCount, &it.Truncated, &it.Processed, &it.CreatedAt, &it.CompletedAt) == nil {
			items = append(items, it)
		}
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleEgoNetworkDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		NetworkID    string `json:"network_id"`
		IncludeNodes bool   `json:"include_nodes"`
		IncludeEdges bool   `json:"include_edges"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var target, status string
	var depth, nodeCount, edgeCount int
	var truncated bool
	var createdAt, completedAt *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT target_qq,depth,status,node_count,edge_count,truncated,created_at,completed_at FROM ego_networks WHERE id=$1`, input.NetworkID).Scan(&target, &depth, &status, &nodeCount, &edgeCount, &truncated, &createdAt, &completedAt); err != nil {
		return errorResult("ego network not found"), nil
	}
	result := map[string]any{"id": input.NetworkID, "target_qq": target, "depth": depth, "status": status, "node_count": nodeCount, "edge_count": edgeCount, "truncated": truncated, "created_at": createdAt, "completed_at": completedAt}
	if input.IncludeNodes {
		rows, err := h.DB.Query(ctx, `SELECT node_key,node_type,label,metadata FROM ego_network_nodes WHERE network_id=$1`, input.NetworkID)
		if err == nil {
			defer rows.Close()
			nodes := []map[string]any{}
			for rows.Next() {
				var key, typ, label string
				var meta []byte
				if rows.Scan(&key, &typ, &label, &meta) == nil {
					var m any
					_ = json.Unmarshal(meta, &m)
					nodes = append(nodes, map[string]any{"key": key, "type": typ, "label": label, "metadata": m})
				}
			}
			result["nodes"] = nodes
		}
	}
	if input.IncludeEdges {
		rows, err := h.DB.Query(ctx, `SELECT source_key,target_key,relation_type,weight,event_count,evidence_ids,first_seen,last_seen FROM ego_network_edges WHERE network_id=$1`, input.NetworkID)
		if err == nil {
			defer rows.Close()
			edges := []map[string]any{}
			for rows.Next() {
				var s, t, rel string
				var weight float64
				var count int
				var ev []string
				var first, last *time.Time
				if rows.Scan(&s, &t, &rel, &weight, &count, &ev, &first, &last) == nil {
					edges = append(edges, map[string]any{"source": s, "target": t, "relation_type": rel, "weight": weight, "event_count": count, "evidence_ids": ev, "first_seen": first, "last_seen": last})
				}
			}
			result["edges"] = edges
		}
	}
	return jsonResult(result)
}

func (h *HandlerRegistry) handleRealtimeStats(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var totalEvents, totalMessages, totalRaw int64
	var lastEvent, lastMessage, lastRaw *time.Time
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM relation_events`).Scan(&totalEvents)
	_ = h.DB.QueryRow(ctx, `SELECT max(occurred_at) FROM relation_events`).Scan(&lastEvent)
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&totalMessages)
	_ = h.DB.QueryRow(ctx, `SELECT max(sent_at) FROM messages`).Scan(&lastMessage)
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM raw_records`).Scan(&totalRaw)
	_ = h.DB.QueryRow(ctx, `SELECT max(collected_at) FROM raw_records`).Scan(&lastRaw)
	return jsonResult(map[string]any{"relation_events": totalEvents, "last_relation_event": lastEvent, "messages": totalMessages, "last_message": lastMessage, "raw_records": totalRaw, "last_raw_record": lastRaw})
}

func (h *HandlerRegistry) handleQZoneConnections(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	rows, err := h.DB.Query(ctx, `SELECT q.id::text,q.account_id::text,q.http_url,q.ws_url,q.enabled,q.status,q.last_connected_at,q.last_event_at,q.last_error,a.name,a.qq_uin FROM qzone_connections q LEFT JOIN napcat_accounts a ON a.id=q.account_id ORDER BY q.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		ID, AccountID, HTTPURL, WSURL, Status, Error, AccountName, QQ string
		Enabled                                                       bool
		LastConnectedAt, LastEventAt                                  *time.Time
	}
	items := []item{}
	for rows.Next() {
		var it item
		var errPtr *string
		if rows.Scan(&it.ID, &it.AccountID, &it.HTTPURL, &it.WSURL, &it.Enabled, &it.Status, &it.LastConnectedAt, &it.LastEventAt, &errPtr, &it.AccountName, &it.QQ) == nil {
			if errPtr != nil {
				it.Error = *errPtr
			}
			items = append(items, it)
		}
	}
	return jsonResult(map[string]any{"connections": items, "count": len(items)})
}

func (h *HandlerRegistry) handleCollectionModules(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	runID := input.RunID
	if runID == "" {
		if err := h.DB.QueryRow(ctx, `SELECT id FROM collection_runs ORDER BY started_at DESC NULLS LAST,created_at DESC LIMIT 1`).Scan(&runID); err != nil {
			return errorResult("no collection run found"), nil
		}
	}
	rows, err := h.DB.Query(ctx, `SELECT module,status,pages_completed,pages_total,records_collected,error,started_at,updated_at,ended_at FROM collection_run_modules WHERE run_id=$1 ORDER BY module`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		Module, Status, Error         string
		PagesCompleted, PagesTotal    *int
		Records                       int64
		StartedAt, UpdatedAt, EndedAt *time.Time
	}
	items := []item{}
	for rows.Next() {
		var it item
		var errPtr *string
		if rows.Scan(&it.Module, &it.Status, &it.PagesCompleted, &it.PagesTotal, &it.Records, &errPtr, &it.StartedAt, &it.UpdatedAt, &it.EndedAt) == nil {
			if errPtr != nil {
				it.Error = *errPtr
			}
			items = append(items, it)
		}
	}
	return jsonResult(map[string]any{"run_id": runID, "modules": items, "count": len(items)})
}

func (h *HandlerRegistry) handleCollectionEvents(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		RunID  string `json:"run_id"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	runID := input.RunID
	if runID == "" {
		if err := h.DB.QueryRow(ctx, `SELECT id FROM collection_runs ORDER BY started_at DESC NULLS LAST,created_at DESC LIMIT 1`).Scan(&runID); err != nil {
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
	if input.Offset < 0 {
		input.Offset = 0
	}
	var total int
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM collection_candidates WHERE run_id=$1`, runID).Scan(&total)
	rows, err := h.DB.Query(ctx, `SELECT cc.id::text,cc.entity_id,cc.depth,COALESCE(cc.contexts[1],''),cc.last_discovered_at,COALESCE(p.display_name,cc.entity_id) FROM collection_candidates cc LEFT JOIN person_identifiers pi ON pi.platform='qq' AND pi.platform_user_id=cc.entity_id LEFT JOIN persons p ON p.id=pi.person_id WHERE cc.run_id=$1 ORDER BY cc.last_discovered_at DESC LIMIT $2 OFFSET $3`, runID, limit, input.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, entity, ctx, name string
		var depth int
		var at *time.Time
		if rows.Scan(&id, &entity, &depth, &ctx, &at, &name) == nil {
			items = append(items, map[string]any{"candidate_id": id, "entity_id": entity, "depth": depth, "context": ctx, "name": name, "last_discovered_at": at})
		}
	}
	return jsonResult(map[string]any{"run_id": runID, "events": items, "page": pagedResult(items, limit, input.Offset, total)})
}

func (h *HandlerRegistry) handleAIRunDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var id, task, provider, model, status, pack string
	var output []byte
	var created, completed *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT id::text,task_type,model_provider,model_name,status,evidence_pack_id::text,output,created_at,completed_at FROM ai_runs WHERE id=$1`, input.RunID).Scan(&id, &task, &provider, &model, &status, &pack, &output, &created, &completed); err != nil {
		return errorResult("AI run not found"), nil
	}
	var out any
	_ = json.Unmarshal(output, &out)
	return jsonResult(map[string]any{"id": id, "task_type": task, "model_provider": provider, "model_name": model, "status": status, "evidence_pack_id": pack, "output": out, "created_at": created, "completed_at": completed})
}

func (h *HandlerRegistry) handleEvidencePackDetail(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		PackID string `json:"pack_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	var id, subject, policy string
	var scope []byte
	var events []string
	var created *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT id::text,subject_id,scope,event_ids,redaction_policy,created_at FROM evidence_packs WHERE id=$1`, input.PackID).Scan(&id, &subject, &scope, &events, &policy, &created); err != nil {
		return errorResult("evidence pack not found"), nil
	}
	var sc any
	_ = json.Unmarshal(scope, &sc)
	return jsonResult(map[string]any{"id": id, "subject_id": subject, "scope": sc, "event_ids": events, "redaction_policy": policy, "created_at": created})
}

func (h *HandlerRegistry) handleOperationAudits(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 100)
	var total int
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM operation_audits`).Scan(&total)
	rows, err := h.DB.Query(ctx, `SELECT id::text,operation_id::text,request,response,status,created_at FROM operation_audits ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, op, status string
		var req, res []byte
		var at *time.Time
		if rows.Scan(&id, &op, &req, &res, &status, &at) == nil {
			var rq, rs any
			_ = json.Unmarshal(req, &rq)
			_ = json.Unmarshal(res, &rs)
			items = append(items, map[string]any{"id": id, "operation_id": op, "request": rq, "response": rs, "status": status, "created_at": at})
		}
	}
	return jsonResult(pagedResult(items, limit, offset, total))
}

func (h *HandlerRegistry) handleEvidenceDetails(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		EvidenceIDs []string `json:"evidence_ids"`
		Limit       int      `json:"limit"`
		Offset      int      `json:"offset"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	if len(input.EvidenceIDs) == 0 {
		return errorResult("evidence_ids is required"), nil
	}
	limit, offset := pagingDefaults(input.Limit, input.Offset, 200)
	var total int
	_ = h.DB.QueryRow(ctx, `SELECT count(*) FROM raw_records WHERE id=ANY($1::uuid[])`, input.EvidenceIDs).Scan(&total)
	rows, err := h.DB.Query(ctx, `SELECT id::text,source,endpoint_or_event_type,payload,payload_hash,collected_at FROM raw_records WHERE id=ANY($1::uuid[]) ORDER BY array_position($1::uuid[], id) LIMIT $2 OFFSET $3`, input.EvidenceIDs, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, source, endpoint, hash string
		var payload []byte
		var at *time.Time
		if rows.Scan(&id, &source, &endpoint, &payload, &hash, &at) == nil {
			var p any
			_ = json.Unmarshal(payload, &p)
			items = append(items, map[string]any{"id": id, "source": source, "endpoint": endpoint, "payload": p, "payload_hash": hash, "collected_at": at})
		}
	}
	return jsonResult(map[string]any{"requested": len(input.EvidenceIDs), "returned": len(items), "evidence": items, "page": pagedResult(items, limit, offset, total)})
}
