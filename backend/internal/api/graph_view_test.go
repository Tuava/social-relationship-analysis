package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestBFSDistances(t *testing.T) {
	nodes := []storedGraphNode{{Key: "person:a"}, {Key: "person:b"}, {Key: "group:c"}, {Key: "person:unreachable"}}
	edges := []storedGraphEdge{{Source: "person:a", Target: "person:b"}, {Source: "person:b", Target: "group:c"}}

	distance := bfsDistances(nodes, edges, "person:a")
	for key, want := range map[string]int{"person:a": 0, "person:b": 1, "group:c": 2, "person:unreachable": -1} {
		if got := distance[key]; got != want {
			t.Fatalf("distance[%q] = %d, want %d", key, got, want)
		}
	}
}

func TestWriteGraphSubsetPreservesStoredTotals(t *testing.T) {
	nodes := []storedGraphNode{{Key: "person:a"}, {Key: "person:b"}}
	edges := []storedGraphEdge{{Source: "person:a", Target: "person:b"}}
	recorder := httptest.NewRecorder()

	writeGraphSubset(recorder, nodes, edges, map[string]bool{"person:a": true, "person:b": true}, "person:a", 873, 2499)

	var response struct {
		Data struct {
			TotalNodes  int  `json:"total_nodes"`
			TotalEdges  int  `json:"total_edges"`
			ScopeNodes  int  `json:"scope_nodes"`
			ScopeEdges  int  `json:"scope_edges"`
			Partial     bool `json:"partial"`
			PartialData bool `json:"partial_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.TotalNodes != 873 || response.Data.TotalEdges != 2499 {
		t.Fatalf("stored totals = %d/%d, want 873/2499", response.Data.TotalNodes, response.Data.TotalEdges)
	}
	if response.Data.ScopeNodes != 2 || response.Data.ScopeEdges != 1 || response.Data.Partial || response.Data.PartialData {
		t.Fatalf("scope = %d/%d partial=%v partial_data=%v, want 2/1 false", response.Data.ScopeNodes, response.Data.ScopeEdges, response.Data.Partial, response.Data.PartialData)
	}
}

func TestWriteGraphSubsetMarksServerLimitAsPartial(t *testing.T) {
	nodes := []storedGraphNode{{Key: "person:a"}, {Key: "person:b"}, {Key: "person:c"}}
	edges := []storedGraphEdge{{Source: "person:a", Target: "person:b"}, {Source: "person:b", Target: "person:c"}}
	recorder := httptest.NewRecorder()

	writeGraphSubset(recorder, nodes, edges, map[string]bool{"person:a": true, "person:b": true}, "person:a", 3, 2)

	var response struct {
		Data struct {
			Partial     bool `json:"partial"`
			PartialData bool `json:"partial_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Data.Partial || !response.Data.PartialData {
		t.Fatalf("partial=%v partial_data=%v, want both true", response.Data.Partial, response.Data.PartialData)
	}
}

func TestCanonicalizeStoredGroupNodesFoldsConversationIntoGroup(t *testing.T) {
	nodes := []storedGraphNode{
		{Key: "person:a", Type: "person", Label: "A"},
		{Key: "group:g", Type: "group", Label: "技术群", Metadata: map[string]any{"group_id": "123"}},
		{Key: "conversation:c", Type: "conversation", Label: "技术群", Metadata: map[string]any{"conversation_type": "group", "group_entity_id": "g", "group_id": "123"}},
	}
	edges := []storedGraphEdge{
		{Source: "person:a", Target: "group:g", RelationType: "sent_message", Weight: 2, EventCount: 2, EvidenceIDs: []string{"e1"}},
		{Source: "person:a", Target: "conversation:c", RelationType: "sent_message", Weight: 3, EventCount: 3, EvidenceIDs: []string{"e2"}},
	}

	gotNodes, gotEdges, _ := canonicalizeStoredGroupNodes(nodes, edges, "person:a")
	if len(gotNodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(gotNodes))
	}
	if len(gotEdges) != 1 {
		t.Fatalf("edges = %d, want 1", len(gotEdges))
	}
	if gotEdges[0].Target != "group:g" || gotEdges[0].Weight != 5 || gotEdges[0].EventCount != 5 {
		t.Fatalf("canonical edge = %+v, want group:g with merged counts", gotEdges[0])
	}
	if len(gotEdges[0].EvidenceIDs) != 2 {
		t.Fatalf("evidence = %v, want both observations", gotEdges[0].EvidenceIDs)
	}
}

func TestDetectCommunitiesSeparatesWeightedClusters(t *testing.T) {
	nodes := []storedGraphNode{
		{Key: "person:a", Type: "person", Label: "A"},
		{Key: "person:b", Type: "person", Label: "B"},
		{Key: "person:c", Type: "person", Label: "C"},
		{Key: "group:d", Type: "group", Label: "D"},
	}
	edges := []storedGraphEdge{
		{Source: "person:a", Target: "person:b", RelationType: "sent_message", Weight: 5},
		{Source: "person:c", Target: "group:d", RelationType: "member_of", Weight: 5},
	}
	communities := detectCommunities(nodes, edges, "person:a")
	if len(communities) != 2 {
		t.Fatalf("communities = %d, want 2", len(communities))
	}
	if !communities[0].Target || communities[0].NodeCount != 2 {
		t.Fatalf("target community = %+v, want target cluster with 2 nodes", communities[0].communitySummary)
	}
	collapsedNodes, collapsedEdges := collapseCommunities(communities, edges, "person:a")
	if len(collapsedNodes) != 2 || len(collapsedEdges) != 0 {
		t.Fatalf("collapsed graph = %d nodes/%d edges, want 2/0", len(collapsedNodes), len(collapsedEdges))
	}
}

func TestCollapseCommunitiesRetainsCrossCommunityEvidence(t *testing.T) {
	nodes := []storedGraphNode{
		{Key: "person:a", Type: "person", Label: "A"},
		{Key: "person:b", Type: "person", Label: "B"},
		{Key: "person:c", Type: "person", Label: "C"},
		{Key: "person:d", Type: "person", Label: "D"},
	}
	edges := []storedGraphEdge{
		{Source: "person:a", Target: "person:b", RelationType: "sent_message", Weight: 4, EventCount: 4, EvidenceIDs: []string{"e1"}},
		{Source: "person:c", Target: "person:d", RelationType: "sent_message", Weight: 4, EventCount: 4, EvidenceIDs: []string{"e2"}},
		{Source: "person:b", Target: "person:c", RelationType: "member_of", Weight: 1, EventCount: 1, EvidenceIDs: []string{"e3"}},
	}
	communities := detectCommunities(nodes, edges, "person:a")
	collapsedNodes, collapsedEdges := collapseCommunities(communities, edges, "person:a")
	if len(collapsedNodes) == 0 || len(collapsedEdges) == 0 {
		t.Fatalf("expected cross-community edge, got %d nodes/%d edges", len(collapsedNodes), len(collapsedEdges))
	}
	if len(collapsedEdges[0].EvidenceIDs) == 0 {
		t.Fatal("collapsed edge lost evidence")
	}
}

func TestDetectCommunitiesReportsEveryNodeTypeAndUsesMeaningfulLabel(t *testing.T) {
	nodes := []storedGraphNode{
		{Key: "person:a", Type: "person", Label: "Alice"},
		{Key: "conversation:b", Type: "conversation", Label: "03e9a8e9-8796-4b78-a17f-17496c5098dc"},
		{Key: "message:c", Type: "message", Label: "Hello"},
		{Key: "group:d", Type: "group", Label: "<$\u00ff\u0100\x11\x10>Research Group"},
		{Key: "unknown:e", Type: "unknown", Label: "Other"},
	}
	edges := []storedGraphEdge{
		{Source: "person:a", Target: "conversation:b", RelationType: "sent_message", Weight: 2},
		{Source: "conversation:b", Target: "message:c", RelationType: "replied_to", Weight: 2},
		{Source: "message:c", Target: "group:d", RelationType: "member_of", Weight: 2},
		{Source: "group:d", Target: "unknown:e", RelationType: "member_of", Weight: 2},
	}
	communities := detectCommunities(nodes, edges, "person:a")
	if len(communities) == 0 {
		t.Fatal("expected at least one community")
	}
	var persons, groups, conversations, messages, others int
	meaningfulLabel := false
	for _, community := range communities {
		persons += community.PersonCount
		groups += community.GroupCount
		conversations += community.ConversationCount
		messages += community.MessageCount
		others += community.OtherCount
		if community.Label != "社区 01" && community.Label != "03e9a8e9-8796-4b78-a17f-17496c5098dc" {
			meaningfulLabel = true
		}
	}
	if persons != 1 || groups != 1 || conversations != 1 || messages != 1 || others != 1 {
		t.Fatalf("unexpected aggregate type counts: %d/%d/%d/%d/%d", persons, groups, conversations, messages, others)
	}
	if !meaningfulLabel {
		t.Fatal("expected a meaningful community label")
	}
}
