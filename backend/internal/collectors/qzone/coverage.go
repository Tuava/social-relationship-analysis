package qzone

import (
	"context"
	"fmt"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type coverageEntry struct {
	items          int
	pages          int
	partial        bool
	partialTargets int
	targets        int
	error          string
	cursor         map[string]any
	scope          string
}

// coverageTracker records the boundary of what a QZone sync actually saw.
// It deliberately lives beside the collector so parser limitations do not get
// mistaken for a complete scan by the API layer.
type coverageTracker struct {
	entries  map[string]map[string]*coverageEntry
	onUpdate func(string, coverageEntry)
}

func newCoverageTracker(onUpdate ...func(string, coverageEntry)) *coverageTracker {
	tracker := &coverageTracker{entries: map[string]map[string]*coverageEntry{}}
	if len(onUpdate) > 0 {
		tracker.onUpdate = onUpdate[0]
	}
	return tracker
}

func (t *coverageTracker) begin(qq, source string) {
	qq = strings.TrimSpace(qq)
	if qq == "" || source == "" {
		return
	}
	bySource := t.entries[qq]
	if bySource == nil {
		bySource = map[string]*coverageEntry{}
		t.entries[qq] = bySource
	}
	if bySource[source] == nil {
		bySource[source] = &coverageEntry{}
	}
}

func (t *coverageTracker) add(qq, source string, count int) {
	t.begin(qq, source)
	if entry := t.entries[qq][source]; entry != nil {
		entry.pages++
		if count > 0 {
			entry.items += count
		}
	}
	t.notify(source)
}

func (t *coverageTracker) setCursor(qq, source string, cursor map[string]any) {
	t.begin(qq, source)
	entry := t.entries[qq][source]
	entry.scope = qq
	entry.cursor = cursor
	t.notify(source)
}

func (t *coverageTracker) summary() map[string]coverageEntry {
	result := map[string]coverageEntry{}
	for qq, bySource := range t.entries {
		for source, entry := range bySource {
			summary := result[source]
			summary.targets++
			summary.items += entry.items
			summary.pages += entry.pages
			summary.partial = summary.partial || entry.partial
			if entry.partial {
				summary.partialTargets++
			}
			if summary.error == "" && entry.error != "" {
				summary.error = entry.error
			}
			if entry.cursor != nil {
				summary.scope = qq
				summary.cursor = entry.cursor
			}
			result[source] = summary
		}
	}
	return result
}

func (t *coverageTracker) markPartial(qq, source string, err error) {
	t.begin(qq, source)
	entry := t.entries[qq][source]
	entry.partial = true
	if err != nil && entry.error == "" {
		entry.error = err.Error()
	}
	t.notify(source)
}

func (t *coverageTracker) notify(source string) {
	if t.onUpdate == nil {
		return
	}
	if summary, ok := t.summary()[source]; ok {
		t.onUpdate(source, summary)
	}
}

func (t *coverageTracker) finish(ctx context.Context, repo persistence.Repository, account domain.NapCatAccount) {
	for qq, bySource := range t.entries {
		var personID string
		if err := repo.DB.QueryRow(ctx, `
			SELECT p.id::text
			FROM persons p
			JOIN person_identifiers pi ON pi.person_id=p.id
			WHERE pi.platform='qq' AND pi.platform_user_id=$1
			LIMIT 1`, qq).Scan(&personID); err != nil {
			continue
		}
		for source, entry := range bySource {
			status := "complete"
			if entry.partial {
				status = "partial"
			} else if entry.items == 0 {
				status = "no_results"
			}
			_, _ = repo.DB.Exec(ctx, `
				INSERT INTO collection_coverage(person_id, source_account_id, data_source, status, items_collected, last_collected_at, last_error)
				VALUES($1,$2,$3,$4,$5,now(),NULLIF($6,''))
				ON CONFLICT(person_id, source_account_id, data_source) DO UPDATE SET
					status=EXCLUDED.status,
					items_collected=EXCLUDED.items_collected,
					last_collected_at=EXCLUDED.last_collected_at,
					last_error=EXCLUDED.last_error`,
				personID, account.ID, source, status, entry.items, entry.error)
		}
	}
}

func (c Collector) syncVisitorCoverage(ctx context.Context, account domain.NapCatAccount, status string, items int, lastError string) {
	var personID string
	if err := c.Repo.DB.QueryRow(ctx, `SELECT person_id::text
		FROM person_identifiers
		WHERE platform='qq' AND platform_user_id=$1
		LIMIT 1`, account.QQUIN).Scan(&personID); err != nil {
		return
	}
	_, _ = c.Repo.DB.Exec(ctx, `INSERT INTO collection_coverage(person_id, source_account_id, data_source, status, items_collected, last_collected_at, last_error)
		VALUES($1,$2,'qzone_visits',$3,$4,now(),NULLIF($5,''))
		ON CONFLICT(person_id, source_account_id, data_source) DO UPDATE SET
			status=EXCLUDED.status,
			items_collected=EXCLUDED.items_collected,
			last_collected_at=EXCLUDED.last_collected_at,
			last_error=EXCLUDED.last_error`,
		personID, account.ID, status, items, lastError)
}

func coveragePartialError(scope string, page int, err error) error {
	return fmt.Errorf("%s page %d: %w", scope, page, err)
}
