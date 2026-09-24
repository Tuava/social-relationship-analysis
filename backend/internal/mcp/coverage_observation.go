package mcp

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// observedCoverageCounts reports data that is already present even when the
// collection run did not write a trustworthy coverage row for it. It never
// upgrades a collection boundary; callers expose these counts as inferred.
func observedCoverageCounts(ctx context.Context, db *pgxpool.Pool, personID string) map[string]int {
	queries := map[string]string{
		"basic_profile":     `SELECT COUNT(*) FROM person_profiles WHERE person_id=$1`,
		"group_memberships": `SELECT COUNT(*) FROM group_memberships WHERE person_id=$1`,
		"group_messages":    `SELECT COUNT(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='group'`,
		"private_messages":  `SELECT COUNT(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 AND c.conversation_type='private'`,
		"qzone_feeds":       `SELECT COUNT(*) FROM contents WHERE author_id=$1`,
		"qzone_comments":    `SELECT COUNT(*) FROM relation_events WHERE action_type='commented' AND actor_person_id=$1`,
		"qzone_likes":       `SELECT COUNT(*) FROM relation_events WHERE action_type='liked' AND actor_person_id=$1`,
		"qzone_visits":      `SELECT COUNT(*) FROM relation_events WHERE action_type='visited' AND (actor_person_id=$1 OR target_person_id=$1)`,
		"avatar":            `SELECT COUNT(*) FROM media_references WHERE person_id=$1 AND media_kind='avatar'`,
		"media":             `SELECT COUNT(*) FROM media_references WHERE person_id=$1`,
	}
	result := make(map[string]int, len(queries))
	for source, query := range queries {
		var count int
		if err := db.QueryRow(ctx, query, personID).Scan(&count); err == nil {
			result[source] = count
		}
	}
	return result
}
