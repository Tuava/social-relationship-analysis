package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

func TestReleaseDatabaseRegressions(t *testing.T) {
	dsn := os.Getenv("SRA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SRA_TEST_DATABASE_URL requires a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := persistence.Repository{DB: pool}
	account, err := repo.CreateAccount(ctx, domain.NapCatAccount{Name: "release-test", QQUIN: "10090001", WSURL: "ws://127.0.0.1:1", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	n := normalization.Normalizer{DB: pool}
	payload := []byte(`{"message_type":"private","user_id":10090001,"sender":{"user_id":10090001},"message_id":"release-message","time":1700000000,"message":[{"type":"text","data":{"text":"fixture outgoing"}}]}`)
	rawID, err := repo.SaveRaw(ctx, account.ID, "napcat_http", "get_friend_msg_history:10090002:message", payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.ProcessRawMessage(ctx, account.ID, rawID, payload, "10090002"); err != nil {
		t.Fatal(err)
	}
	// Reprocessing must retain the peer and deduplicate the message.
	if err := n.ProcessRawMessage(ctx, account.ID, rawID, payload, "10090002"); err != nil {
		t.Fatal(err)
	}
	var messageID, peer, sender string
	if err := pool.QueryRow(ctx, `SELECT m.id::text,c.platform_conversation_id,pi.platform_user_id FROM messages m JOIN conversations c ON c.id=m.conversation_id JOIN person_identifiers pi ON pi.person_id=m.sender_id AND pi.platform='qq' WHERE m.source_account_id=$1`, account.ID).Scan(&messageID, &peer, &sender); err != nil {
		t.Fatal(err)
	}
	if peer != "10090002" || sender != "10090001" {
		t.Fatalf("identity mismatch: %s / %s", peer, sender)
	}
	var assetID string
	if err := pool.QueryRow(ctx, `INSERT INTO media_assets(sha256,mime_type,size,object_path,ocr_text) VALUES($1,'image/png',20,'test/image','release OCR fixture') RETURNING id::text`, account.ID).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	h := &HandlerRegistry{DB: pool, Repo: &repo}
	call := func(name, args string) *ToolCallResult {
		t.Helper()
		result, err := h.CallTool(ctx, name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result == nil || result.IsError {
			t.Fatalf("%s: %+v", name, result)
		}
		return result
	}
	result := call("sra_message_detail", `{"message_id":"`+messageID+`"}`)
	if !strings.Contains(result.Content[0].Text, "fixture outgoing") {
		t.Fatal("message detail lost fixture")
	}
	result = call("sra_media_ocr_search", `{"query":"release OCR fixture"}`)
	if !strings.Contains(result.Content[0].Text, assetID) {
		t.Fatal("OCR asset not returned")
	}
	call("sra_person_timeline", `{"qq":"10090001","event_type":"sent_message","limit":5}`)
	call("sra_research_workspaces", `{"draft_only":true,"limit":5}`)
	if p, err := OpenSQLReadPool(ctx, dsn); err == nil {
		if p != nil {
			p.Close()
		}
		t.Fatal("application owner accepted as SQL reader")
	}
	// Verify database privileges remain effective for an encoded identifier.
	if _, err := pool.Exec(ctx, `CREATE ROLE sra_ci_reader LOGIN PASSWORD 'ci-reader-only'; GRANT USAGE ON SCHEMA public TO sra_ci_reader; GRANT SELECT ON messages TO sra_ci_reader`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `DROP OWNED BY sra_ci_reader; DROP ROLE sra_ci_reader`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("sra_ci_reader", "ci-reader-only")
	reader, err := OpenSQLReadPool(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	h.SQLDB = reader
	call("sra_sql_query", `{"sql":"SELECT count(*) FROM messages"}`)
	bad, _ := json.Marshal(map[string]any{"sql": `SELECT username FROM U&"app\005fusers"`})
	rejected, err := h.handleSQLQuery(ctx, bad)
	if err != nil || rejected == nil || !rejected.IsError {
		t.Fatal("credential table was not blocked by database grants")
	}
}
