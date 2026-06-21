package internal

import (
	"context"
	"fmt"
	"time"
)

func (d *DB) GetContinueWatching(userID string, limit int) ([]ContinueWatchingItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent, updated_at, metadata
		FROM np_progress_positions
		WHERE source_account_id = $1
			AND user_id = $2
			AND completed = FALSE
			AND progress_percent > 1
			AND progress_percent < $3
		ORDER BY updated_at DESC
		LIMIT $4`,
		d.sourceAccountID, userID, d.completeThreshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get continue watching: %w", err)
	}
	defer rows.Close()

	var items []ContinueWatchingItem
	for rows.Next() {
		var item ContinueWatchingItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.PositionSeconds, &item.DurationSeconds, &item.ProgressPercent, &item.UpdatedAt, &item.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scan continue watching: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []ContinueWatchingItem{}
	}
	return items, rows.Err()
}

// GetRecentlyWatched returns the most recently updated positions for a user.
func (d *DB) GetRecentlyWatched(userID string, limit int) ([]RecentlyWatchedItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			position_seconds, duration_seconds, progress_percent, completed,
			completed_at, updated_at, metadata
		FROM np_progress_positions
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
		LIMIT $3`,
		d.sourceAccountID, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recently watched: %w", err)
	}
	defer rows.Close()

	var items []RecentlyWatchedItem
	for rows.Next() {
		var item RecentlyWatchedItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.PositionSeconds, &item.DurationSeconds, &item.ProgressPercent, &item.Completed,
			&item.CompletedAt, &item.UpdatedAt, &item.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scan recently watched: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []RecentlyWatchedItem{}
	}
	return items, rows.Err()
}
