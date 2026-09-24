package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"

	"github.com/go-chi/chi/v5"
)

type storedGraphNode struct {
	Key      string         `json:"key"`
	Type     string         `json:"type"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata"`
}

type storedGraphEdge struct {
	Source       string   `json:"source"`
	Target       string   `json:"target"`
	RelationType string   `json:"relation_type"`
	Weight       int      `json:"weight"`
	EventCount   int      `json:"event_count"`
	EvidenceIDs  []string `json:"evidence_ids"`
	FirstSeen    any      `json:"first_seen"`
	LastSeen     any      `json:"last_seen"`
}

type graphDistanceLevel struct {
	Distance int `json:"distance"`
	Nodes    int `json:"nodes"`
}

type graphLimits struct {
	AvailableNodes       int                  `json:"available_nodes"`
	AvailableEdges       int                  `json:"available_edges"`
	AvailableMaxDistance int                  `json:"available_max_distance"`
	DistanceLevels       []graphDistanceLevel `json:"distance_levels"`
	ServerMaxNodes       *int                 `json:"server_max_nodes"`
	ServerMaxDistance    *int                 `json:"server_max_distance"`
}

func (s *Server) egoNetworkView(w http.ResponseWriter, r *http.Request) {
	limit := boundedGraphLimit(r.URL.Query().Get("limit"), DefaultGraphViewNodes, s.graphViewMaxNodes())
	maxDistance := boundedDistanceParam(r.URL.Query().Get("max_distance"), 0, s.graphMaxDepth(r.Context()))
	nodes, edges, targetKey, err := s.loadStoredGraph(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	totalNodes, totalEdges := len(nodes), len(edges)
	distances := bfsDistances(nodes, edges, targetKey)
	limits := graphLimitsFor(nodes, edges, distances, s.graphMaxDepth(r.Context()), s.graphViewMaxNodes())
	for i := range nodes {
		if nodes[i].Metadata == nil {
			nodes[i].Metadata = map[string]any{}
		}
		nodes[i].Metadata["distance"] = distances[nodes[i].Key]
	}
	if maxDistance > 0 {
		filtered := make([]storedGraphNode, 0, len(nodes))
		included := make(map[string]bool, len(nodes))
		for _, node := range nodes {
			if d := distances[node.Key]; d >= 0 && d <= maxDistance {
				filtered = append(filtered, node)
				included[node.Key] = true
			}
		}
		nodes = filtered
		filteredEdges := make([]storedGraphEdge, 0, len(edges))
		for _, edge := range edges {
			if included[edge.Source] && included[edge.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
		edges = filteredEdges
	}
	selected := rankedGraphNodes(nodes, edges, targetKey, r.URL.Query().Get("focus_node"), limit)
	writeGraphSubsetWithLimits(w, nodes, edges, selected, targetKey, totalNodes, totalEdges, limits)
}

func (s *Server) expandEgoNetworkNode(w http.ResponseWriter, r *http.Request) {
	nodeKey := r.URL.Query().Get("node_key")
	if nodeKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_key is required"})
		return
	}
	limit := boundedGraphLimit(r.URL.Query().Get("limit"), DefaultGraphExpandNodes, s.graphExpandMaxNodes())
	nodes, edges, targetKey, err := s.loadStoredGraph(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	distances := bfsDistances(nodes, edges, targetKey)
	limits := graphLimitsFor(nodes, edges, distances, s.graphMaxDepth(r.Context()), s.graphViewMaxNodes())
	for i := range nodes {
		if nodes[i].Metadata == nil {
			nodes[i].Metadata = map[string]any{}
		}
		nodes[i].Metadata["distance"] = distances[nodes[i].Key]
	}
	weighted := map[string]int{nodeKey: 1 << 30}
	for _, edge := range edges {
		if edge.Source == nodeKey {
			weighted[edge.Target] += edge.Weight
		} else if edge.Target == nodeKey {
			weighted[edge.Source] += edge.Weight
		}
	}
	keys := make([]string, 0, len(weighted))
	for key := range weighted {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return weighted[keys[i]] > weighted[keys[j]] })
	if len(keys) > limit+1 {
		keys = keys[:limit+1]
	}
	selected := make(map[string]bool, len(keys))
	for _, key := range keys {
		selected[key] = true
	}
	writeGraphSubsetWithLimits(w, nodes, edges, selected, targetKey, len(nodes), len(edges), limits)
}

func bfsDistances(nodes []storedGraphNode, edges []storedGraphEdge, targetKey string) map[string]int {
	dist := map[string]int{}
	for _, node := range nodes {
		dist[node.Key] = -1
	}
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.Source] = append(adj[edge.Source], edge.Target)
		adj[edge.Target] = append(adj[edge.Target], edge.Source)
	}
	if targetKey != "" {
		dist[targetKey] = 0
		queue := []string{targetKey}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, nb := range adj[cur] {
				if dist[nb] == -1 {
					dist[nb] = dist[cur] + 1
					queue = append(queue, nb)
				}
			}
		}
	}
	return dist
}

func (s *Server) loadStoredGraph(r *http.Request, networkID string) ([]storedGraphNode, []storedGraphEdge, string, error) {
	var targetQQ string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT target_qq FROM ego_networks WHERE id=$1`, networkID).Scan(&targetQQ); err != nil {
		return nil, nil, "", err
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT node_key,node_type,label,metadata FROM ego_network_nodes WHERE network_id=$1`, networkID)
	if err != nil {
		return nil, nil, "", err
	}
	nodes := []storedGraphNode{}
	targetKey := ""
	for rows.Next() {
		var node storedGraphNode
		var raw []byte
		if err := rows.Scan(&node.Key, &node.Type, &node.Label, &raw); err != nil {
			rows.Close()
			return nil, nil, "", err
		}
		_ = json.Unmarshal(raw, &node.Metadata)
		node.Label = cleanStoredGraphLabel(node)
		if node.Type == "person" && fmt.Sprint(node.Metadata["qq"]) == targetQQ {
			targetKey = node.Key
		}
		nodes = append(nodes, node)
	}
	rows.Close()
	if err := s.hydrateStoredGraphEntities(r.Context(), nodes); err != nil {
		return nil, nil, "", err
	}
	rows, err = s.Repo.DB.Query(r.Context(), `SELECT source_key,target_key,relation_type,weight,event_count,evidence_ids,first_seen,last_seen
		FROM ego_network_edges WHERE network_id=$1`, networkID)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()
	edges := []storedGraphEdge{}
	for rows.Next() {
		var edge storedGraphEdge
		if err := rows.Scan(&edge.Source, &edge.Target, &edge.RelationType, &edge.Weight, &edge.EventCount, &edge.EvidenceIDs, &edge.FirstSeen, &edge.LastSeen); err != nil {
			return nil, nil, "", err
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}
	nodes, edges, targetKey = canonicalizeStoredGroupNodes(nodes, edges, targetKey)
	if err := s.hydrateStoredGraphAvatars(r.Context(), nodes); err != nil {
		return nil, nil, "", err
	}
	return nodes, edges, targetKey, nil
}

func stringMetadata(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	val, ok := metadata[key]
	if !ok || val == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(val))
	if s == "<nil>" || s == "" {
		return ""
	}
	return s
}

func isValidUUID(u string) bool {
	if len(u) != 36 {
		return false
	}
	for i, r := range u {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

// Group messages point at a conversation row while membership events point at
// the group row. They describe one product-level entity and must not render as
// two nodes. Old saved graphs are folded on read; new builds already emit the
// canonical group key.
func canonicalizeStoredGroupNodes(nodes []storedGraphNode, edges []storedGraphEdge, targetKey string) ([]storedGraphNode, []storedGraphEdge, string) {
	aliases := map[string]string{}
	for _, node := range nodes {
		if node.Type != "conversation" || stringMetadata(node.Metadata, "conversation_type") != "group" {
			continue
		}
		groupEntityID := stringMetadata(node.Metadata, "group_entity_id")
		if groupEntityID != "" && !strings.Contains(groupEntityID, ":") {
			aliases[node.Key] = "group:" + groupEntityID
		}
	}

	canonicalNodes := make([]storedGraphNode, 0, len(nodes))
	nodeIndexes := map[string]int{}
	for _, node := range nodes {
		if canonical, ok := aliases[node.Key]; ok {
			node.Key = canonical
			node.Type = "group"
			if groupID := stringMetadata(node.Metadata, "group_id"); groupID != "" {
				node.Metadata["group_id"] = groupID
			}
		}
		if index, exists := nodeIndexes[node.Key]; exists {
			existing := &canonicalNodes[index]
			existing.Metadata = mergeGraphMetadata(existing.Metadata, node.Metadata)
			if (existing.Label == "" || existing.Label == "Group" || domain.IsOpaqueIdentifier(existing.Label)) && node.Label != "" {
				existing.Label = node.Label
			}
			continue
		}
		nodeIndexes[node.Key] = len(canonicalNodes)
		canonicalNodes = append(canonicalNodes, node)
	}

	canonicalEdges := make([]storedGraphEdge, 0, len(edges))
	edgeIndexes := map[string]int{}
	for _, edge := range edges {
		if canonical, ok := aliases[edge.Source]; ok {
			edge.Source = canonical
		}
		if canonical, ok := aliases[edge.Target]; ok {
			edge.Target = canonical
		}
		if edge.Source == edge.Target {
			continue
		}
		key := edge.Source + "\x00" + edge.Target + "\x00" + edge.RelationType
		if index, exists := edgeIndexes[key]; exists {
			existing := &canonicalEdges[index]
			existing.Weight += edge.Weight
			existing.EventCount += edge.EventCount
			existing.EvidenceIDs = mergeStoredEvidence(existing.EvidenceIDs, edge.EvidenceIDs)
			continue
		}
		edgeIndexes[key] = len(canonicalEdges)
		canonicalEdges = append(canonicalEdges, edge)
	}
	if canonical, ok := aliases[targetKey]; ok {
		targetKey = canonical
	}
	return canonicalNodes, canonicalEdges, targetKey
}

func mergeStoredEvidence(existing, incoming []string) []string {
	seen := make(map[string]bool, len(existing)+len(incoming))
	for _, id := range existing {
		seen[id] = true
	}
	for _, id := range incoming {
		if id != "" && !seen[id] {
			existing = append(existing, id)
			seen[id] = true
		}
	}
	return existing
}

func cleanStoredGraphLabel(node storedGraphNode) string {
	label := domain.CleanDisplayText(node.Label)
	if domain.IsOpaqueIdentifier(label) {
		label = ""
	}
	if label != "" {
		return label
	}
	metadata := node.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	switch node.Type {
	case "person":
		if qq := strings.TrimSpace(fmt.Sprint(metadata["qq"])); qq != "" {
			return qq
		}
		return "Person"
	case "group":
		if groupID := strings.TrimSpace(fmt.Sprint(metadata["group_id"])); groupID != "" {
			return groupID
		}
		return "Group"
	case "conversation":
		return "Conversation"
	case "content":
		return "Content"
	case "message":
		return "Message"
	default:
		return "Node"
	}
}

// Older saved graphs classified reply targets as conversations. Hydrate those
// nodes from their real table so existing research drafts gain accurate types
// and labels without rewriting the stored evidence graph.
func (s *Server) hydrateStoredGraphEntities(ctx context.Context, nodes []storedGraphNode) error {
	indexes := map[string][]int{}
	ids := make([]string, 0)
	for index, node := range nodes {
		if node.Type != "conversation" {
			continue
		}
		_, id, ok := strings.Cut(node.Key, ":")
		if !ok || !isValidUUID(id) {
			continue
		}
		if len(indexes[id]) == 0 {
			ids = append(ids, id)
		}
		indexes[id] = append(indexes[id], index)
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := s.Repo.DB.Query(ctx, `SELECT c.id::text,c.conversation_type,c.platform_conversation_id,
		COALESCE(NULLIF(c.name,''),g.group_name,''),g.id::text
		FROM conversations c
		LEFT JOIN groups g ON g.platform='qq' AND g.platform_group_id=c.platform_conversation_id
		WHERE c.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, kind, platformID, name string
		var groupEntityID *string
		if err := rows.Scan(&id, &kind, &platformID, &name, &groupEntityID); err != nil {
			rows.Close()
			return err
		}
		for _, index := range indexes[id] {
			metadata := map[string]any{"conversation_type": kind, "platform_id": platformID}
			if kind == "group" {
				metadata["group_id"] = platformID
				if groupEntityID != nil && isValidUUID(*groupEntityID) {
					metadata["group_entity_id"] = *groupEntityID
				}
			}
			nodes[index].Metadata = mergeGraphMetadata(nodes[index].Metadata, metadata)
			if cleaned := domain.CleanDisplayText(name); cleaned != "" {
				nodes[index].Label = cleaned
			} else if platformID != "" {
				nodes[index].Label = kind + ":" + platformID
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = s.Repo.DB.Query(ctx, `SELECT id::text,body,platform_content_id,context_type,published_at,
		COALESCE((SELECT pi.platform_user_id FROM person_identifiers pi WHERE pi.person_id=contents.author_id AND pi.platform='qq' LIMIT 1),'')
		FROM contents WHERE id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, body, platformID, contextType, authorQQ string
		var publishedAt any
		if err := rows.Scan(&id, &body, &platformID, &contextType, &publishedAt, &authorQQ); err != nil {
			rows.Close()
			return err
		}
		for _, index := range indexes[id] {
			nodes[index].Type = "content"
			nodes[index].Label = compactGraphLabel(body, "Content")
			nodes[index].Metadata = mergeGraphMetadata(nodes[index].Metadata, map[string]any{"content_id": id, "platform_id": platformID, "context_type": contextType, "published_at": publishedAt, "author_qq": authorQQ})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = s.Repo.DB.Query(ctx, `SELECT id::text,raw_text,COALESCE(source_message_id,''),sent_at,conversation_id::text FROM messages WHERE id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, text, sourceID string
		var sentAt any
		var conversationID *string
		if err := rows.Scan(&id, &text, &sourceID, &sentAt, &conversationID); err != nil {
			return err
		}
		for _, index := range indexes[id] {
			nodes[index].Type = "message"
			nodes[index].Label = compactGraphLabel(text, "Message")
			nodes[index].Metadata = mergeGraphMetadata(nodes[index].Metadata, map[string]any{"message_id": sourceID, "sent_at": sentAt, "conversation_id": conversationID})
		}
	}
	return rows.Err()
}

func compactGraphLabel(value, fallback string) string {
	label := domain.CleanDisplayText(value)
	runes := []rune(label)
	if len(runes) > 36 {
		label = string(runes[:36]) + "..."
	}
	if label == "" {
		return fallback
	}
	return label
}

func mergeGraphMetadata(existing, incoming map[string]any) map[string]any {
	if existing == nil {
		existing = map[string]any{}
	}
	for key, value := range incoming {
		existing[key] = value
	}
	return existing
}

// Saved graph nodes can outlive the media download that produced them. Refresh
// the avatar pointer when a view is read so an old research snapshot benefits
// from newly archived assets without requiring a new graph build.
func (s *Server) hydrateStoredGraphAvatars(ctx context.Context, nodes []storedGraphNode) error {
	personIDs := make([]string, 0)
	groupIDs := make([]string, 0)
	seenPersons := map[string]bool{}
	seenGroups := map[string]bool{}
	for _, node := range nodes {
		_, id, ok := strings.Cut(node.Key, ":")
		if !ok || !isValidUUID(id) {
			continue
		}
		switch node.Type {
		case "person":
			if !seenPersons[id] {
				seenPersons[id] = true
				personIDs = append(personIDs, id)
			}
		case "group":
			if !seenGroups[id] {
				seenGroups[id] = true
				groupIDs = append(groupIDs, id)
			}
		case "conversation":
			if stringMetadata(node.Metadata, "conversation_type") == "group" {
				groupEntityID := stringMetadata(node.Metadata, "group_entity_id")
				if isValidUUID(groupEntityID) && !seenGroups[groupEntityID] {
					seenGroups[groupEntityID] = true
					groupIDs = append(groupIDs, groupEntityID)
				}
			}
		}
	}
	personAssets := map[string]string{}
	if len(personIDs) > 0 {
		rows, err := s.Repo.DB.Query(ctx, `SELECT p.id::text,COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr
			 WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL
			 ORDER BY mr.completed_at DESC LIMIT 1),
			(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),'')
			FROM persons p WHERE p.id=ANY($1::uuid[])`, personIDs)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, avatar string
			if err := rows.Scan(&id, &avatar); err != nil {
				rows.Close()
				return err
			}
			personAssets[id] = avatar
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	groupPlatforms := map[string]string{}
	groupNames := map[string]string{}
	groupAssets := map[string]string{}
	if len(groupIDs) > 0 {
		rows, err := s.Repo.DB.Query(ctx, `SELECT g.id::text,COALESCE(g.platform_group_id,''),COALESCE(g.group_name,''),COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr
			 WHERE mr.group_id=g.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL
			 ORDER BY mr.completed_at DESC LIMIT 1),'')
			FROM groups g WHERE g.id=ANY($1::uuid[])`, groupIDs)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, platformID, name, avatar string
			if err := rows.Scan(&id, &platformID, &name, &avatar); err != nil {
				rows.Close()
				return err
			}
			groupPlatforms[id] = platformID
			groupNames[id] = domain.CleanDisplayText(name)
			groupAssets[id] = avatar
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	for i := range nodes {
		if nodes[i].Metadata == nil {
			nodes[i].Metadata = map[string]any{}
		}
		_, id, _ := strings.Cut(nodes[i].Key, ":")
		switch nodes[i].Type {
		case "person":
			qq := stringMetadata(nodes[i].Metadata, "qq")
			if avatar := personAssets[id]; strings.HasPrefix(avatar, "/api/v1/media/assets/") {
				nodes[i].Metadata["avatar_uri"] = avatar
			} else if qq != "" {
				nodes[i].Metadata["avatar_uri"] = "/api/v1/media/avatars/person/" + qq
			} else if avatar := personAssets[id]; avatar != "" {
				nodes[i].Metadata["avatar_uri"] = avatar
			}
			if isBotAccount(qq, nodes[i].Label) {
				nodes[i].Metadata["is_bot"] = true
			}
		case "group":
			platformID := groupPlatforms[id]
			if name := groupNames[id]; name != "" {
				nodes[i].Label = name
			} else if domain.IsOpaqueIdentifier(nodes[i].Label) || nodes[i].Label == "Group" {
				nodes[i].Label = compactGraphLabel(platformID, "Group")
			}
			if platformID != "" {
				nodes[i].Metadata["group_id"] = platformID
			}
			if avatar := groupAssets[id]; avatar != "" {
				nodes[i].Metadata["avatar_uri"] = avatar
			} else if platformID != "" {
				nodes[i].Metadata["avatar_uri"] = "/api/v1/media/avatars/group/" + platformID
			}
		case "conversation":
			if stringMetadata(nodes[i].Metadata, "conversation_type") != "group" {
				continue
			}
			groupEntityID := stringMetadata(nodes[i].Metadata, "group_entity_id")
			platformID := stringMetadata(nodes[i].Metadata, "group_id")
			if platformID == "" && groupEntityID != "" {
				platformID = groupPlatforms[groupEntityID]
				nodes[i].Metadata["group_id"] = platformID
			}
			if name := groupNames[groupEntityID]; name != "" && (nodes[i].Label == "Conversation" || domain.IsOpaqueIdentifier(nodes[i].Label)) {
				nodes[i].Label = name
			}
			if avatar := groupAssets[groupEntityID]; avatar != "" {
				nodes[i].Metadata["avatar_uri"] = avatar
			} else if platformID != "" {
				nodes[i].Metadata["avatar_uri"] = "/api/v1/media/avatars/group/" + platformID
			}
		}
	}
	return nil
}

func rankedGraphNodes(nodes []storedGraphNode, edges []storedGraphEdge, targetKey, focusKey string, limit int) map[string]bool {
	adjacency := map[string][]string{}
	degree := map[string]int{}
	for _, edge := range edges {
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		adjacency[edge.Target] = append(adjacency[edge.Target], edge.Source)
		degree[edge.Source] += edge.Weight
		degree[edge.Target] += edge.Weight
	}
	distance := map[string]int{}
	for _, node := range nodes {
		distance[node.Key] = 1 << 20
	}
	queue := []string{}
	for _, root := range []string{targetKey, focusKey} {
		if root != "" && distance[root] > 0 {
			distance[root] = 0
			queue = append(queue, root)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if distance[next] > distance[current]+1 {
				distance[next] = distance[current] + 1
				queue = append(queue, next)
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if distance[nodes[i].Key] != distance[nodes[j].Key] {
			return distance[nodes[i].Key] < distance[nodes[j].Key]
		}
		if degree[nodes[i].Key] != degree[nodes[j].Key] {
			return degree[nodes[i].Key] > degree[nodes[j].Key]
		}
		return nodes[i].Key < nodes[j].Key
	})
	if limit > 0 && len(nodes) > limit {
		nodes = nodes[:limit]
	}
	selected := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		selected[node.Key] = true
	}
	return selected
}

func writeGraphSubset(w http.ResponseWriter, nodes []storedGraphNode, edges []storedGraphEdge, selected map[string]bool, targetKey string, totalNodes, totalEdges int) {
	distances := bfsDistances(nodes, edges, targetKey)
	// This helper is intentionally independent from a Server so it can be used
	// by focused tests. HTTP handlers pass the configured server limit through
	// writeGraphSubsetWithLimits below.
	writeGraphSubsetWithLimits(w, nodes, edges, selected, targetKey, totalNodes, totalEdges, graphLimitsFor(nodes, edges, distances, maxDistanceFromDistances(distances), 0))
}

func writeGraphSubsetWithLimits(w http.ResponseWriter, nodes []storedGraphNode, edges []storedGraphEdge, selected map[string]bool, targetKey string, totalNodes, totalEdges int, limits graphLimits) {
	resultNodes := make([]storedGraphNode, 0, len(selected))
	for _, node := range nodes {
		if selected[node.Key] {
			resultNodes = append(resultNodes, node)
		}
	}
	resultEdges := []storedGraphEdge{}
	for _, edge := range edges {
		if selected[edge.Source] && selected[edge.Target] {
			resultEdges = append(resultEdges, edge)
		}
	}
	// `total_*` describes the saved network, while `scope_*` describes the
	// current distance-filtered range. A distance filter is not incomplete
	// data; only the server-side limit can make the current range partial.
	partialData := len(resultNodes) < len(nodes) || len(resultEdges) < len(edges)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"nodes": resultNodes, "edges": resultEdges, "target_key": targetKey,
		"returned_nodes": len(resultNodes), "returned_edges": len(resultEdges),
		"scope_nodes": len(nodes), "scope_edges": len(edges),
		"total_nodes": totalNodes, "total_edges": totalEdges,
		"partial":      partialData,
		"partial_data": partialData,
		"limits":       limits,
	}})
}

func graphLimitsFor(nodes []storedGraphNode, edges []storedGraphEdge, distances map[string]int, serverMaxDistance, serverMaxNodes int) graphLimits {
	levelCounts := map[int]int{}
	maxDistance := 0
	for _, node := range nodes {
		distance, ok := distances[node.Key]
		if !ok || distance < 0 {
			continue
		}
		levelCounts[distance]++
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	levels := make([]graphDistanceLevel, 0, len(levelCounts))
	for distance := 0; distance <= maxDistance; distance++ {
		if count := levelCounts[distance]; count > 0 {
			levels = append(levels, graphDistanceLevel{Distance: distance, Nodes: count})
		}
	}
	return graphLimits{
		AvailableNodes:       len(nodes),
		AvailableEdges:       len(edges),
		AvailableMaxDistance: maxDistance,
		DistanceLevels:       levels,
		ServerMaxNodes:       optionalIntLimit(serverMaxNodes),
		ServerMaxDistance:    optionalIntLimit(serverMaxDistance),
	}
}

func optionalIntLimit(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func maxDistanceFromDistances(distances map[string]int) int {
	maxDistance := 0
	for _, distance := range distances {
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	return maxDistance
}

func boundedGraphLimit(raw string, fallback, maximum int) int {
	value, _ := strconv.Atoi(raw)
	if value <= 0 {
		value = fallback
	}
	if maximum > 0 && value > maximum {
		value = maximum
	}
	return value
}

func boundedDistanceParam(raw string, fallback, maximum int) int {
	value, _ := strconv.Atoi(raw)
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func isBotAccount(qq, name string) bool {
	switch qq {
	case "2854196310", "2854196308", "2854196309", "2854196311", "2854196312", "2854212450", "4000000000", "10000", "1000000", "66600000":
		return true
	}
	if strings.Contains(name, "Q群管家") || strings.Contains(name, "群管家") || strings.Contains(name, "签到助手") {
		return true
	}
	return false
}
