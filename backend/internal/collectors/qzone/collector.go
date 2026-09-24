package qzone

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

const (
	qzonePostPageSize    = 100
	qzoneMixedPageSize   = 50
	qzoneCommentPageSize = 50
	qzoneLikePageSize    = 50
)

type Collector struct {
	Repo       persistence.Repository
	Logger     *slog.Logger
	Normalizer Normalizer
}

type crawlTarget struct {
	QQ     string
	Depth  int
	Mode   string
	Parent string
	Reason string
}

// needLoginBreaker tracks whether the QZone bridge token has expired.
// Once tripped, comment and like collection is skipped for the rest of the sync.
type needLoginBreaker struct {
	tripped atomic.Bool
}

func (b *needLoginBreaker) check(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "-77") || strings.Contains(msg, "-87") || strings.Contains(msg, "0103-") || strings.Contains(msg, "76bd-") || strings.Contains(msg, "1503") || strings.Contains(msg, "1502") {
		return false
	}
	if strings.Contains(msg, "未登录") || (strings.Contains(msg, "1401") && strings.Contains(msg, "need login") && !strings.Contains(msg, "|")) {
		b.tripped.Store(true)
		return true
	}
	return false
}

func (b *needLoginBreaker) skip() bool {
	return b.tripped.Load()
}

type qzoneCall func(endpoint string, params any) (json.RawMessage, error)

func (c Collector) RunSync(ctx context.Context, account domain.NapCatAccount, connection domain.QZoneConnection, update func(int)) error {
	return c.RunSyncWithScope(ctx, account, connection, "", (domain.CollectionScope{}).Normalize(account.QQUIN), domain.CollectionObserver{}, update)
}

func (c Collector) RunSyncWithScope(ctx context.Context, account domain.NapCatAccount, connection domain.QZoneConnection, runID string, scope domain.CollectionScope, observer domain.CollectionObserver, update func(int)) error {
	scope = scope.Normalize(account.QQUIN)
	var mediaReferenceStart int64
	_ = c.Repo.DB.QueryRow(ctx, `SELECT count(*) FROM media_references WHERE source_account_id=$1`, account.ID).Scan(&mediaReferenceStart)
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
	discover := func(qq, state string, depth int, contextType, parent string) {
		if observer.Candidate == nil || strings.TrimSpace(qq) == "" {
			return
		}
		if _, allowed := scope.QQMode(qq); !allowed {
			state = "excluded"
		}
		observer.Candidate(domain.CollectionCandidate{
			EntityType: "qq", EntityID: qq, State: state, Depth: depth,
			Contexts:      []string{contextType},
			DiscoveryPath: map[string]any{"context": contextType, "parent": parent},
		})
	}
	if err := EnsureBridge(ctx, connection.HTTPURL, connection.AccessToken, c.Logger); err != nil {
		emit("qzone_profile", "failed", 0, 0, err)
		return fmt.Errorf("QZone bridge pre-flight check failed: %w", err)
	}
	client := NewHTTPClient(connection.HTTPURL, connection.AccessToken)
	gate := requestGateForAccount(account.ID)
	call := func(endpoint string, params any) (json.RawMessage, error) {
		raw, err := gate.call(ctx, func() ([]byte, error) {
			return client.Call(ctx, endpoint, params)
		}, func(attempt int, delay time.Duration, retryErr error) {
			if c.Logger != nil {
				c.Logger.Warn("QZone request retrying", "account_id", account.ID, "endpoint", endpoint, "retry", attempt, "backoff", delay, "error", retryErr)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", endpoint, err)
		}
		if _, err := c.Repo.SaveRaw(ctx, account.ID, "qzone_http", endpoint, raw); err != nil {
			return nil, err
		}
		return raw, nil
	}

	targetDepth := map[string]int{}
	processedDepth := map[string]int{}
	queue := make([]crawlTarget, 0, 128)

	// Checkpoint Recovery: load existing candidates state for this run
	if runID != "" {
		rows, err := c.Repo.DB.Query(ctx, `
			SELECT entity_id, depth, state,
			       COALESCE(contexts[1], 'entry'),
			       COALESCE(discovery_paths[1]->>'parent', '')
			FROM collection_candidates WHERE run_id = $1`, runID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var qq, state, reason, parent string
				var depth int
				if rows.Scan(&qq, &depth, &state, &reason, &parent) == nil {
					targetDepth[qq] = depth
					if state == "expanded" {
						processedDepth[qq] = depth
					} else if state == "expandable" {
						mode, _ := scope.QQMode(qq)
						queue = append(queue, crawlTarget{
							QQ: qq, Depth: depth, Mode: mode, Parent: parent, Reason: reason,
						})
					}
				}
			}
		}
	}

	enqueue := func(qq string, depth int, mode, parent, reason string) {
		qq = strings.TrimSpace(qq)
		if qq == "" || !scope.CanExpand(depth) {
			return
		}
		if _, allowed := scope.QQMode(qq); !allowed {
			discover(qq, "excluded", depth, reason, parent)
			return
		}
		previous, exists := targetDepth[qq]
		if exists && previous <= depth {
			return
		}
		targetDepth[qq] = depth
		queue = append(queue, crawlTarget{QQ: qq, Depth: depth, Mode: mode, Parent: parent, Reason: reason})
		discover(qq, "expandable", depth, reason, parent)
	}
	for _, entry := range scope.EntryQQs() {
		if _, alreadyProcessed := processedDepth[entry.ID]; !alreadyProcessed {
			enqueue(entry.ID, entry.Depth, entry.Mode, "", "entry")
		}
	}

	emit("qzone_profile", "running", 0, 0, nil)
	login, err := call("get_login_info", map[string]any{})
	if err != nil {
		emit("qzone_profile", "failed", 0, 0, err)
		return err
	}
	if err := c.Normalizer.Login(ctx, account, "get_login_info", login); err != nil {
		emit("qzone_profile", "failed", 1, 1, err)
		return err
	}
	emit("qzone_profile", "complete", 1, 1, nil)
	if len(processedDepth) == 0 {
		progress(update, 5)
	}

	if scope.AccountBaselineEnabled() && len(processedDepth) == 0 {
		emit("friends", "running", 0, 0, nil)
		if friends, loadErr := call("get_friend_list", map[string]any{}); loadErr != nil {
			c.warn(account.ID, "get_friend_list", loadErr)
			emit("friends", "partial", 0, 0, loadErr)
		} else {
			if normalizeErr := c.Normalizer.Friends(ctx, account, "get_friend_list", friends); normalizeErr != nil {
				c.warn(account.ID, "normalize friends", normalizeErr)
			}
			qqs := pageQQs(friends, "items", "friends", "list")
			for _, qq := range qqs {
				discover(qq, "collected", 1, "friend", account.QQUIN)
			}
			emit("friends", "complete", 1, int64(len(qqs)), nil)
		}
	} else if scope.AccountBaselineEnabled() {
		emit("friends", "complete", 1, 0, nil)
	} else {
		emit("friends", "complete", 0, 0, nil)
	}
	if len(processedDepth) == 0 {
		progress(update, 10)
	}

	if scope.AccountBaselineEnabled() && len(processedDepth) == 0 {
		emit("qzone_visitors", "running", 0, 0, nil)
		if visitors, loadErr := call("get_visitor_list", map[string]any{"user_id": account.QQUIN}); loadErr != nil {
			c.warn(account.ID, "get_visitor_list", loadErr)
			emit("qzone_visitors", "partial", 0, 0, loadErr)
			c.syncVisitorCoverage(ctx, account, "partial", 0, loadErr.Error())
		} else {
			visitorStatus := "complete"
			var visitorError string
			if normalizeErr := c.Normalizer.Visitors(ctx, account, "get_visitor_list", visitors); normalizeErr != nil {
				c.warn(account.ID, "normalize visitors", normalizeErr)
				visitorStatus = "partial"
				visitorError = normalizeErr.Error()
			}
			qqs := pageQQs(visitors, "items", "visitors", "visitorlist", "list")
			for _, qq := range qqs {
				state := "collected"
				if mode, _ := scope.QQMode(account.QQUIN); mode == domain.SpaceModeExpandPeople {
					state = "expandable"
					enqueue(qq, 1, mode, account.QQUIN, "visitor")
				} else {
					discover(qq, state, 1, "visitor", account.QQUIN)
				}
			}
			if len(qqs) == 0 && visitorStatus == "complete" {
				visitorStatus = "no_results"
			}
			emitError := errorFromText(visitorError)
			emit("qzone_visitors", visitorStatus, 1, int64(len(qqs)), emitError)
			c.syncVisitorCoverage(ctx, account, visitorStatus, len(qqs), visitorError)
		}
	} else if scope.AccountBaselineEnabled() {
		emit("qzone_visitors", "complete", 1, 0, nil)
	} else {
		emit("qzone_visitors", "complete", 0, 0, nil)
	}
	if len(processedDepth) == 0 {
		progress(update, 15)
	}

	interactionDepth := map[string]int{}
	breaker := &needLoginBreaker{}
	moduleForSource := map[string]string{"qzone_feeds": "qzone_posts", "qzone_comments": "qzone_comments", "qzone_likes": "qzone_likes"}
	coverage := newCoverageTracker(func(source string, summary coverageEntry) {
		module := moduleForSource[source]
		if module == "" {
			return
		}
		cursor := cloneMap(summary.cursor)
		cursor["last_scope"] = summary.scope
		cursor["targets"] = summary.targets
		cursor["partial_targets"] = summary.partialTargets
		emitCollectionProgress(observer, domain.CollectionModuleProgress{Module: module, Status: "running", PagesCompleted: summary.pages, RecordsCollected: int64(summary.items), Cursor: cursor, Error: summary.error})
	})
	defer func() {
		coverage.finish(context.Background(), c.Repo, account)
		summaries := coverage.summary()
		for source, summary := range summaries {
			module := moduleForSource[source]
			status := "complete"
			var summaryErr error
			if summary.partial {
				status = "partial"
				summaryErr = fmt.Errorf("%s", summary.error)
			}
			cursor := cloneMap(summary.cursor)
			cursor["last_scope"] = summary.scope
			cursor["targets"] = summary.targets
			cursor["partial_targets"] = summary.partialTargets
			emitCollectionProgress(observer, domain.CollectionModuleProgress{Module: module, Status: status, PagesCompleted: summary.pages, RecordsCollected: int64(summary.items), Cursor: cursor, Error: errorText(summaryErr)})
		}
		for source, module := range moduleForSource {
			if _, observed := summaries[source]; !observed {
				emitCollectionProgress(observer, domain.CollectionModuleProgress{Module: module, Status: "skipped", Cursor: map[string]any{"reason": "not requested by collection scope"}})
			}
		}
	}()
	if scope.AccountBaselineEnabled() && len(processedDepth) == 0 {
		coverage.begin(account.QQUIN, "qzone_feeds")
		coverage.begin(account.QQUIN, "qzone_comments")
		coverage.begin(account.QQUIN, "qzone_likes")
	}
	processInteractions := func(posts []normalizedPost, target crawlTarget) {
		if target.Mode == domain.SpaceModePostsOnly {
			return
		}
		for _, post := range uniquePosts(posts) {
			if previous, exists := interactionDepth[post.ContentID]; exists && previous <= post.RelationDepth {
				continue
			}
			interactionDepth[post.ContentID] = post.RelationDepth
			if !breaker.skip() {
				for _, person := range c.collectComments(ctx, account, call, post, breaker, coverage) {
					state := "collected"
					if target.Mode == domain.SpaceModeExpandPeople {
						state = "expandable"
						enqueue(person.QQ, target.Depth+1, target.Mode, target.QQ, "comment")
					} else {
						discover(person.QQ, state, target.Depth+1, "comment", target.QQ)
					}
				}
			}
			if !breaker.skip() {
				for _, person := range c.collectLikes(ctx, account, call, post, breaker, coverage) {
					state := "collected"
					if target.Mode == domain.SpaceModeExpandPeople {
						state = "expandable"
						enqueue(person.QQ, target.Depth+1, target.Mode, target.QQ, "like")
					} else {
						discover(person.QQ, state, target.Depth+1, "like", target.QQ)
					}
				}
			}
		}
	}

	// Put target resources into the media queue before broader discovery starts.
	emit("qzone_posts", "running", 0, 0, nil)
	emit("qzone_comments", "running", 0, 0, nil)
	emit("qzone_likes", "running", 0, 0, nil)
	processedTargets := len(processedDepth)
	calcProgress := func() int {
		total := len(queue) + processedTargets
		if total == 0 {
			return 25
		}
		return 25 + minInt(73, processedTargets*73/maxInt(1, total))
	}
	selfMode, _ := scope.QQMode(account.QQUIN)
	selfTarget := crawlTarget{QQ: account.QQUIN, Depth: 0, Mode: selfMode, Reason: "entry"}
	if scope.AccountBaselineEnabled() && len(processedDepth) == 0 {
		if _, queued := targetDepth[account.QQUIN]; !queued {
			enqueue(account.QQUIN, 0, selfMode, "", "entry")
		}
	}
	if len(processedDepth) == 0 {
		progress(update, 20)
	} else {
		progress(update, calcProgress())
	}

	if scope.AccountBaselineEnabled() && len(processedDepth) == 0 {
		friendPosts := c.collectFriendFeeds(ctx, account, call, coverage)
		for _, post := range friendPosts {
			discover(post.AuthorQQ, "collected", 1, "friend_feed", account.QQUIN)
		}
		processInteractions(friendPosts, selfTarget)
	}
	for _, entry := range scope.EntryPosts() {
		parts := strings.SplitN(entry.ID, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			c.warn(account.ID, "post entry", fmt.Errorf("post entry %q must use authorQQ:tid", entry.ID))
			continue
		}
		var contentID string
		if err := c.Repo.DB.QueryRow(ctx, `SELECT id::text FROM contents WHERE platform='qzone' AND platform_content_id=$1`, entry.ID).Scan(&contentID); err != nil {
			c.warn(account.ID, "post entry", fmt.Errorf("post entry %q has not been collected yet", entry.ID))
			continue
		}
		postTarget := crawlTarget{QQ: parts[0], Depth: entry.Depth, Mode: entry.Mode, Reason: "post_entry"}
		processInteractions([]normalizedPost{{ContentID: contentID, AuthorQQ: parts[0], TID: parts[1], RelationDepth: entry.Depth}}, postTarget)
	}
	progress(update, calcProgress())

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := queue[0]
		queue = queue[1:]
		if best := targetDepth[target.QQ]; best < target.Depth {
			continue
		}
		if previous, exists := processedDepth[target.QQ]; exists && previous <= target.Depth {
			continue
		}
		processedDepth[target.QQ] = target.Depth
		discover(target.QQ, "expanded", target.Depth, target.Reason, target.Parent)
		posts := append(c.collectEmotionPages(ctx, account, call, target, breaker, coverage), c.collectMixedFeedPages(ctx, account, call, target, breaker, coverage)...)
		processInteractions(posts, target)
		processedTargets++
		progress(update, calcProgress())
	}
	var mediaCount int64
	_ = c.Repo.DB.QueryRow(ctx, `SELECT count(*) FROM media_references WHERE source_account_id=$1`, account.ID).Scan(&mediaCount)
	emit("media", "complete", 1, maxInt64(0, mediaCount-mediaReferenceStart), nil)
	emit("candidate_queue", "complete", processedTargets, int64(len(targetDepth)), nil)
	progress(update, 100)
	return nil
}

func (c Collector) collectMixedFeedPages(ctx context.Context, account domain.NapCatAccount, call qzoneCall, target crawlTarget, breaker *needLoginBreaker, coverage *coverageTracker) []normalizedPost {
	coverage.begin(target.QQ, "qzone_feeds")
	posts := []normalizedPost{}
	start := 0
	seenStart := map[int]bool{}
	for page := 0; ; page++ {
		if ctx.Err() != nil {
			return posts
		}
		if seenStart[start] {
			return posts
		}
		seenStart[start] = true
		raw, err := call("get_feeds_html_act_all", map[string]any{
			"feed_owner": target.QQ, "start": start, "count": qzoneMixedPageSize, "include_image_data": false,
		})
		if err != nil {
			if breaker.check(err) {
				partialErr := coveragePartialError(target.QQ, page+1, err)
				coverage.markPartial(target.QQ, "qzone_feeds", partialErr)
				c.warn(account.ID, "get_feeds_html_act_all partial", partialErr)
				return posts
			}
			c.warn(account.ID, "get_feeds_html_act_all space private or inaccessible", err)
			return posts
		}
		values, normalizeErr := c.Normalizer.PostsAtDepth(ctx, account, "get_feeds_html_act_all", raw, target.Depth)
		if normalizeErr != nil {
			partialErr := coveragePartialError(target.QQ, page+1, normalizeErr)
			coverage.markPartial(target.QQ, "qzone_feeds", partialErr)
			c.warn(account.ID, "normalize mixed feeds partial", partialErr)
			return posts
		}
		posts = append(posts, values...)
		coverage.add(target.QQ, "qzone_feeds", len(values))
		info := parsePageInfo(raw, start, qzoneMixedPageSize, "feeds", "msglist", "posts", "items", "list")
		coverage.setCursor(target.QQ, "qzone_feeds", pageCursor(page+1, start, info))
		c.logPartial(account.ID, "get_feeds_html_act_all", target.QQ, page+1, info, coverage, "qzone_feeds")
		if !info.HasMore {
			return posts
		}
		next := info.NextPosition
		if next <= start {
			next = start + info.ItemCount
		}
		if next <= start {
			return posts
		}
		start = next
	}
}

func (c Collector) collectEmotionPages(ctx context.Context, account domain.NapCatAccount, call qzoneCall, target crawlTarget, breaker *needLoginBreaker, coverage *coverageTracker) []normalizedPost {
	coverage.begin(target.QQ, "qzone_feeds")
	posts := []normalizedPost{}
	position, cursor := 0, ""
	seenCursors := map[string]bool{"": true}
	seenTIDs := map[string]bool{}
	noNewItems := 0
	for page := 0; ; page++ {
		if ctx.Err() != nil {
			return posts
		}
		params := map[string]any{"user_id": target.QQ, "pos": position, "num": qzonePostPageSize, "include_image_data": false}
		if cursor != "" {
			params["cursor"] = cursor
			delete(params, "pos")
		}
		raw, err := call("get_emotion_list", params)
		if err != nil {
			if breaker.check(err) {
				partialErr := coveragePartialError(target.QQ, page+1, err)
				coverage.markPartial(target.QQ, "qzone_feeds", partialErr)
				c.warn(account.ID, "get_emotion_list partial", partialErr)
				return posts
			}
			c.warn(account.ID, "get_emotion_list space private or inaccessible", err)
			return posts
		}
		values, normalizeErr := c.Normalizer.PostsAtDepth(ctx, account, "get_emotion_list", raw, target.Depth)
		if normalizeErr != nil {
			partialErr := coveragePartialError(target.QQ, page+1, normalizeErr)
			coverage.markPartial(target.QQ, "qzone_feeds", partialErr)
			c.warn(account.ID, "normalize emotion partial", partialErr)
			return posts
		}
		// Track new items by TID to detect loops
		newCount := 0
		for _, v := range values {
			if v.TID != "" && !seenTIDs[v.TID] {
				seenTIDs[v.TID] = true
				posts = append(posts, v)
				newCount++
			}
		}
		coverage.add(target.QQ, "qzone_feeds", newCount)
		if newCount == 0 {
			noNewItems++
			if noNewItems >= 2 {
				return posts
			}
		} else {
			noNewItems = 0
		}
		info := parsePageInfo(raw, position, qzonePostPageSize, "msglist", "posts", "items", "list")
		coverage.setCursor(target.QQ, "qzone_feeds", pageCursor(page+1, position, info))
		c.logPartial(account.ID, "get_emotion_list", target.QQ, page+1, info, coverage, "qzone_feeds")
		if !info.HasMore {
			return posts
		}
		// Try cursor-based pagination first
		if info.NextCursor != "" && !seenCursors[info.NextCursor] {
			seenCursors[info.NextCursor] = true
			cursor, position = info.NextCursor, 0
			continue
		}
		// Cursor looped or missing - fall back to position-based
		cursor = ""
		nextPos := position + len(values)
		if info.PositionKnown && info.NextPosition > position {
			nextPos = info.NextPosition
		}
		if nextPos > position {
			position = nextPos
			continue
		}
		return posts
	}
}

func (c Collector) collectFriendFeeds(ctx context.Context, account domain.NapCatAccount, call qzoneCall, coverage *coverageTracker) []normalizedPost {
	coverage.begin(account.QQUIN, "qzone_feeds")
	posts := []normalizedPost{}
	cursor := ""
	seenCursor := map[string]bool{}
	for page := 0; ; page++ {
		if ctx.Err() != nil {
			return posts
		}
		if seenCursor[cursor] {
			return posts
		}
		seenCursor[cursor] = true
		params := map[string]any{"num": qzonePostPageSize, "include_image_data": false, "fast_mode": true}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := call("get_friend_feeds", params)
		if err != nil {
			partialErr := coveragePartialError("friend feeds", page+1, err)
			coverage.markPartial(account.QQUIN, "qzone_feeds", partialErr)
			c.warn(account.ID, "get_friend_feeds partial", partialErr)
			return posts
		}
		values, normalizeErr := c.Normalizer.PostsAtDepth(ctx, account, "get_friend_feeds", raw, 1)
		if normalizeErr != nil {
			partialErr := coveragePartialError("friend feeds", page+1, normalizeErr)
			coverage.markPartial(account.QQUIN, "qzone_feeds", partialErr)
			c.warn(account.ID, "normalize friend feeds partial", partialErr)
			return posts
		}
		posts = append(posts, values...)
		coverage.add(account.QQUIN, "qzone_feeds", len(values))
		info := parsePageInfo(raw, 0, qzonePostPageSize, "msglist", "posts", "items", "list")
		coverage.setCursor(account.QQUIN, "qzone_feeds", pageCursor(page+1, 0, info))
		c.logPartial(account.ID, "get_friend_feeds", "global", page+1, info, coverage, "qzone_feeds")
		if !info.HasMore {
			return posts
		}
		if info.NextCursor == "" || info.NextCursor == cursor {
			return posts
		}
		cursor = info.NextCursor
	}
}

func (c Collector) collectComments(ctx context.Context, account domain.NapCatAccount, call qzoneCall, post normalizedPost, breaker *needLoginBreaker, coverage *coverageTracker) []discoveredPerson {
	coverage.begin(post.AuthorQQ, "qzone_comments")
	people := []discoveredPerson{}
	position := 0
	for page := 0; ; page++ {
		if ctx.Err() != nil {
			return people
		}
		raw, err := call("get_comment_list", map[string]any{
			"user_id": post.AuthorQQ, "tid": post.TID, "num": qzoneCommentPageSize, "pos": position, "fast_mode": false,
		})
		if err != nil {
			if breaker.check(err) {
				partialErr := coveragePartialError(post.TID, page+1, err)
				coverage.markPartial(post.AuthorQQ, "qzone_comments", partialErr)
				c.warn(account.ID, "get_comment_list partial", partialErr)
				return uniqueDiscovered(people)
			}
			c.warn(account.ID, "get_comment_list skip single post", err)
			return uniqueDiscovered(people)
		}
		values, normalizeErr := c.Normalizer.CommentsAtDepth(ctx, account, "get_comment_list", post, raw, post.RelationDepth+1)
		if normalizeErr != nil {
			partialErr := coveragePartialError(post.TID, page+1, normalizeErr)
			coverage.markPartial(post.AuthorQQ, "qzone_comments", partialErr)
			c.warn(account.ID, "normalize comments partial", partialErr)
			return uniqueDiscovered(people)
		}
		people = append(people, values...)
		coverage.add(post.AuthorQQ, "qzone_comments", len(values))
		info := parsePageInfo(raw, position, qzoneCommentPageSize, "commentlist", "comment_list", "comments", "items", "list")
		coverage.setCursor(post.AuthorQQ, "qzone_comments", contentPageCursor(post.TID, page+1, position, info))
		c.logPartial(account.ID, "get_comment_list", post.TID, page+1, info, coverage, "qzone_comments")
		if !info.HasMore {
			return uniqueDiscovered(people)
		}
		next := info.NextPosition
		if info.NextCursor != "" {
			if parsed, valid := integerField(info.NextCursor); valid {
				next = parsed
			}
		}
		if next <= position {
			return uniqueDiscovered(people)
		}
		position = next
	}
}

func (c Collector) collectLikes(ctx context.Context, account domain.NapCatAccount, call qzoneCall, post normalizedPost, breaker *needLoginBreaker, coverage *coverageTracker) []discoveredPerson {
	coverage.begin(post.AuthorQQ, "qzone_likes")
	people := []discoveredPerson{}
	position := 0
	for page := 0; ; page++ {
		if ctx.Err() != nil {
			return people
		}
		raw, err := call("get_like_list", map[string]any{
			"user_id": post.AuthorQQ, "tid": post.TID, "num": qzoneLikePageSize, "pos": position, "fast_mode": false,
		})
		if err != nil {
			if breaker.check(err) {
				partialErr := coveragePartialError(post.TID, page+1, err)
				coverage.markPartial(post.AuthorQQ, "qzone_likes", partialErr)
				c.warn(account.ID, "get_like_list partial", partialErr)
				return uniqueDiscovered(people)
			}
			c.warn(account.ID, "get_like_list skip single post", err)
			return uniqueDiscovered(people)
		}
		values, normalizeErr := c.Normalizer.LikesAtDepth(ctx, account, "get_like_list", post, raw, post.RelationDepth+1)
		if normalizeErr != nil {
			partialErr := coveragePartialError(post.TID, page+1, normalizeErr)
			coverage.markPartial(post.AuthorQQ, "qzone_likes", partialErr)
			c.warn(account.ID, "normalize likes partial", partialErr)
			return uniqueDiscovered(people)
		}
		people = append(people, values...)
		coverage.add(post.AuthorQQ, "qzone_likes", len(values))
		info := parsePageInfo(raw, position, qzoneLikePageSize, "likelist", "like_list", "likes", "items", "list")
		coverage.setCursor(post.AuthorQQ, "qzone_likes", contentPageCursor(post.TID, page+1, position, info))
		c.logPartial(account.ID, "get_like_list", post.TID, page+1, info, coverage, "qzone_likes")
		if !info.HasMore {
			return uniqueDiscovered(people)
		}
		next := info.NextPosition
		if info.NextCursor != "" {
			if parsed, valid := integerField(info.NextCursor); valid {
				next = parsed
			}
		}
		if next <= position {
			return uniqueDiscovered(people)
		}
		position = next
	}
}

func uniquePosts(items []normalizedPost) []normalizedPost {
	indexes := map[string]int{}
	result := make([]normalizedPost, 0, len(items))
	for _, item := range items {
		if item.ContentID == "" {
			continue
		}
		if index, exists := indexes[item.ContentID]; exists {
			if item.RelationDepth < result[index].RelationDepth {
				result[index] = item
			}
			continue
		}
		indexes[item.ContentID] = len(result)
		result = append(result, item)
	}
	return result
}

func pageQQs(raw json.RawMessage, keys ...string) []string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	seen := map[string]bool{}
	result := []string{}
	for _, item := range findMapSlice(value, keys...) {
		qq := firstText(item, "user_id", "uin", "qq", "opuin", "_uin")
		if qq != "" && !seen[qq] {
			seen[qq] = true
			result = append(result, qq)
		}
	}
	return result
}

func (c Collector) logPartial(accountID, endpoint, scope string, page int, info pageInfo, coverage *coverageTracker, source string) {
	if info.Partial {
		coverage.markPartial(scope, source, fmt.Errorf("page %d: %s", page, info.PartialReason))
		c.warn(accountID, endpoint+" partial", fmt.Errorf("scope %s page %d: %s", scope, page, info.PartialReason))
	}
}

func (c Collector) warn(accountID, step string, err error) {
	if c.Logger != nil {
		c.Logger.Warn("QZone collection step failed", "account_id", accountID, "step", step, "error", err)
	}
}

func progress(update func(int), value int) {
	if update != nil {
		update(value)
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func pageCursor(page, position int, info pageInfo) map[string]any {
	return map[string]any{
		"page": page, "position": position, "item_count": info.ItemCount,
		"has_more": info.HasMore, "has_more_known": info.HasMoreKnown,
		"next_position": info.NextPosition, "next_cursor": info.NextCursor,
		"partial": info.Partial, "partial_reason": info.PartialReason,
	}
}

func contentPageCursor(contentID string, page, position int, info pageInfo) map[string]any {
	cursor := pageCursor(page, position, info)
	cursor["content_id"] = contentID
	return cursor
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+3)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func emitCollectionProgress(observer domain.CollectionObserver, progress domain.CollectionModuleProgress) {
	if observer.Module != nil {
		observer.Module(progress)
	}
}

func errorFromText(message string) error {
	if message == "" {
		return nil
	}
	return fmt.Errorf("%s", message)
}
