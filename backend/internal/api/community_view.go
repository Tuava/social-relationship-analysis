package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"

	"github.com/go-chi/chi/v5"
)

type communityMember struct {
	Key      string         `json:"key"`
	Type     string         `json:"type"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata"`
}

type communitySummary struct {
	ID                string            `json:"id"`
	Label             string            `json:"label"`
	NodeCount         int               `json:"node_count"`
	EdgeCount         int               `json:"edge_count"`
	InternalWeight    int               `json:"internal_weight"`
	ExternalWeight    int               `json:"external_weight"`
	PersonCount       int               `json:"person_count"`
	GroupCount        int               `json:"group_count"`
	ContentCount      int               `json:"content_count"`
	ConversationCount int               `json:"conversation_count"`
	MessageCount      int               `json:"message_count"`
	OtherCount        int               `json:"other_count"`
	Target            bool              `json:"target"`
	Members           []communityMember `json:"members"`
	MemberKeys        []string          `json:"member_keys"`
}

type detectedCommunity struct {
	communitySummary
	memberSet map[string]bool
}

// egoNetworkCommunities exposes a compact research view. It computes from the
// saved graph on demand, so community data never replaces the raw graph.
func (s *Server) egoNetworkCommunities(w http.ResponseWriter, r *http.Request) {
	nodes, edges, targetKey, err := s.loadStoredGraph(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if maxDistance := boundedDistanceParam(r.URL.Query().Get("max_distance"), 0, s.graphMaxDepth(r.Context())); maxDistance > 0 {
		distances := bfsDistances(nodes, edges, targetKey)
		included := make(map[string]bool, len(nodes))
		filteredNodes := make([]storedGraphNode, 0, len(nodes))
		for _, node := range nodes {
			if distance := distances[node.Key]; distance >= 0 && distance <= maxDistance {
				included[node.Key] = true
				filteredNodes = append(filteredNodes, node)
			}
		}
		filteredEdges := make([]storedGraphEdge, 0, len(edges))
		for _, edge := range edges {
			if included[edge.Source] && included[edge.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
		nodes, edges = filteredNodes, filteredEdges
	}
	communities := detectCommunities(nodes, edges, targetKey)
	collapsedNodes, collapsedEdges := collapseCommunities(communities, edges, targetKey)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"communities": communities,
		"nodes":       collapsedNodes,
		"edges":       collapsedEdges,
		"target_key":  targetKey,
		"total_nodes": len(nodes),
		"total_edges": len(edges),
	}})
}

func (s *Server) egoNetworkCommunity(w http.ResponseWriter, r *http.Request) {
	nodes, edges, targetKey, err := s.loadStoredGraph(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	communities := detectCommunities(nodes, edges, targetKey)
	communityID := chi.URLParam(r, "communityID")
	var selected *detectedCommunity
	for index := range communities {
		if communities[index].ID == communityID {
			selected = &communities[index]
			break
		}
	}
	if selected == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "community not found"})
		return
	}
	selectedNodes := make([]storedGraphNode, 0, selected.NodeCount)
	for _, node := range nodes {
		if selected.memberSet[node.Key] {
			selectedNodes = append(selectedNodes, node)
		}
	}
	selectedEdges := make([]storedGraphEdge, 0, selected.EdgeCount)
	for _, edge := range edges {
		if selected.memberSet[edge.Source] && selected.memberSet[edge.Target] {
			selectedEdges = append(selectedEdges, edge)
		}
	}
	limit := boundedGraphLimit(r.URL.Query().Get("limit"), DefaultGraphViewNodes, s.graphViewMaxNodes())
	keys := rankedGraphNodes(selectedNodes, selectedEdges, targetKey, r.URL.Query().Get("focus_node"), limit)
	resultNodes := make([]storedGraphNode, 0, len(keys))
	for _, node := range selectedNodes {
		if keys[node.Key] {
			resultNodes = append(resultNodes, node)
		}
	}
	resultEdges := make([]storedGraphEdge, 0, len(selectedEdges))
	for _, edge := range selectedEdges {
		if keys[edge.Source] && keys[edge.Target] {
			resultEdges = append(resultEdges, edge)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"community":      selected,
		"nodes":          resultNodes,
		"edges":          resultEdges,
		"target_key":     targetKey,
		"returned_nodes": len(resultNodes),
		"returned_edges": len(resultEdges),
		"total_nodes":    selected.NodeCount,
		"total_edges":    selected.EdgeCount,
		"partial":        len(resultNodes) < selected.NodeCount,
	}})
}

// detectCommunities is a deterministic weighted label-propagation pass. It is
// deliberately bounded: the saved graph remains the source of truth and a
// community result is always reproducible from it.
func detectCommunities(nodes []storedGraphNode, edges []storedGraphEdge, targetKey string) []detectedCommunity {
	if len(nodes) == 0 {
		return []detectedCommunity{}
	}
	keys := make([]string, 0, len(nodes))
	degree := make(map[string]int, len(nodes))
	adjacency := make(map[string][]communityNeighbor, len(nodes))
	for _, node := range nodes {
		keys = append(keys, node.Key)
	}
	for _, edge := range edges {
		weight := communityWeight(edge)
		adjacency[edge.Source] = append(adjacency[edge.Source], communityNeighbor{key: edge.Target, weight: weight})
		adjacency[edge.Target] = append(adjacency[edge.Target], communityNeighbor{key: edge.Source, weight: weight})
		degree[edge.Source] += edge.Weight
		degree[edge.Target] += edge.Weight
	}
	sort.Slice(keys, func(i, j int) bool {
		if degree[keys[i]] != degree[keys[j]] {
			return degree[keys[i]] > degree[keys[j]]
		}
		return keys[i] < keys[j]
	})
	labels := make(map[string]string, len(keys))
	for _, key := range keys {
		labels[key] = key
	}
	for iteration := 0; iteration < 24; iteration++ {
		changed := false
		for _, key := range keys {
			scores := map[string]float64{}
			for _, neighbor := range adjacency[key] {
				scores[labels[neighbor.key]] += neighbor.weight
			}
			best := labels[key]
			bestScore := 0.0
			for label, score := range scores {
				if score > bestScore || (score == bestScore && label < best) {
					best, bestScore = label, score
				}
			}
			if best != labels[key] {
				changed = true
			}
			// Update in place so a pair of equally weighted nodes does not
			// endlessly swap labels on every synchronous pass.
			labels[key] = best
		}
		if !changed {
			break
		}
	}

	groups := map[string][]string{}
	for _, key := range keys {
		groups[labels[key]] = append(groups[labels[key]], key)
	}
	ordered := make([][]string, 0, len(groups))
	for _, members := range groups {
		sort.Strings(members)
		ordered = append(ordered, members)
	}
	sort.Slice(ordered, func(i, j int) bool {
		containsTargetI := containsString(ordered[i], targetKey)
		containsTargetJ := containsString(ordered[j], targetKey)
		if containsTargetI != containsTargetJ {
			return containsTargetI
		}
		if len(ordered[i]) != len(ordered[j]) {
			return len(ordered[i]) > len(ordered[j])
		}
		return ordered[i][0] < ordered[j][0]
	})

	byKey := make(map[string]storedGraphNode, len(nodes))
	for _, node := range nodes {
		byKey[node.Key] = node
	}
	result := make([]detectedCommunity, 0, len(ordered))
	for index, members := range ordered {
		memberSet := make(map[string]bool, len(members))
		for _, key := range members {
			memberSet[key] = true
		}
		summary := communitySummary{ID: fmt.Sprintf("c-%03d", index+1), Label: fmt.Sprintf("社区 %02d", index+1), MemberKeys: append([]string(nil), members...), Target: containsString(members, targetKey)}
		for _, key := range members {
			node := byKey[key]
			summary.NodeCount++
			switch node.Type {
			case "person":
				summary.PersonCount++
			case "group":
				summary.GroupCount++
			case "content":
				summary.ContentCount++
			case "conversation":
				summary.ConversationCount++
			case "message":
				summary.MessageCount++
			default:
				summary.OtherCount++
			}
		}
		memberDegree := make(map[string]int, len(members))
		for _, edge := range edges {
			insideSource, insideTarget := memberSet[edge.Source], memberSet[edge.Target]
			if insideSource && insideTarget {
				summary.EdgeCount++
				summary.InternalWeight += edge.Weight
				memberDegree[edge.Source] += edge.Weight
				memberDegree[edge.Target] += edge.Weight
			} else if insideSource || insideTarget {
				summary.ExternalWeight += edge.Weight
			}
		}
		sort.Slice(members, func(i, j int) bool {
			if memberDegree[members[i]] != memberDegree[members[j]] {
				return memberDegree[members[i]] > memberDegree[members[j]]
			}
			return members[i] < members[j]
		})
		for _, key := range members {
			node := byKey[key]
			node.Label = cleanStoredGraphLabel(node)
			summary.Members = append(summary.Members, communityMember{Key: node.Key, Type: node.Type, Label: node.Label, Metadata: node.Metadata})
			if len(summary.Members) >= 16 {
				break
			}
		}
		summary.Label = communityDisplayLabel(index, summary.Members)
		result = append(result, detectedCommunity{communitySummary: summary, memberSet: memberSet})
	}
	return result
}

func communityDisplayLabel(index int, members []communityMember) string {
	labels := make([]string, 0, 2)
	seen := map[string]bool{}
	for _, preferredTypes := range [][]string{{"group", "person"}, {"conversation"}, {"content", "message"}} {
		for _, member := range members {
			if !containsString(preferredTypes, member.Type) {
				continue
			}
			label := domain.CleanDisplayText(member.Label)
			if label == "" || domain.IsOpaqueIdentifier(label) || seen[label] || label == "Person" || label == "Group" || label == "Conversation" || label == "Content" || label == "Message" {
				continue
			}
			runes := []rune(label)
			if len(runes) > 18 {
				label = string(runes[:18]) + "..."
			}
			seen[label] = true
			labels = append(labels, label)
			if len(labels) == 2 {
				return strings.Join(labels, " · ")
			}
		}
	}
	if len(labels) > 0 {
		return strings.Join(labels, " · ")
	}
	return fmt.Sprintf("社区 %02d", index+1)
}

type communityNeighbor struct {
	key    string
	weight float64
}

func communityWeight(edge storedGraphEdge) float64 {
	base := float64(maxInt(edge.Weight, 1))
	switch edge.RelationType {
	case "member_of":
		return base * 0.25
	case "published":
		return base * 0.75
	case "liked", "commented", "replied_to":
		return base * 2
	case "sent_message":
		return base * 3
	default:
		return base
	}
}

func collapseCommunities(communities []detectedCommunity, edges []storedGraphEdge, targetKey string) ([]storedGraphNode, []storedGraphEdge) {
	communityByNode := map[string]string{}
	nodes := make([]storedGraphNode, 0, len(communities))
	for _, community := range communities {
		for key := range community.memberSet {
			communityByNode[key] = community.ID
		}
		metadata := map[string]any{
			"community_id":       community.ID,
			"member_count":       community.NodeCount,
			"person_count":       community.PersonCount,
			"group_count":        community.GroupCount,
			"content_count":      community.ContentCount,
			"conversation_count": community.ConversationCount,
			"message_count":      community.MessageCount,
			"other_count":        community.OtherCount,
			"internal_weight":    community.InternalWeight,
			"external_weight":    community.ExternalWeight,
			"is_target":          community.Target,
		}
		nodes = append(nodes, storedGraphNode{Key: "community:" + community.ID, Type: "community", Label: community.Label, Metadata: metadata})
	}
	type aggregate struct {
		weight   int
		events   int
		first    any
		last     any
		evidence []string
	}
	aggregates := map[string]*aggregate{}
	for _, edge := range edges {
		source, sourceOK := communityByNode[edge.Source]
		target, targetOK := communityByNode[edge.Target]
		if !sourceOK || !targetOK || source == target {
			continue
		}
		if source > target {
			source, target = target, source
		}
		key := source + "\x00" + target
		item := aggregates[key]
		if item == nil {
			item = &aggregate{first: edge.FirstSeen, last: edge.LastSeen}
			aggregates[key] = item
		}
		item.weight += edge.Weight
		item.events += edge.EventCount
		item.evidence = mergeEvidenceLimited(item.evidence, edge.EvidenceIDs, 100)
	}
	collapsedEdges := make([]storedGraphEdge, 0, len(aggregates))
	for key, aggregate := range aggregates {
		parts := strings.SplitN(key, "\x00", 2)
		collapsedEdges = append(collapsedEdges, storedGraphEdge{Source: "community:" + parts[0], Target: "community:" + parts[1], RelationType: "community_link", Weight: aggregate.weight, EventCount: aggregate.events, EvidenceIDs: aggregate.evidence, FirstSeen: aggregate.first, LastSeen: aggregate.last})
	}
	sort.Slice(collapsedEdges, func(i, j int) bool {
		return collapsedEdges[i].Source+collapsedEdges[i].Target < collapsedEdges[j].Source+collapsedEdges[j].Target
	})
	for index := range nodes {
		if communityByNode[targetKey] == strings.TrimPrefix(nodes[index].Key, "community:") {
			if nodes[index].Metadata == nil {
				nodes[index].Metadata = map[string]any{}
			}
			nodes[index].Metadata["is_target"] = true
		}
	}
	return nodes, collapsedEdges
}

func mergeEvidenceLimited(existing, incoming []string, limit int) []string {
	seen := make(map[string]bool, len(existing)+len(incoming))
	result := make([]string, 0, minInt(len(existing)+len(incoming), limit))
	for _, id := range append(existing, incoming...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
