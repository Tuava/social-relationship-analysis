package media

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
)

func BackfillReferences(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) {
	normalizer := normalization.Normalizer{DB: db}
	rows, err := db.Query(ctx, `SELECT m.id::text,m.source_account_id::text,m.raw_record_id::text,m.message_segments
        FROM messages m WHERE m.source_account_id IS NOT NULL AND m.raw_record_id IS NOT NULL
        AND NOT EXISTS(SELECT 1 FROM media_references mr WHERE mr.message_id=m.id)`)
	if err == nil {
		for rows.Next() {
			var messageID, accountID, rawID string
			var segments json.RawMessage
			if rows.Scan(&messageID, &accountID, &rawID, &segments) == nil {
				if err := normalizer.QueueMessageMedia(ctx, accountID, messageID, rawID, segments); err != nil && logger != nil {
					logger.Warn("backfill message media", "message_id", messageID, "error", err)
				}
			}
		}
		rows.Close()
	}
	rows, err = db.Query(ctx, `SELECT DISTINCT ON (p.id) p.id::text,pi.platform_user_id,
        COALESCE(rr.account_id,gm.source_account_id,default_account.id)::text,COALESCE(rr.id::text,'')
        FROM persons p JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
        LEFT JOIN person_profiles pp ON pp.person_id=p.id
        LEFT JOIN raw_records rr ON rr.id=pp.raw_record_id
        LEFT JOIN group_memberships gm ON gm.person_id=p.id
        LEFT JOIN LATERAL (SELECT id FROM napcat_accounts WHERE enabled ORDER BY created_at LIMIT 1) default_account ON true
        WHERE COALESCE(rr.account_id,gm.source_account_id,default_account.id) IS NOT NULL
        ORDER BY p.id,pp.valid_from DESC NULLS LAST`)
	if err != nil {
		return
	}
	for rows.Next() {
		var personID, qq, accountID, rawID string
		if rows.Scan(&personID, &qq, &accountID, &rawID) == nil {
			if err := normalizer.QueueAvatar(ctx, accountID, personID, rawID, qq, ""); err != nil && logger != nil {
				logger.Warn("backfill avatar", "person_id", personID, "error", err)
			}
		}
	}
	rows.Close()
	rows, err = db.Query(ctx, `SELECT DISTINCT ON (g.id) g.id::text,g.platform_group_id,
		COALESCE(gm.source_account_id,default_account.id)::text,COALESCE(gm.raw_record_id::text,'')
		FROM groups g
		LEFT JOIN group_memberships gm ON gm.group_id=g.id
		LEFT JOIN LATERAL (SELECT id FROM napcat_accounts WHERE enabled ORDER BY created_at LIMIT 1) default_account ON true
		WHERE g.platform='qq' AND g.platform_group_id<>'' AND COALESCE(gm.source_account_id,default_account.id) IS NOT NULL
		ORDER BY g.id,gm.valid_from DESC NULLS LAST`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, platformGroupID, accountID, rawID string
		if rows.Scan(&groupID, &platformGroupID, &accountID, &rawID) == nil {
			if err := normalizer.QueueGroupAvatar(ctx, accountID, groupID, rawID, platformGroupID, ""); err != nil && logger != nil {
				logger.Warn("backfill group avatar", "group_id", groupID, "error", err)
			}
		}
	}
}
