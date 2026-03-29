package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LinkPreview represents a row in np_link_preview_cache.
type LinkPreview struct {
	ID          string     `json:"id"`
	URL         string     `json:"url"`
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Image       *string    `json:"image"`
	SiteName    *string    `json:"site_name"`
	Type        *string    `json:"type"`
	FetchedAt   time.Time  `json:"fetched_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// Migrate creates the required table if it does not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_link_preview_cache (
			id          TEXT PRIMARY KEY,
			url         TEXT NOT NULL,
			title       TEXT,
			description TEXT,
			image       TEXT,
			site_name   TEXT,
			type        TEXT,
			fetched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at  TIMESTAMPTZ
		);

		CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_url
			ON np_link_preview_cache (url);

		CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_expires_at
			ON np_link_preview_cache (expires_at);
	`)
	return err
}

// InsertPreview inserts or updates a cached preview.
func InsertPreview(pool *pgxpool.Pool, p *LinkPreview) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO np_link_preview_cache (id, url, title, description, image, site_name, type, fetched_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			image = EXCLUDED.image,
			site_name = EXCLUDED.site_name,
			type = EXCLUDED.type,
			fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at
	`, p.ID, p.URL, p.Title, p.Description, p.Image, p.SiteName, p.Type, p.FetchedAt, p.ExpiresAt)
	return err
}

// GetPreviewByURL returns the cached preview for a URL, or nil if not found or expired.
func GetPreviewByURL(pool *pgxpool.Pool, url string) (*LinkPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := pool.QueryRow(ctx, `
		SELECT id, url, title, description, image, site_name, type, fetched_at, expires_at
		FROM np_link_preview_cache
		WHERE url = $1 AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`, url)

	var p LinkPreview
	err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Description, &p.Image, &p.SiteName, &p.Type, &p.FetchedAt, &p.ExpiresAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// DeleteAllPreviews clears the entire cache. Returns the number of rows deleted.
func DeleteAllPreviews(pool *pgxpool.Pool) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx, `DELETE FROM np_link_preview_cache`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
