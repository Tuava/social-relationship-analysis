package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

// graphPersonDepths treats content, conversations, and groups as relationship
// contexts. Entering another person advances one degree; entering a context
// does not. This keeps target -> post -> commenter at one relationship degree.
func graphPersonDepths(graph analysis.Graph, targetQQ string) map[int][]string {
	nodeTypes := make(map[string]string, len(graph.Nodes))
	targetKey := ""
	for _, node := range graph.Nodes {
		nodeTypes[node.Key] = node.Type
		metadata, _ := node.Metadata.(map[string]any)
		if node.Type == "person" && fmt.Sprint(metadata["qq"]) == targetQQ {
			targetKey = node.Key
		}
	}
	if targetKey == "" {
		return nil
	}
	adjacency := make(map[string][]string, len(graph.Nodes))
	for _, edge := range graph.Edges {
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		adjacency[edge.Target] = append(adjacency[edge.Target], edge.Source)
	}
	const unreachable = int(^uint(0) >> 1)
	distance := make(map[string]int, len(graph.Nodes))
	for key := range nodeTypes {
		distance[key] = unreachable
	}
	distance[targetKey] = 0
	queue := []string{targetKey}
	inQueue := map[string]bool{targetKey: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		inQueue[current] = false
		for _, next := range adjacency[current] {
			cost := 0
			if nodeTypes[next] == "person" && next != targetKey {
				cost = 1
			}
			candidate := distance[current] + cost
			if candidate >= distance[next] {
				continue
			}
			distance[next] = candidate
			if !inQueue[next] {
				queue = append(queue, next)
				inQueue[next] = true
			}
		}
	}
	result := map[int][]string{}
	for key, depth := range distance {
		if nodeTypes[key] != "person" || depth == unreachable || depth > 99 {
			continue
		}
		result[depth] = append(result[depth], strings.TrimPrefix(key, "person:"))
	}
	return result
}

func reprioritizeGraphMedia(ctx context.Context, tx pgx.Tx, graph analysis.Graph, targetQQ string) error {
	for depth, personIDs := range graphPersonDepths(graph, targetQQ) {
		if len(personIDs) == 0 {
			continue
		}
		boost := 100 - depth*10
		if boost < 0 {
			boost = 0
		}
		reason := fmt.Sprintf("ego_depth_%d", depth)
		_, err := tx.Exec(ctx, `UPDATE media_references mr SET
			relation_depth=LEAST(mr.relation_depth,$2),
			priority_reason=CASE WHEN $2 < mr.relation_depth OR mr.priority_reason='global' THEN $3 ELSE mr.priority_reason END,
			priority_boost=GREATEST(mr.priority_boost,$4)
			WHERE mr.person_id=ANY($1::uuid[])
			   OR EXISTS(SELECT 1 FROM messages m WHERE m.id=mr.message_id AND m.sender_id=ANY($1::uuid[]))
			   OR EXISTS(SELECT 1 FROM contents c WHERE c.id=mr.content_id AND c.author_id=ANY($1::uuid[]))`,
			personIDs, depth, reason, boost)
		if err != nil {
			return err
		}
	}
	return nil
}

func reprioritizeGraphMediaPool(ctx context.Context, db *pgxpool.Pool, graph analysis.Graph, targetQQ string) error {
	for depth, personIDs := range graphPersonDepths(graph, targetQQ) {
		if len(personIDs) == 0 {
			continue
		}
		boost := 100 - depth*10
		if boost < 0 {
			boost = 0
		}
		reason := fmt.Sprintf("ego_depth_%d", depth)
		_, err := db.Exec(ctx, `UPDATE media_references mr SET
			relation_depth=LEAST(mr.relation_depth,$2),
			priority_reason=CASE WHEN $2 < mr.relation_depth OR mr.priority_reason='global' THEN $3 ELSE mr.priority_reason END,
			priority_boost=GREATEST(mr.priority_boost,$4)
			WHERE mr.person_id=ANY($1::uuid[])
			   OR EXISTS(SELECT 1 FROM messages m WHERE m.id=mr.message_id AND m.sender_id=ANY($1::uuid[]))
			   OR EXISTS(SELECT 1 FROM contents c WHERE c.id=mr.content_id AND c.author_id=ANY($1::uuid[]))`,
			personIDs, depth, reason, boost)
		if err != nil {
			return err
		}
	}
	return nil
}
