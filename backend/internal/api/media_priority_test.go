package api

import (
	"testing"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

func TestGraphPersonDepthsCountsContextAsZeroCost(t *testing.T) {
	graph := analysis.Graph{
		Nodes: []analysis.Node{
			{Key: "person:target", Type: "person", Metadata: map[string]any{"qq": "10001"}},
			{Key: "content:post", Type: "content"},
			{Key: "person:first", Type: "person"},
			{Key: "person:direct", Type: "person"},
			{Key: "conversation:chat", Type: "conversation"},
			{Key: "person:second", Type: "person"},
		},
		Edges: []analysis.Edge{
			{Source: "person:target", Target: "content:post", FirstSeen: time.Now(), LastSeen: time.Now()},
			{Source: "person:first", Target: "content:post", FirstSeen: time.Now(), LastSeen: time.Now()},
			{Source: "person:target", Target: "person:direct", FirstSeen: time.Now(), LastSeen: time.Now()},
			{Source: "person:first", Target: "conversation:chat", FirstSeen: time.Now(), LastSeen: time.Now()},
			{Source: "person:second", Target: "conversation:chat", FirstSeen: time.Now(), LastSeen: time.Now()},
		},
	}
	depths := graphPersonDepths(graph, "10001")
	assertContains := func(depth int, id string) {
		t.Helper()
		for _, item := range depths[depth] {
			if item == id {
				return
			}
		}
		t.Fatalf("person %q not found at depth %d: %#v", id, depth, depths)
	}
	assertContains(0, "target")
	assertContains(1, "first")
	assertContains(1, "direct")
	assertContains(2, "second")
}

func TestGraphPersonDepthsRequiresTarget(t *testing.T) {
	graph := analysis.Graph{Nodes: []analysis.Node{{Key: "person:other", Type: "person", Metadata: map[string]any{"qq": "2"}}}}
	if depths := graphPersonDepths(graph, "1"); len(depths) != 0 {
		t.Fatalf("expected no depths without a target, got %#v", depths)
	}
}
