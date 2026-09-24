package api

import (
	"net/http"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

const (
	MinPageSize             = 1
	DefaultPageSize         = 50
	MaxPageSize             = 500
	MinGraphViewNodes       = 50
	DefaultGraphViewNodes   = 300
	DefaultGraphExpandNodes = 100
)

// graphRenderPresets derives the convenient quick choices from the configured
// server ceiling. The client receives these values and never needs to embed a
// second, potentially stale notion of the maximum.
func graphRenderPresets(max int) []int {
	if max <= 0 {
		return nil
	}
	values := []int{max / 10, max / 4, max / 2, max}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < MinGraphViewNodes {
			value = MinGraphViewNodes
		}
		if value > max {
			value = max
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Server) systemCapabilities(w http.ResponseWriter, r *http.Request) {
	maxDepth := s.graphMaxDepth(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"graph": map[string]any{
			"depth": map[string]any{
				"min":     analysis.MinDepth,
				"max":     optionalLimit(maxDepth),
				"default": analysis.DefaultBuildDepth,
				"source":  "server_config",
			},
			"build": map[string]any{
				"max_nodes":         optionalLimit(s.GraphBuildMaxNodes),
				"max_edges":         optionalLimit(s.GraphBuildMaxEdges),
				"max_events":        optionalLimit(s.GraphBuildMaxEvents),
				"default_unlimited": true,
				"source":            "server_config",
			},
			"view": map[string]any{
				"min_nodes":        MinGraphViewNodes,
				"default_nodes":    DefaultGraphViewNodes,
				"max_nodes":        optionalLimit(s.graphViewMaxNodes()),
				"min_distance":     analysis.MinDepth,
				"max_distance":     optionalLimit(maxDepth),
				"expand_default":   DefaultGraphExpandNodes,
				"expand_max_nodes": optionalLimit(s.graphExpandMaxNodes()),
				"render_presets":   graphRenderPresets(s.graphViewMaxNodes()),
				"source":           "server_config",
			},
		},
		"routing": map[string]any{
			"defaults": map[string]any{
				"max_hops":              s.RoutingPolicy.DefaultMaxHops,
				"max_paths":             s.RoutingPolicy.DefaultMaxPaths,
				"large_group_threshold": s.RoutingPolicy.DefaultLargeGroupThreshold,
			},
			"limits": map[string]any{
				"max_hops":               optionalLimit(s.RoutingPolicy.MaxHops),
				"max_paths":              optionalLimit(s.RoutingPolicy.MaxPaths),
				"max_graph_nodes":        optionalLimit(s.RoutingPolicy.MaxGraphNodes),
				"max_frontier_neighbors": optionalLimit(s.RoutingPolicy.MaxFrontierNeighbors),
				"max_group_co_members":   optionalLimit(s.RoutingPolicy.MaxGroupCoMembers),
				"max_group_memberships":  optionalLimit(s.RoutingPolicy.MaxGroupMemberships),
			},
			"default_unlimited": true,
			"source":            "server_config",
		},
		"lists": map[string]any{
			"min_page_size":     MinPageSize,
			"default_page_size": DefaultPageSize,
			"max_page_size":     MaxPageSize,
			"page_sizes":        []int{25, 50, 100, 250, 500},
		},
	}})
}
