package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────
// Pure unit tests (no database required).
// ─────────────────────────────────────────────────────────────────────

func TestEstimateEventTokens(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"", 40},
		{"abcd", 41},
		{"你好世界", 41}, // 4 runes → 1 + 40 overhead
		{"x", 40},
	}
	for _, c := range cases {
		if got := estimateEventTokens(c.body); got != c.want {
			t.Errorf("estimateEventTokens(%q) = %d, want %d", c.body, got, c.want)
		}
	}
}

func TestSnippetBody(t *testing.T) {
	long := strings.Repeat("汉", 500)
	got, truncated := snippetBody(long, 100)
	if !truncated {
		t.Fatalf("expected truncation for long body")
	}
	if len([]rune(got)) != 101 {
		t.Fatalf("truncated rune count = %d, want 101 (100 + ellipsis)", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got[len([]rune(got))-1:])
	}

	short := "short"
	if out, truncated := snippetBody(short, 100); truncated || out != short {
		t.Fatalf("short body should pass through untouched, got %q truncated=%v", out, truncated)
	}

	if out, truncated := snippetBody(long, 0); truncated || out != long {
		t.Fatalf("limit=0 means no truncation, got truncated=%v", truncated)
	}

	if out, truncated := snippetBody(long, -1); truncated || out != long {
		t.Fatalf("negative limit means no truncation, got truncated=%v", truncated)
	}
}

func TestEvidenceDirection(t *testing.T) {
	cases := []struct {
		support, contradict int
		want                string
	}{
		{0, 0, "证据不足"},
		{3, 1, "支持为主"},
		{1, 3, "反证为主"},
		{2, 2, "证据不足"},
	}
	for _, c := range cases {
		if got := evidenceDirection(c.support, c.contradict); got != c.want {
			t.Errorf("evidenceDirection(%d,%d) = %q, want %q", c.support, c.contradict, got, c.want)
		}
	}
}

func TestNegationPatternsCoverCommonChineseNegations(t *testing.T) {
	required := []string{"不是", "没有", "并非", "从未", "不喜欢", "no ", "not ", "never "}
	for _, want := range required {
		found := false
		for _, p := range negationPatterns {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("negationPatterns missing %q", want)
		}
	}
}

func TestAnalysisToolsAdvertisedReadOnly(t *testing.T) {
	names := map[string]bool{
		"sra_content_comments":    false,
		"sra_visitor_stream":      false,
		"sra_build_evidence_pack": false,
		"sra_hypothesis_check":    false,
		"sra_trace_claim":         false,
	}
	for _, tool := range AllTools() {
		if _, ok := names[tool.Name]; !ok {
			continue
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %s must advertise ReadOnlyHint=true", tool.Name)
		}
		names[tool.Name] = true
	}
	for name, seen := range names {
		if !seen {
			t.Errorf("planned analysis tool %s is not advertised", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Live database tests. These seed isolated fixture rows with a unique
// marker and remove everything afterwards; they only run when
// DATABASE_URL is set (same convention as backend/internal/api/e2e_routing_test.go).
// ─────────────────────────────────────────────────────────────────────

type analysisFixture struct {
	subjectID, subjectQQ       string
	visitorID, visitorQQ       string
	commenterID, commenterQQ   string
	mainID, commentID, replyID string
	messageID                  string
	visitedEventID             string
	rawIDs                     []string
	accountID                  string
	conversationID             string
	sha256                     string
}

func seedAnalysisFixture(t *testing.T, db *pgxpool.Pool) *analysisFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("mcp-t%d", time.Now().UnixNano())

	fx := &analysisFixture{
		subjectQQ:   "9" + suffix,
		visitorQQ:   "8" + suffix,
		commenterQQ: "7" + suffix,
	}

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture exec failed: %v\nSQL: %s", err, sql)
		}
	}
	mustQueryRow := func(sql string, args ...any) (out string) {
		t.Helper()
		if err := db.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
			t.Fatalf("fixture query failed: %v\nSQL: %s", err, sql)
		}
		return out
	}

	// NapCat account is required by media_references.source_account_id.
	fx.accountID = mustQueryRow(`INSERT INTO napcat_accounts(name, qq_uin, ws_url, http_url)
		VALUES($1,'test','','') RETURNING id::text`, "mcp-test-"+suffix)

	fx.subjectID = mustQueryRow(`INSERT INTO persons(display_name) VALUES('MCP测试主体') RETURNING id::text`)
	fx.visitorID = mustQueryRow(`INSERT INTO persons(display_name) VALUES('MCP测试访客') RETURNING id::text`)
	fx.commenterID = mustQueryRow(`INSERT INTO persons(display_name) VALUES('MCP测试评论者') RETURNING id::text`)

	mustExec(`INSERT INTO person_identifiers(person_id, platform, platform_user_id)
		VALUES($1,'qq',$2)`, fx.subjectID, fx.subjectQQ)
	mustExec(`INSERT INTO person_identifiers(person_id, platform, platform_user_id)
		VALUES($1,'qq',$2)`, fx.visitorID, fx.visitorQQ)
	mustExec(`INSERT INTO person_identifiers(person_id, platform, platform_user_id)
		VALUES($1,'qq',$2)`, fx.commenterID, fx.commenterQQ)

	newRaw := func(endpoint string) string {
		t.Helper()
		return mustQueryRow(`INSERT INTO raw_records(source, endpoint_or_event_type, payload, payload_hash)
			VALUES('mcp-test',$1,'{"fixture":true}'::jsonb,$2) RETURNING id::text`,
			endpoint, "hash-"+suffix+"-"+endpoint)
	}
	for i := 0; i < 8; i++ {
		fx.rawIDs = append(fx.rawIDs, newRaw(fmt.Sprintf("fixture-%d", i)))
	}

	fx.mainID = mustQueryRow(`INSERT INTO contents(platform, platform_content_id, author_id, context_type, body, published_at, raw_record_id)
		VALUES('qzone',$1,$2,'qzone','主帖正文',now(),$3) RETURNING id::text`,
		"platform-"+suffix+"-main", fx.subjectID, fx.rawIDs[0])
	fx.commentID = mustQueryRow(`INSERT INTO contents(platform, platform_content_id, author_id, context_type, body, published_at, parent_content_id, raw_record_id)
		VALUES('qzone',$1,$2,'qzone','评论正文',now(),$3,$4) RETURNING id::text`,
		"platform-"+suffix+"-c1", fx.visitorID, fx.mainID, fx.rawIDs[1])
	fx.replyID = mustQueryRow(`INSERT INTO contents(platform, platform_content_id, author_id, context_type, body, published_at, parent_content_id, reply_to_content_id, raw_record_id)
		VALUES('qzone',$1,$2,'qzone','回复正文',now(),$3,$4,$5) RETURNING id::text`,
		"platform-"+suffix+"-c2", fx.visitorID, fx.mainID, fx.commentID, fx.rawIDs[2])

	fx.conversationID = mustQueryRow(`INSERT INTO conversations(conversation_type, platform_conversation_id, name)
		VALUES('group',$1,'MCP测试群') RETURNING id::text`, "conv-"+suffix)

	fx.messageID = mustQueryRow(`INSERT INTO messages(source_message_id, conversation_id, sender_id, sent_at, raw_text, message_segments, raw_record_id)
		VALUES($1,$2,$3,now(),'火锅是世界上最好吃的食物','[]'::jsonb,$4) RETURNING id::text`,
		"msg-"+suffix+"-support", fx.conversationID, fx.subjectID, fx.rawIDs[3])
	mustExec(`INSERT INTO messages(source_message_id, conversation_id, sender_id, sent_at, raw_text, message_segments, raw_record_id)
		VALUES($1,$2,$3,now(),'我其实不是火锅爱好者','[]'::jsonb,$4)`,
		"msg-"+suffix+"-contra", fx.conversationID, fx.subjectID, fx.rawIDs[4])
	mustExec(`INSERT INTO messages(source_message_id, conversation_id, sender_id, sent_at, raw_text, message_segments, raw_record_id)
		VALUES($1,$2,$3,now(),$4||' 也玩火锅','[]'::jsonb,$5)`,
		"msg-"+suffix+"-other", fx.conversationID, fx.commenterID, fx.subjectQQ, fx.rawIDs[5])

	fx.visitedEventID = mustQueryRow(`INSERT INTO relation_events(actor_person_id, target_person_id, action_type, context_type, occurred_at, evidence_ids, raw_record_id)
		VALUES($1,$2,'visited','qzone',now(),'{}'::uuid[],$3) RETURNING id::text`,
		fx.visitorID, fx.subjectID, fx.rawIDs[6])
	mustExec(`INSERT INTO relation_events(actor_person_id, target_person_id, action_type, context_type, occurred_at, evidence_ids, raw_record_id)
		VALUES($1,$2,'liked','qzone',now(),'{}'::uuid[],$3)`,
		fx.visitorID, fx.subjectID, fx.rawIDs[7])

	// Media asset + reference linked to the supporting message, so the
	// evidence pack can also demonstrate include_media_meta path.
	fx.sha256 = "sha-" + suffix
	assetID := mustQueryRow(`INSERT INTO media_assets(sha256, mime_type, size, object_path)
		VALUES($1,'image/png',1234,'/tmp/mcp-test/'||$1) RETURNING id::text`, fx.sha256)
	mustExec(`INSERT INTO media_references(source_account_id, message_id, segment_index, segment_type, media_kind, source_url, status, asset_id)
		VALUES($1,$2,0,'image','image','','completed',$3)`,
		fx.accountID, fx.messageID, assetID)

	t.Cleanup(func() {
		cleanupFixture(t, db, fx)
	})
	return fx
}

func cleanupFixture(t *testing.T, db *pgxpool.Pool, fx *analysisFixture) {
	t.Helper()
	ctx := context.Background()
	ids := []string{fx.subjectID, fx.visitorID, fx.commenterID}
	sqls := []string{
		`DELETE FROM inferences WHERE subject_id = ANY($1::text[])`,
		`DELETE FROM contents WHERE author_id = ANY($1::uuid[])`,
		`DELETE FROM messages WHERE sender_id = ANY($1::uuid[])`,
		`DELETE FROM relation_events WHERE actor_person_id = ANY($1::uuid[]) OR target_person_id = ANY($1::uuid[])`,
		`DELETE FROM media_references WHERE source_account_id = $1`,
		`DELETE FROM raw_records WHERE id = ANY($1::uuid[])`,
		`DELETE FROM conversations WHERE id = $1`,
		`DELETE FROM persons WHERE id = ANY($1::uuid[])`,
		`DELETE FROM media_assets WHERE sha256 = $1`,
		`DELETE FROM napcat_accounts WHERE id = $1`,
	}
	for _, sql := range sqls {
		var args []any
		switch {
		case strings.Contains(sql, "media_references"), strings.Contains(sql, "napcat_accounts"), strings.Contains(sql, "conversations"):
			args = []any{fx.accountID}
			if strings.Contains(sql, "conversations") {
				args = []any{fx.conversationID}
			}
		case strings.Contains(sql, "media_assets"):
			args = []any{fx.sha256}
		case strings.Contains(sql, "raw_records"):
			args = []any{fx.rawIDs}
		default:
			args = []any{ids}
		}
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Logf("cleanup failed (ignored): %v\nSQL: %s", err, sql)
		}
	}
}

// callTool is a tiny helper that invokes a handler through the registry and
// unmarshals the JSON text content into out.
func callTool(t *testing.T, db *pgxpool.Pool, name string, args map[string]any, out any) *ToolCallResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	h := &HandlerRegistry{DB: db}
	res, err := h.CallTool(context.Background(), name, raw)
	if err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
	if res.IsError {
		return res
	}
	if out != nil {
		if err := json.Unmarshal([]byte(res.Content[0].Text), out); err != nil {
			t.Fatalf("%s unmarshal result: %v", name, err)
		}
	}
	return res
}

func TestLiveAnalysisTools(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live MCP analysis tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close) // registered before fixture cleanup so pool closes last

	fx := seedAnalysisFixture(t, pool)

	// 1. sra_content_comments — full thread.
	var comments struct {
		IncludeReplies bool `json:"include_replies"`
		Comments       struct {
			Data []struct {
				ID               string `json:"id"`
				AuthorQQ         string `json:"author_qq"`
				ReplyToContentID string `json:"reply_to_content_id"`
			} `json:"data"`
			Total int `json:"total"`
		} `json:"comments"`
	}
	res := callTool(t, pool, "sra_content_comments", map[string]any{
		"content_id": fx.mainID,
		"limit":      50,
	}, &comments)
	if res.IsError {
		t.Fatalf("content_comments error: %s", res.Content[0].Text)
	}
	if comments.Comments.Total != 2 || len(comments.Comments.Data) != 2 {
		t.Fatalf("expected 2 comments total, got total=%d data=%d", comments.Comments.Total, len(comments.Comments.Data))
	}
	replyFound := false
	for _, c := range comments.Comments.Data {
		if c.ReplyToContentID != "" {
			replyFound = true
			if c.AuthorQQ != fx.visitorQQ {
				t.Fatalf("reply author QQ = %s, want %s", c.AuthorQQ, fx.visitorQQ)
			}
		}
	}
	if !replyFound {
		t.Fatalf("expected a nested reply with reply_to_content_id set")
	}

	// 2. Top-level only — reply must be filtered out.
	var top struct {
		Comments struct {
			Total int              `json:"total"`
			Data  []map[string]any `json:"data"`
		} `json:"comments"`
	}
	callTool(t, pool, "sra_content_comments", map[string]any{
		"content_id":      fx.mainID,
		"include_replies": false,
		"limit":           50,
	}, &top)
	if top.Comments.Total != 1 || len(top.Comments.Data) != 1 {
		t.Fatalf("top-level only: expected 1, got total=%d data=%d", top.Comments.Total, len(top.Comments.Data))
	}

	// 3. sra_visitor_stream — scoped by target.
	var visitors struct {
		Data []struct {
			VisitorQQ  string `json:"visitor_qq"`
			TargetQQ   string `json:"target_qq"`
			VisitCount int    `json:"visit_count"`
		} `json:"data"`
		Total int `json:"total"`
	}
	callTool(t, pool, "sra_visitor_stream", map[string]any{
		"target_qq": fx.subjectQQ,
		"limit":     50,
	}, &visitors)
	if visitors.Total != 1 || len(visitors.Data) != 1 {
		t.Fatalf("expected 1 visitor pair, got total=%d data=%d", visitors.Total, len(visitors.Data))
	}
	if visitors.Data[0].VisitorQQ != fx.visitorQQ || visitors.Data[0].VisitCount != 1 {
		t.Fatalf("visitor = %s count=%d, want %s count=1",
			visitors.Data[0].VisitorQQ, visitors.Data[0].VisitCount, fx.visitorQQ)
	}

	// 4. No scope params → hard error.
	res = callTool(t, pool, "sra_visitor_stream", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("visitor_stream without scope must error")
	}

	// 5. Evidence pack preview (not persisted).
	var pack struct {
		Persisted       bool `json:"persisted"`
		EventCount      int  `json:"event_count"`
		EstimatedTokens int  `json:"estimated_tokens"`
		Events          []struct {
			EventID    string `json:"event_id"`
			ActionType string `json:"action_type"`
		} `json:"events"`
	}
	callTool(t, pool, "sra_build_evidence_pack", map[string]any{
		"subject_qq":   fx.subjectQQ,
		"question":     "测试证据包",
		"token_budget": 60000,
	}, &pack)
	if pack.Persisted {
		t.Fatalf("preview pack must not be persisted")
	}
	if pack.EventCount == 0 || pack.EstimatedTokens <= 0 {
		t.Fatalf("expected non-empty pack, events=%d tokens=%d", pack.EventCount, pack.EstimatedTokens)
	}
	hasMessage := false
	for _, e := range pack.Events {
		if e.ActionType == "sent_message" {
			hasMessage = true
		}
	}
	if !hasMessage {
		t.Fatalf("expected at least one sent_message event in pack")
	}

	// 6. Evidence pack preview with media meta (paths must never appear).
	var packMedia struct {
		Events []struct {
			Media []struct {
				Sha256   string `json:"sha256"`
				MimeType string `json:"mime_type"`
				Size     int64  `json:"size"`
			} `json:"media"`
		} `json:"events"`
	}
	callTool(t, pool, "sra_build_evidence_pack", map[string]any{
		"subject_qq":         fx.subjectQQ,
		"include_media_meta": true,
		"token_budget":       60000,
	}, &packMedia)
	mediaFound := false
	for _, ev := range packMedia.Events {
		for _, m := range ev.Media {
			mediaFound = true
			if m.Sha256 == "" || m.MimeType == "" || m.Size == 0 {
				t.Fatalf("media meta incomplete: %+v", m)
			}
		}
	}
	if !mediaFound {
		t.Fatalf("expected media meta on the message event")
	}

	// 7. Persist branch — row must be created and return pack_id.
	var packPersist struct {
		Persisted bool   `json:"persisted"`
		PackID    string `json:"pack_id"`
	}
	callTool(t, pool, "sra_build_evidence_pack", map[string]any{
		"subject_qq":   fx.subjectQQ,
		"question":     "持久化测试",
		"persist":      true,
		"token_budget": 60000,
	}, &packPersist)
	if !packPersist.Persisted || packPersist.PackID == "" {
		t.Fatalf("expected persisted pack with id, got %+v", packPersist)
	}
	// A second persist of the same subject creates a second row (append-only).
	callTool(t, pool, "sra_build_evidence_pack", map[string]any{
		"subject_qq":   fx.subjectQQ,
		"question":     "持久化测试2",
		"persist":      true,
		"token_budget": 60000,
	}, &packPersist)
	if !packPersist.Persisted || packPersist.PackID == "" {
		t.Fatalf("expected second persisted pack, got %+v", packPersist)
	}
	var packCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM evidence_packs WHERE subject_id=$1`, fx.subjectID).Scan(&packCount); err != nil {
		t.Fatalf("count persisted packs: %v", err)
	}
	if packCount < 2 {
		t.Fatalf("expected >=2 persisted packs, got %d", packCount)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM evidence_packs WHERE subject_id=$1`, fx.subjectID); err != nil {
		t.Fatalf("cleanup evidence_packs: %v", err)
	}

	// 8. Hypothesis check — support (self + other) > contradiction (self).
	var hypo struct {
		SupportingCount    int    `json:"supporting_count"`
		ContradictingCount int    `json:"contradicting_count"`
		Direction          string `json:"direction"`
		Methodology        string `json:"methodology"`
	}
	callTool(t, pool, "sra_hypothesis_check", map[string]any{
		"subject_qq": fx.subjectQQ,
		"claim":      "火锅",
	}, &hypo)
	if hypo.SupportingCount < 2 {
		t.Fatalf("expected >=2 supporting, got %d", hypo.SupportingCount)
	}
	if hypo.ContradictingCount < 1 {
		t.Fatalf("expected >=1 contradicting, got %d", hypo.ContradictingCount)
	}
	if hypo.Direction != "支持为主" {
		t.Fatalf("direction = %s, want 支持为主", hypo.Direction)
	}
	if hypo.Methodology == "" {
		t.Fatalf("methodology warning must be present")
	}

	// 9. Trace claim — full chain resolves.
	var inferenceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO inferences(subject_id, attribute_type, value, confidence, method_version, evidence_event_ids, review_status)
		VALUES($1,'hobby','{"value":"火锅"}'::jsonb,0.7,'mcp-test',$2::uuid[],'pending') RETURNING id::text`,
		fx.subjectID, []string{fx.visitedEventID}).Scan(&inferenceID); err != nil {
		t.Fatalf("insert inference: %v", err)
	}
	var trace struct {
		TraceComplete bool     `json:"trace_complete"`
		MissingIDs    []string `json:"missing_event_ids"`
		Events        []struct {
			ID         string `json:"id"`
			ActionType string `json:"action_type"`
		} `json:"events"`
	}
	callTool(t, pool, "sra_trace_claim", map[string]any{"inference_id": inferenceID}, &trace)
	if !trace.TraceComplete || len(trace.MissingIDs) != 0 {
		t.Fatalf("expected complete trace, complete=%v missing=%v", trace.TraceComplete, trace.MissingIDs)
	}
	if len(trace.Events) != 1 || trace.Events[0].ActionType != "visited" {
		t.Fatalf("expected 1 visited event, got %+v", trace.Events)
	}

	// 10. Trace claim — broken link is reported, not silently dropped.
	var brokenID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO inferences(subject_id, attribute_type, value, confidence, method_version, evidence_event_ids, review_status)
		VALUES($1,'hobby','{"value":"x"}'::jsonb,0.5,'mcp-test',$2::uuid[],'pending') RETURNING id::text`,
		fx.subjectID, []string{"00000000-0000-0000-0000-000000000000"}).Scan(&brokenID); err != nil {
		t.Fatalf("insert broken inference: %v", err)
	}
	var broken struct {
		TraceComplete bool     `json:"trace_complete"`
		MissingIDs    []string `json:"missing_event_ids"`
	}
	callTool(t, pool, "sra_trace_claim", map[string]any{"inference_id": brokenID}, &broken)
	if broken.TraceComplete || len(broken.MissingIDs) != 1 {
		t.Fatalf("expected incomplete trace with 1 missing id, got %+v", broken)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM inferences WHERE subject_id=$1`, fx.subjectID); err != nil {
		t.Fatalf("cleanup inferences: %v", err)
	}
}

func TestLiveAnalysisMultiAccountIsolation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live MCP isolation tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)

	// Two isolated subjects; visitor stream scoped to subject A must never
	// surface subject B's visitors.
	fxA := seedAnalysisFixture(t, pool)
	fxB := seedAnalysisFixture(t, pool)

	var visitorsA struct {
		Data []struct {
			TargetQQ string `json:"target_qq"`
		} `json:"data"`
		Total int `json:"total"`
	}
	callTool(t, pool, "sra_visitor_stream", map[string]any{
		"target_qq": fxA.subjectQQ,
		"limit":     50,
	}, &visitorsA)
	if visitorsA.Total != 1 {
		t.Fatalf("subject A visitors total = %d, want 1", visitorsA.Total)
	}
	for _, v := range visitorsA.Data {
		if v.TargetQQ == fxB.subjectQQ {
			t.Fatalf("cross-account leak: A's visitor stream contains B's target %s", v.TargetQQ)
		}
	}

	// Content comments must also stay scoped.
	var commentsB struct {
		Comments struct {
			Total int `json:"total"`
		} `json:"comments"`
	}
	callTool(t, pool, "sra_content_comments", map[string]any{
		"content_id": fxB.mainID,
		"limit":      50,
	}, &commentsB)
	if commentsB.Comments.Total != 2 {
		t.Fatalf("subject B comments total = %d, want 2", commentsB.Comments.Total)
	}
}
