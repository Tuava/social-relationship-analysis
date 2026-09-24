package analysis

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Router performs graph-based path finding and routing between entities.
type Router struct {
	DB     *pgxpool.Pool
	Policy RoutingPolicy
}

// RoutingPolicy contains deployment-level defaults and optional safety limits.
// A zero Max* value means unlimited. Limits are returned in route statistics so
// a caller can distinguish a complete search from a policy-truncated search.
type RoutingPolicy struct {
	DefaultMaxHops             int
	DefaultMaxPaths            int
	DefaultLargeGroupThreshold int
	MaxHops                    int
	MaxPaths                   int
	MaxGraphNodes              int
	MaxFrontierNeighbors       int
	MaxGroupCoMembers          int
	MaxGroupMemberships        int
}

// RouteRequest defines the input parameters for path planning.
type RouteRequest struct {
	SourceQQ            string     `json:"source_qq"`
	TargetQQ            string     `json:"target_qq"`
	Profile             string     `json:"profile"` // "least_cost" (default), "shortest", "covert"
	MaxHops             int        `json:"max_hops"`
	MaxPaths            int        `json:"max_paths"`
	AvoidNodes          []string   `json:"avoid_nodes"`          // node keys or QQs/group IDs to avoid
	Waypoints           []string   `json:"waypoints"`            // required intermediate node keys/QQs
	ExcludeLargeGroups  bool       `json:"exclude_large_groups"` // exclude groups with member count > threshold
	LargeGroupThreshold int        `json:"large_group_threshold"`
	TimeStart           *time.Time `json:"time_start,omitempty"`
	TimeEnd             *time.Time `json:"time_end,omitempty"`
}

// RouteStep represents a single hop along a planned route.
type RouteStep struct {
	StepNumber        int            `json:"step_number"`
	SourceKey         string         `json:"source_key"`
	SourceType        string         `json:"source_type"`
	SourceLabel       string         `json:"source_label"`
	SourceMeta        map[string]any `json:"source_meta"`
	TargetKey         string         `json:"target_key"`
	TargetType        string         `json:"target_type"`
	TargetLabel       string         `json:"target_label"`
	TargetMeta        map[string]any `json:"target_meta"`
	RelationType      string         `json:"relation_type"`
	Weight            int            `json:"weight"`
	Cost              float64        `json:"cost"`
	MediumType        string         `json:"medium_type"`
	MediumSummary     string         `json:"medium_summary"`
	EvidenceIDs       []string       `json:"evidence_ids"`
	EvidenceDirection string         `json:"evidence_direction"`
	EvidenceActorKey  string         `json:"evidence_actor_key"`
	EvidenceTargetKey string         `json:"evidence_target_key"`
	FirstSeen         time.Time      `json:"first_seen"`
	LastSeen          time.Time      `json:"last_seen"`
}

// PlannedRoute represents one complete end-to-end path with metrics.
type PlannedRoute struct {
	PathIndex   int         `json:"path_index"`
	Title       string      `json:"title"`
	TotalHops   int         `json:"total_hops"`
	TotalWeight int         `json:"total_weight"`
	TotalCost   float64     `json:"total_cost"`
	Confidence  int         `json:"confidence"` // 0 - 100
	Steps       []RouteStep `json:"steps"`
	Nodes       []Node      `json:"nodes"`
	Edges       []Edge      `json:"edges"`
	Summary     string      `json:"summary"`
}

// RouteResult is the return payload for RouteRequest.
type RouteResult struct {
	SourceQQ   string         `json:"source_qq"`
	TargetQQ   string         `json:"target_qq"`
	Profile    string         `json:"profile"`
	PathsFound int            `json:"paths_found"`
	Routes     []PlannedRoute `json:"routes"`
	Stats      map[string]any `json:"stats"`
}

// PlanRoutes executes path finding between SourceQQ and TargetQQ.
func (r Router) PlanRoutes(ctx context.Context, req RouteRequest) (RouteResult, error) {
	if req.MaxHops <= 0 {
		req.MaxHops = r.Policy.DefaultMaxHops
	}
	if req.MaxPaths <= 0 {
		req.MaxPaths = r.Policy.DefaultMaxPaths
	}
	if req.MaxHops <= 0 {
		return RouteResult{}, fmt.Errorf("max_hops must be greater than zero")
	}
	if req.MaxPaths <= 0 {
		return RouteResult{}, fmt.Errorf("max_paths must be greater than zero")
	}
	if r.Policy.MaxHops > 0 && req.MaxHops > r.Policy.MaxHops {
		return RouteResult{}, fmt.Errorf("max_hops %d exceeds configured routing limit %d", req.MaxHops, r.Policy.MaxHops)
	}
	if r.Policy.MaxPaths > 0 && req.MaxPaths > r.Policy.MaxPaths {
		return RouteResult{}, fmt.Errorf("max_paths %d exceeds configured routing limit %d", req.MaxPaths, r.Policy.MaxPaths)
	}
	if req.Profile == "" {
		req.Profile = "least_cost"
	}
	switch req.Profile {
	case "least_cost", "shortest", "covert":
	default:
		return RouteResult{}, fmt.Errorf("unsupported routing profile %q", req.Profile)
	}
	if len(req.Waypoints) > 0 {
		return RouteResult{}, fmt.Errorf("waypoints are not implemented; refusing to ignore requested intermediates")
	}
	if req.TimeStart != nil || req.TimeEnd != nil {
		return RouteResult{}, fmt.Errorf("time-bounded routing is not implemented; refusing to ignore the requested time range")
	}
	if req.LargeGroupThreshold <= 0 {
		req.LargeGroupThreshold = r.Policy.DefaultLargeGroupThreshold
	}
	if req.ExcludeLargeGroups && req.LargeGroupThreshold <= 0 {
		return RouteResult{}, fmt.Errorf("large_group_threshold must be greater than zero when exclude_large_groups is enabled")
	}

	// 1. Resolve source and target persons
	var sourceID, targetID string
	if err := r.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, req.SourceQQ).Scan(&sourceID); err != nil {
		return RouteResult{}, fmt.Errorf("source QQ %s not found: %w", req.SourceQQ, err)
	}
	if err := r.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, req.TargetQQ).Scan(&targetID); err != nil {
		return RouteResult{}, fmt.Errorf("target QQ %s not found: %w", req.TargetQQ, err)
	}

	sourceKey := "person:" + sourceID
	targetKey := "person:" + targetID

	if sourceKey == targetKey {
		return RouteResult{}, fmt.Errorf("source and target are the same person")
	}

	// 2. Build local subgraph through bidirectional expansion
	subgraph, err := r.buildRoutingGraph(ctx, sourceID, targetID, req)
	if err != nil {
		return RouteResult{}, fmt.Errorf("failed to build routing graph: %w", err)
	}

	// 3. Prepare the caller-supplied avoidance set. The router does not silently
	// exclude bots or high-degree nodes because that would alter graph truth.
	avoidMap := make(map[string]bool)
	for _, avoid := range req.AvoidNodes {
		clean := strings.TrimSpace(avoid)
		if clean == "" {
			continue
		}
		if !strings.Contains(clean, ":") {
			// Try person or group
			var pID string
			if err := r.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, clean).Scan(&pID); err == nil {
				avoidMap["person:"+pID] = true
			}
			var gID string
			if err := r.DB.QueryRow(ctx, `SELECT id FROM "groups" WHERE platform_group_id=$1 LIMIT 1`, clean).Scan(&gID); err == nil {
				avoidMap["group:"+gID] = true
			}
		} else {
			avoidMap[clean] = true
		}
	}
	// Never avoid source or target
	delete(avoidMap, sourceKey)
	delete(avoidMap, targetKey)

	// 4. Calculate K-Shortest Paths
	rawPaths := r.findKShortestPaths(subgraph, sourceKey, targetKey, req.MaxPaths, req.MaxHops, req.Profile, avoidMap)

	// 5. Enrich paths with node labels and turn-by-turn steps
	routes, err := r.enrichPlannedRoutes(ctx, rawPaths, req)
	if err != nil {
		return RouteResult{}, fmt.Errorf("failed to enrich routes: %w", err)
	}

	return RouteResult{
		SourceQQ:   req.SourceQQ,
		TargetQQ:   req.TargetQQ,
		Profile:    req.Profile,
		PathsFound: len(routes),
		Routes:     routes,
		Stats: map[string]any{
			"graph_nodes":        len(subgraph.adj),
			"graph_edges":        len(subgraph.edgeMap),
			"profile":            req.Profile,
			"max_hops":           req.MaxHops,
			"truncated":          subgraph.truncated,
			"truncation_reasons": subgraph.truncationReasons(),
			"policy": map[string]int{
				"max_graph_nodes":        r.Policy.MaxGraphNodes,
				"max_frontier_neighbors": r.Policy.MaxFrontierNeighbors,
				"max_group_co_members":   r.Policy.MaxGroupCoMembers,
				"max_group_memberships":  r.Policy.MaxGroupMemberships,
			},
		},
	}, nil
}

// ─── Graph Building & Cost Modeling ──────────────────────────────────────────

type routingEdge struct {
	source       string
	target       string
	relationType string
	weight       int
	cost         float64
	evidenceIDs  []string
	firstSeen    time.Time
	lastSeen     time.Time
	reversed     bool
}

type routingGraph struct {
	adj              map[string][]routingEdge
	edgeMap          map[string]routingEdge
	truncated        bool
	truncationReason map[string]struct{}
}

func (g *routingGraph) markTruncated(reason string) {
	g.truncated = true
	if g.truncationReason == nil {
		g.truncationReason = make(map[string]struct{})
	}
	g.truncationReason[reason] = struct{}{}
}

func (g *routingGraph) truncationReasons() []string {
	reasons := make([]string, 0, len(g.truncationReason))
	for reason := range g.truncationReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return reasons
}

func (r Router) buildRoutingGraph(ctx context.Context, sourceID, targetID string, req RouteRequest) (*routingGraph, error) {
	// Bidirectional frontier expansion to collect all relevant nodes up to maxHops/2 + 1
	allPersonIDs := map[string]bool{sourceID: true, targetID: true}

	expansionDepth := (req.MaxHops / 2) + 1
	if expansionDepth < 2 {
		expansionDepth = 2
	}

	g := &routingGraph{
		adj:              make(map[string][]routingEdge),
		edgeMap:          make(map[string]routingEdge),
		truncationReason: make(map[string]struct{}),
	}
	addPerson := func(personID string) bool {
		if personID == "" || allPersonIDs[personID] {
			return false
		}
		if r.Policy.MaxGraphNodes > 0 && len(allPersonIDs) >= r.Policy.MaxGraphNodes {
			g.markTruncated("max_graph_nodes")
			return false
		}
		allPersonIDs[personID] = true
		return true
	}

	srcFrontier := []string{sourceID}
	tgtFrontier := []string{targetID}

	for level := 0; level < expansionDepth; level++ {
		nextSrcFrontier := []string{}
		nextTgtFrontier := []string{}

		// Expand forward from source frontier
		if len(srcFrontier) > 0 {
			rows, err := r.DB.Query(ctx, `
				SELECT DISTINCT 
					CASE 
						WHEN re.actor_person_id = ANY($1::uuid[]) THEN COALESCE(re.target_person_id, c.author_id)
						ELSE re.actor_person_id 
					END as neighbor
				FROM relation_events re
				LEFT JOIN contents c ON c.id = re.target_object_id
				WHERE (re.actor_person_id = ANY($1::uuid[]) OR COALESCE(re.target_person_id, c.author_id) = ANY($1::uuid[]))
				  AND re.actor_person_id IS NOT NULL 
				  AND COALESCE(re.target_person_id, c.author_id) IS NOT NULL
				  AND re.actor_person_id <> COALESCE(re.target_person_id, c.author_id)
					LIMIT NULLIF($2, 0)`, srcFrontier, r.Policy.MaxFrontierNeighbors)
			if err != nil {
				return nil, fmt.Errorf("expand source interactions: %w", err)
			}
			sourceNeighborCount := 0
			for rows.Next() {
				var nID *string
				if err := rows.Scan(&nID); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scan source interaction neighbor: %w", err)
				}
				sourceNeighborCount++
				if nID != nil && addPerson(*nID) {
					nextSrcFrontier = append(nextSrcFrontier, *nID)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("iterate source interaction neighbors: %w", err)
			}
			rows.Close()
			if r.Policy.MaxFrontierNeighbors > 0 && sourceNeighborCount >= r.Policy.MaxFrontierNeighbors {
				g.markTruncated("max_frontier_neighbors")
			}

			// Group co-members on level 0 — only those who have at least one
			// interaction recorded (EXISTS avoids a full-table scan of relation_events).
			if level == 0 {
				groupRows, err := r.DB.Query(ctx, `
					WITH my_groups AS (
						SELECT group_id FROM group_memberships WHERE person_id = ANY($1::uuid[])
					)
					SELECT DISTINCT gm.person_id::text
					FROM group_memberships gm
					JOIN my_groups mg ON mg.group_id = gm.group_id
					WHERE NOT (gm.person_id = ANY($1::uuid[]))
					  AND (
							EXISTS (SELECT 1 FROM relation_events re WHERE re.actor_person_id = gm.person_id AND re.action_type <> 'member_of')
							OR EXISTS (SELECT 1 FROM relation_events re WHERE re.target_person_id = gm.person_id AND re.action_type <> 'member_of')
					  )
						LIMIT NULLIF($2, 0)`, srcFrontier, r.Policy.MaxGroupCoMembers)
				if err != nil {
					return nil, fmt.Errorf("expand source group co-members: %w", err)
				}
				sourceGroupNeighborCount := 0
				for groupRows.Next() {
					var nID string
					if err := groupRows.Scan(&nID); err != nil {
						groupRows.Close()
						return nil, fmt.Errorf("scan source group co-member: %w", err)
					}
					sourceGroupNeighborCount++
					if addPerson(nID) {
						nextSrcFrontier = append(nextSrcFrontier, nID)
					}
				}
				if err := groupRows.Err(); err != nil {
					groupRows.Close()
					return nil, fmt.Errorf("iterate source group co-members: %w", err)
				}
				groupRows.Close()
				if r.Policy.MaxGroupCoMembers > 0 && sourceGroupNeighborCount >= r.Policy.MaxGroupCoMembers {
					g.markTruncated("max_group_co_members")
				}
			}
		}

		// Expand backward from target frontier
		if len(tgtFrontier) > 0 {
			rowsTgt, err := r.DB.Query(ctx, `
				SELECT DISTINCT 
					CASE 
						WHEN re.actor_person_id = ANY($1::uuid[]) THEN COALESCE(re.target_person_id, c.author_id)
						ELSE re.actor_person_id 
					END as neighbor
				FROM relation_events re
				LEFT JOIN contents c ON c.id = re.target_object_id
				WHERE (re.actor_person_id = ANY($1::uuid[]) OR COALESCE(re.target_person_id, c.author_id) = ANY($1::uuid[]))
				  AND re.actor_person_id IS NOT NULL 
				  AND COALESCE(re.target_person_id, c.author_id) IS NOT NULL
				  AND re.actor_person_id <> COALESCE(re.target_person_id, c.author_id)
					LIMIT NULLIF($2, 0)`, tgtFrontier, r.Policy.MaxFrontierNeighbors)
			if err != nil {
				return nil, fmt.Errorf("expand target interactions: %w", err)
			}
			targetNeighborCount := 0
			for rowsTgt.Next() {
				var nID *string
				if err := rowsTgt.Scan(&nID); err != nil {
					rowsTgt.Close()
					return nil, fmt.Errorf("scan target interaction neighbor: %w", err)
				}
				targetNeighborCount++
				if nID != nil && addPerson(*nID) {
					nextTgtFrontier = append(nextTgtFrontier, *nID)
				}
			}
			if err := rowsTgt.Err(); err != nil {
				rowsTgt.Close()
				return nil, fmt.Errorf("iterate target interaction neighbors: %w", err)
			}
			rowsTgt.Close()
			if r.Policy.MaxFrontierNeighbors > 0 && targetNeighborCount >= r.Policy.MaxFrontierNeighbors {
				g.markTruncated("max_frontier_neighbors")
			}

			// Group co-members on level 0 — only those with recorded interactions.
			if level == 0 {
				groupRowsTgt, err := r.DB.Query(ctx, `
					WITH my_groups AS (
						SELECT group_id FROM group_memberships WHERE person_id = ANY($1::uuid[])
					)
					SELECT DISTINCT gm.person_id::text
					FROM group_memberships gm
					JOIN my_groups mg ON mg.group_id = gm.group_id
					WHERE NOT (gm.person_id = ANY($1::uuid[]))
					  AND (
							EXISTS (SELECT 1 FROM relation_events re WHERE re.actor_person_id = gm.person_id AND re.action_type <> 'member_of')
							OR EXISTS (SELECT 1 FROM relation_events re WHERE re.target_person_id = gm.person_id AND re.action_type <> 'member_of')
					  )
						LIMIT NULLIF($2, 0)`, tgtFrontier, r.Policy.MaxGroupCoMembers)
				if err != nil {
					return nil, fmt.Errorf("expand target group co-members: %w", err)
				}
				targetGroupNeighborCount := 0
				for groupRowsTgt.Next() {
					var nID string
					if err := groupRowsTgt.Scan(&nID); err != nil {
						groupRowsTgt.Close()
						return nil, fmt.Errorf("scan target group co-member: %w", err)
					}
					targetGroupNeighborCount++
					if addPerson(nID) {
						nextTgtFrontier = append(nextTgtFrontier, nID)
					}
				}
				if err := groupRowsTgt.Err(); err != nil {
					groupRowsTgt.Close()
					return nil, fmt.Errorf("iterate target group co-members: %w", err)
				}
				groupRowsTgt.Close()
				if r.Policy.MaxGroupCoMembers > 0 && targetGroupNeighborCount >= r.Policy.MaxGroupCoMembers {
					g.markTruncated("max_group_co_members")
				}
			}
		}

		if len(nextSrcFrontier) == 0 && len(nextTgtFrontier) == 0 {
			break
		}
		if r.Policy.MaxGraphNodes > 0 && len(allPersonIDs) >= r.Policy.MaxGraphNodes {
			break
		}
		srcFrontier = nextSrcFrontier
		tgtFrontier = nextTgtFrontier
	}

	personSlice := make([]string, 0, len(allPersonIDs))
	for p := range allPersonIDs {
		personSlice = append(personSlice, p)
	}

	// Now pull all relation events and group memberships connecting these persons.

	// 1. Direct and mediated relation events (including QZone feed likes/comments linked to author)
	eventQuery := `
		WITH matched_events AS (
			SELECT re.actor_person_id,
			       COALESCE(re.target_person_id, c.author_id) AS target_person_id,
			       re.action_type,
			       re.occurred_at,
			       re.evidence_ids
			FROM relation_events re
			LEFT JOIN contents c ON c.id = re.target_object_id
			WHERE re.actor_person_id = ANY($1::uuid[])
			  AND COALESCE(re.target_person_id, c.author_id) = ANY($1::uuid[])
			  AND re.actor_person_id <> COALESCE(re.target_person_id, c.author_id)
		), edge_stats AS (
			SELECT actor_person_id,
			       target_person_id,
			       action_type,
			       COUNT(*) AS weight,
			       MIN(occurred_at) AS first_seen,
			       MAX(occurred_at) AS last_seen
			FROM matched_events
			GROUP BY actor_person_id, target_person_id, action_type
		), edge_evidence AS (
			SELECT me.actor_person_id,
			       me.target_person_id,
			       me.action_type,
			       ARRAY_AGG(DISTINCT evidence_id) AS evidence_ids
			FROM matched_events me
			CROSS JOIN LATERAL UNNEST(me.evidence_ids) AS evidence_id
			GROUP BY me.actor_person_id, me.target_person_id, me.action_type
		)
		SELECT 
			es.actor_person_id::text,
			es.target_person_id::text,
			es.action_type,
			es.weight,
			COALESCE(ee.evidence_ids, '{}'::uuid[]),
			es.first_seen,
			es.last_seen
		FROM edge_stats es
		LEFT JOIN edge_evidence ee
		  ON ee.actor_person_id = es.actor_person_id
		 AND ee.target_person_id = es.target_person_id
		 AND ee.action_type = es.action_type`

	rows, err := r.DB.Query(ctx, eventQuery, personSlice)
	if err != nil {
		return nil, fmt.Errorf("load interaction edges: %w", err)
	}
	for rows.Next() {
		var actorID, targetID, actionType string
		var weight int
		var evIDs []string
		var firstSeen, lastSeen time.Time
		if err := rows.Scan(&actorID, &targetID, &actionType, &weight, &evIDs, &firstSeen, &lastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan interaction edge: %w", err)
		}
		sKey := "person:" + actorID
		tKey := "person:" + targetID
		cost := calculateEdgeCost(actionType, weight, lastSeen, req.Profile, 0)
		edge := routingEdge{
			source:       sKey,
			target:       tKey,
			relationType: actionType,
			weight:       weight,
			cost:         cost,
			evidenceIDs:  evIDs,
			firstSeen:    firstSeen,
			lastSeen:     lastSeen,
		}
		g.addEdge(edge)
		// Directional evidence remains directional; the reverse traversal has a
		// small cost penalty but retains the original evidence and action type.
		revEdge := edge
		revEdge.source = tKey
		revEdge.target = sKey
		revEdge.cost = cost * 1.05
		revEdge.reversed = true
		g.addEdge(revEdge)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate interaction edges: %w", err)
	}
	rows.Close()

	// 2. Group membership edges among graph nodes. Groups are represented as
	// explicit nodes, avoiding the O(n^2) person-pair explosion of large groups.
	groupQuery := `
		WITH active_memberships AS (
			SELECT gm.person_id, gm.group_id, gm.raw_record_id, gm.valid_from, gm.valid_to
			FROM group_memberships gm
			WHERE gm.person_id = ANY($1::uuid[])
		), group_sizes AS (
			SELECT group_id, COUNT(*)::int AS member_count
			FROM group_memberships
			WHERE group_id IN (SELECT DISTINCT group_id FROM active_memberships)
			GROUP BY group_id
		)
		SELECT 
			gm.person_id::text,
			g.id::text, 
			COALESCE(g.platform_group_id, ''), 
			COALESCE(g.group_name, '未命名群'),
			gs.member_count,
			ARRAY_REMOVE(ARRAY[gm.raw_record_id], NULL) AS evidence_ids,
			gm.valid_from AS first_seen,
			COALESCE(gm.valid_to, gm.valid_from) AS last_seen
		FROM active_memberships gm
		JOIN "groups" g ON g.id = gm.group_id
		LEFT JOIN group_sizes gs ON gs.group_id = g.id
		LIMIT NULLIF($2, 0)`

	gRows, err := r.DB.Query(ctx, groupQuery, personSlice, r.Policy.MaxGroupMemberships)
	if err != nil {
		return nil, fmt.Errorf("load shared-group edges: %w", err)
	}
	groupEdgeCount := 0
	for gRows.Next() {
		var personID, gid, platformGID, gname string
		var memberCount int
		var evidenceIDs []string
		var firstSeen, lastSeen time.Time
		if err := gRows.Scan(&personID, &gid, &platformGID, &gname, &memberCount, &evidenceIDs, &firstSeen, &lastSeen); err != nil {
			gRows.Close()
			return nil, fmt.Errorf("scan group membership edge: %w", err)
		}
		groupEdgeCount++
		if req.ExcludeLargeGroups && memberCount > req.LargeGroupThreshold {
			continue
		}
		personKey := "person:" + personID
		groupKey := "group:" + gid
		cost := calculateEdgeCost("member_of", 1, lastSeen, req.Profile, memberCount) / 2
		edge := routingEdge{
			source:       personKey,
			target:       groupKey,
			relationType: "member_of",
			weight:       1,
			cost:         cost,
			evidenceIDs:  evidenceIDs,
			firstSeen:    firstSeen,
			lastSeen:     lastSeen,
		}
		g.addEdge(edge)

		// Membership is traversable in both directions. A person -> group ->
		// person traversal counts as one logical relationship hop.
		revEdge := edge
		revEdge.source = groupKey
		revEdge.target = personKey
		g.addEdge(revEdge)
	}
	if err := gRows.Err(); err != nil {
		gRows.Close()
		return nil, fmt.Errorf("iterate group membership edges: %w", err)
	}
	gRows.Close()
	if r.Policy.MaxGroupMemberships > 0 && groupEdgeCount >= r.Policy.MaxGroupMemberships {
		g.markTruncated("max_group_memberships")
	}

	return g, nil
}

func (g *routingGraph) addEdge(e routingEdge) {
	key := e.source + "\x00" + e.target + "\x00" + e.relationType
	if existing, ok := g.edgeMap[key]; ok {
		if e.cost < existing.cost {
			g.edgeMap[key] = e
			for i := range g.adj[e.source] {
				candidate := g.adj[e.source][i]
				if candidate.target == e.target && candidate.relationType == e.relationType {
					g.adj[e.source][i] = e
					break
				}
			}
		}
	} else {
		g.edgeMap[key] = e
		g.adj[e.source] = append(g.adj[e.source], e)
	}
}

// calculateEdgeCost models resistance based on relation type, weight, freshness, and group size.
func calculateEdgeCost(actionType string, weight int, lastSeen time.Time, profile string, memberCount int) float64 {
	if profile == "shortest" {
		return 1.0
	}

	var typeAlpha float64
	switch actionType {
	case "sent_message":
		typeAlpha = 0.5 // High directness
	case "commented", "replied_to":
		typeAlpha = 0.8
	case "liked":
		typeAlpha = 1.0
	case "mentioned":
		typeAlpha = 1.1
	case "member_of":
		if memberCount <= 20 {
			typeAlpha = 1.2
		} else if memberCount <= 100 {
			typeAlpha = 2.5
		} else {
			typeAlpha = 6.0 // High cost for massive groups
		}
		if profile == "covert" {
			typeAlpha *= 3.0 // Strongly penalize public groups in covert mode
		}
	default:
		typeAlpha = 2.0
	}

	// Time decay factor beta (last 30 days = 1.0, 1 year ago = 1.5, 3 years ago = 2.5)
	monthsAgo := time.Since(lastSeen).Hours() / (24 * 30)
	beta := 1.0 + (monthsAgo * 0.05)
	if beta > 3.0 {
		beta = 3.0
	}

	weightFactor := 1.0 / math.Max(1.0, math.Sqrt(float64(weight)))
	return typeAlpha * beta * weightFactor
}

// ─── Path Finding Algorithms ──────────────────────────────────────────────────

type rawPath struct {
	nodes []string
	edges []routingEdge
	cost  float64
}

type pqItem struct {
	node  string
	cost  float64
	hops  int
	path  []string
	edges []routingEdge
	index int
}

type priorityQueue []*pqItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].cost < pq[j].cost }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *priorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*pqItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// findKShortestPaths finds up to K loopless shortest paths using Yen's algorithm.
func (r Router) findKShortestPaths(g *routingGraph, source, target string, k, maxHops int, profile string, avoid map[string]bool) []rawPath {
	if avoid[source] || avoid[target] {
		return nil
	}

	// 1. Find the first shortest path using Dijkstra
	p0 := dijkstra(g, source, target, maxHops, avoid, nil)
	if p0 == nil {
		return nil
	}

	A := []rawPath{*p0}
	B := make(map[string]rawPath)

	for i := 1; i < k; i++ {
		prevPath := A[i-1]
		for spurNodeIdx := 0; spurNodeIdx < len(prevPath.nodes)-1; spurNodeIdx++ {
			spurNode := prevPath.nodes[spurNodeIdx]
			rootPath := prevPath.nodes[:spurNodeIdx+1]
			rootEdges := prevPath.edges[:spurNodeIdx]

			// Exclude edges that coincide with previous paths
			excludedEdges := make(map[string]bool)
			for _, p := range A {
				if len(p.nodes) > spurNodeIdx && sliceEqual(p.nodes[:spurNodeIdx+1], rootPath) {
					eKey := p.nodes[spurNodeIdx] + "->" + p.nodes[spurNodeIdx+1]
					excludedEdges[eKey] = true
				}
			}

			// Exclude root nodes (except spur node) to prevent loops
			tempAvoid := make(map[string]bool)
			for nodeKey, isAvoid := range avoid {
				tempAvoid[nodeKey] = isAvoid
			}
			for _, n := range rootPath[:len(rootPath)-1] {
				tempAvoid[n] = true
			}

			remainingHops := maxHops - logicalHopCount(rootEdges)
			spurPath := dijkstra(g, spurNode, target, remainingHops, tempAvoid, excludedEdges)

			if spurPath != nil {
				totalNodes := append(append([]string(nil), rootPath...), spurPath.nodes[1:]...)
				totalEdges := append(append([]routingEdge(nil), rootEdges...), spurPath.edges...)
				totalCost := 0.0
				for _, e := range totalEdges {
					totalCost += e.cost
				}
				fullCandidate := rawPath{
					nodes: totalNodes,
					edges: totalEdges,
					cost:  totalCost,
				}
				key := strings.Join(totalNodes, "->")
				if _, exists := B[key]; !exists {
					B[key] = fullCandidate
				}
			}
		}

		if len(B) == 0 {
			break
		}

		// Pick the lowest cost path from B
		var bestKey string
		bestCost := math.MaxFloat64
		for key, candidate := range B {
			if candidate.cost < bestCost {
				bestCost = candidate.cost
				bestKey = key
			}
		}
		A = append(A, B[bestKey])
		delete(B, bestKey)
	}

	return A
}

func dijkstra(g *routingGraph, source, target string, maxHops int, avoid map[string]bool, excludedEdges map[string]bool) *rawPath {
	pq := &priorityQueue{}
	heap.Init(pq)

	heap.Push(pq, &pqItem{
		node:  source,
		cost:  0,
		hops:  0,
		path:  []string{source},
		edges: []routingEdge{},
	})

	dist := map[string]float64{routingStateKey(source, 0): 0}

	for pq.Len() > 0 {
		top := heap.Pop(pq).(*pqItem)

		if top.node == target {
			return &rawPath{
				nodes: top.path,
				edges: top.edges,
				cost:  top.cost,
			}
		}

		if top.hops >= maxHops && !strings.HasPrefix(top.node, "group:") {
			continue
		}

		for _, edge := range g.adj[top.node] {
			neighbor := edge.target
			if avoid != nil && avoid[neighbor] {
				continue
			}
			eKey := top.node + "->" + neighbor
			if excludedEdges != nil && excludedEdges[eKey] {
				continue
			}
			// Avoid immediate cycles in path
			hasCycle := false
			for _, pNode := range top.path {
				if pNode == neighbor {
					hasCycle = true
					break
				}
			}
			if hasCycle {
				continue
			}

			nextHops := top.hops + edgeHopIncrement(edge)
			if nextHops > maxHops {
				continue
			}
			nextCost := top.cost + edge.cost
			stateKey := routingStateKey(neighbor, nextHops)
			if d, ok := dist[stateKey]; !ok || nextCost < d {
				dist[stateKey] = nextCost
				newPath := append(append([]string(nil), top.path...), neighbor)
				newEdges := append(append([]routingEdge(nil), top.edges...), edge)
				heap.Push(pq, &pqItem{
					node:  neighbor,
					cost:  nextCost,
					hops:  nextHops,
					path:  newPath,
					edges: newEdges,
				})
			}
		}
	}

	return nil
}

func edgeHopIncrement(edge routingEdge) int {
	if strings.HasPrefix(edge.target, "group:") {
		return 0
	}
	return 1
}

func logicalHopCount(edges []routingEdge) int {
	total := 0
	for _, edge := range edges {
		total += edgeHopIncrement(edge)
	}
	return total
}

func routingStateKey(node string, hops int) string {
	return fmt.Sprintf("%s\x00%d", node, hops)
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ─── Route Enrichment & Turn-by-turn Step Generation ─────────────────────────

func (r Router) enrichPlannedRoutes(ctx context.Context, rawPaths []rawPath, req RouteRequest) ([]PlannedRoute, error) {
	if len(rawPaths) == 0 {
		return nil, nil
	}

	// Collect all unique node keys across all paths
	nodeKeys := make(map[string]bool)
	for _, p := range rawPaths {
		for _, n := range p.nodes {
			nodeKeys[n] = true
		}
	}

	// Load node metadata using EgoBuilder helper
	builder := EgoBuilder{DB: r.DB}
	nodes, err := builder.loadNodes(ctx, nodeKeys)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		nodeMap[n.Key] = n
	}

	routes := make([]PlannedRoute, 0, len(rawPaths))
	for idx, rp := range rawPaths {
		title := fmt.Sprintf("推荐路线 %d", idx+1)
		switch idx {
		case 0:
			title += " · 最强加权主干道"
		case 1:
			title += " · 备选连通链路"
		case 2:
			title += " · 隐蔽绕行通道"
		}
		if req.Profile == "covert" && idx == 0 {
			title = "隐秘通道 · 避开公开大群"
		} else if req.Profile == "shortest" && idx == 0 {
			title = "极简直达 · 最少中转跳数"
		}

		steps := make([]RouteStep, 0, len(rp.edges))
		totalWeight := 0
		for stepIdx, edge := range rp.edges {
			totalWeight += edge.weight
			sNode := nodeMap[edge.source]
			tNode := nodeMap[edge.target]

			sMeta, _ := sNode.Metadata.(map[string]any)
			tMeta, _ := tNode.Metadata.(map[string]any)

			mediumType, summary := formatStepSummary(sNode, tNode, edge.relationType, edge.weight, edge.firstSeen, edge.lastSeen, edge.reversed)
			evidenceDirection := "forward"
			evidenceActorKey := edge.source
			evidenceTargetKey := edge.target
			if edge.reversed {
				evidenceDirection = "reverse"
				evidenceActorKey = edge.target
				evidenceTargetKey = edge.source
			}

			steps = append(steps, RouteStep{
				StepNumber:        stepIdx + 1,
				SourceKey:         edge.source,
				SourceType:        sNode.Type,
				SourceLabel:       sNode.Label,
				SourceMeta:        sMeta,
				TargetKey:         edge.target,
				TargetType:        tNode.Type,
				TargetLabel:       tNode.Label,
				TargetMeta:        tMeta,
				RelationType:      edge.relationType,
				Weight:            edge.weight,
				Cost:              math.Round(edge.cost*100) / 100,
				MediumType:        mediumType,
				MediumSummary:     summary,
				EvidenceIDs:       edge.evidenceIDs,
				EvidenceDirection: evidenceDirection,
				EvidenceActorKey:  evidenceActorKey,
				EvidenceTargetKey: evidenceTargetKey,
				FirstSeen:         edge.firstSeen,
				LastSeen:          edge.lastSeen,
			})
		}

		logicalHops := logicalHopCount(rp.edges)
		// Confidence score between 30 and 98 based on cost and logical person hops.
		confidence := int(math.Max(30, math.Min(98, 100.0-(rp.cost*8.0)-float64(logicalHops*5))))

		// Extract route nodes and edges for Cytoscape highlight
		routeNodes := make([]Node, 0, len(rp.nodes))
		for _, nk := range rp.nodes {
			if n, ok := nodeMap[nk]; ok {
				routeNodes = append(routeNodes, n)
			}
		}
		routeEdges := make([]Edge, 0, len(rp.edges))
		for _, e := range rp.edges {
			evidenceDirection := "forward"
			evidenceActorKey := e.source
			evidenceTargetKey := e.target
			if e.reversed {
				evidenceDirection = "reverse"
				evidenceActorKey = e.target
				evidenceTargetKey = e.source
			}
			routeEdges = append(routeEdges, Edge{
				Source:            e.source,
				Target:            e.target,
				RelationType:      e.relationType,
				Weight:            e.weight,
				EventCount:        e.weight,
				EvidenceIDs:       e.evidenceIDs,
				EvidenceDirection: evidenceDirection,
				EvidenceActorKey:  evidenceActorKey,
				EvidenceTargetKey: evidenceTargetKey,
				FirstSeen:         e.firstSeen,
				LastSeen:          e.lastSeen,
			})
		}

		summary := fmt.Sprintf("总计 %d 跳中转，综合置信度 %d%%，累计发现 %d 条交互证据记录", logicalHops, confidence, totalWeight)

		routes = append(routes, PlannedRoute{
			PathIndex:   idx,
			Title:       title,
			TotalHops:   logicalHops,
			TotalWeight: totalWeight,
			TotalCost:   math.Round(rp.cost*100) / 100,
			Confidence:  confidence,
			Steps:       steps,
			Nodes:       routeNodes,
			Edges:       routeEdges,
			Summary:     summary,
		})
	}

	// Sort routes by cost ascending
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].TotalCost < routes[j].TotalCost
	})
	for i := range routes {
		routes[i].PathIndex = i
	}

	return routes, nil
}

func formatStepSummary(src, tgt Node, relType string, weight int, first, last time.Time, reversed bool) (mediumType string, summary string) {
	timeStr := last.Format("2006-01-02")
	switch relType {
	case "member_of", "co_member", "group_membership":
		mediumType = "group_membership"
		if tgt.Type == "group" {
			summary = fmt.Sprintf("%s 是群 %s 的成员（记录起于 %s）", src.Label, tgt.Label, first.Format("2006-01-02"))
		} else if src.Type == "group" {
			summary = fmt.Sprintf("%s 是群 %s 的成员（记录起于 %s）", tgt.Label, src.Label, first.Format("2006-01-02"))
		} else {
			summary = fmt.Sprintf("双方存在共同群成员记录（记录起于 %s）", first.Format("2006-01-02"))
		}
	case "liked", "qzone_like":
		mediumType = "qzone_like"
		summary = fmt.Sprintf("存在 QQ 空间说说点赞互动共 %d 次（最近一次: %s）", weight, timeStr)
	case "commented", "qzone_comment":
		mediumType = "qzone_comment"
		summary = fmt.Sprintf("发表 QQ 空间说说评论互动共 %d 次（最近一次: %s）", weight, timeStr)
	case "feed_reply":
		mediumType = "qzone_reply"
		summary = fmt.Sprintf("QQ 空间说说评论楼中楼回复共 %d 次（最近一次: %s）", weight, timeStr)
	case "replied_to", "quote_reply":
		mediumType = "quote_reply"
		summary = fmt.Sprintf("群聊消息引用回复互动共 %d 次（最近一次: %s）", weight, timeStr)
	case "sent_message", "message":
		mediumType = "direct_message"
		summary = fmt.Sprintf("存在直接私聊/群会话消息发送共 %d 条（最近一次: %s）", weight, timeStr)
	case "mentioned", "at_message":
		mediumType = "mention"
		summary = fmt.Sprintf("群聊@提及定向发言互动共 %d 次（最近一次: %s）", weight, timeStr)
	case "published", "published_feed":
		mediumType = "qzone_post"
		summary = fmt.Sprintf("发布 QQ 空间说说动态共 %d 条（最近一次: %s）", weight, timeStr)
	case "friend_visible", "qzone_friend":
		mediumType = "qzone_friend"
		summary = fmt.Sprintf("存在 QQ 空间好友动态流可见关联共 %d 次（最近一次: %s）", weight, timeStr)
	default:
		mediumType = relType
		summary = fmt.Sprintf("发生关联动作 %s 共 %d 次", relType, weight)
	}
	if reversed && relType != "member_of" && relType != "co_member" && relType != "group_membership" {
		summary = fmt.Sprintf("反向追溯原始证据（%s → %s）：%s", tgt.Label, src.Label, summary)
	}
	return mediumType, summary
}
