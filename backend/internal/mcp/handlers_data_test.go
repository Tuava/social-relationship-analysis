package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMessageHistoryAndRelationEventsWithFilters(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live MCP handlers_data tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	registry := &HandlerRegistry{DB: pool}

	t.Run("MessageHistoryWithTimeAndQueryFilters", func(t *testing.T) {
		args := json.RawMessage(`{
			"query": "hello",
			"time_start": "2020-01-01T00:00:00Z",
			"time_end": "2030-01-01T00:00:00Z",
			"limit": 10
		}`)
		res, err := registry.handleMessageHistory(ctx, args)
		if err != nil {
			t.Fatalf("handleMessageHistory failed: %v", err)
		}
		if res.IsError {
			t.Fatalf("handleMessageHistory returned error result: %s", res.Content[0].Text)
		}
	})

	t.Run("RelationEventsWithTimeAndActionTypeFilters", func(t *testing.T) {
		args := json.RawMessage(`{
			"action_types": ["sent_message", "liked"],
			"time_start": "2020-01-01T00:00:00Z",
			"time_end": "2030-01-01T00:00:00Z",
			"limit": 10
		}`)
		res, err := registry.handleRelationEvents(ctx, args)
		if err != nil {
			t.Fatalf("handleRelationEvents failed: %v", err)
		}
		if res.IsError {
			t.Fatalf("handleRelationEvents returned error result: %s", res.Content[0].Text)
		}
	})
}
