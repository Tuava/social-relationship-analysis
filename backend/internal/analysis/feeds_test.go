package analysis

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLiveFeedDynamicsAnalysis(t *testing.T) {
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

	// Test on user 3819100392 (星雨)
	res, err := AnalyzeFeedDynamics(ctx, pool, "3819100392")
	if err != nil {
		t.Fatalf("AnalyzeFeedDynamics failed: %v", err)
	}

	t.Logf("Feed Dynamics for %s (%s): Posts=%d, Interactions=%d",
		res.DisplayName, res.QQ, res.TotalPosts, res.TotalInteractions)
	t.Logf("Sentiment months=%d, Milestones=%d, Tier1=%d, Tier2=%d, Tier3=%d",
		len(res.SentimentFlow), len(res.NarrativeMilestones),
		len(res.CircleHierarchy.Tier1Speedy), len(res.CircleHierarchy.Tier2Deep), len(res.CircleHierarchy.Tier3Casual))
	t.Logf("Circle Summary: %s", res.CircleSummary)
}
