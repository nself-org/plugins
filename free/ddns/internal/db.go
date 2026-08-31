package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// NewDB creates and connects a pgxpool from the given DATABASE_URL.
func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	d.Pool.Close()
}

// InitSchema creates all DDNS tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_ddns_config (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			provider VARCHAR(64) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			hostname VARCHAR(255) NOT NULL DEFAULT '@',
			token TEXT NOT NULL,
			api_key TEXT,
			zone_id VARCHAR(128),
			record_id VARCHAR(128),
			record_type VARCHAR(10) NOT NULL DEFAULT 'A',
			ttl INTEGER NOT NULL DEFAULT 300,
			current_ip VARCHAR(45),
			last_updated TIMESTAMPTZ,
			check_interval INTEGER NOT NULL DEFAULT 300,
			enabled BOOLEAN DEFAULT true,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, provider, domain, hostname)
		);

		CREATE INDEX IF NOT EXISTS idx_np_ddns_config_source_account
			ON np_ddns_config(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_ddns_config_provider
			ON np_ddns_config(source_account_id, provider);
		CREATE INDEX IF NOT EXISTS idx_np_ddns_config_enabled
			ON np_ddns_config(source_account_id, enabled) WHERE enabled = true;

		CREATE TABLE IF NOT EXISTS np_ddns_update_log (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			config_id UUID NOT NULL REFERENCES np_ddns_config(id) ON DELETE CASCADE,
			provider VARCHAR(64) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			old_ip VARCHAR(45),
			new_ip VARCHAR(45) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'success',
			response_code INTEGER,
			response_message TEXT,
			error TEXT,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_source_account
			ON np_ddns_update_log(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_config
			ON np_ddns_update_log(config_id);
		CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_status
			ON np_ddns_update_log(source_account_id, status);
		CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_created
			ON np_ddns_update_log(source_account_id, created_at DESC);
	`

	_, err := d.Pool.Exec(ctx, schema)
	return err
}
