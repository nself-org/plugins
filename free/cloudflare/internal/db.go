package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used by handlers. It exists
// as a testability seam: production code passes a *pgxpool.Pool, tests pass a
// pgxmock.PgxPoolIface — both satisfy this interface structurally.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool   Querier
	closer func()
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

	return &DB{Pool: pool, closer: pool.Close}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	if d.closer != nil {
		d.closer()
	}
}

// InitSchema creates all cloudflare tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS cf_zones (
			id VARCHAR(64) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id VARCHAR(64),
			name VARCHAR(255) NOT NULL,
			status VARCHAR(50),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cf_zones_source_account ON cf_zones(source_account_id);

		CREATE TABLE IF NOT EXISTS cf_dns_records (
			id VARCHAR(64) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id VARCHAR(64) NOT NULL,
			record_id VARCHAR(64),
			type VARCHAR(10) NOT NULL,
			name VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			ttl INTEGER DEFAULT 1,
			proxied BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cf_dns_records_source_account ON cf_dns_records(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cf_dns_records_zone ON cf_dns_records(zone_id);

		CREATE TABLE IF NOT EXISTS cf_r2_buckets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(63) NOT NULL,
			location VARCHAR(50),
			storage_class VARCHAR(50) DEFAULT 'Standard',
			object_count BIGINT DEFAULT 0,
			total_size_bytes BIGINT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, name)
		);

		CREATE INDEX IF NOT EXISTS idx_cf_r2_buckets_source_account ON cf_r2_buckets(source_account_id);

		CREATE TABLE IF NOT EXISTS cf_cache_purge_log (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id VARCHAR(64) NOT NULL,
			purge_type VARCHAR(20) NOT NULL,
			urls TEXT[],
			tags TEXT[],
			prefixes TEXT[],
			status VARCHAR(20) NOT NULL DEFAULT 'completed',
			cf_response JSONB,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cf_cache_purge_log_source_account ON cf_cache_purge_log(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cf_cache_purge_log_zone ON cf_cache_purge_log(zone_id);

		CREATE TABLE IF NOT EXISTS cf_analytics (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id VARCHAR(64) NOT NULL,
			date DATE NOT NULL,
			requests_total BIGINT DEFAULT 0,
			requests_cached BIGINT DEFAULT 0,
			bandwidth_total BIGINT DEFAULT 0,
			threats_total BIGINT DEFAULT 0,
			unique_visitors BIGINT DEFAULT 0,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, zone_id, date)
		);

		CREATE INDEX IF NOT EXISTS idx_cf_analytics_source_account ON cf_analytics(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cf_analytics_zone_date ON cf_analytics(zone_id, date DESC);

		CREATE TABLE IF NOT EXISTS cf_webhook_events (
			id VARCHAR(255) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			event_type VARCHAR(128) NOT NULL,
			payload JSONB NOT NULL DEFAULT '{}',
			processed BOOLEAN DEFAULT false,
			processed_at TIMESTAMPTZ,
			error TEXT,
			retry_count INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cf_webhook_events_source_account ON cf_webhook_events(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cf_webhook_events_processed ON cf_webhook_events(processed);
	`

	_, err := d.Pool.Exec(ctx, schema)
	return err
}
