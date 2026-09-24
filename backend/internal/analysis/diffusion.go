package analysis

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/media"
)

type ImageDiffusionResult struct {
	AssetID          string                  `json:"asset_id"`
	SHA256           string                  `json:"sha256"`
	PHash            string                  `json:"phash"`
	TotalOccurrences int                     `json:"total_occurrences"`
	GroupCount       int                     `json:"group_count"`
	UserCount        int                     `json:"user_count"`
	FirstSeenAt      *time.Time              `json:"first_seen_at"`
	LastSeenAt       *time.Time              `json:"last_seen_at"`
	Timeline         []DiffusionTimelineItem `json:"timeline"`
	Graph            DiffusionGraph          `json:"graph"`
}

type DiffusionTimelineItem struct {
	ID             string     `json:"id"`
	OccurredAt     *time.Time `json:"occurred_at"`
	ContextType    string     `json:"context_type"` // 'group' or 'private' or 'qzone'
	GroupID        string     `json:"group_id,omitempty"`
	GroupName      string     `json:"group_name,omitempty"`
	SenderQQ       string     `json:"sender_qq"`
	SenderName     string     `json:"sender_name"`
	MessageSnippet string     `json:"message_snippet,omitempty"`
	MatchMethod    string     `json:"match_method"` // 'exact_hash' or 'phash_similarity'
	HammingDist    int        `json:"hamming_dist"`
}

type DiffusionGraph struct {
	Nodes []DiffusionNode `json:"nodes"`
	Edges []DiffusionEdge `json:"edges"`
}

type DiffusionNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // 'user', 'group', 'post'
	Label string `json:"label"`
}

type DiffusionEdge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Timestamp string `json:"timestamp"`
	Relation  string `json:"relation"`
}

// TraceImageDiffusion traces the propagation of an image asset across conversations and feeds.
func TraceImageDiffusion(ctx context.Context, pool *pgxpool.Pool, assetIDOrHash string) (*ImageDiffusionResult, error) {
	var targetAssetID, targetSHA, targetPHash string
	err := pool.QueryRow(ctx, `
		SELECT id::text, sha256, COALESCE(phash, '')
		FROM media_assets
		WHERE id::text = $1 OR sha256 = $1
		LIMIT 1
	`, assetIDOrHash).Scan(&targetAssetID, &targetSHA, &targetPHash)
	if err != nil {
		return nil, fmt.Errorf("find target media asset: %w", err)
	}

	// 1. Fetch exact occurrences in messages
	rows, err := pool.Query(ctx, `
		SELECT m.id::text, m.sent_at, COALESCE(c.conversation_type, 'group'),
		       COALESCE(c.platform_conversation_id, ''), COALESCE(g.group_name, ''),
		       COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, ''),
		       COALESCE(LEFT(m.raw_text, 100), ''), 'exact_hash', 0
		FROM message_media mm
		JOIN messages m ON m.id = mm.message_id
		LEFT JOIN conversations c ON c.id = m.conversation_id
		LEFT JOIN groups g ON g.platform_group_id = c.platform_conversation_id
		LEFT JOIN persons p ON p.id = m.sender_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE mm.media_asset_id = $1::uuid
		ORDER BY m.sent_at ASC NULLS LAST
	`, targetAssetID)
	if err != nil {
		return nil, fmt.Errorf("query exact message diffusion: %w", err)
	}
	defer rows.Close()

	var timeline []DiffusionTimelineItem
	groupsSet := make(map[string]bool)
	usersSet := make(map[string]bool)
	var firstSeen, lastSeen *time.Time

	for rows.Next() {
		var item DiffusionTimelineItem
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.ContextType,
			&item.GroupID, &item.GroupName, &item.SenderQQ, &item.SenderName,
			&item.MessageSnippet, &item.MatchMethod, &item.HammingDist); err != nil {
			return nil, err
		}
		if item.GroupID != "" {
			groupsSet[item.GroupID] = true
		}
		if item.SenderQQ != "" {
			usersSet[item.SenderQQ] = true
		}
		if item.OccurredAt != nil {
			if firstSeen == nil || item.OccurredAt.Before(*firstSeen) {
				firstSeen = item.OccurredAt
			}
			if lastSeen == nil || item.OccurredAt.After(*lastSeen) {
				lastSeen = item.OccurredAt
			}
		}
		timeline = append(timeline, item)
	}
	rows.Close()

	// 2. If target has a valid pHash, look for similar visual assets across the entire database
	if len(targetPHash) == 16 {
		phashRows, err := pool.Query(ctx, `
			SELECT ma.id::text, ma.phash
			FROM media_assets ma
			WHERE ma.phash IS NOT NULL AND ma.phash != '' AND ma.id != $1::uuid
			LIMIT 500
		`, targetAssetID)
		if err == nil {
			defer phashRows.Close()
			var matchedSimilarAssetIDs []string
			for phashRows.Next() {
				var aid, ph string
				if err := phashRows.Scan(&aid, &ph); err == nil {
					dist := media.HammingDistance(targetPHash, ph)
					if dist >= 0 && dist <= 6 { // high perceptual similarity
						matchedSimilarAssetIDs = append(matchedSimilarAssetIDs, aid)
					}
				}
			}
			phashRows.Close()

			if len(matchedSimilarAssetIDs) > 0 {
				simRows, err := pool.Query(ctx, `
					SELECT m.id::text, m.sent_at, COALESCE(c.conversation_type, 'group'),
					       COALESCE(c.platform_conversation_id, ''), COALESCE(g.group_name, ''),
					       COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, ''),
					       COALESCE(LEFT(m.raw_text, 100), ''), 'phash_similarity', 4
					FROM message_media mm
					JOIN messages m ON m.id = mm.message_id
					LEFT JOIN conversations c ON c.id = m.conversation_id
					LEFT JOIN groups g ON g.platform_group_id = c.platform_conversation_id
					LEFT JOIN persons p ON p.id = m.sender_id
					LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
					WHERE mm.media_asset_id = ANY($1::uuid[])
					ORDER BY m.sent_at ASC NULLS LAST
				`, matchedSimilarAssetIDs)
				if err == nil {
					defer simRows.Close()
					for simRows.Next() {
						var item DiffusionTimelineItem
						if err := simRows.Scan(&item.ID, &item.OccurredAt, &item.ContextType,
							&item.GroupID, &item.GroupName, &item.SenderQQ, &item.SenderName,
							&item.MessageSnippet, &item.MatchMethod, &item.HammingDist); err == nil {
							if item.GroupID != "" {
								groupsSet[item.GroupID] = true
							}
							if item.SenderQQ != "" {
								usersSet[item.SenderQQ] = true
							}
							timeline = append(timeline, item)
						}
					}
				}
			}
		}
	}

	// 3. Build graph representation
	nodesMap := make(map[string]DiffusionNode)
	edges := make([]DiffusionEdge, 0, len(timeline))

	for i, item := range timeline {
		userNodeID := "u:" + item.SenderQQ
		if item.SenderQQ != "" {
			if _, exists := nodesMap[userNodeID]; !exists {
				label := item.SenderName
				if label == "" {
					label = item.SenderQQ
				}
				nodesMap[userNodeID] = DiffusionNode{ID: userNodeID, Type: "user", Label: label}
			}
		}
		if item.GroupID != "" {
			groupNodeID := "g:" + item.GroupID
			if _, exists := nodesMap[groupNodeID]; !exists {
				label := item.GroupName
				if label == "" {
					label = "群 " + item.GroupID
				}
				nodesMap[groupNodeID] = DiffusionNode{ID: groupNodeID, Type: "group", Label: label}
			}
			tStr := ""
			if item.OccurredAt != nil {
				tStr = item.OccurredAt.Format(time.RFC3339)
			}
			edges = append(edges, DiffusionEdge{
				Source:    userNodeID,
				Target:    groupNodeID,
				Timestamp: tStr,
				Relation:  "posted_to",
			})
		}
		if i > 0 && timeline[i-1].SenderQQ != item.SenderQQ && item.SenderQQ != "" {
			tStr := ""
			if item.OccurredAt != nil {
				tStr = item.OccurredAt.Format(time.RFC3339)
			}
			edges = append(edges, DiffusionEdge{
				Source:    "u:" + timeline[i-1].SenderQQ,
				Target:    userNodeID,
				Timestamp: tStr,
				Relation:  "propagated_after",
			})
		}
	}

	nodes := make([]DiffusionNode, 0, len(nodesMap))
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}

	return &ImageDiffusionResult{
		AssetID:          targetAssetID,
		SHA256:           targetSHA,
		PHash:            targetPHash,
		TotalOccurrences: len(timeline),
		GroupCount:       len(groupsSet),
		UserCount:        len(usersSet),
		FirstSeenAt:      firstSeen,
		LastSeenAt:       lastSeen,
		Timeline:         timeline,
		Graph: DiffusionGraph{
			Nodes: nodes,
			Edges: edges,
		},
	}, nil
}
