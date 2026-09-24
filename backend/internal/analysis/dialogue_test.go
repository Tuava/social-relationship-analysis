package analysis

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLiveDialogueDisentanglement(t *testing.T) {
	if os.Getenv("TEST_LIVE_LLM") != "1" {
		t.Skip("Skipping live LLM test; set TEST_LIVE_LLM=1 to run against real AI models")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/social_relationship_analysis?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	// Test on conversation 19af1ded-3a27-4246-8a87-5a9b1ee60f31 (星筱AI交流群)
	threads, err := DisentangleConversation(ctx, pool, "19af1ded-3a27-4246-8a87-5a9b1ee60f31", 60)
	if err != nil {
		t.Fatalf("DisentangleConversation failed: %v", err)
	}

	t.Logf("Disentangled %d threads:", len(threads))
	for i, th := range threads {
		t.Logf("[%d] Thread: Title='%s', Category='%s', Stance='%s', Msgs=%d, Users=%d",
			i+1, th.Title, th.TopicCategory, th.Stance, th.MessageCount, th.ParticipantCount)
		t.Logf("    Summary: %s", th.Summary)
		t.Logf("    KeyEntities: %v", th.KeyEntities)
	}
}
