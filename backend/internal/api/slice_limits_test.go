package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

// TestPersonDetailSliceLimits verifies the person detail slices honor their
// optional limit query params (message_limit/content_limit/membership_limit)
// and keep the old behavior when the params are absent.
func TestPersonDetailSliceLimits(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live slice-limit tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	// Pick a person who has plenty of messages so the limit is observable.
	var id string
	err = pool.QueryRow(ctx, `
		SELECT p.id::text FROM persons p
		WHERE (SELECT count(*) FROM messages m WHERE m.sender_id=p.id) >= 60
		ORDER BY (SELECT count(*) FROM messages m WHERE m.sender_id=p.id) DESC LIMIT 1`).Scan(&id)
	if err != nil {
		t.Skip("no person with >=60 messages in the database")
	}

	srv := &Server{Repo: persistence.Repository{DB: pool}}

	call := func(query string) (messages int, contents int, memberships int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+id+query, nil)
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
		rec := httptest.NewRecorder()
		srv.person(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("person detail status = %d: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data struct {
				Messages    []any `json:"messages"`
				Contents    []any `json:"contents"`
				Memberships []any `json:"memberships"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return len(body.Data.Messages), len(body.Data.Contents), len(body.Data.Memberships)
	}

	// Default behavior stays at the historical caps.
	defMsgs, defContents, _ := call("")
	if defMsgs > 50 {
		t.Fatalf("default message slice should cap at 50, got %d", defMsgs)
	}
	if defContents > 50 {
		t.Fatalf("default content slice should cap at 50, got %d", defContents)
	}

	// An explicit limit must be honored up to MaxPageSize.
	msgs, contents, memberships := call("?message_limit=500&content_limit=500&membership_limit=500")
	if msgs < 60 {
		t.Fatalf("message_limit=500 should return >=60 messages, got %d", msgs)
	}
	if msgs > 500 {
		t.Fatalf("message_limit should clamp at MaxPageSize 500, got %d", msgs)
	}
	if memberships > 500 {
		t.Fatalf("membership_limit should clamp at MaxPageSize 500, got %d", memberships)
	}
	if contents > 500 {
		t.Fatalf("content_limit should clamp at MaxPageSize 500, got %d", contents)
	}
}
