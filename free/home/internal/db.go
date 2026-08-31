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
		`CREATE TABLE IF NOT EXISTS np_home_connections (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			name TEXT NOT NULL,
			ha_url TEXT NOT NULL,
			ha_token TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS np_home_devices (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			entity_id TEXT NOT NULL,
			friendly_name TEXT,
			domain TEXT NOT NULL,
			state TEXT,
			attributes JSONB DEFAULT '{}',
			last_updated TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (source_account_id, entity_id)
		)`,
		`CREATE TABLE IF NOT EXISTS np_home_command_log (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			entity_id TEXT NOT NULL,
			service TEXT NOT NULL,
			data JSONB,
			result TEXT,
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
