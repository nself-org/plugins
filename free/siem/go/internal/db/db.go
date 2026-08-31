// Package db handles PostgreSQL connections and migrations for the SIEM plugin.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to PostgreSQL.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded SQL migrations in order.
// Migrations are idempotent (IF NOT EXISTS / CREATE TABLE IF NOT EXISTS).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	for _, sql := range migrations {
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

// migrations holds ordered DDL statements applied at startup.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS np_siem_destinations (
		id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id        UUID,
		name             TEXT NOT NULL,
		type             TEXT NOT NULL CHECK (type IN ('loki','datadog','splunk','elastic','webhook')),
		endpoint_url     TEXT NOT NULL,
		api_key_ref      TEXT,
		batch_size       INT NOT NULL DEFAULT 500,
		flush_interval_s INT NOT NULL DEFAULT 30,
		schema           TEXT NOT NULL DEFAULT 'ocsf' CHECK (schema IN ('ocsf','ecs','raw')),
		enabled          BOOLEAN NOT NULL DEFAULT TRUE,
		last_shipped_at  TIMESTAMPTZ,
		error_count      INT NOT NULL DEFAULT 0,
		created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS np_siem_ship_log (
		id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		destination_id UUID REFERENCES np_siem_destinations(id) ON DELETE CASCADE,
		batch_size     INT,
		shipped_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		status         TEXT NOT NULL CHECK (status IN ('ok','error','partial')),
		error_detail   TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_siem_ship_log_dest ON np_siem_ship_log (destination_id, shipped_at DESC)`,
}
