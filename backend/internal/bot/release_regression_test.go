package bot

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
	"log/slog"
	"os"
	"testing"
)

func TestReleaseEncryptedAccountAndBotSchema(t *testing.T) {
	dsn := os.Getenv("SRA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated SRA_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := persistence.Repository{DB: pool, EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	account, err := repo.CreateAccount(ctx, domain.NapCatAccount{Name: "bot-release-test", QQUIN: "10091001", WSURL: "ws://127.0.0.1:1", HTTPToken: "ci-napcat-token", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT http_token FROM napcat_accounts WHERE id=$1`, account.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !secrets.IsEncrypted(stored) {
		t.Fatal("token was not encrypted at rest")
	}
	s := NewService(pool, repo, slog.Default())
	loaded, err := s.accountForSend(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HTTPToken != "ci-napcat-token" {
		t.Fatal("sending did not use decrypted token")
	}
	var botID string
	if err := pool.QueryRow(ctx, `INSERT INTO bot_instances(account_id) VALUES($1) RETURNING id::text`, account.ID).Scan(&botID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bot_user_whitelist(bot_id,user_qq) VALUES($1,'10091002')`, botID); err != nil {
		t.Fatal(err)
	}
	if !s.isWhitelistedUser(ctx, botID, "10091002") {
		t.Fatal("whitelist schema missing")
	}
	id := s.startAudit(ctx, Instance{ID: botID, AccountID: account.ID}, message{UserID: "10091002", Private: true}, "ci-test", "fixture")
	if id == "" {
		t.Fatal("audit insert failed")
	}
	s.finishAudit(ctx, id, "completed", "fixture", []string{"ci-test"}, 1, "")
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bot_audit_log WHERE id=$1`, id).Scan(&status); err != nil || status != "completed" {
		t.Fatalf("audit schema: %s %v", status, err)
	}
}
