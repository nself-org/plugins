package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	return pool, nil
}

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS np_podcast_podcasts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			title TEXT NOT NULL,
			description TEXT,
			author TEXT,
			image_url TEXT,
			language TEXT NOT NULL DEFAULT 'en',
			is_public BOOLEAN NOT NULL DEFAULT false,
			episode_count INT NOT NULL DEFAULT 0,
			subscriber_count INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS np_podcast_episodes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			podcast_id UUID NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			title TEXT NOT NULL,
			description TEXT,
			audio_url TEXT,
			duration_seconds INT NOT NULL DEFAULT 0,
			season INT NOT NULL DEFAULT 1,
			episode_number INT NOT NULL DEFAULT 0,
			is_published BOOLEAN NOT NULL DEFAULT false,
			play_count INT NOT NULL DEFAULT 0,
			published_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS np_podcast_subscriptions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			podcast_id UUID NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			user_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (podcast_id, source_account_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS np_podcast_plays (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			episode_id UUID NOT NULL REFERENCES np_podcast_episodes(id) ON DELETE CASCADE,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	}
	for _, sql := range stmts {
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("schema migration failed: %w", err)
		}
	}
	return nil
}
