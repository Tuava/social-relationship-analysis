package api

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

func TestLiveDatabaseRoutingAndDeepRelationship(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live routing tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	router := analysis.Router{DB: pool, Policy: liveRoutingPolicy()}

	// 1. Test least_cost profile routing between two real users
	t.Log("Testing least_cost profile routing between 10000001 and 10000002...")
	res, err := router.PlanRoutes(ctx, analysis.RouteRequest{
		SourceQQ: "10000001",
		TargetQQ: "10000002",
		Profile:  "least_cost",
		MaxHops:  4,
		MaxPaths: 3,
	})
	if err != nil {
		t.Fatalf("PlanRoutes failed: %v", err)
	}

	t.Logf("Paths found: %d", res.PathsFound)
	t.Logf("Routing stats: %+v", res.Stats)
	if res.PathsFound == 0 {
		t.Fatalf("expected at least 1 path, got 0")
	}

	for i, r := range res.Routes {
		t.Logf("Route %d: %s (Hops: %d, Weight: %d, Cost: %.2f, Confidence: %d%%)",
			i+1, r.Title, r.TotalHops, r.TotalWeight, r.TotalCost, r.Confidence)
		for _, step := range r.Steps {
			t.Logf("  Step %d: [%s] %s -> [%s] %s via %s (cost: %.2f, summary: %s)",
				step.StepNumber, step.SourceType, step.SourceLabel, step.TargetType, step.TargetLabel,
				step.RelationType, step.Cost, step.MediumSummary)
		}
	}

	// 2. Test covert profile (excluding large groups)
	t.Log("Testing covert profile routing...")
	resCovert, err := router.PlanRoutes(ctx, analysis.RouteRequest{
		SourceQQ:           "10000001",
		TargetQQ:           "10000002",
		Profile:            "covert",
		MaxHops:            4,
		MaxPaths:           2,
		ExcludeLargeGroups: true,
	})
	if err != nil {
		t.Fatalf("covert PlanRoutes failed: %v", err)
	}
	t.Logf("Covert paths found: %d", resCovert.PathsFound)

	// 3. Test shortest profile
	t.Log("Testing shortest profile routing...")
	resShortest, err := router.PlanRoutes(ctx, analysis.RouteRequest{
		SourceQQ: "10000001",
		TargetQQ: "10000002",
		Profile:  "shortest",
		MaxHops:  4,
		MaxPaths: 2,
	})
	if err != nil {
		t.Fatalf("shortest PlanRoutes failed: %v", err)
	}
	t.Logf("Shortest paths found: %d", resShortest.PathsFound)
}

func TestLiveEvidenceDetails(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live routing tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	router := analysis.Router{DB: pool, Policy: liveRoutingPolicy()}
	res, err := router.PlanRoutes(ctx, analysis.RouteRequest{
		SourceQQ: "10000001",
		TargetQQ: "10000002",
		Profile:  "least_cost",
		MaxHops:  1,
	})
	if err != nil || len(res.Routes) == 0 || len(res.Routes[0].Steps) == 0 {
		t.Fatalf("plan routes failed or empty: %v", err)
	}

	step := res.Routes[0].Steps[0]
	t.Logf("Step has %d evidence IDs", len(step.EvidenceIDs))

	// Query raw_records (where step.EvidenceIDs actually point to)
	var rowsCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM raw_records WHERE id = ANY($1::uuid[])`, step.EvidenceIDs).Scan(&rowsCount)
	if err != nil {
		t.Fatalf("query count failed: %v", err)
	}
	t.Logf("Found %d matching raw_records in database", rowsCount)

	// Fetch detailed raw_records sample
	rows, err := pool.Query(ctx, `
		SELECT 
			id::text, endpoint_or_event_type, collected_at::text, payload::text
		FROM raw_records
		WHERE id = ANY($1::uuid[])
		LIMIT 5`, step.EvidenceIDs)
	if err != nil {
		t.Fatalf("detailed query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, endpoint, collectedAt, raw string
		if err := rows.Scan(&id, &endpoint, &collectedAt, &raw); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Raw Record: [%s] at %s | Payload Len: %d | Preview: %.120s", endpoint, collectedAt, len(raw), raw)
	}
}

func TestLiveKnownRoutingRegressions(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is required for live routing tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	router := analysis.Router{DB: pool, Policy: liveRoutingPolicy()}

	cases := []struct {
		name     string
		targetQQ string
		maxHops  int
		maxPaths int
		profile  string
	}{
		{name: "direct shared group", targetQQ: "30819792", maxHops: 1, maxPaths: 3, profile: "least_cost"},
		{name: "deep qzone chain", targetQQ: "1134441887", maxHops: 6, maxPaths: 8, profile: "shortest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			started := time.Now()
			result, err := router.PlanRoutes(ctx, analysis.RouteRequest{
				SourceQQ: "10000001",
				TargetQQ: tc.targetQQ,
				Profile:  tc.profile,
				MaxHops:  tc.maxHops,
				MaxPaths: tc.maxPaths,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.PathsFound == 0 {
				t.Fatalf("no route found: %+v", result.Stats)
			}
			if truncated, _ := result.Stats["truncated"].(bool); truncated {
				t.Fatalf("route search was unexpectedly truncated: %+v", result.Stats)
			}
			if tc.name == "deep qzone chain" {
				foundReverseLike := false
				for _, step := range result.Routes[0].Steps {
					if step.SourceLabel == "SyntheticPerson" && step.TargetLabel == "suG_05657D" && step.RelationType == "liked" {
						foundReverseLike = step.EvidenceDirection == "reverse" && step.EvidenceActorKey == step.TargetKey && step.EvidenceTargetKey == step.SourceKey
					}
				}
				if !foundReverseLike {
					t.Fatalf("deep route did not preserve reverse like evidence direction: %+v", result.Routes[0].Steps)
				}
			}
			t.Logf("target=%s paths=%d hops=%d elapsed=%s stats=%+v", tc.targetQQ, result.PathsFound, result.Routes[0].TotalHops, time.Since(started), result.Stats)
			for routeIndex, route := range result.Routes {
				t.Logf("route %d hops=%d cost=%.2f", routeIndex+1, route.TotalHops, route.TotalCost)
				for _, step := range route.Steps {
					t.Logf("  %s -> %s via %s", step.SourceLabel, step.TargetLabel, step.RelationType)
				}
			}
		})
	}
}

func liveRoutingPolicy() analysis.RoutingPolicy {
	return analysis.RoutingPolicy{
		DefaultMaxHops:             4,
		DefaultMaxPaths:            3,
		DefaultLargeGroupThreshold: 100,
	}
}
