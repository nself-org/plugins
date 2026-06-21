package internal

import (
	"context"
	"fmt"
	"time"
)

// =========================================================================
// Progress History
// =========================================================================

// insertHistoryEvent records a history event directly (no sampling).
func (d *DB) insertHistoryEvent(userID, contentType, contentID, action string, positionSeconds *float64, deviceID *string, sessionID *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = d.pool.Exec(ctx,
		`INSERT INTO np_progress_history (
			source_account_id, user_id, content_type, content_id,
			action, position_seconds, device_id, session_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		d.sourceAccountID, userID, contentType, contentID, action, positionSeconds, deviceID, sessionID,
	)
}

// GetUserHistory returns history events for a user, ordered by created_at DESC.
func (d *DB) GetUserHistory(userID string, limit, offset int) ([]ProgressHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			action, position_seconds, device_id, session_id, created_at
		FROM np_progress_history
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get user history: %w", err)
	}
	defer rows.Close()

	var items []ProgressHistory
	for rows.Next() {
		var h ProgressHistory
		if err := rows.Scan(
			&h.ID, &h.SourceAccountID, &h.UserID, &h.ContentType, &h.ContentID,
			&h.Action, &h.PositionSeconds, &h.DeviceID, &h.SessionID, &h.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		items = append(items, h)
	}
	if items == nil {
		items = []ProgressHistory{}
	}
	return items, rows.Err()
}

