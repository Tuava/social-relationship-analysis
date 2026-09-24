package napcat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type Collector struct {
	Cursors    CursorStore
	Repo       persistence.Repository
	Logger     *slog.Logger
	Normalizer normalization.Normalizer
}

func (c Collector) RunVisibleDataWithProgress(ctx context.Context, account domain.NapCatAccount, runID string, update func(int, error)) error {
	return c.RunVisibleDataWithScope(ctx, account, runID, (domain.CollectionScope{}).Normalize(account.QQUIN), domain.CollectionObserver{}, update)
}

func (c Collector) RunVisibleDataWithScope(ctx context.Context, account domain.NapCatAccount, runID string, scope domain.CollectionScope, observer domain.CollectionObserver, update func(int, error)) error {
	if update != nil {
		update(1, nil)
	}
	err := c.runVisibleData(ctx, account, runID, scope.Normalize(account.QQUIN), observer, update)
	if update != nil {
		update(100, err)
	}
	return err
}

func (c Collector) RunVisibleData(ctx context.Context, account domain.NapCatAccount, runID string) error {
	return c.runVisibleData(ctx, account, runID, (domain.CollectionScope{}).Normalize(account.QQUIN), domain.CollectionObserver{}, nil)
}

func (c Collector) runVisibleData(ctx context.Context, account domain.NapCatAccount, runID string, scope domain.CollectionScope, observer domain.CollectionObserver, update func(int, error)) error {
	client := NewHTTPClient(account.HTTPURL, account.HTTPToken)
	if account.HTTPURL == "" {
		return fmt.Errorf("HTTP API URL is required")
	}
	emit := func(module, status string, pages int, records int64, moduleErr error) {
		if observer.Module == nil {
			return
		}
		message := ""
		if moduleErr != nil {
			message = moduleErr.Error()
		}
		observer.Module(domain.CollectionModuleProgress{Module: module, Status: status, PagesCompleted: pages, RecordsCollected: records, Error: message})
	}
	mediaReferenceStart := c.mediaReferenceCount(ctx, account.ID)
	steps := []struct {
		name, module string
		load         func(context.Context) (json.RawMessage, error)
	}{
		{"get_login_info", "profile", client.GetLoginInfo},
		{"get_group_list", "groups", client.GetGroupList},
		{"get_friend_list", "friends", client.GetFriendList},
	}
	for index, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		emit(step.module, "running", 0, 0, nil)
		raw, err := step.load(ctx)
		if err != nil {
			emit(step.module, "failed", 0, 0, err)
			return err
		}
		if err := c.saveResponse(ctx, account, runID, step.name, raw); err != nil {
			emit(step.module, "failed", 0, 0, err)
			return err
		}
		if step.name == "get_login_info" {
			if err := c.normalizeLogin(ctx, account, raw, step.name); err != nil {
				c.logError(account, step.name+":normalize", err)
			}
		}
		if step.name == "get_friend_list" {
			if err := c.normalizeFriends(ctx, account, raw, step.name); err != nil {
				c.logError(account, step.name+":normalize", err)
			}
			c.observeQQCandidates(raw, 1, "friend", scope, observer)
		}
		emit(step.module, "complete", 1, int64(jsonItemCount(raw)), nil)
		if update != nil {
			update(2+(index+1)*2, nil)
		}
	}
	groups, err := client.GetGroupList(ctx)
	if err != nil {
		return err
	}
	var groupList []struct {
		GroupID   any    `json:"group_id"`
		GroupName string `json:"group_name"`
	}
	if err := decodeJSON(groups, &groupList); err != nil {
		return fmt.Errorf("decode group list: %w", err)
	}
	selectedGroups := make([]struct {
		GroupID   any    `json:"group_id"`
		GroupName string `json:"group_name"`
	}, 0, len(groupList))
	groupModes := make(map[string]string, len(groupList))
	groupListRawID, _ := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", "get_group_list", groups)
	for _, group := range groupList {
		groupID := fmt.Sprint(group.GroupID)
		if mode, allowed := scope.GroupMode(groupID); allowed {
			selectedGroups = append(selectedGroups, group)
			groupModes[groupID] = mode
		}
	}
	// get_recent_contact primes QQ's AIO cache. Without this, inactive group
	// histories can return an empty latest page even though the group is visible.
	if len(selectedGroups) > 0 {
		if recent, recentErr := client.GetRecentContacts(ctx); recentErr != nil {
			c.logError(account, "get_recent_contact:history_warmup", recentErr)
		} else if saveErr := c.saveResponse(ctx, account, runID, "get_recent_contact:history_warmup", recent); saveErr != nil {
			c.logError(account, "get_recent_contact:history_warmup:save", saveErr)
		}
	}
	emit("group_members", "running", 0, 0, nil)
	emit("group_messages", "running", 0, 0, nil)
	memberRecords := int64(0)
	historyGroups := 0
	historyPages := 0
	historyRecords := int64(0)
	var historyCursor map[string]any
	memberFailures := 0
	historyFailures := 0
	failedHistoryGroups := []string{}
	unconfirmedHistoryGroups := []string{}
	for index, group := range selectedGroups {
		if err := ctx.Err(); err != nil {
			return err
		}
		groupID := fmt.Sprint(group.GroupID)
		groupName := domain.CleanDisplayText(group.GroupName)
		var groupUUID string
		if err := c.Repo.DB.QueryRow(ctx, `INSERT INTO groups(platform,platform_group_id,group_name) VALUES('qq',$1,$2)
			ON CONFLICT(platform,platform_group_id) DO UPDATE SET group_name=CASE WHEN EXCLUDED.group_name='' THEN groups.group_name ELSE EXCLUDED.group_name END
			RETURNING id::text`, groupID, groupName).Scan(&groupUUID); err == nil {
			if avatarErr := c.Normalizer.QueueGroupAvatar(ctx, account.ID, groupUUID, groupListRawID, groupID, ""); avatarErr != nil {
				c.logError(account, "queue_group_avatar:"+groupID, avatarErr)
			}
		}
		memberEndpoint := fmt.Sprintf("get_group_member_list:%s", groupID)
		members, memberErr := client.GetGroupMemberList(ctx, groupID)
		if memberErr == nil {
			memberErr = c.saveResponse(ctx, account, runID, memberEndpoint, members)
		}
		if memberErr != nil {
			memberFailures++
			c.logError(account, memberEndpoint, memberErr)
		} else {
			memberRecords += int64(jsonItemCount(members))
			if err := c.normalizeMembers(ctx, account, group.GroupID, members, memberEndpoint); err != nil {
				memberFailures++
				c.logError(account, memberEndpoint+":normalize", err)
			}
			c.observeGroupCandidates(members, groupID, groupModes[groupID], scope, observer)
		}
		if groupModes[groupID] != domain.GroupModeRecordOnly {
			historyGroups++
			groupPages := 0
			groupRecords := int64(0)
			if err := c.collectGroupHistory(ctx, account, client, groupID, func(progress domain.HistoryProgress) {
				historyPages += progress.Pages - groupPages
				historyRecords += progress.Records - groupRecords
				groupPages, groupRecords = progress.Pages, progress.Records
				historyCursor = progress.Cursor
				emitProgress(observer, domain.CollectionModuleProgress{Module: "group_messages", Status: "running", PagesCompleted: historyPages, RecordsCollected: historyRecords, Cursor: historyCursor})
			}); err != nil {
				historyFailures++
				failedHistoryGroups = append(failedHistoryGroups, groupID)
				c.logError(account, fmt.Sprintf("get_group_msg_history:%s", groupID), err)
			} else if confirmed, ok := historyCursor["boundary_confirmed"].(bool); !ok || !confirmed {
				unconfirmedHistoryGroups = append(unconfirmedHistoryGroups, groupID)
			}
		}
		emit("group_members", "running", index+1, memberRecords, nil)
		emitProgress(observer, domain.CollectionModuleProgress{Module: "group_messages", Status: "running", PagesCompleted: historyPages, RecordsCollected: historyRecords, Cursor: historyCursor})
		if update != nil && len(selectedGroups) > 0 {
			update(8+(index+1)*78/len(selectedGroups), nil)
		}
	}
	if memberFailures > 0 {
		emit("group_members", "partial", len(selectedGroups), memberRecords, fmt.Errorf("%d groups failed", memberFailures))
	} else {
		emit("group_members", "complete", len(selectedGroups), memberRecords, nil)
	}
	if historyFailures > 0 || len(unconfirmedHistoryGroups) > 0 {
		if historyCursor == nil {
			historyCursor = map[string]any{}
		}
		historyCursor["failed_groups"] = failedHistoryGroups
		historyCursor["unconfirmed_boundary_groups"] = unconfirmedHistoryGroups
		emitProgress(observer, domain.CollectionModuleProgress{Module: "group_messages", Status: "partial", PagesCompleted: historyPages, RecordsCollected: historyRecords, Cursor: historyCursor, Error: fmt.Sprintf("failed=%d/%d [%s]; boundary_unconfirmed=%d [%s]", historyFailures, historyGroups, strings.Join(failedHistoryGroups, ","), len(unconfirmedHistoryGroups), strings.Join(unconfirmedHistoryGroups, ","))})
	} else {
		emitProgress(observer, domain.CollectionModuleProgress{Module: "group_messages", Status: "complete", PagesCompleted: historyPages, RecordsCollected: historyRecords, Cursor: historyCursor})
	}

	// Collect private chat history for each friend
	emit("private_messages", "running", 0, 0, nil)
	friendRaw, friendErr := client.GetFriendList(ctx)
	privatePages := 0
	privateRecords := int64(0)
	var privateCursor map[string]any
	privateFailures := 0
	failedPrivateQQ := []string{}
	unconfirmedPrivateQQ := []string{}
	if friendErr != nil {
		privateFailures++
		c.logError(account, "get_friend_list:history", friendErr)
		failedPrivateQQ = append(failedPrivateQQ, "friend_list")
		emit("private_messages", "failed", 0, 0, friendErr)
	} else {
		var friendList []map[string]any
		if err := decodeJSON(friendRaw, &friendList); err == nil {
			for _, friend := range friendList {
				if err := ctx.Err(); err != nil {
					return err
				}
				friendQQ := fmt.Sprint(friend["user_id"])
				if friendQQ == "" || friendQQ == "<nil>" {
					continue
				}
				if !scope.PrivateConversationAllowed(friendQQ) {
					continue
				}
				conversationPages := 0
				conversationRecords := int64(0)
				if err := c.collectFriendHistory(ctx, account, client, friendQQ, func(progress domain.HistoryProgress) {
					privatePages += progress.Pages - conversationPages
					privateRecords += progress.Records - conversationRecords
					conversationPages, conversationRecords = progress.Pages, progress.Records
					privateCursor = progress.Cursor
					emitProgress(observer, domain.CollectionModuleProgress{Module: "private_messages", Status: "running", PagesCompleted: privatePages, RecordsCollected: privateRecords, Cursor: privateCursor})
				}); err != nil {
					privateFailures++
					failedPrivateQQ = append(failedPrivateQQ, friendQQ)
					c.logError(account, fmt.Sprintf("get_friend_msg_history:%s", friendQQ), err)
				} else if confirmed, ok := privateCursor["boundary_confirmed"].(bool); !ok || !confirmed {
					unconfirmedPrivateQQ = append(unconfirmedPrivateQQ, friendQQ)
				}
				emitProgress(observer, domain.CollectionModuleProgress{Module: "private_messages", Status: "running", PagesCompleted: privatePages, RecordsCollected: privateRecords, Cursor: privateCursor})
			}
		}
		if privateFailures > 0 || len(unconfirmedPrivateQQ) > 0 {
			if privateCursor == nil {
				privateCursor = map[string]any{}
			}
			privateCursor["failed_conversations"] = failedPrivateQQ
			privateCursor["unconfirmed_boundary_conversations"] = unconfirmedPrivateQQ
			emitProgress(observer, domain.CollectionModuleProgress{Module: "private_messages", Status: "partial", PagesCompleted: privatePages, RecordsCollected: privateRecords, Cursor: privateCursor, Error: fmt.Sprintf("failed=%d [%s]; boundary_unconfirmed=%d [%s]", privateFailures, strings.Join(failedPrivateQQ, ","), len(unconfirmedPrivateQQ), strings.Join(unconfirmedPrivateQQ, ","))})
		} else {
			emitProgress(observer, domain.CollectionModuleProgress{Module: "private_messages", Status: "complete", PagesCompleted: privatePages, RecordsCollected: privateRecords, Cursor: privateCursor})
		}
	}
	mediaCollected := countDelta(mediaReferenceStart, c.mediaReferenceCount(ctx, account.ID))
	emit("media", "running", 0, mediaCollected, nil)
	emit("media", "complete", 1, mediaCollected, nil)
	c.syncSelfCoverage(ctx, account, map[string]struct {
		status string
		error  string
	}{
		"basic_profile":     {status: "complete"},
		"group_memberships": {status: map[bool]string{true: "partial", false: "complete"}[memberFailures > 0], error: errorText(memberFailures, "group membership requests failed")},
		"group_messages":    {status: map[bool]string{true: "partial", false: "complete"}[historyFailures > 0 || len(unconfirmedHistoryGroups) > 0], error: errorText(historyFailures+len(unconfirmedHistoryGroups), "history boundaries are not fully confirmed")},
		"private_messages":  {status: map[bool]string{true: "partial", false: "complete"}[privateFailures > 0 || len(unconfirmedPrivateQQ) > 0], error: errorText(privateFailures+len(unconfirmedPrivateQQ), "conversation history boundaries are not fully confirmed")},
		"media":             {status: "complete"},
	})

	return nil
}

func errorText(count int, message string) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%s (%d)", message, count)
}

func (c Collector) syncSelfCoverage(ctx context.Context, account domain.NapCatAccount, statuses map[string]struct {
	status string
	error  string
}) {
	var personID string
	if err := c.Repo.DB.QueryRow(ctx, `SELECT person_id::text FROM person_identifiers
		WHERE platform='qq' AND platform_user_id=$1 LIMIT 1\`, account.QQUIN).Scan(&personID); err != nil {
		return
	}
	queries := map[string]string{
		"basic_profile":     `SELECT COUNT(*) FROM person_profiles WHERE person_id=$1`,
		"group_memberships": `SELECT COUNT(*) FROM group_memberships WHERE person_id=$1`,
		"group_messages":    `SELECT COUNT(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='group'`,
		"private_messages":  `SELECT COUNT(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='private'`,
		"media":             `SELECT COUNT(*) FROM media_references WHERE source_account_id=$1`,
	}
	for source, state := range statuses {
		items := 0
		if query, ok := queries[source]; ok {
			if source == "media" {
				_ = c.Repo.DB.QueryRow(ctx, query, account.ID).Scan(&items)
			} else {
				_ = c.Repo.DB.QueryRow(ctx, query, personID).Scan(&items)
			}
		}
		_, _ = c.Repo.DB.Exec(ctx, `INSERT INTO collection_coverage(person_id, source_account_id, data_source, status, items_collected, last_collected_at, last_error)
			VALUES($1,$2,$3,$4,$5,now(),NULLIF($6,''))
			ON CONFLICT(person_id, source_account_id, data_source) DO UPDATE SET
				status=EXCLUDED.status,
				items_collected=EXCLUDED.items_collected,
				last_collected_at=EXCLUDED.last_collected_at,
			last_error=EXCLUDED.last_error`,
			personID, account.ID, source, state.status, items, state.error)
	}
}

func emitProgress(observer domain.CollectionObserver, progress domain.CollectionModuleProgress) {
	if observer.Module != nil {
		observer.Module(progress)
	}
}

func jsonItemCount(raw json.RawMessage) int {
	var values []any
	if decodeJSON(raw, &values) == nil {
		return len(values)
	}
	var wrapper map[string]any
	if decodeJSON(raw, &wrapper) == nil {
		for _, key := range []string{"items", "messages", "data", "list"} {
			if values, ok := wrapper[key].([]any); ok {
				return len(values)
			}
		}
		if len(wrapper) > 0 {
			return 1
		}
	}
	return 0
}

func (c Collector) observeQQCandidates(raw json.RawMessage, depth int, contextType string, scope domain.CollectionScope, observer domain.CollectionObserver) {
	if observer.Candidate == nil {
		return
	}
	var values []map[string]any
	if decodeJSON(raw, &values) != nil {
		return
	}
	for _, value := range values {
		qq := fmt.Sprint(value["user_id"])
		if qq == "" || qq == "<nil>" {
			continue
		}
		state := "collected"
		if _, allowed := scope.QQMode(qq); !allowed {
			state = "excluded"
		}
		observer.Candidate(domain.CollectionCandidate{EntityType: "qq", EntityID: qq, State: state, Depth: depth, Contexts: []string{contextType}, DiscoveryPath: map[string]any{"context": contextType}})
	}
}

func (c Collector) observeGroupCandidates(raw json.RawMessage, groupID, groupMode string, scope domain.CollectionScope, observer domain.CollectionObserver) {
	if observer.Candidate == nil {
		return
	}
	var values []map[string]any
	if decodeJSON(raw, &values) != nil {
		return
	}
	for _, value := range values {
		qq := fmt.Sprint(value["user_id"])
		if qq == "" || qq == "<nil>" {
			continue
		}
		state := "collected"
		if groupMode == domain.GroupModeFullExpand {
			state = "expandable"
		}
		if _, allowed := scope.QQMode(qq); !allowed {
			state = "excluded"
		}
		observer.Candidate(domain.CollectionCandidate{EntityType: "qq", EntityID: qq, State: state, Depth: 1, Contexts: []string{"group_member"}, DiscoveryPath: map[string]any{"context": "group_member", "group_id": groupID}})
	}
}

func (c Collector) messageCount(ctx context.Context, accountID, conversationType string) int64 {
	var count int64
	_ = c.Repo.DB.QueryRow(ctx, `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.source_account_id=$1 AND c.conversation_type=$2`, accountID, conversationType).Scan(&count)
	return count
}

func (c Collector) mediaReferenceCount(ctx context.Context, accountID string) int64 {
	var count int64
	_ = c.Repo.DB.QueryRow(ctx, `SELECT count(*) FROM media_references WHERE source_account_id=$1`, accountID).Scan(&count)
	return count
}

func countDelta(before, after int64) int64 {
	if after <= before {
		return 0
	}
	return after - before
}

func (c Collector) normalizeLogin(ctx context.Context, account domain.NapCatAccount, raw json.RawMessage, source string) error {
	var login struct {
		UserID   any    `json:"user_id"`
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(raw, &login); err != nil {
		return err
	}
	userID := fmt.Sprint(login.UserID)
	if userID == "" || userID == "<nil>" {
		userID = account.QQUIN
	}
	if userID == "" {
		return nil
	}
	personID, err := c.Normalizer.UpsertPerson(ctx, userID, login.Nickname)
	if err != nil {
		return err
	}
	rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", source+":profile", raw)
	if err != nil {
		return err
	}
	if err := c.Normalizer.UpsertProfile(ctx, personID, login.Nickname, "", "", source, &rawID); err != nil {
		return err
	}
	var loginMap map[string]any
	_ = decodeJSON(raw, &loginMap)
	avatarURL := ""
	if v, ok := loginMap["avatar"].(string); ok && v != "" {
		avatarURL = v
	}
	if v, ok := loginMap["avatar_url"].(string); ok && v != "" {
		avatarURL = v
	}
	if avatarURL != "" {
		_, _ = c.Repo.DB.Exec(ctx, "UPDATE person_profiles SET avatar_uri=$2 WHERE person_id=$1 AND avatar_uri=''", personID, avatarURL)
	}
	return c.Normalizer.QueueAvatar(ctx, account.ID, personID, rawID, userID, avatarURL)
}

func (c Collector) normalizeFriends(ctx context.Context, account domain.NapCatAccount, raw json.RawMessage, source string) error {
	var friends []map[string]any
	if err := decodeJSON(raw, &friends); err != nil {
		return err
	}
	for _, friend := range friends {
		userID := fmt.Sprint(friend["user_id"])
		if userID == "" || userID == "<nil>" {
			continue
		}
		nickname, _ := friend["nickname"].(string)
		remark, _ := friend["remark"].(string)
		personID, err := c.Normalizer.UpsertPerson(ctx, userID, nickname)
		if err != nil {
			return err
		}
		rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", source+":person", raw)
		if err != nil {
			return err
		}
		if err := c.Normalizer.UpsertProfileData(ctx, personID, account.ID, nickname, "", remark, source, &rawID, friend); err != nil {
			return err
		}
		avatarURL := ""
		if v, ok := friend["avatar"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := friend["avatar_url"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := friend["portrait_url"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := friend["figureurl"].(string); ok && v != "" {
			avatarURL = v
		}
		if avatarURL != "" {
			_, _ = c.Repo.DB.Exec(ctx, "UPDATE person_profiles SET avatar_uri=$2 WHERE person_id=$1 AND avatar_uri=''", personID, avatarURL)
		}
		if err := c.Normalizer.QueueAvatar(ctx, account.ID, personID, rawID, userID, avatarURL); err != nil {
			return err
		}
		if _, err := c.Repo.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_person_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) SELECT $1,self.id,$2,'friend_visible','private',now(),ARRAY[$3]::uuid[],$3 FROM persons self JOIN person_identifiers si ON si.person_id=self.id WHERE si.platform='qq' AND si.platform_user_id=$4 ON CONFLICT DO NOTHING`, account.ID, personID, rawID, account.QQUIN); err != nil {
			return err
		}
	}
	return nil
}

func (c Collector) normalizeMembers(ctx context.Context, account domain.NapCatAccount, groupID any, raw json.RawMessage, endpoint string) error {
	var members []map[string]any
	if err := decodeJSON(raw, &members); err != nil {
		return fmt.Errorf("decode members: %w", err)
	}
	var groupUUID string
	groupValue := fmt.Sprint(groupID)
	if err := c.Repo.DB.QueryRow(ctx, `INSERT INTO groups(platform,platform_group_id) VALUES('qq',$1) ON CONFLICT(platform,platform_group_id) DO UPDATE SET group_name=groups.group_name RETURNING id`, groupValue).Scan(&groupUUID); err != nil {
		return err
	}
	rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", endpoint, raw)
	if err != nil {
		return err
	}
	for _, member := range members {
		userID := fmt.Sprint(member["user_id"])
		if userID == "" || userID == "<nil>" {
			continue
		}
		nickname, _ := member["nickname"].(string)
		card, _ := member["card"].(string)
		role, _ := member["role"].(string)
		personID, err := c.Normalizer.UpsertPerson(ctx, userID, nickname)
		if err != nil {
			return err
		}
		if err := c.Normalizer.UpsertProfileData(ctx, personID, account.ID, nickname, "", "", endpoint, &rawID, member); err != nil {
			return err
		}
		avatarURL := ""
		if v, ok := member["avatar"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := member["avatar_url"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := member["portrait_url"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := member["figureurl"].(string); ok && v != "" {
			avatarURL = v
		}
		if v, ok := member["q_avatar"].(string); ok && v != "" {
			avatarURL = v
		}
		if avatarURL != "" {
			_, _ = c.Repo.DB.Exec(ctx, "UPDATE person_profiles SET avatar_uri=$2 WHERE person_id=$1 AND avatar_uri=''", personID, avatarURL)
		}
		if err := c.Normalizer.QueueAvatar(ctx, account.ID, personID, rawID, userID, avatarURL); err != nil {
			return err
		}
		if _, err := c.Repo.DB.Exec(ctx, `INSERT INTO group_memberships(group_id,person_id,source_account_id,role,card,raw_record_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(group_id,person_id,source_account_id) DO UPDATE SET role=EXCLUDED.role,card=EXCLUDED.card,raw_record_id=EXCLUDED.raw_record_id,valid_to=NULL`, groupUUID, personID, account.ID, role, card, rawID); err != nil {
			return err
		}
		_, err = c.Repo.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) VALUES($1,$2,$3,'member_of','group',now(),ARRAY[$4]::uuid[],$4) ON CONFLICT DO NOTHING`, account.ID, personID, groupUUID, rawID)
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func (c Collector) normalizeHistory(ctx context.Context, account domain.NapCatAccount, raw json.RawMessage, endpoint string) error {
	var wrapper struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return fmt.Errorf("decode history %s: %w", endpoint, err)
	}
	for _, message := range wrapper.Messages {
		rawID, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", endpoint+":message", message)
		if err != nil {
			return err
		}
		peer := ""
		if strings.HasPrefix(endpoint, "get_friend_msg_history:") {
			peer = strings.TrimPrefix(endpoint, "get_friend_msg_history:")
		}
		if err := c.Normalizer.ProcessRawMessage(ctx, account.ID, rawID, message, peer); err != nil && c.Logger != nil {
			c.Logger.Warn("normalize history message failed", "error", err)
		}
	}
	return nil
}

func (c Collector) saveResponse(ctx context.Context, account domain.NapCatAccount, runID, endpoint string, raw json.RawMessage) error {
	_, err := c.Repo.SaveRaw(ctx, account.ID, "napcat_http", endpoint, raw)
	return err
}
func (c Collector) logError(account domain.NapCatAccount, endpoint string, err error) {
	if c.Logger != nil {
		c.Logger.Warn("NapCat collection step failed", "account_id", account.ID, "endpoint", endpoint, "error", err)
	}
}
