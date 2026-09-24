package analysis

import (
	"strings"
	"testing"
	"time"
)

func TestCalculateEdgeCost(t *testing.T) {
	now := time.Now()
	costMessage := calculateEdgeCost("sent_message", 10, now, "least_cost", 0)
	costLike := calculateEdgeCost("liked", 10, now, "least_cost", 0)
	costLargeGroup := calculateEdgeCost("member_of", 1, now, "least_cost", 500)

	if costMessage >= costLike {
		t.Fatalf("expected direct message cost (%f) to be lower than like cost (%f)", costMessage, costLike)
	}
	if costLike >= costLargeGroup {
		t.Fatalf("expected like cost (%f) to be lower than large group cost (%f)", costLike, costLargeGroup)
	}

	// In shortest mode, all costs should be 1.0
	costShortest := calculateEdgeCost("member_of", 1, now, "shortest", 500)
	if costShortest != 1.0 {
		t.Fatalf("expected shortest mode cost to be 1.0, got %f", costShortest)
	}
}

func TestDijkstraAndKShortestPathsInMemory(t *testing.T) {
	// Build a test graph:
	// A -> B (cost 1.0) -> D (cost 1.0)  [Path 1: A-B-D, total 2.0]
	// A -> C (cost 1.5) -> D (cost 1.0)  [Path 2: A-C-D, total 2.5]
	// A -> E (cost 3.0) -> D (cost 1.0)  [Path 3: A-E-D, total 4.0]
	g := &routingGraph{
		adj:     make(map[string][]routingEdge),
		edgeMap: make(map[string]routingEdge),
	}

	g.addEdge(routingEdge{source: "A", target: "B", relationType: "liked", weight: 5, cost: 1.0})
	g.addEdge(routingEdge{source: "B", target: "D", relationType: "liked", weight: 5, cost: 1.0})
	g.addEdge(routingEdge{source: "A", target: "C", relationType: "commented", weight: 2, cost: 1.5})
	g.addEdge(routingEdge{source: "C", target: "D", relationType: "liked", weight: 5, cost: 1.0})
	g.addEdge(routingEdge{source: "A", target: "E", relationType: "member_of", weight: 1, cost: 3.0})
	g.addEdge(routingEdge{source: "E", target: "D", relationType: "member_of", weight: 1, cost: 1.0})

	r := Router{}
	p0 := dijkstra(g, "A", "D", 4, nil, nil)
	if p0 == nil {
		t.Fatalf("expected path from A to D, got nil")
	}
	if len(p0.nodes) != 3 || p0.nodes[1] != "B" {
		t.Fatalf("expected shortest path A->B->D, got %v", p0.nodes)
	}

	kPaths := r.findKShortestPaths(g, "A", "D", 3, 4, "least_cost", nil)
	if len(kPaths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(kPaths))
	}
	if kPaths[0].nodes[1] != "B" || kPaths[1].nodes[1] != "C" || kPaths[2].nodes[1] != "E" {
		t.Fatalf("unexpected order of K-shortest paths: %v, %v, %v", kPaths[0].nodes, kPaths[1].nodes, kPaths[2].nodes)
	}

	// Test avoidance: avoid B
	avoidB := map[string]bool{"B": true}
	pAvoid := dijkstra(g, "A", "D", 4, avoidB, nil)
	if pAvoid == nil || pAvoid.nodes[1] != "C" {
		t.Fatalf("expected path avoiding B to be A->C->D, got %v", pAvoid)
	}
}

func TestRoutingGraphReplacesAdjacencyEdgeWhenCheaperEvidenceArrives(t *testing.T) {
	g := &routingGraph{
		adj:              make(map[string][]routingEdge),
		edgeMap:          make(map[string]routingEdge),
		truncationReason: make(map[string]struct{}),
	}
	g.addEdge(routingEdge{source: "A", target: "B", relationType: "member_of", cost: 6})
	g.addEdge(routingEdge{source: "A", target: "B", relationType: "member_of", cost: 1.2})

	if got := g.adj["A"][0].cost; got != 1.2 {
		t.Fatalf("adjacency retained stale edge cost %v", got)
	}
}

func TestRoutingPolicyRejectsConfiguredMaximumWithoutSilentFallback(t *testing.T) {
	router := Router{Policy: RoutingPolicy{
		DefaultMaxHops:  4,
		DefaultMaxPaths: 3,
		MaxHops:         6,
	}}
	_, err := router.PlanRoutes(t.Context(), RouteRequest{
		SourceQQ: "1",
		TargetQQ: "2",
		MaxHops:  7,
		MaxPaths: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds configured routing limit") {
		t.Fatalf("expected explicit policy error, got %v", err)
	}
}

func TestRouterRejectsParametersItCannotHonor(t *testing.T) {
	policy := RoutingPolicy{DefaultMaxHops: 4, DefaultMaxPaths: 3}
	waypointRouter := Router{Policy: policy}
	_, err := waypointRouter.PlanRoutes(t.Context(), RouteRequest{
		SourceQQ: "1", TargetQQ: "2", Waypoints: []string{"3"},
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to ignore") {
		t.Fatalf("expected explicit waypoint error, got %v", err)
	}

	now := time.Now()
	_, err = waypointRouter.PlanRoutes(t.Context(), RouteRequest{
		SourceQQ: "1", TargetQQ: "2", TimeStart: &now,
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to ignore") {
		t.Fatalf("expected explicit time range error, got %v", err)
	}
}

func TestReverseTraversalSummaryPreservesEvidenceDirection(t *testing.T) {
	_, summary := formatStepSummary(
		Node{Type: "person", Label: "SyntheticPerson"},
		Node{Type: "person", Label: "suG_05657D"},
		"liked", 1, time.Now(), time.Now(), true,
	)
	if !strings.Contains(summary, "suG_05657D → SyntheticPerson") {
		t.Fatalf("reverse traversal summary lost evidence direction: %s", summary)
	}
}
