package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PipelineManager coordinates batch collection & persona intelligence pipelines.
type PipelineManager struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	cancels map[string]context.CancelFunc
}

func NewPipelineManager(pool *pgxpool.Pool) *PipelineManager {
	return &PipelineManager{
		pool:    pool,
		cancels: make(map[string]context.CancelFunc),
	}
}

func effectivePipelineConcurrency(ctx context.Context, pool *pgxpool.Pool, requested int) int {
	if requested <= 0 {
		requested = 2
	}
	var configured int
	if err := pool.QueryRow(ctx, `SELECT (value #>> '{}')::int FROM system_configs WHERE key = 'ai.max_concurrency'`).Scan(&configured); err == nil && configured > 0 && requested > configured {
		return configured
	}
	return requested
}

// ResumeActivePipelines reattaches unfinished pipelines after a server restart.
// Processing items are returned to the durable pending queue because their
// previous worker may have been terminated before it could commit completion.
func (pm *PipelineManager) ResumeActivePipelines(ctx context.Context) error {
	if _, err := pm.pool.Exec(ctx, `
		UPDATE batch_pipeline_items bpi
		SET status = 'pending', started_at = NULL
		FROM batch_persona_pipelines bpp
		WHERE bpi.pipeline_id = bpp.id
		  AND bpp.status IN ('queued', 'running')
		  AND bpi.status = 'processing'
	`); err != nil {
		return fmt.Errorf("reset interrupted pipeline items: %w", err)
	}

	rows, err := pm.pool.Query(ctx, `
		SELECT id::text, concurrency, auto_retry, max_retries, retry_backoff_seconds, queue_wait_seconds
		FROM batch_persona_pipelines
		WHERE status IN ('queued', 'running')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return fmt.Errorf("query unfinished pipelines: %w", err)
	}
	defer rows.Close()

	type pendingPipeline struct {
		id               string
		concurrency      int
		autoRetry        bool
		maxRetries       int
		backoffSeconds   int
		queueWaitSeconds int
	}
	var pipelines []pendingPipeline
	for rows.Next() {
		var p pendingPipeline
		if err := rows.Scan(&p.id, &p.concurrency, &p.autoRetry, &p.maxRetries, &p.backoffSeconds, &p.queueWaitSeconds); err != nil {
			return fmt.Errorf("scan unfinished pipeline: %w", err)
		}
		pipelines = append(pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read unfinished pipelines: %w", err)
	}

	for _, p := range pipelines {
		pm.startPipeline(p.id, effectivePipelineConcurrency(ctx, pm.pool, p.concurrency), p.autoRetry, p.maxRetries, p.backoffSeconds, p.queueWaitSeconds, false)
	}
	return nil
}

func (pm *PipelineManager) startPipeline(pipelineID string, concurrency int, autoRetry bool, maxRetries int, backoffSeconds int, queueWaitSeconds int, force bool) {
	bgCtx, cancel := context.WithCancel(context.Background())
	pm.mu.Lock()
	if _, exists := pm.cancels[pipelineID]; exists {
		pm.mu.Unlock()
		cancel()
		return
	}
	pm.cancels[pipelineID] = cancel
	pm.mu.Unlock()

	go func() {
		defer func() {
			pm.mu.Lock()
			delete(pm.cancels, pipelineID)
			pm.mu.Unlock()
		}()
		if err := pm.ExecutePipeline(bgCtx, pipelineID, concurrency, autoRetry, maxRetries, backoffSeconds, queueWaitSeconds, force); err != nil {
			slog.Error("pipeline execution failed", "pipeline_id", pipelineID, "error", err)
		}
	}()
}

type PipelineCreateInput struct {
	Title               string   `json:"title"`
	ScopeType           string   `json:"scope_type"`   // 'group', 'top_active', 'custom_qqs', 'all_persons'
	ScopeTarget         string   `json:"scope_target"` // group id or search query or comma-separated QQs
	PersonIDs           []string `json:"person_ids"`   // explicit person UUIDs
	Concurrency         int      `json:"concurrency"`
	AutoRetry           bool     `json:"auto_retry"`
	MaxRetries          int      `json:"max_retries"`
	RetryBackoffSeconds int      `json:"retry_backoff_seconds"`
	QueueWaitSeconds    int      `json:"queue_wait_seconds"`
	ForceAnalyze        bool     `json:"force_analyze"`
}

type TaskSummaryData struct {
	TotalPersons int             `json:"total_persons"`
	Roles        []SummaryRole   `json:"roles"`
	Catchphrases []SummaryPhrase `json:"catchphrases"`
	Slang        []SummaryPhrase `json:"slang"`
	Domains      []SummaryDomain `json:"domains"`
	CompiledAt   time.Time       `json:"compiled_at"`
}

type SummaryRole struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Percent int    `json:"percent"`
}

type SummaryPhrase struct {
	Phrase string `json:"phrase"`
	Count  int    `json:"count"`
}

type SummaryDomain struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	AvgScore int    `json:"avg_score"`
}

// CreatePipeline creates a new batch pipeline in PostgreSQL and registers target persons.
func (pm *PipelineManager) CreatePipeline(ctx context.Context, in PipelineCreateInput) (string, error) {
	if in.Concurrency <= 0 {
		in.Concurrency = 2
	}
	if in.Concurrency > 8 {
		in.Concurrency = 8
	}
	in.Concurrency = effectivePipelineConcurrency(ctx, pm.pool, in.Concurrency)
	if in.MaxRetries <= 0 {
		in.MaxRetries = 3
	}
	if in.RetryBackoffSeconds <= 0 {
		in.RetryBackoffSeconds = 15
	}
	if in.QueueWaitSeconds <= 0 {
		in.QueueWaitSeconds = 180
	}
	if in.Title == "" {
		in.Title = fmt.Sprintf("批量画像研判流水线_%s", time.Now().Format("2006-01-02_1504"))
	}
	seenTargets := make(map[string]struct{})
	filteredTargets := make([]string, 0, len(in.PersonIDs))
	for _, targetID := range in.PersonIDs {
		if targetID == "" {
			continue
		}
		if _, seen := seenTargets[targetID]; seen {
			continue
		}
		seenTargets[targetID] = struct{}{}
		filteredTargets = append(filteredTargets, targetID)
	}

	// 1. Resolve Target Person IDs
	var targetIDs []string
	if len(in.PersonIDs) > 0 {
		targetIDs = filteredTargets
	} else {
		switch in.ScopeType {
		case "group":
			rows, err := pm.pool.Query(ctx, `
				SELECT gm.person_id::text FROM group_memberships gm
				LEFT JOIN person_identifiers pi ON pi.person_id = gm.person_id AND pi.platform = 'qq'
				WHERE gm.group_id = $1 AND NOT EXISTS (SELECT 1 FROM napcat_accounts na WHERE na.qq_uin = pi.platform_user_id AND na.qq_uin <> '')
				ORDER BY gm.join_time ASC NULLS LAST
			`, in.ScopeTarget)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var pid string
					if err := rows.Scan(&pid); err == nil && pid != "" {
						targetIDs = append(targetIDs, pid)
					}
				}
			}
		case "with_messages":
			rows, err := pm.pool.Query(ctx, `
				SELECT p.id::text FROM persons p
				INNER JOIN messages m ON m.sender_id = p.id
				LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
				WHERE NOT EXISTS (SELECT 1 FROM napcat_accounts na WHERE na.qq_uin = pi.platform_user_id AND na.qq_uin <> '')
				GROUP BY p.id
				ORDER BY count(m.id) DESC
			`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var pid string
					if err := rows.Scan(&pid); err == nil && pid != "" {
						targetIDs = append(targetIDs, pid)
					}
				}
			}
		case "top_active", "top_100":
			limit := 100
			rows, err := pm.pool.Query(ctx, `
				SELECT p.id::text FROM persons p
				INNER JOIN messages m ON m.sender_id = p.id
				LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
				WHERE NOT EXISTS (SELECT 1 FROM napcat_accounts na WHERE na.qq_uin = pi.platform_user_id AND na.qq_uin <> '')
				GROUP BY p.id
				ORDER BY count(m.id) DESC
				LIMIT $1
			`, limit)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var pid string
					if err := rows.Scan(&pid); err == nil && pid != "" {
						targetIDs = append(targetIDs, pid)
					}
				}
			}
		case "all_persons":
			fallthrough
		default: // all_persons: Full library without any arbitrary limits
			rows, err := pm.pool.Query(ctx, `
				SELECT p.id::text FROM persons p
				LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
				WHERE NOT EXISTS (SELECT 1 FROM napcat_accounts na WHERE na.qq_uin = pi.platform_user_id AND na.qq_uin <> '')
				ORDER BY p.last_seen_at DESC NULLS LAST
			`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var pid string
					if err := rows.Scan(&pid); err == nil && pid != "" {
						targetIDs = append(targetIDs, pid)
					}
				}
			}
		}
	}

	if len(targetIDs) == 0 {
		return "", fmt.Errorf("no target persons found for scope '%s'", in.ScopeType)
	}
	dedupedTargets := make([]string, 0, len(targetIDs))
	seenTargets = make(map[string]struct{}, len(targetIDs))
	for _, targetID := range targetIDs {
		if targetID == "" {
			continue
		}
		if _, seen := seenTargets[targetID]; seen {
			continue
		}
		seenTargets[targetID] = struct{}{}
		dedupedTargets = append(dedupedTargets, targetID)
	}
	targetIDs = dedupedTargets
	if len(targetIDs) == 0 {
		return "", fmt.Errorf("no target persons found for scope '%s'", in.ScopeType)
	}

	// 2. Insert batch_persona_pipelines record
	var pipelineID string
	err := pm.pool.QueryRow(ctx, `
		INSERT INTO batch_persona_pipelines (
			title, status, scope_type, scope_target, total_targets, concurrency, auto_retry, max_retries, retry_backoff_seconds, queue_wait_seconds
		) VALUES ($1, 'queued', $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text
	`, in.Title, in.ScopeType, in.ScopeTarget, len(targetIDs), in.Concurrency, in.AutoRetry, in.MaxRetries, in.RetryBackoffSeconds, in.QueueWaitSeconds).Scan(&pipelineID)
	if err != nil {
		return "", fmt.Errorf("insert pipeline: %w", err)
	}

	// 3. Insert batch_pipeline_items
	for _, pid := range targetIDs {
		if _, err := pm.pool.Exec(ctx, `
			INSERT INTO batch_pipeline_items (pipeline_id, person_id, status)
			VALUES ($1::uuid, $2::uuid, 'pending')
		`, pipelineID, pid); err != nil {
			_, _ = pm.pool.Exec(ctx, `DELETE FROM batch_persona_pipelines WHERE id=$1::uuid`, pipelineID)
			return "", fmt.Errorf("insert pipeline target %s: %w", pid, err)
		}
	}

	// 4. Trigger asynchronous execution. The same durable launcher is used by
	// startup recovery so newly created and resumed jobs share lifecycle rules.
	pm.startPipeline(pipelineID, in.Concurrency, in.AutoRetry, in.MaxRetries, in.RetryBackoffSeconds, in.QueueWaitSeconds, in.ForceAnalyze)

	return pipelineID, nil
}

// RetryFailedPipeline requeues only failed items and preserves successful evidence.
func (pm *PipelineManager) RetryFailedPipeline(ctx context.Context, pipelineID string) error {
	if _, err := pm.pool.Exec(ctx, `
		UPDATE batch_pipeline_items
		SET status = 'pending', attempts = 0, error_message = NULL,
			started_at = NULL, finished_at = NULL
		WHERE pipeline_id = $1::uuid AND status = 'failed'
	`, pipelineID); err != nil {
		return fmt.Errorf("requeue failed pipeline items: %w", err)
	}
	if _, err := pm.pool.Exec(ctx, `
		UPDATE batch_persona_pipelines
		SET status = 'queued', failed_targets = 0, error_message = NULL, updated_at = NOW()
		WHERE id = $1::uuid AND EXISTS (
			SELECT 1 FROM batch_pipeline_items WHERE pipeline_id = $1::uuid AND status = 'pending'
		)
	`, pipelineID); err != nil {
		return fmt.Errorf("reset pipeline status: %w", err)
	}
	var concurrency int
	var autoRetry bool
	var maxRetries, backoffSeconds, queueWaitSeconds int
	if err := pm.pool.QueryRow(ctx, `SELECT concurrency, auto_retry, max_retries, retry_backoff_seconds, queue_wait_seconds FROM batch_persona_pipelines WHERE id = $1::uuid`, pipelineID).Scan(&concurrency, &autoRetry, &maxRetries, &backoffSeconds, &queueWaitSeconds); err != nil {
		return fmt.Errorf("load retry pipeline: %w", err)
	}
	concurrency = effectivePipelineConcurrency(ctx, pm.pool, concurrency)
	_, _ = pm.pool.Exec(ctx, `UPDATE batch_persona_pipelines SET concurrency = $1 WHERE id = $2::uuid`, concurrency, pipelineID)
	pm.startPipeline(pipelineID, concurrency, autoRetry, maxRetries, backoffSeconds, queueWaitSeconds, false)
	return nil
}

// ExecutePipeline drives the parallel worker pool with exponential backoff retry.
func (pm *PipelineManager) ExecutePipeline(ctx context.Context, pipelineID string, concurrency int, autoRetry bool, maxRetries int, backoffSeconds int, queueWaitSeconds int, force bool) error {
	_, _ = pm.pool.Exec(ctx, `
		UPDATE batch_persona_pipelines 
		SET status = 'running', updated_at = NOW() 
		WHERE id = $1::uuid
	`, pipelineID)

	rows, err := pm.pool.Query(ctx, `
		SELECT id::text, person_id::text 
		FROM batch_pipeline_items 
		WHERE pipeline_id = $1::uuid AND status = 'pending'
		ORDER BY id ASC
	`, pipelineID)
	if err != nil {
		return fmt.Errorf("query pipeline items: %w", err)
	}

	type Item struct {
		ID       string
		PersonID string
	}
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.PersonID); err == nil {
			items = append(items, it)
		}
	}
	rows.Close()

	var completedCount, failedCount int
	if err := pm.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'completed'),
			count(*) FILTER (WHERE status = 'failed')
		FROM batch_pipeline_items
		WHERE pipeline_id = $1::uuid
	`, pipelineID).Scan(&completedCount, &failedCount); err != nil {
		return fmt.Errorf("count existing pipeline item status: %w", err)
	}
	_, _ = pm.pool.Exec(ctx, `
		UPDATE batch_persona_pipelines
		SET completed_targets = $1, failed_targets = $2, updated_at = NOW()
		WHERE id = $3::uuid
	`, completedCount, failedCount, pipelineID)

	if len(items) == 0 {
		_, _ = pm.pool.Exec(ctx, `
			UPDATE batch_persona_pipelines 
			SET status = 'completed', updated_at = NOW() 
			WHERE id = $1::uuid
		`, pipelineID)
		return nil
	}

	itemChan := make(chan Item, len(items))
	for _, it := range items {
		itemChan <- it
	}
	close(itemChan)

	var wg sync.WaitGroup
	var countMu sync.Mutex

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range itemChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				_, _ = pm.pool.Exec(ctx, `
					UPDATE batch_pipeline_items 
					SET status = 'processing', started_at = NOW() 
					WHERE id = $1::uuid
				`, it.ID)

				var attempts int
				var success bool
				var lastErr error
				var profile *PersonaProfile

				retries := 1
				if autoRetry {
					retries = maxRetries
				}

				if !force {
					var valJSON []byte
					var infID string
					var createdAt time.Time
					err := pm.pool.QueryRow(ctx, `
						SELECT id::text, value, created_at 
						FROM inferences 
						WHERE subject_id = $1 AND attribute_type = 'persona_profile'
						ORDER BY created_at DESC LIMIT 1
					`, it.PersonID).Scan(&infID, &valJSON, &createdAt)
					if err == nil && len(valJSON) > 0 {
						var parsed struct {
							BiographicalAnchors   []BiographicalAnchor  `json:"biographical_anchors"`
							PsychologicalDefense  PsychologicalDefense  `json:"psychological_defense"`
							VerbatimAnchorQuotes  []VerbatimQuote       `json:"verbatim_anchor_quotes"`
							LinguisticFingerprint LinguisticFingerprint `json:"linguistic_fingerprint"`
							InterestSpectrum      []InterestDomain      `json:"interest_spectrum"`
							SocialArchetype       SocialArchetype       `json:"social_archetype"`
						}
						if err := json.Unmarshal(valJSON, &parsed); err == nil {
							profile = &PersonaProfile{
								PersonID:              it.PersonID,
								BiographicalAnchors:   parsed.BiographicalAnchors,
								PsychologicalDefense:  parsed.PsychologicalDefense,
								VerbatimAnchorQuotes:  parsed.VerbatimAnchorQuotes,
								LinguisticFingerprint: parsed.LinguisticFingerprint,
								InterestSpectrum:      parsed.InterestSpectrum,
								SocialArchetype:       parsed.SocialArchetype,
								InferenceID:           infID,
								AnalyzedAt:            createdAt,
							}
							success = true
						}
					}
				}

				for attempts < retries && !success {
					attempts++
					select {
					case <-ctx.Done():
						return
					default:
					}

					if attempts > 1 {
						// Provider overloads need a real cooldown; short retries amplify 429s.
						backoff := time.Duration(float64(backoffSeconds)*math.Pow(3, float64(attempts-2))) * time.Second
						time.Sleep(backoff)
					}

					requestCtx := withLLMQueueTimeout(ctx, time.Duration(queueWaitSeconds)*time.Second)
					profile, lastErr = AnalyzePersonPersona(requestCtx, pm.pool, it.PersonID)
					if lastErr == nil && profile != nil {
						success = true
					}
				}

				countMu.Lock()
				if success {
					completedCount++
					var infID *string
					if profile.InferenceID != "" {
						infID = &profile.InferenceID
					}
					_, _ = pm.pool.Exec(ctx, `
						UPDATE batch_pipeline_items 
						SET status = 'completed', attempts = $1, persona_inference_id = $2::uuid, finished_at = NOW() 
						WHERE id = $3::uuid
					`, attempts, infID, it.ID)
				} else {
					failedCount++
					errMsg := "unknown error"
					if lastErr != nil {
						errMsg = lastErr.Error()
					}
					_, _ = pm.pool.Exec(ctx, `
						UPDATE batch_pipeline_items 
						SET status = 'failed', attempts = $1, error_message = $2, finished_at = NOW() 
						WHERE id = $3::uuid
					`, attempts, errMsg, it.ID)
				}

				// Update pipeline progress counts in real-time
				_, _ = pm.pool.Exec(ctx, `
					UPDATE batch_persona_pipelines 
					SET completed_targets = $1, failed_targets = $2, updated_at = NOW() 
					WHERE id = $3::uuid
				`, completedCount, failedCount, pipelineID)
				countMu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 5. Compile Task Summary
	summary, err := pm.CompileTaskSummary(ctx, pipelineID)
	var summaryJSON []byte
	if err == nil && summary != nil {
		summaryJSON, _ = json.Marshal(summary)
	}

	finalStatus := "completed"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	} else if failedCount > 0 && completedCount == 0 {
		finalStatus = "failed"
	} else if failedCount > 0 {
		finalStatus = "partial"
	}

	_, _ = pm.pool.Exec(context.Background(), `
		UPDATE batch_persona_pipelines 
		SET status = $1, summary_data = $2, updated_at = NOW() 
		WHERE id = $3::uuid
	`, finalStatus, summaryJSON, pipelineID)

	return nil
}

// CancelPipeline cancels a running pipeline job.
func (pm *PipelineManager) CancelPipeline(pipelineID string) bool {
	pm.mu.Lock()
	cancel, ok := pm.cancels[pipelineID]
	pm.mu.Unlock()
	if ok && cancel != nil {
		cancel()
		ctx := context.Background()
		_, _ = pm.pool.Exec(ctx, `
			UPDATE batch_pipeline_items
			SET status = 'cancelled', finished_at = NOW(),
				error_message = COALESCE(error_message, '任务已取消')
			WHERE pipeline_id = $1::uuid AND status IN ('pending', 'processing')
		`, pipelineID)
		_, _ = pm.pool.Exec(ctx, `
			UPDATE batch_persona_pipelines
			SET status = 'cancelled', updated_at = NOW()
			WHERE id = $1::uuid
		`, pipelineID)
		return true
	}
	return false
}

// CompileTaskSummary aggregates all generated personas in a pipeline into high-level sociological and linguistic insights.
func (pm *PipelineManager) CompileTaskSummary(ctx context.Context, pipelineID string) (*TaskSummaryData, error) {
	rows, err := pm.pool.Query(ctx, `
		SELECT inf.value 
		FROM batch_pipeline_items bpi
		INNER JOIN inferences inf ON inf.id = bpi.persona_inference_id
		WHERE bpi.pipeline_id = $1::uuid AND bpi.status = 'completed'
	`, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("query finished inferences: %w", err)
	}
	defer rows.Close()

	roleCounts := make(map[string]int)
	cpCounts := make(map[string]int)
	slangCounts := make(map[string]int)
	domainStats := make(map[string]*struct {
		count      int
		totalScore float64
	})
	var totalPersons int

	for rows.Next() {
		var valJSON []byte
		if err := rows.Scan(&valJSON); err != nil {
			continue
		}
		var p PersonaProfile
		if err := json.Unmarshal(valJSON, &p); err != nil {
			continue
		}
		totalPersons++

		role := p.SocialArchetype.PrimaryRole
		if role == "" {
			role = "未定型"
		}
		roleCounts[role]++

		for _, cp := range p.LinguisticFingerprint.Catchphrases {
			if cp != "" {
				cpCounts[cp]++
			}
		}
		for _, sl := range p.LinguisticFingerprint.SlangAndSubculture {
			if sl != "" {
				slangCounts[sl]++
			}
		}
		for _, dom := range p.InterestSpectrum {
			if dom.Domain != "" {
				st, ok := domainStats[dom.Domain]
				if !ok {
					st = &struct {
						count      int
						totalScore float64
					}{}
					domainStats[dom.Domain] = st
				}
				st.count++
				st.totalScore += dom.Score
			}
		}
	}

	if totalPersons == 0 {
		return &TaskSummaryData{TotalPersons: 0, CompiledAt: time.Now()}, nil
	}

	// 1. Roles
	var roles []SummaryRole
	for name, count := range roleCounts {
		roles = append(roles, SummaryRole{
			Name:    name,
			Count:   count,
			Percent: int(math.Round(float64(count) / float64(totalPersons) * 100)),
		})
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Count > roles[j].Count })

	// 2. Catchphrases
	var catchphrases []SummaryPhrase
	for phrase, count := range cpCounts {
		catchphrases = append(catchphrases, SummaryPhrase{Phrase: phrase, Count: count})
	}
	sort.Slice(catchphrases, func(i, j int) bool { return catchphrases[i].Count > catchphrases[j].Count })
	if len(catchphrases) > 15 {
		catchphrases = catchphrases[:15]
	}

	// 3. Slang
	var slang []SummaryPhrase
	for phrase, count := range slangCounts {
		slang = append(slang, SummaryPhrase{Phrase: phrase, Count: count})
	}
	sort.Slice(slang, func(i, j int) bool { return slang[i].Count > slang[j].Count })
	if len(slang) > 15 {
		slang = slang[:15]
	}

	// 4. Domains
	var domains []SummaryDomain
	for name, st := range domainStats {
		avg := 50
		if st.count > 0 {
			avg = int(math.Round(st.totalScore / float64(st.count) * 100))
		}
		domains = append(domains, SummaryDomain{
			Name:     name,
			Count:    st.count,
			AvgScore: avg,
		})
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].Count > domains[j].Count })
	if len(domains) > 8 {
		domains = domains[:8]
	}

	return &TaskSummaryData{
		TotalPersons: totalPersons,
		Roles:        roles,
		Catchphrases: catchphrases,
		Slang:        slang,
		Domains:      domains,
		CompiledAt:   time.Now(),
	}, nil
}
