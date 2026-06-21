package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"fmt"
	"strings"
	"time"
)

// =========================================================================
// Watchlist
// =========================================================================

// AddToWatchlist inserts or updates a watchlist item (upsert on conflict).
func (d *DB) AddToWatchlist(req AddToWatchlistRequest) (*WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	var item WatchlistItem
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_progress_watchlists (
			source_account_id, user_id, content_type, content_id,
			priority, added_from, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source_account_id, user_id, content_type, content_id)
		DO UPDATE SET
			priority = EXCLUDED.priority,
			notes = EXCLUDED.notes
		RETURNING id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at`,
		d.sourceAccountID, req.UserID, req.ContentType, req.ContentID,
		priority, req.AddedFrom, req.Notes,
	).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add to watchlist: %w", err)
	}
	return &item, nil
}

// GetWatchlist returns a user's watchlist ordered by priority DESC then created_at DESC.
func (d *DB) GetWatchlist(userID string, limit, offset int) ([]WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at
		FROM np_progress_watchlists
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY priority DESC, created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get watchlist: %w", err)
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan watchlist: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []WatchlistItem{}
	}
	return items, rows.Err()
}

// UpdateWatchlistItem updates priority and/or notes on a watchlist item.
func (d *DB) UpdateWatchlistItem(userID, contentType, contentID string, req UpdateWatchlistRequest) (*WatchlistItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setParts := []string{}
	args := []interface{}{d.sourceAccountID, userID, contentType, contentID}
	idx := 5

	if req.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *req.Priority)
		idx++
	}
	if req.Notes != nil {
		setParts = append(setParts, fmt.Sprintf("notes = $%d", idx))
		args = append(args, *req.Notes)
		idx++
	}

	if len(setParts) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(
		`UPDATE np_progress_watchlists
		SET %s
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
		RETURNING id, source_account_id, user_id, content_type, content_id,
			priority, added_from, notes, created_at`,
		strings.Join(setParts, ", "),
	)

	var item WatchlistItem
	err := d.pool.QueryRow(ctx, query, args...).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.Priority, &item.AddedFrom, &item.Notes, &item.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update watchlist item: %w", err)
	}
	return &item, nil
}

// RemoveFromWatchlist deletes a watchlist item. Returns true if a row was deleted.
func (d *DB) RemoveFromWatchlist(userID, contentType, contentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_progress_watchlists
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	)
	if err != nil {
		return false, fmt.Errorf("remove from watchlist: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

