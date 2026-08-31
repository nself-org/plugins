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

// InitSchema creates the np_post_* tables and indexes if they do not exist.
// All statements are idempotent (IF NOT EXISTS).
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS np_post_accounts (
			id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT        NOT NULL,
			platform          TEXT        NOT NULL CHECK (platform IN (
			                      'wordpress', 'ghost', 'twitter', 'linkedin', 'facebook',
			                      'telegram_channel', 'devto', 'hashnode'
			                  )),
			name              TEXT        NOT NULL,
			endpoint          TEXT,
			credentials       JSONB       NOT NULL DEFAULT '{}',
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_post_accounts_source
		 ON np_post_accounts(source_account_id)`,
		`CREATE TABLE IF NOT EXISTS np_post_queue (
			id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT        NOT NULL,
			platform          TEXT        NOT NULL,
			account_id        UUID        NOT NULL REFERENCES np_post_accounts(id) ON DELETE CASCADE,
			title             TEXT        NOT NULL,
			content           TEXT        NOT NULL,
			tags              TEXT[]      NOT NULL DEFAULT '{}',
			schedule_at       TIMESTAMPTZ,
			status            TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN (
			                      'queued', 'publishing', 'published', 'failed'
			                  )),
			result            JSONB,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS np_post_queue_schedule
		 ON np_post_queue(schedule_at) WHERE status = 'queued'`,
		`CREATE INDEX IF NOT EXISTS idx_np_post_queue_source
		 ON np_post_queue(source_account_id)`,
	}

	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("schema init: %w", err)
		}
	}
	return nil
}
