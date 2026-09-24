package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

func TestSystemCapabilitiesExposeServerLimits(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Server{}).systemCapabilities(recorder, httptest.NewRequest("GET", "/api/v1/system/capabilities", nil))

	var response struct {
		Data struct {
			Graph struct {
				Depth struct {
					Min int `json:"min"`
					Max any `json:"max"`
				} `json:"depth"`
				Build struct {
					MaxNodes any `json:"max_nodes"`
				} `json:"build"`
				View struct {
					MaxNodes    int `json:"max_nodes"`
					MaxDistance any `json:"max_distance"`
				} `json:"view"`
			} `json:"graph"`
			Lists struct {
				DefaultPageSize int `json:"default_page_size"`
				MaxPageSize     int `json:"max_page_size"`
			} `json:"lists"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Graph.Depth.Min <= 0 {
		t.Fatalf("invalid depth limits: %+v", response.Data.Graph.Depth)
	}
	if response.Data.Graph.Build.MaxNodes != nil {
		t.Fatalf("default build node limit = %v, want unlimited nil", response.Data.Graph.Build.MaxNodes)
	}
	if response.Data.Graph.View.MaxDistance != nil || response.Data.Lists.DefaultPageSize <= 0 || response.Data.Lists.MaxPageSize < response.Data.Lists.DefaultPageSize {
		t.Fatalf("invalid list or distance limits: %+v", response.Data)
	}
}

func TestSystemCapabilitiesUseResolvedServerConfiguration(t *testing.T) {
	recorder := httptest.NewRecorder()
	server := &Server{
		GraphMaxDepth:       37,
		GraphBuildMaxNodes:  0,
		GraphBuildMaxEdges:  24000,
		GraphBuildMaxEvents: 0,
		GraphViewMaxNodes:   12000,
		GraphExpandMaxNodes: 700,
		RoutingPolicy: analysis.RoutingPolicy{
			DefaultMaxHops:       6,
			DefaultMaxPaths:      4,
			MaxGraphNodes:        9000,
			MaxFrontierNeighbors: 1200,
		},
	}
	server.systemCapabilities(recorder, httptest.NewRequest("GET", "/api/v1/system/capabilities", nil))

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	data := response["data"].(map[string]any)
	graph := data["graph"].(map[string]any)
	depth := graph["depth"].(map[string]any)
	view := graph["view"].(map[string]any)
	build := graph["build"].(map[string]any)
	routing := data["routing"].(map[string]any)
	routingDefaults := routing["defaults"].(map[string]any)
	routingLimits := routing["limits"].(map[string]any)
	if depth["max"] != float64(37) || view["max_nodes"] != float64(12000) || view["expand_max_nodes"] != float64(700) {
		t.Fatalf("capabilities did not use server config: %+v", graph)
	}
	if build["max_nodes"] != nil || build["max_edges"] != float64(24000) || build["max_events"] != nil {
		t.Fatalf("build limits = %+v", build)
	}
	if routingDefaults["max_hops"] != float64(6) || routingDefaults["max_paths"] != float64(4) {
		t.Fatalf("routing defaults = %+v", routingDefaults)
	}
	if routingLimits["max_graph_nodes"] != float64(9000) || routingLimits["max_frontier_neighbors"] != float64(1200) || routingLimits["max_group_memberships"] != nil {
		t.Fatalf("routing limits = %+v", routingLimits)
	}
}

func TestGraphLimitsReportReachableDistanceLevels(t *testing.T) {
	nodes := []storedGraphNode{{Key: "person:a"}, {Key: "person:b"}, {Key: "group:c"}, {Key: "person:x"}}
	edges := []storedGraphEdge{{Source: "person:a", Target: "person:b"}, {Source: "person:b", Target: "group:c"}}
	limits := graphLimitsFor(nodes, edges, bfsDistances(nodes, edges, "person:a"), analysis.DefaultBuildDepth, 5000)
	if limits.AvailableNodes != 4 || limits.AvailableEdges != 2 || limits.AvailableMaxDistance != 2 {
		t.Fatalf("limits = %+v", limits)
	}
	if len(limits.DistanceLevels) != 3 || limits.DistanceLevels[0].Nodes != 1 || limits.DistanceLevels[2].Nodes != 1 {
		t.Fatalf("distance levels = %+v", limits.DistanceLevels)
	}
}
