package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EgoBuilder struct{ DB *pgxpool.Pool }

// BuildOptions limits graph construction so a dense group cannot take over the
// whole graph. Group membership events are represented as group edges, but a
// group is never used as a traversal queue item.
type BuildOptions struct {
	MaxNodes  int    `json:"max_nodes,omitempty"`
	MaxEdges  int    `json:"max_edges,omitempty"`
	MaxEvents int    `json:"max_events,omitempty"`
	GroupUUID string `json:"group_uuid,omitempty"`
}

const (
	MinDepth          = 1
	DefaultBuildDepth = 2
)

func (o BuildOptions) WithDefaults() BuildOptions {
	// Zero means unlimited. The collection scope and recursion depth are the
	// default boundaries; these limits are optional emergency safeguards.
	if o.MaxNodes < 0 {
		o.MaxNodes = 0
	}
	if o.MaxEdges < 0 {
		o.MaxEdges = 0
	}
	if o.MaxEvents < 0 {
		o.MaxEvents = 0
	}
	return o
}

func nullableUUID(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

type Node struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Label    string `json:"label"`
	Metadata any    `json:"metadata"`
}

type Edge struct {
	Source            string    `json:"source"`
	Target            string    `json:"target"`
	RelationType      string    `json:"relation_type"`
	Weight            int       `json:"weight"`
	EventCount        int       `json:"event_count"`
	EvidenceIDs       []string  `json:"evidence_ids"`
	EvidenceDirection string    `json:"evidence_direction,omitempty"`
	EvidenceActorKey  string    `json:"evidence_actor_key,omitempty"`
	EvidenceTargetKey string    `json:"evidence_target_key,omitempty"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
}

type Graph struct {
	Nodes               []Node `json:"nodes"`
	Edges               []Edge `json:"edges"`
	Truncated           bool   `json:"truncated"`
	ProcessedEventCount int    `json:"processed_event_count"`
}

// Build preserves the original public API and uses bounded defaults.
func (b EgoBuilder) Build(ctx context.Context, targetQQ string, depth int) (Graph, error) {
	return b.BuildWithOptions(ctx, targetQQ, depth, BuildOptions{})
}

func (b EgoBuilder) BuildWithOptions(ctx context.Context, targetQQ string, depth int, options BuildOptions) (Graph, error) {
	options = options.WithDefaults()
	if depth < 1 {
		depth = 1
	}
	var target string
	if err := b.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1 LIMIT 1`, targetQQ).Scan(&target); err != nil {
		return Graph{}, fmt.Errorf("target QQ not found: %w", err)
	}

	keys := map[string]bool{"person:" + target: true}
	queue := []string{target}
	groupQueue := []string{}
	edges := []Edge{}
	edgeIndex := map[string]int{}
	processedEvents := 0
	truncated := false
	seenEvents := map[string]struct{}{}

	for level := 0; level < depth && (len(queue) > 0 || len(groupQueue) > 0) && !reachedBuildLimit(processedEvents, options.MaxEvents); level++ {
		next := []string{}
		nextGroups := []string{}
		contentToExpand := map[string]struct{}{}
		for _, person := range queue {
			if reachedBuildLimit(processedEvents, options.MaxEvents) {
				truncated = true
				break
			}

			// Read one extra row to distinguish an exact limit from a truncated
			// result. The ordering makes repeated builds deterministic.
			remaining := remainingBuildEvents(options.MaxEvents, processedEvents)
			rows, err := b.DB.Query(ctx, `
						SELECT re.id::text, re.action_type,
						       re.actor_person_id::text, re.target_person_id::text,
						       re.target_object_id::text, re.occurred_at, re.evidence_ids,
						       CASE
						         WHEN re.target_person_id IS NOT NULL THEN 'person'
						         WHEN re.action_type='member_of' THEN 'group'
						         WHEN tc.id IS NOT NULL THEN 'content'
						         WHEN tm.id IS NOT NULL THEN 'message'
						         WHEN cg.id IS NOT NULL THEN 'group'
						         ELSE 'conversation'
						       END,
						       COALESCE(cg.id::text,'')
						FROM relation_events re
						LEFT JOIN contents tc ON tc.id=re.target_object_id
						LEFT JOIN messages tm ON tm.id=re.target_object_id
						LEFT JOIN conversations cv ON cv.id=re.target_object_id
						LEFT JOIN groups cg ON cg.platform='qq' AND cv.conversation_type='group' AND cg.platform_group_id=cv.platform_conversation_id
				WHERE (re.actor_person_id=$1 OR re.target_person_id=$1)
				  AND ($2::uuid IS NULL OR (re.context_type='group' AND (
					 re.target_object_id=$2::uuid OR re.target_object_id IN (
						SELECT c.id FROM conversations c WHERE c.conversation_type='group' AND c.platform_conversation_id=(SELECT platform_group_id FROM groups WHERE id=$2::uuid)
					 )
				 )))
				ORDER BY re.occurred_at DESC, re.id DESC
				LIMIT $3`, person, nullableUUID(options.GroupUUID), remaining+1)
			if err != nil {
				return Graph{}, err
			}

			readRows := 0
			for rows.Next() {
				readRows++
				if reachedBuildLimit(processedEvents, options.MaxEvents) {
					truncated = true
					break
				}
				var eventID, typ, objectType, groupEntityID string
				var actorID, targetPersonID, targetObjectID *string
				var at time.Time
				var evidence []string
				if err := rows.Scan(&eventID, &typ, &actorID, &targetPersonID, &targetObjectID, &at, &evidence, &objectType, &groupEntityID); err != nil {
					rows.Close()
					return Graph{}, err
				}
				if _, seen := seenEvents[eventID]; seen {
					continue
				}
				seenEvents[eventID] = struct{}{}
				processedEvents++

				sourceKey, targetKey, discoveredKey, discoveredType := "", "", "", ""
				if actorID != nil && *actorID == person {
					sourceKey = "person:" + *actorID
					targetKey, discoveredType, _ = relationEndpoint(targetPersonID, targetObjectID, typ, objectType, groupEntityID)
					discoveredKey = targetKey
				} else if targetPersonID != nil && *targetPersonID == person && actorID != nil {
					// The target-side query keeps the event's real direction.
					sourceKey = "person:" + *actorID
					targetKey = "person:" + *targetPersonID
					discoveredKey, discoveredType = sourceKey, "person"
				}
				if sourceKey == "" || targetKey == "" {
					continue
				}

				if !keys[discoveredKey] {
					if reachedBuildLimit(len(keys), options.MaxNodes) {
						truncated = true
						continue
					}
					keys[discoveredKey] = true
					if discoveredType == "person" {
						next = append(next, strings.TrimPrefix(discoveredKey, "person:"))
					}
					if discoveredType == "group" {
						nextGroups = append(nextGroups, strings.TrimPrefix(discoveredKey, "group:"))
					}
				}
				if discoveredType == "content" {
					contentToExpand[strings.TrimPrefix(discoveredKey, "content:")] = struct{}{}
				}
				if !keys[targetKey] || (discoveredType == "person" && !keys[discoveredKey]) {
					// The node limit may have rejected the newly discovered endpoint;
					// never persist an edge that cannot be rendered or traced.
					continue
				}

				edgeKey := sourceKey + "\x00" + targetKey + "\x00" + typ
				if index, ok := edgeIndex[edgeKey]; ok {
					edges[index].Weight++
					edges[index].EventCount++
					edges[index].EvidenceIDs = mergeEvidence(edges[index].EvidenceIDs, evidence)
					if at.Before(edges[index].FirstSeen) {
						edges[index].FirstSeen = at
					}
					if at.After(edges[index].LastSeen) {
						edges[index].LastSeen = at
					}
					continue
				}
				if reachedBuildLimit(len(edges), options.MaxEdges) {
					truncated = true
					continue
				}
				edgeIndex[edgeKey] = len(edges)
				edges = append(edges, Edge{Source: sourceKey, Target: targetKey, RelationType: typ, Weight: 1, EventCount: 1, EvidenceIDs: append([]string(nil), evidence...), FirstSeen: at, LastSeen: at})
				_ = eventID // retained in the source query for deterministic row identity.
			}
			if rows.Err() != nil {
				err := rows.Err()
				rows.Close()
				return Graph{}, err
			}
			if options.MaxEvents > 0 && readRows > remaining {
				truncated = true
			}
			rows.Close()
		}
		// A group is an actual traversal surface, not a terminal node. The old
		// builder stored the target's group edge but never loaded the other
		// members, which made a 20-level build stop at a tiny three-hop graph.
		// Membership events are expanded in the same level and discovered people
		// enter the next level. No member-count limit is applied here; callers use
		// explicit build options or depth when they need a narrower research scope.
		for _, groupID := range uniqueIDs(groupQueue) {
			if reachedBuildLimit(processedEvents, options.MaxEvents) {
				truncated = true
				break
			}
			remaining := remainingBuildEvents(options.MaxEvents, processedEvents)
			rows, err := b.DB.Query(ctx, `
				SELECT re.id::text,re.actor_person_id::text,re.occurred_at,re.evidence_ids
				FROM relation_events re
				WHERE re.action_type='member_of' AND re.target_object_id=$1
				ORDER BY re.occurred_at DESC,re.id DESC LIMIT $2`, groupID, remaining+1)
			if err != nil {
				return Graph{}, err
			}
			readRows := 0
			for rows.Next() {
				readRows++
				if reachedBuildLimit(processedEvents, options.MaxEvents) {
					truncated = true
					break
				}
				var eventID, personID string
				var at time.Time
				var evidence []string
				if err := rows.Scan(&eventID, &personID, &at, &evidence); err != nil {
					rows.Close()
					return Graph{}, err
				}
				if personID == "" {
					continue
				}
				if _, seen := seenEvents[eventID]; seen {
					continue
				}
				seenEvents[eventID] = struct{}{}
				processedEvents++
				personKey := "person:" + personID
				groupKey := "group:" + groupID
				if !keys[personKey] {
					if reachedBuildLimit(len(keys), options.MaxNodes) {
						truncated = true
						continue
					}
					keys[personKey] = true
					next = append(next, personID)
				}
				if !keys[groupKey] {
					continue
				}
				edgeKey := personKey + "\x00" + groupKey + "\x00member_of"
				if index, ok := edgeIndex[edgeKey]; ok {
					edges[index].Weight++
					edges[index].EventCount++
					edges[index].EvidenceIDs = mergeEvidence(edges[index].EvidenceIDs, evidence)
					if at.Before(edges[index].FirstSeen) {
						edges[index].FirstSeen = at
					}
					if at.After(edges[index].LastSeen) {
						edges[index].LastSeen = at
					}
					continue
				}
				if reachedBuildLimit(len(edges), options.MaxEdges) {
					truncated = true
					continue
				}
				edgeIndex[edgeKey] = len(edges)
				edges = append(edges, Edge{Source: personKey, Target: groupKey, RelationType: "member_of", Weight: 1, EventCount: 1, EvidenceIDs: append([]string(nil), evidence...), FirstSeen: at, LastSeen: at})
			}
			if rows.Err() != nil {
				err := rows.Err()
				rows.Close()
				return Graph{}, err
			}
			if options.MaxEvents > 0 && readRows > remaining {
				truncated = true
			}
			rows.Close()
		}
		// A person's QZone relationship is mediated by a post. Expand all
		// interactions on newly reached posts before moving to the next person
		// depth, while keeping the event's real person -> content direction.
		for contentID := range contentToExpand {
			if reachedBuildLimit(processedEvents, options.MaxEvents) {
				truncated = true
				break
			}
			remaining := remainingBuildEvents(options.MaxEvents, processedEvents)
			rows, err := b.DB.Query(ctx, `
				SELECT re.id::text,re.action_type,re.actor_person_id::text,re.occurred_at,re.evidence_ids
				FROM relation_events re
					WHERE re.target_object_id=$1 AND re.action_type IN ('published','commented','liked','replied_to')
				ORDER BY re.occurred_at DESC,re.id DESC LIMIT $2`, contentID, remaining+1)
			if err != nil {
				return Graph{}, err
			}
			readRows := 0
			for rows.Next() {
				readRows++
				if reachedBuildLimit(processedEvents, options.MaxEvents) {
					truncated = true
					break
				}
				var eventID, typ string
				var actorID *string
				var at time.Time
				var evidence []string
				if err := rows.Scan(&eventID, &typ, &actorID, &at, &evidence); err != nil {
					rows.Close()
					return Graph{}, err
				}
				if actorID == nil {
					continue
				}
				if _, seen := seenEvents[eventID]; seen {
					continue
				}
				seenEvents[eventID] = struct{}{}
				processedEvents++
				personKey := "person:" + *actorID
				contentKey := "content:" + contentID
				if !keys[personKey] {
					if reachedBuildLimit(len(keys), options.MaxNodes) {
						truncated = true
						continue
					}
					keys[personKey] = true
					next = append(next, *actorID)
				}
				edgeKey := personKey + "\x00" + contentKey + "\x00" + typ
				if index, ok := edgeIndex[edgeKey]; ok {
					edges[index].Weight++
					edges[index].EventCount++
					edges[index].EvidenceIDs = mergeEvidence(edges[index].EvidenceIDs, evidence)
					if at.Before(edges[index].FirstSeen) {
						edges[index].FirstSeen = at
					}
					if at.After(edges[index].LastSeen) {
						edges[index].LastSeen = at
					}
					continue
				}
				if reachedBuildLimit(len(edges), options.MaxEdges) {
					truncated = true
					continue
				}
				edgeIndex[edgeKey] = len(edges)
				edges = append(edges, Edge{Source: personKey, Target: contentKey, RelationType: typ, Weight: 1, EventCount: 1, EvidenceIDs: append([]string(nil), evidence...), FirstSeen: at, LastSeen: at})
			}
			if rows.Err() != nil {
				err := rows.Err()
				rows.Close()
				return Graph{}, err
			}
			if options.MaxEvents > 0 && readRows > remaining {
				truncated = true
			}
			rows.Close()
		}
		queue = uniqueIDs(next)
		groupQueue = uniqueIDs(nextGroups)
	}
	if reachedBuildLimit(processedEvents, options.MaxEvents) {
		truncated = true
	}

	nodes, err := b.loadNodes(ctx, keys)
	if err != nil {
		return Graph{}, err
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Key < nodes[j].Key })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source == edges[j].Source {
			if edges[i].Target == edges[j].Target {
				return edges[i].RelationType < edges[j].RelationType
			}
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Source < edges[j].Source
	})
	return Graph{Nodes: nodes, Edges: edges, Truncated: truncated, ProcessedEventCount: processedEvents}, nil
}

func (b EgoBuilder) loadNodes(ctx context.Context, keys map[string]bool) ([]Node, error) {
	byKey := make(map[string]Node, len(keys))
	ids := map[string][]string{"person": {}, "group": {}, "conversation": {}, "content": {}, "message": {}}
	for key := range keys {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		typ, id := parts[0], parts[1]
		byKey[key] = Node{Key: key, Type: typ, Label: id, Metadata: map[string]any{}}
		if _, ok := ids[typ]; ok {
			ids[typ] = append(ids[typ], id)
		}
	}
	if err := b.loadPersonNodes(ctx, ids["person"], byKey); err != nil {
		return nil, err
	}
	if err := b.loadGroupNodes(ctx, ids["group"], byKey); err != nil {
		return nil, err
	}
	if err := b.loadConversationNodes(ctx, ids["conversation"], byKey); err != nil {
		return nil, err
	}
	if err := b.loadContentNodes(ctx, ids["content"], byKey); err != nil {
		return nil, err
	}
	if err := b.loadMessageNodes(ctx, ids["message"], byKey); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(byKey))
	for _, node := range byKey {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (b EgoBuilder) loadPersonNodes(ctx context.Context, ids []string, nodes map[string]Node) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := b.DB.Query(ctx, `SELECT p.id::text,p.display_name,COALESCE(pi.platform_user_id,''),COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
		CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id ELSE '' END,
		'')
		FROM persons p LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE p.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, qq, avatar string
		if err := rows.Scan(&id, &name, &qq, &avatar); err != nil {
			return err
		}
		label := domain.CleanDisplayText(name)
		if label == "" {
			label = qq
		}
		if label == "" {
			label = id
		}
		nodes["person:"+id] = Node{Key: "person:" + id, Type: "person", Label: label, Metadata: map[string]any{"qq": qq, "avatar_uri": avatar}}
	}
	return rows.Err()
}

func (b EgoBuilder) loadGroupNodes(ctx context.Context, ids []string, nodes map[string]Node) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := b.DB.Query(ctx, `SELECT g.id::text,g.group_name,g.platform_group_id,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.group_id=g.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		CASE WHEN g.platform_group_id<>'' THEN '/api/v1/media/avatars/group/'||g.platform_group_id ELSE '' END,
		'') FROM groups g WHERE g.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, groupID, avatarURI string
		if err := rows.Scan(&id, &name, &groupID, &avatarURI); err != nil {
			return err
		}
		label := domain.CleanDisplayText(name)
		if label == "" {
			label = groupID
		}
		if label == "" {
			label = id
		}
		nodes["group:"+id] = Node{Key: "group:" + id, Type: "group", Label: label, Metadata: map[string]any{"group_id": groupID, "avatar_uri": avatarURI}}
	}
	return rows.Err()
}

func (b EgoBuilder) loadConversationNodes(ctx context.Context, ids []string, nodes map[string]Node) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := b.DB.Query(ctx, `SELECT c.id::text,c.conversation_type,c.platform_conversation_id,COALESCE(NULLIF(c.name,''),g.group_name,'') FROM conversations c LEFT JOIN groups g ON g.platform='qq' AND g.platform_group_id=c.platform_conversation_id WHERE c.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind, platformID, name string
		if err := rows.Scan(&id, &kind, &platformID, &name); err != nil {
			return err
		}
		name = domain.CleanDisplayText(name)
		if name == "" {
			name = kind + ":" + platformID
		}
		nodes["conversation:"+id] = Node{Key: "conversation:" + id, Type: "conversation", Label: name, Metadata: map[string]any{"conversation_type": kind, "platform_id": platformID}}
	}
	return rows.Err()
}

func (b EgoBuilder) loadContentNodes(ctx context.Context, ids []string, nodes map[string]Node) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := b.DB.Query(ctx, `SELECT c.id::text,c.body,c.platform_content_id,c.context_type,c.published_at,COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,'') FROM contents c LEFT JOIN persons p ON p.id=c.author_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE c.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, body, platformID, contextType, authorName, authorQQ string
		var publishedAt *time.Time
		if err := rows.Scan(&id, &body, &platformID, &contextType, &publishedAt, &authorName, &authorQQ); err != nil {
			return err
		}
		label := domain.CleanDisplayText(body)
		if len([]rune(label)) > 36 {
			label = string([]rune(label)[:36]) + "..."
		}
		if label == "" {
			label = "空间内容"
		}
		nodes["content:"+id] = Node{Key: "content:" + id, Type: "content", Label: label, Metadata: map[string]any{"content_id": id, "platform_id": platformID, "context_type": contextType, "published_at": publishedAt, "author": authorName, "author_qq": authorQQ}}
	}
	return rows.Err()
}

func (b EgoBuilder) loadMessageNodes(ctx context.Context, ids []string, nodes map[string]Node) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := b.DB.Query(ctx, `SELECT id::text,raw_text,COALESCE(source_message_id,''),sent_at,conversation_id::text FROM messages WHERE id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, text, sourceID string
		var sentAt *time.Time
		var conversationID *string
		if err := rows.Scan(&id, &text, &sourceID, &sentAt, &conversationID); err != nil {
			return err
		}
		label := domain.CleanDisplayText(text)
		if len([]rune(label)) > 36 {
			label = string([]rune(label)[:36]) + "..."
		}
		if label == "" {
			label = "Message"
		}
		nodes["message:"+id] = Node{Key: "message:" + id, Type: "message", Label: label, Metadata: map[string]any{"message_id": sourceID, "sent_at": sentAt, "conversation_id": conversationID}}
	}
	return rows.Err()
}

func relationEndpoint(targetPersonID, targetObjectID *string, actionType, objectType, groupEntityID string) (key, typ, id string) {
	if targetPersonID != nil {
		return "person:" + *targetPersonID, "person", *targetPersonID
	}
	if targetObjectID == nil {
		return "", "", ""
	}
	if objectType == "group" || actionType == "member_of" {
		if groupEntityID != "" {
			return "group:" + groupEntityID, "group", ""
		}
		return "group:" + *targetObjectID, "group", ""
	}
	if objectType == "content" {
		return "content:" + *targetObjectID, "content", *targetObjectID
	}
	if objectType == "message" {
		return "message:" + *targetObjectID, "message", *targetObjectID
	}
	return "conversation:" + *targetObjectID, "conversation", ""
}

func mergeEvidence(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, id := range existing {
		seen[id] = struct{}{}
	}
	for _, id := range incoming {
		if _, ok := seen[id]; !ok && id != "" {
			existing = append(existing, id)
			seen[id] = struct{}{}
		}
	}
	return existing
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func reachedBuildLimit(value, limit int) bool {
	return limit > 0 && value >= limit
}

func remainingBuildEvents(limit, processed int) int {
	if limit <= 0 {
		return int(^uint(0)>>1) - 1
	}
	return limit - processed
}
