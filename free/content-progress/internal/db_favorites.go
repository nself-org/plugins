package internal

import (
	"context"
	"fmt"
	"time"
)

// =========================================================================
// Favorites
// =========================================================================

// AddToFavorites inserts a favorite item. On conflict, returns the existing row.
func (d *DB) AddToFavorites(req AddToFavoritesRequest) (*FavoriteItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var item FavoriteItem
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_progress_favorites (
			source_account_id, user_id, content_type, content_id
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source_account_id, user_id, content_type, content_id)
		DO UPDATE SET source_account_id = EXCLUDED.source_account_id
		RETURNING id, source_account_id, user_id, content_type, content_id, created_at`,
		d.sourceAccountID, req.UserID, req.ContentType, req.ContentID,
	).Scan(
		&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add to favorites: %w", err)
	}
	return &item, nil
}

// GetFavorites returns a user's favorites ordered by created_at DESC.
func (d *DB) GetFavorites(userID string, limit, offset int) ([]FavoriteItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, content_type, content_id, created_at
		FROM np_progress_favorites
		WHERE source_account_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		d.sourceAccountID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get favorites: %w", err)
	}
	defer rows.Close()

	var items []FavoriteItem
	for rows.Next() {
		var item FavoriteItem
		if err := rows.Scan(
			&item.ID, &item.SourceAccountID, &item.UserID, &item.ContentType, &item.ContentID,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []FavoriteItem{}
	}
	return items, rows.Err()
}

// RemoveFromFavorites deletes a favorite item. Returns true if a row was deleted.
func (d *DB) RemoveFromFavorites(userID, contentType, contentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_progress_favorites
		WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
		d.sourceAccountID, userID, contentType, contentID,
	)
	if err != nil {
		return false, fmt.Errorf("remove from favorites: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

