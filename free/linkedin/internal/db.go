package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDB creates a new pgxpool connection pool.
func NewDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// InitSchema creates the np_linkedin_* tables if they do not exist.
// All statements are idempotent (IF NOT EXISTS).
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		// Stores OAuth tokens per source account. One row per connected LinkedIn account.
		`CREATE TABLE IF NOT EXISTS np_linkedin_tokens (
			id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT        NOT NULL UNIQUE,
			access_token      TEXT        NOT NULL,
			expires_at        TIMESTAMPTZ NOT NULL,
			linkedin_id       TEXT        NOT NULL,
			name              TEXT        NOT NULL,
			email             TEXT,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_linkedin_tokens_source
		 ON np_linkedin_tokens(source_account_id)`,

		// Stores every post published via this plugin.
		`CREATE TABLE IF NOT EXISTS np_linkedin_posts (
			id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT        NOT NULL,
			linkedin_post_id  TEXT,
			text              TEXT        NOT NULL,
			image_url         TEXT,
			visibility        TEXT        NOT NULL DEFAULT 'PUBLIC',
			status            TEXT        NOT NULL DEFAULT 'published'
			                                CHECK (status IN ('published', 'failed')),
			error_message     TEXT,
			published_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_linkedin_posts_source
		 ON np_linkedin_posts(source_account_id, published_at DESC)`,
	}

	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("schema init: %w", err)
		}
	}
	return nil
}
