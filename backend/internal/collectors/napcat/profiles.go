package napcat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

type ProfileSyncResult struct {
	Total  int `json:"total"`
	Synced int `json:"synced"`
	Failed int `json:"failed"`
}

// SyncPersonProfile retains the complete response because NapCat adds profile
// fields over time and callers must not depend on a fixed subset.
func (c Collector) SyncPersonProfile(ctx context.Context, account domain.NapCatAccount, personID, qq string) (map[string]any, string, error) {
	raw, err := NewHTTPClient(account.HTTPURL, account.HTTPToken).GetStrangerInfo(ctx, qq)
	if err != nil {
		return nil, "", err
	}
	rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_stranger_info:profile", raw)
	if err != nil {
		return nil, "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", err
	}
	nickname := profileValue(data, "nickname")
	remark := profileValue(data, "remark")
	if nickname != "" {
		_, _ = c.Repo.DB.Exec(ctx, `UPDATE persons SET display_name=$2,last_seen_at=now() WHERE id=$1`, personID, nickname)
	}
	if err := c.Normalizer.UpsertProfileData(ctx, personID, account.ID, nickname, "", remark, "get_stranger_info", &rawID, data); err != nil {
		return nil, "", err
	}
	if err := c.Normalizer.QueueAvatar(ctx, account.ID, personID, rawID, qq, ""); err != nil {
		return nil, "", err
	}
	return data, rawID, nil
}

// RunProfileSyncWithProgress performs one NapCat request at a time for all eligible contacts.
func (c Collector) RunProfileSyncWithProgress(ctx context.Context, account domain.NapCatAccount, update func(int)) (ProfileSyncResult, error) {
	return c.RunProfileSyncWithScope(ctx, account, domain.CollectionScope{}, update)
}

// RunProfileSyncWithScope performs one NapCat request at a time within the given scope.
// If scope.Entries is specified, it targets only the specified QQs and group members.
func (c Collector) RunProfileSyncWithScope(ctx context.Context, account domain.NapCatAccount, scope domain.CollectionScope, update func(int)) (ProfileSyncResult, error) {
	result := ProfileSyncResult{}
	if strings.TrimSpace(account.HTTPURL) == "" {
		return result, fmt.Errorf("HTTP API URL is required")
	}

	type targetItem struct {
		personID string
		qq       string
	}
	var targets []targetItem

	// 1. If scoped entries are provided, resolve candidate targets explicitly
	if len(scope.Entries) > 0 {
		seenQQ := make(map[string]bool)
		for _, qq := range scope.ExcludedQQs {
			seenQQ[strings.TrimSpace(qq)] = true
		}

		for _, entry := range scope.Entries {
			cleanID := strings.TrimSpace(entry.ID)
			if cleanID == "" {
				continue
			}
			if entry.Type == "qq" || entry.Type == "conversation" {
				cleanQQ := strings.TrimPrefix(cleanID, "private:")
				if seenQQ[cleanQQ] {
					continue
				}
				seenQQ[cleanQQ] = true
				var pid string
				if err := c.Repo.DB.QueryRow(ctx, `SELECT person_id::text FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, cleanQQ).Scan(&pid); err == nil {
					targets = append(targets, targetItem{personID: pid, qq: cleanQQ})
				} else {
					// Also support syncing stranger directly by querying/creating person
					newPID, err := c.Normalizer.UpsertPerson(ctx, cleanQQ, "")
					if err == nil {
						targets = append(targets, targetItem{personID: newPID, qq: cleanQQ})
					}
				}
			} else if entry.Type == "group" {
				groupRows, err := c.Repo.DB.Query(ctx, `
					SELECT DISTINCT pi.person_id::text, pi.platform_user_id
					FROM group_memberships gm
					JOIN "groups" g ON g.id = gm.group_id
					JOIN person_identifiers pi ON pi.person_id = gm.person_id AND pi.platform = 'qq'
					WHERE (g.platform_group_id = $1 OR g.id::text = $1)`, cleanID)
				if err == nil {
					for groupRows.Next() {
						var item targetItem
						if err := groupRows.Scan(&item.personID, &item.qq); err == nil && !seenQQ[item.qq] {
							seenQQ[item.qq] = true
							targets = append(targets, item)
						}
					}
					groupRows.Close()
				}
			}
		}
		result.Total = len(targets)
	} else {
		// 2. Global scan for contacts not synced within 7 days
		if err := c.Repo.DB.QueryRow(ctx, `SELECT count(*) FROM person_identifiers pi
			WHERE pi.platform='qq' AND NOT EXISTS (
				SELECT 1 FROM person_profiles pp WHERE pp.person_id=pi.person_id
				  AND pp.source='get_stranger_info' AND pp.source_account_id=$1
				  AND pp.valid_from >= now()-interval '7 days'
			)`, account.ID).Scan(&result.Total); err != nil {
			return result, err
		}
	}

	if result.Total == 0 {
		if update != nil {
			update(100)
		}
		return result, nil
	}

	// If explicit targets were resolved, process them directly
	if len(targets) > 0 {
		for i, item := range targets {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if _, _, err := c.SyncPersonProfile(ctx, account, item.personID, item.qq); err != nil {
				result.Failed++
				c.logError(account, "get_stranger_info:"+item.qq, err)
			} else {
				result.Synced++
			}
			progress := (i + 1) * 100 / result.Total
			if update != nil {
				update(progress)
			}
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
		return result, nil
	}

	// Otherwise, batch query paginated candidates from DB
	lastPersonID := ""
	processed := 0
	lastProgress := -1
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var afterID any
		if lastPersonID != "" {
			afterID = lastPersonID
		}
		rows, err := c.Repo.DB.Query(ctx, `SELECT pi.person_id::text, pi.platform_user_id
			FROM person_identifiers pi
			WHERE pi.platform='qq' AND ($2::uuid IS NULL OR pi.person_id > $2::uuid)
			  AND NOT EXISTS (
				SELECT 1 FROM person_profiles pp WHERE pp.person_id=pi.person_id
				  AND pp.source='get_stranger_info' AND pp.source_account_id=$1
				  AND pp.valid_from >= now()-interval '7 days'
			  )
			ORDER BY pi.person_id LIMIT 100`, account.ID, afterID)
		if err != nil {
			return result, err
		}
		batch := make([]targetItem, 0, 100)
		for rows.Next() {
			var item targetItem
			if err := rows.Scan(&item.personID, &item.qq); err != nil {
				rows.Close()
				return result, err
			}
			batch = append(batch, item)
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return result, rowsErr
		}
		if len(batch) == 0 {
			break
		}

		for _, item := range batch {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			lastPersonID = item.personID
			if _, _, err := c.SyncPersonProfile(ctx, account, item.personID, item.qq); err != nil {
				result.Failed++
				c.logError(account, "get_stranger_info:"+item.qq, err)
			} else {
				result.Synced++
			}
			processed++
			progress := processed * 100 / result.Total
			if update != nil && progress != lastProgress {
				update(progress)
				lastProgress = progress
			}
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	return result, nil
}

func profileValue(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
