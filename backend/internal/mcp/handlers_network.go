package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

// handleEgoNetwork handles enhanced ego network with metrics
type EgoNetworkArgs struct {
	TargetQQ       string   `json:"target_qq"`
	GroupID        string   `json:"group_id"`
	Depth          int      `json:"depth"`
	RelationFilter []string `json:"relation_filter"`
	MaxNodes       int      `json:"max_nodes"`
	IncludeMetrics bool     `json:"include_metrics"`
	Format         string   `json:"format"`
}

type NodeMetrics struct {
	Key                   string  `json:"key"`
	Label                 string  `json:"label"`
	Type                  string  `json:"type"`
	QQ                    string  `json:"qq,omitempty"`
	DegreeCentrality      float64 `json:"degree_centrality"`
	BetweennessCentrality float64 `json:"betweenness_centrality"`
	CommunityID           int     `json:"community_id"`
	TotalEdgeWeight       int     `json:"total_edge_weight"`
}

func (h *HandlerRegistry) handleEgoNetwork(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input EgoNetworkArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Depth is the scope boundary. max_nodes is optional; zero means all nodes
	// within that scope rather than an implicit render/data cap.
	if input.Depth == 0 {
		input.Depth = 1
	}
	if input.Format == "" {
		input.Format = "summary"
	}
	// By default IncludeMetrics is conceptually true from requirements, though bool defaults to false
	// We'll just compute it if it's explicitly asked or by default? Let's just always compute if not format=full without it.
	// We'll compute metrics if IncludeMetrics is true or if Format is summary.

	var groupUUID string
	if input.GroupID != "" {
		if err := h.DB.QueryRow(ctx, `SELECT id::text FROM "groups" WHERE platform='qq' AND platform_group_id=$1`, input.GroupID).Scan(&groupUUID); err != nil {
			return errorResult(fmt.Sprintf("group not found: %s", input.GroupID)), nil
		}
	}
	builder := analysis.EgoBuilder{DB: h.DB}
	graph, err := builder.BuildWithOptions(ctx, input.TargetQQ, input.Depth, analysis.BuildOptions{MaxNodes: input.MaxNodes, GroupUUID: groupUUID})
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to build ego network: %v", err)), nil
	}

	// Filter edges if RelationFilter is set
	var filteredEdges []analysis.Edge
	if len(input.RelationFilter) > 0 {
		allowed := make(map[string]bool)
		for _, rf := range input.RelationFilter {
			allowed[rf] = true
		}
		for _, e := range graph.Edges {
			if allowed[e.RelationType] {
				filteredEdges = append(filteredEdges, e)
			}
		}
	} else {
		filteredEdges = graph.Edges
	}

	var metrics []NodeMetrics
	var numCommunities int
	if input.IncludeMetrics || input.Format == "summary" {
		metrics, numCommunities = computeGraphMetrics(graph.Nodes, filteredEdges)
	}

	if input.Format == "summary" {
		// Return only top 20 nodes by centrality + community assignments + bridge nodes
		sort.Slice(metrics, func(i, j int) bool {
			return metrics[i].DegreeCentrality > metrics[j].DegreeCentrality
		})
		topNodes := metrics
		if len(topNodes) > 20 {
			topNodes = topNodes[:20]
		}
		summary := map[string]interface{}{
			"target_qq":      input.TargetQQ,
			"total_nodes":    len(graph.Nodes),
			"total_edges":    len(filteredEdges),
			"communities":    numCommunities,
			"top_nodes":      topNodes,
			"metrics_status": "computed",
		}
		return jsonResult(summary)
	}

	// format = "full"
	fullRes := map[string]interface{}{
		"target_qq": input.TargetQQ,
		"nodes":     graph.Nodes,
		"edges":     filteredEdges,
		"metrics":   metrics,
	}
	return jsonResult(fullRes)
}

func computeGraphMetrics(nodes []analysis.Node, edges []analysis.Edge) ([]NodeMetrics, int) {
	if len(nodes) == 0 {
		return nil, 0
	}

	adj := make(map[string][]string)
	edgeCount := make(map[string]int)

	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
		edgeCount[e.Source]++
		edgeCount[e.Target]++
	}

	nodeMap := make(map[string]analysis.Node)
	metricsMap := make(map[string]*NodeMetrics)
	communities := make(map[string]int)

	for i, n := range nodes {
		nodeMap[n.Key] = n
		communities[n.Key] = i // init community
		metricsMap[n.Key] = &NodeMetrics{
			Key:              n.Key,
			Label:            n.Label,
			Type:             n.Type,
			QQ:               nodeQQ(n),
			DegreeCentrality: float64(edgeCount[n.Key]) / float64(max(1, len(nodes)-1)),
			TotalEdgeWeight:  edgeCount[n.Key],
		}
	}

	// Betweenness centrality (simplified BFS)
	sampleSize := min(50, len(nodes))
	for i := 0; i < sampleSize; i++ {
		startNode := nodes[i].Key
		queue := []string{startNode}
		visited := map[string]bool{startNode: true}
		paths := map[string][]string{startNode: {startNode}}

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			for _, neighbor := range adj[curr] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
					newPath := append([]string{}, paths[curr]...)
					newPath = append(newPath, neighbor)
					paths[neighbor] = newPath

					// Count intermediate nodes
					for _, intermediate := range newPath[1 : len(newPath)-1] {
						metricsMap[intermediate].BetweennessCentrality += 1.0
					}
				}
			}
		}
	}

	// Label propagation for communities
	for iter := 0; iter < 5; iter++ {
		for _, n := range nodes {
			counts := make(map[int]int)
			maxCount := 0
			bestComm := communities[n.Key]

			for _, neighbor := range adj[n.Key] {
				c := communities[neighbor]
				counts[c]++
				if counts[c] > maxCount {
					maxCount = counts[c]
					bestComm = c
				}
			}
			communities[n.Key] = bestComm
		}
	}

	uniqueCommunities := make(map[int]bool)
	var finalMetrics []NodeMetrics
	for _, n := range nodes {
		m := metricsMap[n.Key]
		m.CommunityID = communities[n.Key]
		uniqueCommunities[m.CommunityID] = true
		finalMetrics = append(finalMetrics, *m)
	}

	return finalMetrics, len(uniqueCommunities)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type FindPathArgs struct {
	SourceQQ       string   `json:"source_qq"`
	TargetQQ       string   `json:"target_qq"`
	MaxHops        int      `json:"max_hops"`
	MaxPaths       int      `json:"max_paths"`
	RelationFilter []string `json:"relation_filter"`
}

type PathHop struct {
	personID       string   `json:"-"`
	QQ             string   `json:"qq"`
	DisplayName    string   `json:"display_name"`
	RelationToNext string   `json:"relation_to_next,omitempty"`
	SharedGroups   []string `json:"shared_groups,omitempty"`
}

type ConnectionPath struct {
	Hops     []PathHop `json:"hops"`
	Length   int       `json:"length"`
	ViaTypes []string  `json:"via_types"`
}

func (h *HandlerRegistry) handleFindPath(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input FindPathArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	policy := normalizedRoutingPolicy(h.Policy)
	if input.MaxHops == 0 {
		input.MaxHops = policy.DefaultMaxHops
	}
	if input.MaxPaths == 0 {
		input.MaxPaths = policy.DefaultMaxPaths
	}
	if input.MaxHops <= 0 {
		input.MaxHops = 4
	}
	if input.MaxPaths <= 0 {
		input.MaxPaths = 3
	}
	if len(input.RelationFilter) == 0 {
		return h.findPathViaRouter(ctx, input)
	}

	srcPersonID, err := resolvePersonID(ctx, h.DB, input.SourceQQ)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to resolve source QQ: %v", err)), nil
	}
	targetPersonID, err := resolvePersonID(ctx, h.DB, input.TargetQQ)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to resolve target QQ: %v", err)), nil
	}

	if srcPersonID == targetPersonID {
		return jsonResult(map[string]interface{}{
			"source_qq":   input.SourceQQ,
			"target_qq":   input.TargetQQ,
			"paths_found": 0,
			"message":     "Source and target are the same person",
		})
	}

	frontier := []string{srcPersonID}
	visited := map[string]bool{srcPersonID: true}
	parentMap := make(map[string][]string)
	parentRelations := make(map[string]map[string]string)
	targetFoundAtHop := -1

	for hop := 0; hop < input.MaxHops; hop++ {
		if len(frontier) == 0 {
			break
		}

		var newFrontier []string
		queryArgs := []interface{}{frontier}
		edgeQuery := `
			SELECT actor_person_id::text, target_person_id::text, action_type
			FROM relation_events
			WHERE (actor_person_id = ANY($1::uuid[]) OR target_person_id = ANY($1::uuid[]))
			  AND actor_person_id IS NOT NULL AND target_person_id IS NOT NULL
		`
		if len(input.RelationFilter) > 0 {
			edgeQuery += " AND action_type = ANY($2::text[])"
			queryArgs = append(queryArgs, input.RelationFilter)
		}
		eRows, err := h.DB.Query(ctx, edgeQuery, queryArgs...)
		if err != nil {
			return nil, fmt.Errorf("query relations failed: %w", err)
		}
		for eRows.Next() {
			var a, t, relation string
			if err := eRows.Scan(&a, &t, &relation); err != nil {
				eRows.Close()
				return nil, err
			}
			inFrontierA := false
			for _, f := range frontier {
				if a == f {
					inFrontierA = true
					break
				}
			}
			if inFrontierA && !visited[t] {
				parentMap[t] = append(parentMap[t], a)
				if parentRelations[t] == nil {
					parentRelations[t] = make(map[string]string)
				}
				parentRelations[t][a] = relation
				newFrontier = append(newFrontier, t)
			}
			inFrontierT := false
			for _, f := range frontier {
				if t == f {
					inFrontierT = true
					break
				}
			}
			if inFrontierT && !visited[a] {
				parentMap[a] = append(parentMap[a], t)
				if parentRelations[a] == nil {
					parentRelations[a] = make(map[string]string)
				}
				parentRelations[a][t] = relation
				newFrontier = append(newFrontier, a)
			}
		}
		eRows.Close()

		uniqueNew := []string{}
		seenNew := make(map[string]bool)
		for _, nf := range newFrontier {
			if !seenNew[nf] && !visited[nf] {
				seenNew[nf] = true
				uniqueNew = append(uniqueNew, nf)
			}
		}

		for _, nf := range uniqueNew {
			visited[nf] = true
			if nf == targetPersonID {
				targetFoundAtHop = hop + 1
			}
		}
		frontier = uniqueNew

		if targetFoundAtHop != -1 {
			break
		}
	}

	var paths []ConnectionPath
	if targetFoundAtHop != -1 {
		var reconstruct func(current string, currentPath []string)
		reconstruct = func(current string, currentPath []string) {
			if len(paths) >= input.MaxPaths {
				return
			}
			if current == srcPersonID {
				rev := make([]string, len(currentPath))
				for i, p := range currentPath {
					rev[len(currentPath)-1-i] = p
				}
				var hops []PathHop
				for i, personID := range rev {
					relation := ""
					if i < len(rev)-1 {
						relation = parentRelations[rev[i+1]][personID]
					}
					hops = append(hops, PathHop{personID: personID, RelationToNext: relation})
				}
				paths = append(paths, ConnectionPath{Hops: hops, Length: len(hops) - 1, ViaTypes: []string{"interaction"}})
				return
			}
			for _, p := range parentMap[current] {
				reconstruct(p, append(currentPath, p))
			}
		}
		reconstruct(targetPersonID, []string{targetPersonID})
	}
	if err := h.populatePathPeople(ctx, paths); err != nil {
		return nil, fmt.Errorf("load path people failed: %w", err)
	}

	return jsonResult(map[string]interface{}{
		"source_qq":   input.SourceQQ,
		"target_qq":   input.TargetQQ,
		"paths_found": len(paths),
		"paths":       paths,
	})
}

func (h *HandlerRegistry) findPathViaRouter(ctx context.Context, input FindPathArgs) (*ToolCallResult, error) {
	router := analysis.Router{DB: h.DB, Policy: normalizedRoutingPolicy(h.Policy)}
	result, err := router.PlanRoutes(ctx, analysis.RouteRequest{
		SourceQQ: input.SourceQQ,
		TargetQQ: input.TargetQQ,
		MaxHops:  input.MaxHops,
		MaxPaths: input.MaxPaths,
		Profile:  "shortest",
	})
	if err != nil {
		return errorResult(fmt.Sprintf("path analysis failed: %v", err)), nil
	}
	paths := make([]map[string]any, 0, len(result.Routes))
	for _, route := range result.Routes {
		hops := make([]map[string]any, 0, len(route.Steps)+1)
		if len(route.Steps) > 0 {
			hops = append(hops, map[string]any{
				"qq":           metaQQ(route.Steps[0].SourceMeta),
				"display_name": route.Steps[0].SourceLabel,
			})
		}
		for _, step := range route.Steps {
			hops = append(hops, map[string]any{
				"qq":                 metaQQ(step.TargetMeta),
				"display_name":       step.TargetLabel,
				"relation_to_next":   step.RelationType,
				"evidence_direction": step.EvidenceDirection,
			})
		}
		paths = append(paths, map[string]any{
			"length": route.TotalHops, "cost": route.TotalCost, "confidence": route.Confidence,
			"hops": hops, "summary": route.Summary,
		})
	}
	return jsonResult(map[string]any{
		"source_qq": result.SourceQQ, "target_qq": result.TargetQQ,
		"paths_found": result.PathsFound, "paths": paths, "stats": result.Stats,
	})
}

func metaQQ(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["qq"]; ok {
		return fmt.Sprint(v)
	}
	if v, ok := meta["platform_user_id"]; ok {
		return fmt.Sprint(v)
	}
	if v, ok := meta["author_qq"]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (h *HandlerRegistry) populatePathPeople(ctx context.Context, paths []ConnectionPath) error {
	ids := make([]string, 0)
	seen := make(map[string]bool)
	for _, path := range paths {
		for _, hop := range path.Hops {
			if hop.personID != "" && !seen[hop.personID] {
				seen[hop.personID] = true
				ids = append(ids, hop.personID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := h.DB.Query(ctx, `
		SELECT p.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id = ANY($1::uuid[])
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	type personInfo struct{ QQ, Name string }
	info := make(map[string]personInfo, len(ids))
	for rows.Next() {
		var id, qq, name string
		if err := rows.Scan(&id, &qq, &name); err != nil {
			return err
		}
		info[id] = personInfo{QQ: qq, Name: name}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range paths {
		for j := range paths[i].Hops {
			if p, ok := info[paths[i].Hops[j].personID]; ok {
				paths[i].Hops[j].QQ = p.QQ
				paths[i].Hops[j].DisplayName = p.Name
			}
		}
	}
	return nil
}

func nodeQQ(n analysis.Node) string {
	meta, ok := n.Metadata.(map[string]any)
	if !ok {
		return ""
	}
	if qq, ok := meta["qq"].(string); ok {
		return qq
	}
	if qq, ok := meta["author_qq"].(string); ok {
		return qq
	}
	return ""
}

type MutualAnalysisArgs struct {
	QQList   []string `json:"qq_list"`
	Sections []string `json:"sections"`
}

func (h *HandlerRegistry) handleMutualAnalysis(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input MutualAnalysisArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if len(input.QQList) != 2 {
		return errorResult("mutual analysis currently only supports exactly 2 QQs"), nil
	}

	if len(input.Sections) == 0 {
		input.Sections = []string{"all"}
	}

	p1, err := resolvePersonID(ctx, h.DB, input.QQList[0])
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to resolve first QQ: %v", err)), nil
	}
	p2, err := resolvePersonID(ctx, h.DB, input.QQList[1])
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to resolve second QQ: %v", err)), nil
	}

	sectionsToRun := make(map[string]bool)
	for _, s := range input.Sections {
		sectionsToRun[s] = true
	}
	if sectionsToRun["all"] {
		sectionsToRun["shared_groups"] = true
		sectionsToRun["mutual_friends"] = true
		sectionsToRun["interactions"] = true
		sectionsToRun["strength_score"] = true
	}

	result := make(map[string]interface{})

	var sharedGroupsCount int
	if sectionsToRun["shared_groups"] {
		q := `
			SELECT g.platform_group_id, g.group_name
			FROM group_memberships gm1
			JOIN group_memberships gm2 ON gm1.group_id = gm2.group_id
			JOIN "groups" g ON g.id = gm1.group_id
			WHERE gm1.person_id = $1 AND gm2.person_id = $2
		`
		rows, err := h.DB.Query(ctx, q, p1, p2)
		if err == nil {
			var groups []map[string]string
			for rows.Next() {
				var id, name string
				if err := rows.Scan(&id, &name); err == nil {
					groups = append(groups, map[string]string{"group_id": id, "name": name})
				}
			}
			rows.Close()
			result["shared_groups"] = groups
			sharedGroupsCount = len(groups)
		}
	}

	var mutualFriendsCount int
	if sectionsToRun["mutual_friends"] {
		q := `
			SELECT p.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
			FROM (
			  SELECT DISTINCT target_person_id as friend_id FROM relation_events WHERE actor_person_id = $1 AND target_person_id IS NOT NULL
			  UNION SELECT DISTINCT actor_person_id FROM relation_events WHERE target_person_id = $1 AND actor_person_id IS NOT NULL
			) f1
			INNER JOIN (
			  SELECT DISTINCT target_person_id as friend_id FROM relation_events WHERE actor_person_id = $2 AND target_person_id IS NOT NULL  
			  UNION SELECT DISTINCT actor_person_id FROM relation_events WHERE target_person_id = $2 AND actor_person_id IS NOT NULL
			) f2 ON f1.friend_id = f2.friend_id
			JOIN persons p ON p.id = f1.friend_id
			LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE f1.friend_id != $1 AND f1.friend_id != $2
			LIMIT 50
		`
		rows, err := h.DB.Query(ctx, q, p1, p2)
		if err == nil {
			var friends []map[string]string
			for rows.Next() {
				var id, qq, name string
				if err := rows.Scan(&id, &qq, &name); err == nil {
					friends = append(friends, map[string]string{"person_id": id, "qq": qq, "name": name})
				}
			}
			rows.Close()
			result["mutual_friends"] = friends
			mutualFriendsCount = len(friends)
		}
	}

	var interactionsCount int
	if sectionsToRun["interactions"] {
		q := `
			SELECT action_type,
			  COUNT(CASE WHEN actor_person_id = $1 THEN 1 END) as forward_count,
			  COUNT(CASE WHEN actor_person_id = $2 THEN 1 END) as reverse_count,
			  MIN(occurred_at) as first_at,
			  MAX(occurred_at) as last_at
			FROM relation_events
			WHERE (actor_person_id = $1 AND target_person_id = $2)
			   OR (actor_person_id = $2 AND target_person_id = $1)
			GROUP BY action_type
			ORDER BY forward_count + reverse_count DESC
		`
		rows, err := h.DB.Query(ctx, q, p1, p2)
		if err == nil {
			var inters []map[string]interface{}
			for rows.Next() {
				var action string
				var fwd, rev int
				var first, last *time.Time
				if err := rows.Scan(&action, &fwd, &rev, &first, &last); err == nil {
					inters = append(inters, map[string]interface{}{
						"action_type":   action,
						"forward_count": fwd,
						"reverse_count": rev,
						"first_at":      first,
						"last_at":       last,
					})
					interactionsCount += fwd + rev
				}
			}
			rows.Close()
			result["interactions"] = inters
		}
	}

	if sectionsToRun["strength_score"] {
		score := sharedGroupsCount*10 + mutualFriendsCount*5 + interactionsCount*1
		result["strength_score"] = map[string]interface{}{
			"score": score,
		}
	}

	return jsonResult(result)
}
