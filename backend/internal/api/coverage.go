package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
)

var DataSourceLabels = map[string]string{
	"basic_profile":     "基础资料",
	"group_memberships": "群成员关系",
	"group_messages":    "群历史消息",
	"private_messages":  "私聊历史",
	"qzone_feeds":       "空间动态",
	"qzone_comments":    "动态评论",
	"qzone_likes":       "动态点赞",
	"qzone_visits":      "空间访客",
	"avatar":            "头像",
	"media":             "媒体资源",
}

func UpdateCoverage(ctx context.Context, db *pgxpool.Pool, personID, accountID, dataSource, status string, itemsCollected int, lastError string) {
	_, _ = db.Exec(ctx, `
		INSERT INTO collection_coverage(person_id, source_account_id, data_source, status, items_collected, last_collected_at, last_error)
		VALUES($1, NULLIF($2,'')::uuid, $3, $4, $5, now(), NULLIF($6,''))
		ON CONFLICT(person_id, source_account_id, data_source) DO UPDATE SET
			status=EXCLUDED.status, items_collected=EXCLUDED.items_collected,
			last_collected_at=EXCLUDED.last_collected_at, last_error=EXCLUDED.last_error`,
		personID, accountID, dataSource, status, itemsCollected, lastError)
}

func (s *Server) coverage(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")

	for source := range DataSourceLabels {
		_, _ = s.Repo.DB.Exec(r.Context(), `
			INSERT INTO collection_coverage(person_id, data_source, status)
			SELECT $1,$2,'not_collected'
			WHERE NOT EXISTS (SELECT 1 FROM collection_coverage WHERE person_id=$1 AND data_source=$2)`, personID, source)
	}

	rows, err := s.Repo.DB.Query(r.Context(), `SELECT data_source, status, items_collected, items_total, last_collected_at, last_error, COALESCE(source_account_id::text,'')
		FROM collection_coverage WHERE person_id=$1 ORDER BY data_source, last_collected_at DESC NULLS LAST, source_account_id NULLS LAST`, personID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()

	type dataCount struct {
		Source     string `json:"source"`
		Label      string `json:"label"`
		Status     string `json:"status"`
		Items      int    `json:"items_collected"`
		ItemsTotal *int   `json:"items_total,omitempty"`
		Last       any    `json:"last_collected_at,omitempty"`
		Error      string `json:"last_error,omitempty"`
		AccountID  string `json:"account_id,omitempty"`
	}

	results := []dataCount{}
	coverageMap := map[string]*dataCount{}
	for rows.Next() {
		var dc dataCount
		var lastError *string
		if err := rows.Scan(&dc.Source, &dc.Status, &dc.Items, &dc.ItemsTotal, &dc.Last, &lastError, &dc.AccountID); err != nil {
			writeError(w, 500, err)
			return
		}
		if lastError != nil {
			dc.Error = *lastError
		}
		dc.Label = DataSourceLabels[dc.Source]
		if dc.Label == "" {
			dc.Label = dc.Source
		}
		// The query is ordered newest first. The first row is the current
		// account-scoped result; do not let a legacy placeholder replace it.
		if _, exists := coverageMap[dc.Source]; !exists {
			coverageMap[dc.Source] = &dc
			results = append(results, dc)
		}
	}

	// Infer status from actual data
	infer := func(source, query string) {
		var tmp int
		s.Repo.DB.QueryRow(r.Context(), query, personID).Scan(&tmp)
		if entry, ok := coverageMap[source]; ok && tmp > 0 {
			if entry.Status == "not_collected" {
				entry.Status = "inferred"
			}
			if entry.Items < tmp {
				entry.Items = tmp
			}
		}
	}
	infer("basic_profile", `SELECT count(*) FROM person_profiles WHERE person_id=$1`)
	infer("group_memberships", `SELECT count(*) FROM group_memberships WHERE person_id=$1`)
	infer("group_messages", `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='group'`)
	infer("private_messages", `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='private'`)
	infer("qzone_feeds", `SELECT count(*) FROM contents WHERE author_id=$1`)
	infer("qzone_comments", `SELECT count(*) FROM relation_events re WHERE re.action_type='commented' AND re.actor_person_id=$1`)
	infer("qzone_likes", `SELECT count(*) FROM relation_events re WHERE re.action_type='liked' AND re.actor_person_id=$1`)
	infer("qzone_visits", `SELECT count(*) FROM relation_events re WHERE re.action_type='visited' AND re.actor_person_id=$1`)
	infer("avatar", `SELECT count(*) FROM media_references WHERE person_id=$1 AND media_kind='avatar'`)

	writeJSON(w, 200, map[string]any{"data": results})
}

func (s *Server) collectPerson(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	var in struct {
		DataSource string `json:"data_source"`
		AccountID  string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.DataSource == "" {
		writeJSON(w, 400, map[string]string{"error": "data_source is required"})
		return
	}

	var qq string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT platform_user_id FROM person_identifiers WHERE person_id=$1 AND platform='qq' LIMIT 1`, personID).Scan(&qq); err != nil {
		writeJSON(w, 404, map[string]string{"error": "QQ not found for this person"})
		return
	}

	accountID := in.AccountID
	if accountID == "" {
		if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text FROM napcat_accounts WHERE enabled=true ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
			writeJSON(w, 400, map[string]string{"error": "no enabled NapCat account"})
			return
		}
	}
	account, err := s.Repo.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, 404, err)
		return
	}

	UpdateCoverage(r.Context(), s.Repo.DB, personID, account.ID, in.DataSource, "collecting", 0, "")

	go func() {
		ctx := context.Background()
		client := napcat.NewHTTPClient(account.HTTPURL, account.HTTPToken)
		switch in.DataSource {
		case "basic_profile":
			raw, err := client.GetStrangerInfo(ctx, qq)
			if err != nil {
				UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "basic_profile", "failed", 0, err.Error())
				return
			}
			s.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_stranger_info", raw)
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "basic_profile", "complete", 1, "")

		case "avatar":
			// Queue avatar download using canonical QQ avatar URL
			canonicalURL := "https://q1.qlogo.cn/g?b=qq&nk=" + qq + "&s=640"
			_, err := s.Repo.DB.Exec(ctx, `INSERT INTO media_references(source_account_id,person_id,segment_type,media_kind,source_url,metadata,status)
				VALUES($1,$2,'avatar','avatar',$3,jsonb_build_object('qq',$4),'pending')
				ON CONFLICT(source_account_id,person_id,source_url) WHERE person_id IS NOT NULL AND media_kind='avatar'
				DO UPDATE SET status=CASE WHEN media_references.status='completed' THEN 'completed' ELSE 'pending' END,
					attempt_count=CASE WHEN media_references.status='completed' THEN media_references.attempt_count ELSE 0 END`,
				account.ID, personID, canonicalURL, qq)
			if err != nil {
				UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "avatar", "failed", 0, err.Error())
				return
			}
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "avatar", "complete", 1, "")

		case "private_messages":
			raw, err := client.GetFriendMsgHistory(ctx, qq, 50, 0)
			if err != nil {
				UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "private_messages", "failed", 0, err.Error())
				return
			}
			s.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_friend_msg_history:"+qq, raw)
			// Count messages in response
			var resp struct {
				Data []any `json:"data"`
			}
			count := 0
			if json.Unmarshal(raw, &resp) == nil {
				count = len(resp.Data)
			}
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "private_messages", "complete", count, "")

		case "group_memberships":
			// Get group list, then for each group check if person is a member
			groupsRaw, err := client.GetGroupList(ctx)
			if err != nil {
				UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "group_memberships", "failed", 0, err.Error())
				return
			}
			s.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_group_list", groupsRaw)
			var groupsResp []struct {
				GroupID any `json:"group_id"`
			}
			count := 0
			if json.Unmarshal(groupsRaw, &groupsResp) == nil {
				for _, g := range groupsResp {
					membersRaw, mErr := client.GetGroupMemberList(ctx, g.GroupID)
					if mErr != nil {
						continue
					}
					s.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_group_member_list:"+fmt.Sprint(g.GroupID), membersRaw)
					var members []struct {
						UserID any `json:"user_id"`
					}
					if json.Unmarshal(membersRaw, &members) == nil {
						for _, m := range members {
							if fmt.Sprint(m.UserID) == qq {
								count++
								break
							}
						}
					}
				}
			}
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "group_memberships", "complete", count, "")

		case "group_messages":
			// Query DB for groups this person is in, then get group history
			rows, err := s.Repo.DB.Query(ctx, `SELECT g.platform_group_id FROM groups g
				JOIN group_memberships gm ON gm.group_id=g.id
				JOIN person_identifiers pi ON pi.person_id=gm.person_id
				WHERE pi.platform='qq' AND pi.platform_user_id=$1`, qq)
			if err != nil {
				UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "group_messages", "failed", 0, err.Error())
				return
			}
			var groupIDs []string
			for rows.Next() {
				var gid string
				if rows.Scan(&gid) == nil {
					groupIDs = append(groupIDs, gid)
				}
			}
			rows.Close()
			totalMsgs := 0
			for _, gid := range groupIDs {
				historyRaw, hErr := client.GetGroupHistory(ctx, gid, 50, 0)
				if hErr != nil {
					continue
				}
				s.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_group_history:"+gid, historyRaw)
				var history []any
				if json.Unmarshal(historyRaw, &history) == nil {
					totalMsgs += len(history)
				}
			}
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, "group_messages", "complete", totalMsgs, "")

		default:
			UpdateCoverage(ctx, s.Repo.DB, personID, account.ID, in.DataSource, "not_collected", 0, "trigger a full collection run for this data source")
		}
	}()

	writeJSON(w, 202, map[string]any{"status": "collecting", "data_source": in.DataSource, "qq": qq})
}
