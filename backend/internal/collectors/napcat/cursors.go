package napcat

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CursorStore persists collection progress so interrupted runs can resume
// instead of restarting from page zero.
type CursorStore struct{ DB *pgxpool.Pool }

// SaveCursor stores a JSON-serialisable cursor value under (accountID, scope).
// The scope is typically "group_msg_history:<group_id>" or "friend_msg_history:<qq>".
func (s CursorStore) SaveCursor(ctx context.Context, accountID, scope string, cursor any) error {
	data, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO collection_cursors(account_id, scope, cursor, updated_at)
		VALUES($1, $2, $3, now())
		ON CONFLICT(account_id, scope) DO UPDATE SET cursor=EXCLUDED.cursor, updated_at=now()`,
		accountID, scope, data)
	return err
}

// LoadCursor reads a cursor value into dst. Returns false when no cursor exists.
func (s CursorStore) LoadCursor(ctx context.Context, accountID, scope string, dst any) (bool, error) {
	var data []byte
	err := s.DB.QueryRow(ctx, `SELECT cursor FROM collection_cursors WHERE account_id=$1 AND scope=$2`, accountID, scope).Scan(&data)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return false, nil
		}
		return false, err
	}
	return json.Unmarshal(data, dst) == nil, nil
}

// GroupHistoryCursor tracks how far we have paginated through a group's messages.
type GroupHistoryCursor struct {
	LastSeq           int64  `json:"last_seq"`
	HasMore           bool   `json:"has_more"`
	TotalSeen         int    `json:"total_seen"`
	BoundaryConfirmed bool   `json:"boundary_confirmed"`
	BoundaryReason    string `json:"boundary_reason,omitempty"`
}

// FriendHistoryCursor tracks private chat pagination progress.
type FriendHistoryCursor struct {
	LastSeq           int64  `json:"last_seq"`
	HasMore           bool   `json:"has_more"`
	TotalSeen         int    `json:"total_seen"`
	BoundaryConfirmed bool   `json:"boundary_confirmed"`
	BoundaryReason    string `json:"boundary_reason,omitempty"`
}
