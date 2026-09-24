package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

type collectionOrchestrateArgs struct {
	Targets []struct {
		Type string `json:"type"` // qq or group
		ID   string `json:"id"`
	} `json:"targets"`
	Modules  []string `json:"modules"`   // specific modules to collect, nil means all
	MaxDepth *int     `json:"max_depth"` // default 1
	Mode     *string  `json:"mode"`      // expand_people, full_collect, profile_only. Default full_collect
}

func (h *HandlerRegistry) handleCollectionOrchestrate(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input collectionOrchestrateArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if len(input.Targets) == 0 {
		return errorResult("targets cannot be empty"), nil
	}

	maxDepth := 1
	if input.MaxDepth != nil {
		maxDepth = *input.MaxDepth
	}

	mode := "full_collect"
	if input.Mode != nil && *input.Mode != "" {
		mode = *input.Mode
	}

	scope := domain.CollectionScope{
		MaxDepth: &maxDepth,
		Entries:  make([]domain.CollectionEntry, 0, len(input.Targets)),
	}

	for _, t := range input.Targets {
		scope.Entries = append(scope.Entries, domain.CollectionEntry{
			Type:  t.Type,
			ID:    t.ID,
			Depth: 0,
			Mode:  mode,
			Metadata: map[string]any{
				"source":  "mcp",
				"modules": input.Modules,
			},
		})
	}

	run, err := h.Repo.CreateCollectionRunWithScope(ctx, nil, "qzone", scope)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection run: %w", err)
	}

	return jsonResult(map[string]any{
		"run_id":    run.ID,
		"targets":   input.Targets,
		"mode":      mode,
		"max_depth": maxDepth,
		"modules":   input.Modules,
		"status":    "Collection run successfully orchestrated",
	})
}

type collectionStatusArgs struct {
	RunID string `json:"run_id"` // optional, defaults to latest
}

func (h *HandlerRegistry) handleCollectionStatus(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input collectionStatusArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var runID string
	if input.RunID != "" {
		runID = input.RunID
	} else {
		// Attempt to get the latest run
		err := h.DB.QueryRow(ctx, "SELECT id FROM collection_runs ORDER BY started_at DESC NULLS LAST LIMIT 1").Scan(&runID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return errorResult("no collection runs found"), nil
			}
			return nil, fmt.Errorf("failed to get latest run: %w", err)
		}
	}

	// 1. Query collection_runs
	var run struct {
		ID        string     `json:"id"`
		AccountID *string    `json:"account_id"`
		Type      string     `json:"type"`
		Status    string     `json:"status"`
		Progress  *float64   `json:"progress"`
		StartedAt *time.Time `json:"started_at"`
		EndedAt   *time.Time `json:"ended_at"`
		Config    any        `json:"config"`
	}
	err := h.DB.QueryRow(ctx, `
		SELECT id, account_id, type, status, progress, started_at, ended_at, config
		FROM collection_runs WHERE id = $1
	`, runID).Scan(&run.ID, &run.AccountID, &run.Type, &run.Status, &run.Progress, &run.StartedAt, &run.EndedAt, &run.Config)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errorResult(fmt.Sprintf("run %s not found", runID)), nil
		}
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	// 2. Query collection_run_modules
	rows, err := h.DB.Query(ctx, `
		SELECT module, status, pages_completed, pages_total, records_collected, error, started_at, ended_at
		FROM collection_run_modules
		WHERE run_id = $1
		ORDER BY module
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query modules: %w", err)
	}
	defer rows.Close()

	var modules []map[string]any
	for rows.Next() {
		var module, status string
		var pagesCompleted, pagesTotal, recordsCollected *int
		var errStr *string
		var startedAt, endedAt *time.Time
		if err := rows.Scan(&module, &status, &pagesCompleted, &pagesTotal, &recordsCollected, &errStr, &startedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("failed to scan module: %w", err)
		}
		modules = append(modules, map[string]any{
			"module":            module,
			"status":            status,
			"pages_completed":   pagesCompleted,
			"pages_total":       pagesTotal,
			"records_collected": recordsCollected,
			"error":             errStr,
			"started_at":        startedAt,
			"ended_at":          endedAt,
		})
	}
	rows.Close()

	// 3. Query top 10 pending candidates
	candRows, err := h.DB.Query(ctx, `
		SELECT entity_type, entity_id, state, depth, priority, discovery_count
		FROM collection_candidates
		WHERE run_id = $1 AND state IN ('discovered', 'expandable')
		ORDER BY priority DESC, depth, last_discovered_at
		LIMIT 10
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query candidates: %w", err)
	}
	defer candRows.Close()

	var candidates []map[string]any
	for candRows.Next() {
		var eType, eID, state string
		var depth, priority, discoveryCount int
		if err := candRows.Scan(&eType, &eID, &state, &depth, &priority, &discoveryCount); err != nil {
			return nil, fmt.Errorf("failed to scan candidate: %w", err)
		}
		candidates = append(candidates, map[string]any{
			"entity_type":     eType,
			"entity_id":       eID,
			"state":           state,
			"depth":           depth,
			"priority":        priority,
			"discovery_count": discoveryCount,
		})
	}
	candRows.Close()

	// 4. Count candidates by state
	stateRows, err := h.DB.Query(ctx, `
		SELECT state, COUNT(*) FROM collection_candidates WHERE run_id = $1 GROUP BY state
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query candidate states: %w", err)
	}
	defer stateRows.Close()

	stateCounts := make(map[string]int)
	for stateRows.Next() {
		var state string
		var count int
		if err := stateRows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("failed to scan candidate state: %w", err)
		}
		stateCounts[state] = count
	}
	stateRows.Close()

	return jsonResult(map[string]any{
		"run":             run,
		"modules":         modules,
		"candidates_top":  candidates,
		"candidate_stats": stateCounts,
	})
}

type coverageAuditArgs struct {
	QQList             []string `json:"qq_list"`
	SuggestNextTargets *bool    `json:"suggest_next_targets"`
}

func (h *HandlerRegistry) handleCoverageAudit(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input coverageAuditArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	suggestNextTargets := true
	if input.SuggestNextTargets != nil {
		suggestNextTargets = *input.SuggestNextTargets
	}

	if len(input.QQList) == 0 {
		return errorResult("qq_list cannot be empty"), nil
	}

	allSources := []string{"avatar", "basic_profile", "group_memberships", "group_messages", "media", "private_messages", "qzone_comments", "qzone_feeds", "qzone_likes", "qzone_visits"}

	var audit []map[string]any
	var personIDs []string

	for _, qq := range input.QQList {
		pid, err := resolvePersonID(ctx, h.DB, qq)
		if err != nil {
			audit = append(audit, map[string]any{"qq": qq, "error": err.Error()})
			continue
		}

		personIDs = append(personIDs, pid)

		var displayName *string
		err = h.DB.QueryRow(ctx, "SELECT display_name FROM persons WHERE id = $1", pid).Scan(&displayName)
		if err != nil && err != pgx.ErrNoRows {
			return nil, fmt.Errorf("failed to get person: %w", err)
		}

		rows, err := h.DB.Query(ctx, "SELECT data_source, status, last_collected_at FROM collection_coverage WHERE person_id = $1", pid)
		if err != nil {
			return nil, fmt.Errorf("failed to get coverage: %w", err)
		}

		collected := []string{}
		missing := []string{}
		details := make(map[string]any)

		covMap := make(map[string]struct {
			Status          string
			LastCollectedAt *time.Time
		})

		for rows.Next() {
			var ds, status string
			var lca *time.Time
			if err := rows.Scan(&ds, &status, &lca); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan coverage: %w", err)
			}
			covMap[ds] = struct {
				Status          string
				LastCollectedAt *time.Time
			}{status, lca}

			if status == "complete" || status == "partial" {
				collected = append(collected, ds)
			}
			details[ds] = map[string]any{
				"status":            status,
				"last_collected_at": lca,
			}
		}
		rows.Close()

		observed := observedCoverageCounts(ctx, h.DB, pid)
		unverified := []string{}
		for _, src := range allSources {
			if observed[src] <= 0 {
				continue
			}
			current, exists := covMap[src]
			if exists && current.Status != "not_collected" && current.Status != "failed" {
				continue
			}
			unverified = append(unverified, src)
			covMap[src] = struct {
				Status          string
				LastCollectedAt *time.Time
			}{"inferred", current.LastCollectedAt}
			details[src] = map[string]any{
				"status":            "inferred",
				"items_observed":    observed[src],
				"last_collected_at": current.LastCollectedAt,
			}
			if !exists || current.Status == "not_collected" || current.Status == "failed" {
				collected = append(collected, src)
			}
		}

		for _, src := range allSources {
			if st, ok := covMap[src]; !ok || (st.Status == "not_collected" || st.Status == "failed") {
				missing = append(missing, src)
			}
		}

		covPct := 0
		if len(allSources) > 0 {
			covPct = (len(collected) * 100) / len(allSources)
		}

		auditItem := map[string]any{
			"qq":                            qq,
			"person_id":                     pid,
			"display_name":                  displayName,
			"coverage_percentage":           covPct,
			"collected":                     collected,
			"missing":                       missing,
			"details":                       details,
			"unverified":                    unverified,
			"confirmed_coverage_percentage": (len(collected) - len(unverified)) * 100 / len(allSources),
		}
		audit = append(audit, auditItem)
	}

	res := map[string]any{
		"audit": audit,
	}

	if suggestNextTargets && len(personIDs) > 0 {
		rows, err := h.DB.Query(ctx, `
			SELECT pi.platform_user_id, p.display_name,
				(SELECT COUNT(DISTINCT data_source) FROM collection_coverage cc WHERE cc.person_id = p.id AND cc.status IN ('complete', 'partial')) as collected,
				(SELECT COUNT(*) FROM relation_events WHERE actor_person_id = p.id OR target_person_id = p.id) as interaction_count
			FROM persons p
			JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE p.id = ANY($1::uuid[])
			ORDER BY collected ASC, interaction_count DESC
			LIMIT 10
		`, personIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to suggest targets: %w", err)
		}
		defer rows.Close()

		var suggestions []map[string]any
		for rows.Next() {
			var qq string
			var disp *string
			var col, inter int
			if err := rows.Scan(&qq, &disp, &col, &inter); err != nil {
				return nil, fmt.Errorf("failed to scan suggestion: %w", err)
			}
			suggestions = append(suggestions, map[string]any{
				"qq":                qq,
				"display_name":      disp,
				"collected_count":   col,
				"interaction_count": inter,
			})
		}
		rows.Close()
		res["suggestions"] = suggestions
	}

	return jsonResult(res)
}

type rawEvidenceArgs struct {
	EvidenceID string `json:"evidence_id"`
}

func (h *HandlerRegistry) handleRawEvidence(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input rawEvidenceArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if input.EvidenceID == "" {
		return errorResult("evidence_id cannot be empty"), nil
	}

	var source, eventType, hash string
	var payload any
	var collectedAt *time.Time

	err := h.DB.QueryRow(ctx, `
		SELECT source, endpoint_or_event_type, payload, payload_hash, collected_at
		FROM raw_records WHERE id = $1
	`, input.EvidenceID).Scan(&source, &eventType, &payload, &hash, &collectedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return errorResult(fmt.Sprintf("evidence %s not found", input.EvidenceID)), nil
		}
		return nil, fmt.Errorf("failed to get raw evidence: %w", err)
	}

	return jsonResult(map[string]any{
		"evidence_id":            input.EvidenceID,
		"source":                 source,
		"endpoint_or_event_type": eventType,
		"payload":                payload,
		"payload_hash":           hash,
		"collected_at":           collectedAt,
	})
}

func (h *HandlerRegistry) handleSystemOverview(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var personsCount, groupsCount, membershipsCount, contentsCount, messagesCount, eventsCount, mediaCount, rawRecordsCount int64

	err := h.DB.QueryRow(ctx, `
		SELECT 
			(SELECT COUNT(*) FROM persons) as persons_count,
			(SELECT COUNT(*) FROM "groups") as groups_count,
			(SELECT COUNT(*) FROM group_memberships) as memberships_count,
			(SELECT COUNT(*) FROM contents) as contents_count,
			(SELECT COUNT(*) FROM messages) as messages_count,
			(SELECT COUNT(*) FROM relation_events) as events_count,
			(SELECT COUNT(*) FROM media_assets) as media_count,
			(SELECT COUNT(*) FROM raw_records) as raw_records_count
	`).Scan(&personsCount, &groupsCount, &membershipsCount, &contentsCount, &messagesCount, &eventsCount, &mediaCount, &rawRecordsCount)
	if err != nil {
		return nil, fmt.Errorf("failed to query overall counts: %w", err)
	}

	var latestRun *map[string]any
	var runID, runType, status string
	var progress *float64
	var startedAt, endedAt *time.Time

	// Query latest collection run
	err = h.DB.QueryRow(ctx, `
		SELECT id, type, status, progress, started_at, ended_at 
		FROM collection_runs ORDER BY created_at DESC LIMIT 1
	`).Scan(&runID, &runType, &status, &progress, &startedAt, &endedAt)

	if err != nil && err != pgx.ErrNoRows {
		// Fallback if created_at might not exist
		err = h.DB.QueryRow(ctx, `
			SELECT id, type, status, progress, started_at, ended_at 
			FROM collection_runs ORDER BY started_at DESC NULLS LAST LIMIT 1
		`).Scan(&runID, &runType, &status, &progress, &startedAt, &endedAt)
	}

	if err == nil {
		latestRun = &map[string]any{
			"id":         runID,
			"type":       runType,
			"status":     status,
			"progress":   progress,
			"started_at": startedAt,
			"ended_at":   endedAt,
		}
	} else if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to get latest run: %w", err)
	}

	rows, err := h.DB.Query(ctx, `
		SELECT action_type, COUNT(*) FROM relation_events GROUP BY action_type ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get action type counts: %w", err)
	}
	defer rows.Close()

	actionCounts := make(map[string]int64)
	for rows.Next() {
		var actionType string
		var count int64
		if err := rows.Scan(&actionType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan action count: %w", err)
		}
		actionCounts[actionType] = count
	}
	rows.Close()

	return jsonResult(map[string]any{
		"counts": map[string]int64{
			"persons":           personsCount,
			"groups":            groupsCount,
			"group_memberships": membershipsCount,
			"contents":          contentsCount,
			"messages":          messagesCount,
			"relation_events":   eventsCount,
			"media_assets":      mediaCount,
			"raw_records":       rawRecordsCount,
		},
		"latest_collection_run":         latestRun,
		"relation_event_counts_by_type": actionCounts,
	})
}
