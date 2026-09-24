package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

func (r Repository) CreateCollectionRunWithScope(ctx context.Context, accountID *string, runType string, scope domain.CollectionScope) (domain.CollectionRun, error) {
	config, err := json.Marshal(map[string]any{"scope": scope})
	if err != nil {
		return domain.CollectionRun{}, err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return domain.CollectionRun{}, err
	}
	defer tx.Rollback(ctx)
	var run domain.CollectionRun
	err = tx.QueryRow(ctx, `
		INSERT INTO collection_runs(account_id,type,config)
		VALUES($1,$2,$3)
		RETURNING id,account_id,type,status,progress,error,started_at,ended_at,created_at,config`,
		accountID, runType, config,
	).Scan(&run.ID, &run.AccountID, &run.Type, &run.Status, &run.Progress, &run.Error, &run.StartedAt, &run.EndedAt, &run.CreatedAt, &run.Config)
	if err != nil {
		return run, err
	}
	for _, entry := range scope.Entries {
		if entry.ID == "" {
			continue
		}
		metadata := entry.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["depth"] = entry.Depth
		if _, err := tx.Exec(ctx, `
			INSERT INTO collection_scope_rules(run_id,rule_type,target_type,target_id,mode,metadata)
			VALUES($1,'entry',$2,$3,$4,$5)
			ON CONFLICT(run_id,rule_type,target_type,target_id)
			DO UPDATE SET mode=EXCLUDED.mode,metadata=EXCLUDED.metadata`,
			run.ID, entry.Type, entry.ID, entry.Mode, metadata); err != nil {
			return run, err
		}
	}
	for _, groupID := range scope.ExcludedGroups {
		if _, err := tx.Exec(ctx, `INSERT INTO collection_scope_rules(run_id,rule_type,target_type,target_id,mode) VALUES($1,'exclude','group',$2,'excluded') ON CONFLICT DO NOTHING`, run.ID, groupID); err != nil {
			return run, err
		}
	}
	for _, qq := range scope.ExcludedQQs {
		if _, err := tx.Exec(ctx, `INSERT INTO collection_scope_rules(run_id,rule_type,target_type,target_id,mode) VALUES($1,'exclude','qq',$2,'excluded') ON CONFLICT DO NOTHING`, run.ID, qq); err != nil {
			return run, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CollectionRun{}, err
	}
	return run, nil
}

// RecoverInterruptedCollectionRuns closes jobs whose in-memory workers were
// lost during a server stop. The stored cursors remain intact for a new run.
func (r Repository) RecoverInterruptedCollectionRuns(ctx context.Context) (int64, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE collection_run_modules
		SET status=CASE WHEN status='waiting' THEN 'cancelled' ELSE 'partial' END,
			error=CASE WHEN status='waiting' THEN error ELSE COALESCE(error,'collection interrupted by service restart') END,
			updated_at=now(),ended_at=now()
		WHERE run_id IN (SELECT id FROM collection_runs WHERE status IN ('queued','running'))
		  AND status IN ('waiting','running','retrying')`); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE collection_runs
		SET status='partial', ended_at=now(),
			error=CASE WHEN COALESCE(error,'')='' THEN 'collection interrupted by service restart; start a new run to continue'
				ELSE error||'; collection interrupted by service restart' END
		WHERE status IN ('queued','running')`)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r Repository) UpsertCollectionModule(ctx context.Context, runID string, progress domain.CollectionModuleProgress) error {
	if progress.Module == "" || progress.Status == "" {
		return fmt.Errorf("module and status are required")
	}
	cursor := progress.Cursor
	if cursor == nil {
		cursor = map[string]any{}
	}
	_, err := r.DB.Exec(ctx, `
		INSERT INTO collection_run_modules(
			run_id,module,status,pages_completed,pages_total,records_collected,cursor,error,started_at,ended_at
		) VALUES(
			$1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),
			CASE WHEN $3='running' THEN now() END,
			CASE WHEN $3 IN ('complete','partial','failed','no_permission','no_results','cancelled','skipped') THEN now() END
		)
		ON CONFLICT(run_id,module) DO UPDATE SET
			status=EXCLUDED.status,
			pages_completed=GREATEST(collection_run_modules.pages_completed,EXCLUDED.pages_completed),
			pages_total=COALESCE(EXCLUDED.pages_total,collection_run_modules.pages_total),
			records_collected=GREATEST(collection_run_modules.records_collected,EXCLUDED.records_collected),
			cursor=CASE WHEN EXCLUDED.cursor='{}'::jsonb THEN collection_run_modules.cursor ELSE EXCLUDED.cursor END,
			error=EXCLUDED.error,
			started_at=COALESCE(collection_run_modules.started_at,EXCLUDED.started_at),
			updated_at=now(),
			ended_at=EXCLUDED.ended_at`, runID, progress.Module, progress.Status, progress.PagesCompleted,
		progress.PagesTotal, progress.RecordsCollected, cursor, progress.Error)
	return err
}

func (r Repository) ListCollectionModules(ctx context.Context, runID string) ([]domain.CollectionModuleProgress, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT module,status,pages_completed,pages_total,records_collected,cursor,error,started_at,updated_at,ended_at
		FROM collection_run_modules WHERE run_id=$1 ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.CollectionModuleProgress{}
	for rows.Next() {
		var item domain.CollectionModuleProgress
		var cursor []byte
		var runError *string
		if err := rows.Scan(&item.Module, &item.Status, &item.PagesCompleted, &item.PagesTotal, &item.RecordsCollected, &cursor, &runError, &item.StartedAt, &item.UpdatedAt, &item.EndedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(cursor, &item.Cursor)
		if runError != nil {
			item.Error = *runError
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r Repository) CollectionRunHasPartial(ctx context.Context, runID string) (bool, error) {
	var partial bool
	err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_run_modules WHERE run_id=$1 AND status IN ('partial','failed','no_permission'))`, runID).Scan(&partial)
	return partial, err
}

func (r Repository) UpsertCollectionCandidate(ctx context.Context, runID, accountID string, candidate domain.CollectionCandidate) error {
	if strings.TrimSpace(candidate.EntityID) == "" {
		return nil
	}
	if candidate.State == "" {
		candidate.State = "discovered"
	}
	if candidate.Depth < 0 {
		candidate.Depth = 0
	}
	path := candidate.DiscoveryPath
	if path == nil {
		path = map[string]any{}
	}
	_, err := r.DB.Exec(ctx, `
		INSERT INTO collection_candidates(
			run_id,account_id,entity_type,entity_id,state,depth,priority,selected,contexts,discovery_paths
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,
			CASE WHEN $10='{}'::jsonb THEN '[]'::jsonb ELSE jsonb_build_array($10::jsonb) END)
		ON CONFLICT(run_id,entity_type,entity_id) DO UPDATE SET
			state=CASE
				WHEN collection_candidates.state='excluded' THEN 'excluded'
				WHEN EXCLUDED.state='expanded' THEN 'expanded'
				WHEN EXCLUDED.state='expandable' AND collection_candidates.state IN ('discovered','collected') THEN 'expandable'
				WHEN EXCLUDED.state='collected' AND collection_candidates.state='discovered' THEN 'collected'
				ELSE collection_candidates.state END,
			depth=LEAST(collection_candidates.depth,EXCLUDED.depth),
			priority=GREATEST(collection_candidates.priority,EXCLUDED.priority),
			selected=collection_candidates.selected OR EXCLUDED.selected,
			discovery_count=CASE
				WHEN EXCLUDED.state='expanded' THEN collection_candidates.discovery_count
				WHEN $10<>'{}'::jsonb AND collection_candidates.discovery_paths @> jsonb_build_array($10::jsonb) THEN collection_candidates.discovery_count
				ELSE collection_candidates.discovery_count+1 END,
			contexts=ARRAY(SELECT DISTINCT unnest(collection_candidates.contexts || EXCLUDED.contexts)),
			discovery_paths=CASE
				WHEN $10='{}'::jsonb THEN collection_candidates.discovery_paths
				WHEN collection_candidates.discovery_paths @> jsonb_build_array($10::jsonb) THEN collection_candidates.discovery_paths
				ELSE collection_candidates.discovery_paths || jsonb_build_array($10::jsonb) END,
			last_discovered_at=now(),
			expanded_at=CASE WHEN EXCLUDED.state='expanded' THEN now() ELSE collection_candidates.expanded_at END`,
		runID, nullable(accountID), candidate.EntityType, candidate.EntityID, candidate.State, candidate.Depth,
		candidate.Priority, candidate.Selected, candidate.Contexts, path)
	return err
}

func (r Repository) ListCollectionCandidates(ctx context.Context, runID, state string, limit, offset int) ([]domain.CollectionCandidate, int, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := "run_id=$1"
	args := []any{runID}
	if state != "" {
		args = append(args, state)
		where += fmt.Sprintf(" AND state=$%d", len(args))
	}
	var total int
	if err := r.DB.QueryRow(ctx, "SELECT count(*) FROM collection_candidates WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.DB.Query(ctx, fmt.Sprintf(`
		SELECT 
			cc.id, cc.run_id, COALESCE(cc.account_id::text,''), cc.entity_type, cc.entity_id, cc.state, cc.depth, cc.priority, cc.selected,
			cc.discovery_count, cc.contexts, cc.discovery_paths,
			COALESCE(NULLIF(p.display_name, ''), NULLIF(g.group_name, ''), cc.entity_id) AS name,
			COALESCE(pp.avatar_uri, '') AS avatar_url,
			cc.first_discovered_at, cc.last_discovered_at
		FROM collection_candidates cc
		LEFT JOIN person_identifiers pi ON cc.entity_type = 'qq' AND pi.platform_user_id = cc.entity_id
		LEFT JOIN persons p ON pi.person_id = p.id
		LEFT JOIN LATERAL (
			SELECT avatar_uri FROM person_profiles WHERE person_id = p.id AND avatar_uri != '' ORDER BY last_observed_at DESC LIMIT 1
		) pp ON true
		LEFT JOIN groups g ON cc.entity_type = 'group' AND g.platform_group_id = cc.entity_id
		WHERE %s
		ORDER BY cc.selected DESC, cc.priority DESC, cc.depth, cc.last_discovered_at DESC 
		LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []domain.CollectionCandidate{}
	for rows.Next() {
		var item domain.CollectionCandidate
		var paths []byte
		if err := rows.Scan(
			&item.ID, &item.RunID, &item.AccountID, &item.EntityType, &item.EntityID, &item.State,
			&item.Depth, &item.Priority, &item.Selected, &item.DiscoveryCount, &item.Contexts, &paths,
			&item.Name, &item.AvatarURL,
			&item.FirstDiscovered, &item.LastDiscovered,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(paths, &item.DiscoveryPaths)
		result = append(result, item)
	}
	return result, total, rows.Err()
}

func (r Repository) UpdateCollectionCandidate(ctx context.Context, runID, candidateID, state string, selected *bool) error {
	valid := map[string]bool{"discovered": true, "collected": true, "expandable": true, "expanded": true, "excluded": true}
	if !valid[state] {
		return fmt.Errorf("invalid candidate state")
	}
	tag, err := r.DB.Exec(ctx, `
		UPDATE collection_candidates
		SET state=$3,selected=COALESCE($4,selected),last_discovered_at=now(),
			expanded_at=CASE WHEN $3='expanded' THEN now() ELSE expanded_at END
		WHERE id=$1 AND run_id=$2`, candidateID, runID, state, selected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r Repository) UpdateCollectionCandidates(ctx context.Context, runID string, candidateIDs []string, state string, selected *bool) (int64, error) {
	valid := map[string]bool{"discovered": true, "collected": true, "expandable": true, "expanded": true, "excluded": true}
	if !valid[state] {
		return 0, fmt.Errorf("invalid candidate state")
	}
	if len(candidateIDs) == 0 || len(candidateIDs) > 500 {
		return 0, fmt.Errorf("candidate ids must contain between 1 and 500 items")
	}
	tag, err := r.DB.Exec(ctx, `
		UPDATE collection_candidates
		SET state=$3,selected=COALESCE($4,selected),last_discovered_at=now(),
			expanded_at=CASE WHEN $3='expanded' THEN now() ELSE expanded_at END
		WHERE run_id=$1 AND id=ANY($2::uuid[])`, runID, candidateIDs, state, selected)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
