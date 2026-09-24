package qzone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

var mentionRegex = regexp.MustCompile(`@\{uin:(\d+)(?:,nick:([^,}]*))?[^}]*\}`)

func extractMentions(text string, depth int) []discoveredPerson {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var res []discoveredPerson
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" && !seen[m[1]] {
			seen[m[1]] = true
			res = append(res, discoveredPerson{QQ: m[1], Depth: depth})
		}
	}
	return res
}

type Normalizer struct {
	Repo       persistence.Repository
	Normalizer normalization.Normalizer
}

type normalizedPost struct {
	ContentID     string
	TID           string
	AuthorQQ      string
	RelationDepth int
	PublishedAt   time.Time
}

type discoveredPerson struct {
	QQ    string
	Depth int
}

func (n Normalizer) Login(ctx context.Context, account domain.NapCatAccount, endpoint string, raw json.RawMessage) error {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	qq := firstText(value, "user_id", "uin")
	if qq == "" {
		qq = account.QQUIN
	}
	if qq == "" {
		return nil
	}
	nickname := firstText(value, "nickname", "name")
	personID, err := n.Normalizer.UpsertPerson(ctx, qq, nickname)
	if err != nil {
		return err
	}
	rawID, err := n.Repo.SaveRaw(ctx, account.ID, "qzone_http", endpoint+":profile", raw)
	if err != nil {
		return err
	}
	if err := n.Normalizer.UpsertProfileData(ctx, personID, account.ID, nickname, "", "", endpoint, &rawID, value); err != nil {
		return err
	}
	return n.queueAvatar(ctx, account.ID, personID, rawID, qq, "", 0)
}

func (n Normalizer) Friends(ctx context.Context, account domain.NapCatAccount, endpoint string, raw json.RawMessage) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	items := findMapSlice(value, "items", "friends", "list")
	selfID, _ := n.personID(ctx, account.QQUIN, "")
	for _, item := range items {
		qq := firstText(item, "user_id", "uin", "qq")
		if qq == "" {
			continue
		}
		rawID, err := n.saveItem(ctx, account.ID, endpoint+":friend", item)
		if err != nil {
			return err
		}
		personID, err := n.personID(ctx, qq, firstText(item, "nickname", "name"))
		if err != nil {
			return err
		}
		if err := n.Normalizer.UpsertProfileData(ctx, personID, account.ID, firstText(item, "nickname", "name"), "", firstText(item, "remark"), endpoint, &rawID, item); err != nil {
			return err
		}
		if err := n.queueAvatar(ctx, account.ID, personID, rawID, qq, "", 1); err != nil {
			return err
		}
		if selfID != "" {
			if err := n.relation(ctx, account.ID, selfID, &personID, nil, "friend_visible", "qzone", eventTimeFrom(item), rawID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n Normalizer) Visitors(ctx context.Context, account domain.NapCatAccount, endpoint string, raw json.RawMessage) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	items := findMapSlice(value, "items", "visitors", "visitorlist", "list")
	selfID, err := n.personID(ctx, account.QQUIN, "")
	if err != nil {
		return err
	}
	for _, item := range items {
		qq := firstText(item, "user_id", "uin", "qq")
		if qq == "" {
			continue
		}
		rawID, err := n.saveItem(ctx, account.ID, endpoint+":visitor", item)
		if err != nil {
			return err
		}
		personID, err := n.personID(ctx, qq, firstText(item, "nickname", "name"))
		if err != nil {
			return err
		}
		if err := n.Normalizer.UpsertProfileData(ctx, personID, account.ID, firstText(item, "nickname", "name"), "", "", endpoint, &rawID, item); err != nil {
			return err
		}
		if err := n.queueAvatar(ctx, account.ID, personID, rawID, qq, "", 1); err != nil {
			return err
		}
		if err := n.relation(ctx, account.ID, personID, &selfID, nil, "visited", "qzone_profile", eventTimeFrom(item), rawID); err != nil {
			return err
		}
	}
	return nil
}

func (n Normalizer) Posts(ctx context.Context, account domain.NapCatAccount, endpoint string, raw json.RawMessage) ([]normalizedPost, error) {
	return n.PostsAtDepth(ctx, account, endpoint, raw, 1)
}

func (n Normalizer) PostsAtDepth(ctx context.Context, account domain.NapCatAccount, endpoint string, raw json.RawMessage, relationDepth int) ([]normalizedPost, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := findMapSlice(value, "msglist", "posts", "items", "list")
	result := make([]normalizedPost, 0, len(items))
	for _, item := range items {
		post, err := n.PostAtDepth(ctx, account, endpoint+":post", item, relationDepth)
		if err != nil {
			return result, err
		}
		if post.ContentID != "" {
			result = append(result, post)
		}
	}
	return result, nil
}

func (n Normalizer) Post(ctx context.Context, account domain.NapCatAccount, endpoint string, item map[string]any) (normalizedPost, error) {
	relationDepth := 1
	if qq := firstText(item, "uin", "user_id", "_uin", "opuin", "_author_uin"); qq == "" || qq == account.QQUIN {
		relationDepth = 0
	}
	return n.PostAtDepth(ctx, account, endpoint, item, relationDepth)
}

func (n Normalizer) PostAtDepth(ctx context.Context, account domain.NapCatAccount, endpoint string, item map[string]any, relationDepth int) (normalizedPost, error) {
	relationDepth = clampRelationDepth(relationDepth)
	tid := firstText(item, "tid", "cellid", "_tid", "message_id")
	authorQQ := firstText(item, "uin", "user_id", "_uin", "opuin", "_author_uin")
	if authorQQ == "" {
		authorQQ = account.QQUIN
	}
	if tid == "" || authorQQ == "" {
		return normalizedPost{}, nil
	}
	rawID, err := n.saveItem(ctx, account.ID, endpoint, item)
	if err != nil {
		return normalizedPost{}, err
	}
	name := firstText(item, "nickname", "name", "uinname")
	authorID, err := n.personID(ctx, authorQQ, name)
	if err != nil {
		return normalizedPost{}, err
	}
	if err := n.Normalizer.UpsertProfileData(ctx, authorID, account.ID, name, "", "", endpoint, &rawID, item); err != nil {
		return normalizedPost{}, err
	}
	if err := n.queueAvatar(ctx, account.ID, authorID, rawID, authorQQ, "", relationDepth); err != nil {
		return normalizedPost{}, err
	}
	platformID := authorQQ + ":" + tid
	body := firstText(item, "content", "con", "raw_message", "message")
	publishedAt := eventTimeFrom(item)
	metadata, _ := json.Marshal(item)
	var contentID string
	err = n.Repo.DB.QueryRow(ctx, `INSERT INTO contents(platform,platform_content_id,author_id,context_type,body,published_at,raw_record_id,source_account_id,metadata)
		VALUES('qzone',$1,$2,'qzone_post',$3,$4,$5,$6,$7)
		ON CONFLICT(platform,platform_content_id) DO UPDATE SET author_id=EXCLUDED.author_id,body=EXCLUDED.body,
		published_at=COALESCE(EXCLUDED.published_at,contents.published_at),raw_record_id=EXCLUDED.raw_record_id,
		source_account_id=EXCLUDED.source_account_id,metadata=EXCLUDED.metadata,updated_at=now() RETURNING id::text`,
		platformID, authorID, body, publishedAt, rawID, account.ID, metadata).Scan(&contentID)
	if err != nil {
		return normalizedPost{}, err
	}
	if err := n.queueContentMedia(ctx, account.ID, contentID, rawID, item, relationDepth, false); err != nil {
		return normalizedPost{}, err
	}
	if err := n.relation(ctx, account.ID, authorID, nil, &contentID, "published", "qzone", publishedAt, rawID); err != nil {
		return normalizedPost{}, err
	}
	for _, mention := range extractMentions(body, clampRelationDepth(relationDepth+1)) {
		mentionPersonID, err := n.personID(ctx, mention.QQ, "")
		if err == nil {
			_ = n.queueAvatar(ctx, account.ID, mentionPersonID, rawID, mention.QQ, "", clampRelationDepth(relationDepth+1))
			_ = n.relation(ctx, account.ID, authorID, &mentionPersonID, &contentID, "mentioned", "qzone_post", publishedAt, rawID)
		}
	}
	return normalizedPost{ContentID: contentID, TID: tid, AuthorQQ: authorQQ, RelationDepth: relationDepth, PublishedAt: publishedAt}, nil
}

func (n Normalizer) Comments(ctx context.Context, account domain.NapCatAccount, endpoint string, post normalizedPost, raw json.RawMessage) error {
	_, err := n.CommentsAtDepth(ctx, account, endpoint, post, raw, clampRelationDepth(post.RelationDepth+1))
	return err
}

func (n Normalizer) CommentsAtDepth(ctx context.Context, account domain.NapCatAccount, endpoint string, post normalizedPost, raw json.RawMessage, relationDepth int) ([]discoveredPerson, error) {
	relationDepth = clampRelationDepth(relationDepth)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := findMapSlice(value, "commentlist", "comment_list", "comments", "data", "items", "list")
	responseContext := interactionResponseContext(value)
	discovered := make([]discoveredPerson, 0, len(items))
	for index, item := range items {
		qq := firstText(item, "uin", "user_id", "qq")
		if qq == "" {
			continue
		}
		itemWithCtx := withInteractionContext(item, post)
		mergeInteractionResponseContext(itemWithCtx, responseContext)
		rawID, err := n.saveItem(ctx, account.ID, endpoint+":comment", itemWithCtx)
		if err != nil {
			return discovered, err
		}
		personID, err := n.personID(ctx, qq, firstText(item, "nickname", "name", "uinname"))
		if err != nil {
			return discovered, err
		}
		if err := n.Normalizer.UpsertProfileData(ctx, personID, account.ID, firstText(item, "nickname", "name", "uinname"), "", "", endpoint, &rawID, item); err != nil {
			return discovered, err
		}
		if err := n.queueAvatar(ctx, account.ID, personID, rawID, qq, "", relationDepth); err != nil {
			return discovered, err
		}
		discovered = append(discovered, discoveredPerson{QQ: qq, Depth: relationDepth})
		commentID := firstText(item, "commentid", "comment_id", "commentId", "tid", "id")
		if commentID == "" {
			commentID = fmt.Sprintf("%d:%s", index, rawID)
		}
		body := firstText(item, "content", "comment_content", "con")
		metadata, _ := json.Marshal(itemWithCtx)
		var contentID string
		// Try to find the comment being replied to for nesting
		replyToContentID := ""
		replyQQ := firstText(item, "replyToUin", "reply_to_uin", "_reply_to_uin", "t2_uin")
		if replyQQ != "" {
			_ = n.Repo.DB.QueryRow(ctx, `SELECT id::text FROM contents WHERE platform='qzone' AND context_type='qzone_comment' AND parent_content_id=$1 AND author_id=(SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$2) ORDER BY published_at DESC LIMIT 1`, post.ContentID, replyQQ).Scan(&replyToContentID)
		}
		commentTime := eventTimeFrom(itemWithCtx)
		err = n.Repo.DB.QueryRow(ctx, `INSERT INTO contents(platform,platform_content_id,author_id,context_type,body,published_at,raw_record_id,source_account_id,parent_content_id,reply_to_content_id,metadata)
			VALUES('qzone',$1,$2,'qzone_comment',$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9)
			ON CONFLICT(platform,platform_content_id) DO UPDATE SET body=EXCLUDED.body,metadata=EXCLUDED.metadata,
			published_at=COALESCE(EXCLUDED.published_at,contents.published_at),
			raw_record_id=EXCLUDED.raw_record_id,reply_to_content_id=COALESCE(NULLIF(EXCLUDED.reply_to_content_id::text,'')::uuid,contents.reply_to_content_id),updated_at=now() RETURNING id::text`,
			post.AuthorQQ+":"+post.TID+":comment:"+commentID, personID, body, commentTime, rawID, account.ID, post.ContentID, replyToContentID, metadata).Scan(&contentID)
		if err != nil {
			return discovered, err
		}
		if err := n.queueContentMedia(ctx, account.ID, contentID, rawID, item, relationDepth, true); err != nil {
			return discovered, err
		}
		if err := n.relation(ctx, account.ID, personID, nil, &post.ContentID, "commented", "qzone_post", commentTime, rawID); err != nil {
			return discovered, err
		}
		for _, mention := range extractMentions(body, relationDepth) {
			mentionPersonID, err := n.personID(ctx, mention.QQ, "")
			if err == nil {
				_ = n.queueAvatar(ctx, account.ID, mentionPersonID, rawID, mention.QQ, "", relationDepth)
				_ = n.relation(ctx, account.ID, personID, &mentionPersonID, &contentID, "mentioned", "qzone_comment", commentTime, rawID)
				discovered = append(discovered, mention)
			}
		}
		replyQQ = firstText(item, "replyToUin", "reply_to_uin", "_reply_to_uin", "t2_uin")
		if replyQQ != "" {
			replyName := firstText(item, "replyToNickname", "reply_to_nickname")
			replyPersonID, err := n.personID(ctx, replyQQ, replyName)
			if err != nil {
				return discovered, err
			}
			if err := n.queueAvatar(ctx, account.ID, replyPersonID, rawID, replyQQ, "", relationDepth); err != nil {
				return discovered, err
			}
			discovered = append(discovered, discoveredPerson{QQ: replyQQ, Depth: relationDepth})
			if err := n.relation(ctx, account.ID, personID, &replyPersonID, &post.ContentID, "replied_to", "qzone_comment", commentTime, rawID); err != nil {
				return discovered, err
			}
		}
	}
	return uniqueDiscovered(discovered), nil
}

func (n Normalizer) Likes(ctx context.Context, account domain.NapCatAccount, endpoint string, post normalizedPost, raw json.RawMessage) error {
	_, err := n.LikesAtDepth(ctx, account, endpoint, post, raw, clampRelationDepth(post.RelationDepth+1))
	return err
}

func (n Normalizer) LikesAtDepth(ctx context.Context, account domain.NapCatAccount, endpoint string, post normalizedPost, raw json.RawMessage, relationDepth int) ([]discoveredPerson, error) {
	relationDepth = clampRelationDepth(relationDepth)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	items := findMapSlice(value, "likes", "list", "items", "data")
	responseContext := interactionResponseContext(value)
	discovered := make([]discoveredPerson, 0, len(items))
	for _, item := range items {
		qq := firstText(item, "uin", "user_id", "qq")
		if qq == "" {
			continue
		}
		// Include post context in the item so the raw record is unique per post
		// and the evidence chain can trace back to which post was liked
		itemWithCtx := withInteractionContext(item, post)
		mergeInteractionResponseContext(itemWithCtx, responseContext)
		rawID, err := n.saveItem(ctx, account.ID, endpoint+":like", itemWithCtx)
		if err != nil {
			return discovered, err
		}
		personID, err := n.personID(ctx, qq, firstText(item, "nickname", "name", "uinname"))
		if err != nil {
			return discovered, err
		}
		if err := n.Normalizer.UpsertProfileData(ctx, personID, account.ID, firstText(item, "nickname", "name", "uinname"), "", "", endpoint, &rawID, item); err != nil {
			return discovered, err
		}
		if err := n.queueAvatar(ctx, account.ID, personID, rawID, qq, "", relationDepth); err != nil {
			return discovered, err
		}
		discovered = append(discovered, discoveredPerson{QQ: qq, Depth: relationDepth})
		likeTime := eventTimeFrom(itemWithCtx)
		if (likeTime.Equal(time.Now()) || likeTime.After(time.Now().Add(-1*time.Minute))) && !post.PublishedAt.IsZero() {
			likeTime = post.PublishedAt
		}
		if err := n.relation(ctx, account.ID, personID, nil, &post.ContentID, "liked", "qzone_post", likeTime, rawID); err != nil {
			return discovered, err
		}
	}
	return uniqueDiscovered(discovered), nil
}

func (n Normalizer) Realtime(ctx context.Context, account domain.NapCatAccount, rawID string, raw json.RawMessage) error {
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		return err
	}
	postType := firstText(item, "post_type")
	noticeType := firstText(item, "notice_type")
	if postType == "message" && firstText(item, "_tid") != "" {
		_, err := n.Post(ctx, account, "qzone_ws:post", item)
		return err
	}
	postTid := firstText(item, "post_tid")
	postQQ := firstText(item, "post_uin")
	if postTid == "" || postQQ == "" {
		return nil
	}
	post, err := n.lookupPost(ctx, postQQ, postTid)
	if err != nil {
		placeholder := map[string]any{"tid": postTid, "uin": postQQ, "created_time": item["time"]}
		post, err = n.Post(ctx, account, "qzone_ws:referenced_post", placeholder)
	}
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"data": []map[string]any{item}})
	switch noticeType {
	case "qzone_comment":
		item["content"] = firstText(item, "comment_content")
		item["nickname"] = firstText(item, "sender_name")
		payload, _ = json.Marshal(map[string]any{"data": []map[string]any{item}})
		return n.Comments(ctx, account, "qzone_ws", post, payload)
	case "qzone_like":
		item["nickname"] = firstText(item, "sender_name")
		payload, _ = json.Marshal(map[string]any{"data": []map[string]any{item}})
		return n.Likes(ctx, account, "qzone_ws", post, payload)
	}
	_ = rawID
	return nil
}

func (n Normalizer) lookupPost(ctx context.Context, authorQQ, tid string) (normalizedPost, error) {
	var post normalizedPost
	post.AuthorQQ, post.TID = authorQQ, tid
	err := n.Repo.DB.QueryRow(ctx, `SELECT id::text FROM contents WHERE platform='qzone' AND platform_content_id=$1`, authorQQ+":"+tid).Scan(&post.ContentID)
	return post, err
}

func (n Normalizer) personID(ctx context.Context, qq, nickname string) (string, error) {
	if qq == "" {
		return "", nil
	}
	return n.Normalizer.UpsertPerson(ctx, qq, nickname)
}

func (n Normalizer) saveItem(ctx context.Context, accountID, endpoint string, item map[string]any) (string, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	return n.Repo.SaveRaw(ctx, accountID, "qzone_http", endpoint, raw)
}

func (n Normalizer) relation(ctx context.Context, accountID, actorID string, targetPersonID, targetObjectID *string, action, contextType string, at time.Time, rawID string) error {
	if actorID == "" {
		return nil
	}
	if action == "liked" && targetObjectID != nil {
		_, err := n.Repo.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,ARRAY[$8]::uuid[],$8)
			ON CONFLICT (source_account_id,actor_person_id,target_object_id,action_type,context_type)
			WHERE action_type='liked' AND target_object_id IS NOT NULL
			DO UPDATE SET
				evidence_ids=CASE WHEN EXCLUDED.raw_record_id=ANY(relation_events.evidence_ids) THEN relation_events.evidence_ids ELSE array_append(relation_events.evidence_ids,EXCLUDED.raw_record_id) END,
				raw_record_id=EXCLUDED.raw_record_id,
				occurred_at=LEAST(relation_events.occurred_at,EXCLUDED.occurred_at)`,
			accountID, actorID, targetPersonID, targetObjectID, action, contextType, at, rawID)
		return err
	}
	_, err := n.Repo.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,ARRAY[$8]::uuid[],$8) ON CONFLICT DO NOTHING`, accountID, actorID, targetPersonID, targetObjectID, action, contextType, at, rawID)
	return err
}

func (n Normalizer) queueContentMedia(ctx context.Context, accountID, contentID, rawID string, item map[string]any, relationDepth int, commentItem bool) error {
	relationDepth = clampRelationDepth(relationDepth)
	reason := priorityReason(relationDepth, "content")
	refs := contentMedia(item, commentItem)
	tx, err := n.Repo.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for index, ref := range refs {
		metadataValue := mediaMetadata(ref.identity, ref.metadata)
		metadata, _ := json.Marshal(metadataValue)
		var referenceID string
		err := tx.QueryRow(ctx, `INSERT INTO media_references(source_account_id,content_id,raw_record_id,segment_index,segment_type,media_kind,source_ref,source_url,resolver_endpoint,metadata,relation_depth,priority_reason)
			VALUES($1,$2,$3,$4,$5,$6,$7,$7,$8,$9,$10,$11)
			ON CONFLICT(content_id,segment_index,segment_type) WHERE content_id IS NOT NULL DO UPDATE SET
			raw_record_id=EXCLUDED.raw_record_id,source_ref=EXCLUDED.source_ref,source_url=EXCLUDED.source_url,resolver_endpoint=EXCLUDED.resolver_endpoint,metadata=EXCLUDED.metadata,
			relation_depth=LEAST(media_references.relation_depth,EXCLUDED.relation_depth),
			priority_reason=CASE WHEN EXCLUDED.relation_depth < media_references.relation_depth OR media_references.priority_reason='global' THEN EXCLUDED.priority_reason ELSE media_references.priority_reason END,
			status=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' AND media_references.status='completed' THEN 'completed' ELSE 'pending' END,
			asset_id=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' THEN media_references.asset_id ELSE NULL END,
			completed_at=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' THEN media_references.completed_at ELSE NULL END,
			attempt_count=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' THEN media_references.attempt_count ELSE 0 END,
			last_error=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' THEN media_references.last_error ELSE NULL END,
			next_attempt_at=CASE WHEN media_references.metadata->>'_media_identity' = EXCLUDED.metadata->>'_media_identity' THEN media_references.next_attempt_at ELSE now() END
			RETURNING id::text`,
			accountID, contentID, rawID, index, ref.kind, ref.kind, ref.url, ref.resolver, metadata, relationDepth, reason).Scan(&referenceID)
		if err != nil {
			return err
		}
		// A changed media kind at the same position is a replacement, not a
		// second current asset for this content.
		if _, err := tx.Exec(ctx, `DELETE FROM media_references WHERE content_id=$1 AND segment_index=$2 AND segment_type<>$3`, contentID, index, ref.kind); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO content_media(content_id,media_reference_id,media_asset_id,position,media_kind,metadata)
			VALUES($1,$2,(SELECT asset_id FROM media_references WHERE id=$2::uuid),$3,$4,$5)
			ON CONFLICT(content_id,media_reference_id) DO UPDATE SET media_asset_id=EXCLUDED.media_asset_id,position=EXCLUDED.position,media_kind=EXCLUDED.media_kind,metadata=EXCLUDED.metadata`,
			contentID, referenceID, index, ref.kind, metadata); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM media_references WHERE content_id=$1 AND (segment_index IS NULL OR segment_index >= $2)`, contentID, len(refs)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (n Normalizer) queueAvatar(ctx context.Context, accountID, personID, rawID, qq, avatarURL string, relationDepth int) error {
	if err := n.Normalizer.QueueAvatar(ctx, accountID, personID, rawID, qq, avatarURL); err != nil {
		return err
	}
	relationDepth = clampRelationDepth(relationDepth)
	_, err := n.Repo.DB.Exec(ctx, `UPDATE media_references SET
		relation_depth=LEAST(relation_depth,$3),
		priority_reason=CASE WHEN $3 < relation_depth OR priority_reason='global' THEN $4 ELSE priority_reason END
		WHERE source_account_id=$1 AND person_id=$2 AND media_kind='avatar'`,
		accountID, personID, relationDepth, priorityReason(relationDepth, "avatar"))
	return err
}

func clampRelationDepth(depth int) int {
	if depth < 0 {
		return 0
	}
	if depth > 99 {
		return 99
	}
	return depth
}

func priorityReason(depth int, resource string) string {
	depth = clampRelationDepth(depth)
	level := fmt.Sprintf("depth_%d", depth)
	switch clampRelationDepth(depth) {
	case 0:
		level = "target"
	case 1:
		level = "first_degree"
	case 2:
		level = "second_degree"
	}
	return "qzone_" + level + "_" + resource
}

func uniqueDiscovered(items []discoveredPerson) []discoveredPerson {
	depths := make(map[string]int, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		if item.QQ == "" {
			continue
		}
		depth := clampRelationDepth(item.Depth)
		previous, exists := depths[item.QQ]
		if !exists {
			order = append(order, item.QQ)
		}
		if !exists || depth < previous {
			depths[item.QQ] = depth
		}
	}
	result := make([]discoveredPerson, 0, len(order))
	for _, qq := range order {
		result = append(result, discoveredPerson{QQ: qq, Depth: depths[qq]})
	}
	return result
}

type contentMediaRef struct {
	kind     string
	url      string
	identity string
	resolver string
	metadata any
}

func contentMedia(item map[string]any, commentItem bool) []contentMediaRef {
	result := []contentMediaRef{}
	seen := map[string]bool{}
	appendURL := func(kind, raw string, metadata any) {
		raw = strings.TrimSpace(raw)
		identity := mediaIdentity(raw)
		if raw == "" || seen[kind+":"+identity] || strings.HasPrefix(raw, "base64://") {
			return
		}
		seen[kind+":"+identity] = true
		resolver := ""
		if kind == "image" {
			resolver = "qzone:/fetch_image"
		}
		result = append(result, contentMediaRef{kind: kind, url: raw, identity: identity, resolver: resolver, metadata: metadata})
	}
	imageKeys := []string{"pic", "pics", "_pics", "images"}
	if commentItem {
		// QZone comment payloads attach the comment picture under
		// _comment_pic. Feed (post) payloads may carry the same key as the
		// first comment's preview, which must never be treated as a post
		// image, otherwise posts end up with a foreign comment picture.
		imageKeys = append(imageKeys, "_comment_pic")
	}
	for _, key := range imageKeys {
		walkMediaValue(item[key], func(url string, meta any) { appendURL("image", url, meta) })
	}
	for _, key := range []string{"video", "videos", "_videos", "video_url"} {
		walkMediaValue(item[key], func(url string, meta any) { appendURL("video", url, meta) })
	}
	return result
}

// QZone CDN URLs contain short-lived signature parameters. Use the first
// query component (the stable object key) as the identity so a later sync
// refreshes the URL without redownloading the same object.
func mediaIdentity(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.TrimSuffix(strings.TrimSpace(raw), "#")
	}
	if strings.Contains(parsed.Host, "qpic.cn") || strings.Contains(parsed.Host, "photo.store.qq.com") {
		if first := strings.Split(parsed.RawQuery, "&")[0]; first != "" {
			return parsed.Scheme + "://" + parsed.Host + parsed.Path + "?" + first
		}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func mediaMetadata(identity string, source any) map[string]any {
	metadata := map[string]any{"_media_identity": identity}
	if sourceMap, ok := source.(map[string]any); ok {
		for key, value := range sourceMap {
			metadata[key] = value
		}
	} else if source != nil {
		metadata["source"] = source
	}
	return metadata
}

func withInteractionContext(item map[string]any, post normalizedPost) map[string]any {
	withContext := make(map[string]any, len(item)+4)
	for key, value := range item {
		withContext[key] = value
	}
	withContext["_post_content_id"] = post.ContentID
	withContext["_post_author_qq"] = post.AuthorQQ
	withContext["_post_tid"] = post.TID
	if !post.PublishedAt.IsZero() {
		withContext["_post_published_at"] = post.PublishedAt.Unix()
	}
	return withContext
}

func interactionResponseContext(value any) map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	context := make(map[string]any)
	for sourceKey, targetKey := range map[string]string{
		"availability":  "_upstream_availability",
		"reliability":   "_upstream_reliability",
		"source":        "_upstream_source",
		"diagnostic_id": "_upstream_diagnostic_id",
		"identity":      "_upstream_identity",
	} {
		if value, exists := root[sourceKey]; exists && value != nil {
			context[targetKey] = value
		}
	}
	return context
}

func mergeInteractionResponseContext(item, responseContext map[string]any) {
	for key, value := range responseContext {
		item[key] = value
	}
}

func walkMediaValue(value any, appendURL func(string, any)) {
	switch typed := value.(type) {
	case string:
		appendURL(typed, typed)
	case []any:
		for _, item := range typed {
			walkMediaValue(item, appendURL)
		}
	case map[string]any:
		url := firstText(typed, "url", "url3", "pic_url", "origin_url", "raw", "src", "video_url", "cover")
		if url != "" {
			appendURL(url, typed)
		}
	}
}

func findMapSlice(value any, keys ...string) []map[string]any {
	if items, ok := value.([]any); ok {
		return mapsFromAny(items)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range keys {
		if items, ok := root[key].([]any); ok {
			return mapsFromAny(items)
		}
	}
	for _, nestedKey := range []string{"data", "result"} {
		if nested, ok := root[nestedKey]; ok {
			if result := findMapSlice(nested, keys...); len(result) > 0 {
				return result
			}
		}
	}
	return nil
}

func mapsFromAny(items []any) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if value, ok := item.(map[string]any); ok {
			result = append(result, value)
		}
	}
	return result
}

func firstText(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := textValue(value[key]); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case float32:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func int64Value(value any) int64 {
	text := textValue(value)
	result, _ := strconv.ParseInt(text, 10, 64)
	return result
}

func eventTimeFrom(value map[string]any) time.Time {
	if value == nil {
		return time.Now()
	}
	for _, key := range []string{
		"createtime", "create_time", "created_time", "createTime", "createTime2",
		"abstime", "_abstime", "time", "_time", "pubtime", "pub_time", "post_time",
		"postTime", "timestamp", "createdAt", "lastSeen", "_post_published_at",
	} {
		v, exists := value[key]
		if !exists || v == nil {
			continue
		}
		if seconds := int64Value(v); seconds > 0 {
			if seconds > 1e12 {
				seconds /= 1000
			}
			return time.Unix(seconds, 0)
		}
		if str, ok := v.(string); ok && str != "" {
			str = strings.TrimSpace(str)
			for _, layout := range []string{
				"2006-01-02 15:04:05",
				"2006-01-02 15:04",
				"2006/01/02 15:04:05",
				"2006/01/02 15:04",
				time.RFC3339,
			} {
				if t, err := time.ParseInLocation(layout, str, time.Local); err == nil {
					return t
				}
			}
		}
	}
	if identity, ok := value["_upstream_identity"].(map[string]any); ok {
		if seconds := int64Value(identity["abstime"]); seconds > 0 {
			if seconds > 1e12 {
				seconds /= 1000
			}
			return time.Unix(seconds, 0)
		}
	}
	return time.Now()
}
