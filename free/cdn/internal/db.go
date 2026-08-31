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

// InitSchema creates all CDN tables and views if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS cdn_zones (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			provider VARCHAR(64) NOT NULL,
			zone_id VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			origin_url TEXT,
			ssl_enabled BOOLEAN DEFAULT TRUE,
			cache_ttl INTEGER DEFAULT 86400,
			status VARCHAR(32) DEFAULT 'active',
			config JSONB DEFAULT '{}',
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, provider, zone_id)
		);

		CREATE INDEX IF NOT EXISTS idx_cdn_zones_source_account ON cdn_zones(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cdn_zones_provider ON cdn_zones(provider);
		CREATE INDEX IF NOT EXISTS idx_cdn_zones_domain ON cdn_zones(domain);
		CREATE INDEX IF NOT EXISTS idx_cdn_zones_status ON cdn_zones(status);

		CREATE TABLE IF NOT EXISTS cdn_purge_requests (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id UUID REFERENCES cdn_zones(id),
			purge_type VARCHAR(16) NOT NULL,
			urls JSONB DEFAULT '[]',
			tags JSONB DEFAULT '[]',
			prefixes JSONB DEFAULT '[]',
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			provider_request_id VARCHAR(255),
			requested_by VARCHAR(255),
			completed_at TIMESTAMPTZ,
			error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cdn_purge_source_account ON cdn_purge_requests(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cdn_purge_status ON cdn_purge_requests(status);
		CREATE INDEX IF NOT EXISTS idx_cdn_purge_zone ON cdn_purge_requests(zone_id);

		CREATE TABLE IF NOT EXISTS cdn_analytics (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id UUID REFERENCES cdn_zones(id),
			date DATE NOT NULL,
			requests_total BIGINT DEFAULT 0,
			requests_cached BIGINT DEFAULT 0,
			bandwidth_total BIGINT DEFAULT 0,
			bandwidth_cached BIGINT DEFAULT 0,
			unique_visitors BIGINT DEFAULT 0,
			threats_blocked BIGINT DEFAULT 0,
			status_2xx BIGINT DEFAULT 0,
			status_3xx BIGINT DEFAULT 0,
			status_4xx BIGINT DEFAULT 0,
			status_5xx BIGINT DEFAULT 0,
			top_paths JSONB DEFAULT '[]',
			top_countries JSONB DEFAULT '[]',
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, zone_id, date)
		);

		CREATE INDEX IF NOT EXISTS idx_cdn_analytics_source_account ON cdn_analytics(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cdn_analytics_date ON cdn_analytics(date);
		CREATE INDEX IF NOT EXISTS idx_cdn_analytics_zone ON cdn_analytics(zone_id);

		CREATE TABLE IF NOT EXISTS cdn_signed_urls (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			zone_id UUID REFERENCES cdn_zones(id),
			original_url TEXT NOT NULL,
			signed_url TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			ip_restriction VARCHAR(45),
			access_count INTEGER DEFAULT 0,
			max_access INTEGER,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_cdn_signed_source_account ON cdn_signed_urls(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_cdn_signed_expires ON cdn_signed_urls(expires_at);
		CREATE INDEX IF NOT EXISTS idx_cdn_signed_zone ON cdn_signed_urls(zone_id);

		CREATE OR REPLACE VIEW cdn_bandwidth_by_zone AS
		SELECT z.source_account_id,
		       z.name AS zone_name,
		       z.domain,
		       z.provider,
		       a.date,
		       a.bandwidth_total,
		       a.bandwidth_cached,
		       ROUND(100.0 * a.bandwidth_cached / NULLIF(a.bandwidth_total, 0), 1) AS cache_bandwidth_pct,
		       a.requests_total,
		       a.requests_cached
		FROM cdn_analytics a
		JOIN cdn_zones z ON a.zone_id = z.id
		WHERE a.date > CURRENT_DATE - INTERVAL '30 days'
		ORDER BY a.date DESC, z.name;

		CREATE OR REPLACE VIEW cdn_cache_hit_rate AS
		SELECT z.source_account_id,
		       z.name AS zone_name,
		       z.domain,
		       SUM(a.requests_total) AS total_requests,
		       SUM(a.requests_cached) AS cached_requests,
		       ROUND(100.0 * SUM(a.requests_cached) / NULLIF(SUM(a.requests_total), 0), 1) AS hit_rate_pct,
		       SUM(a.bandwidth_total) AS total_bandwidth,
		       SUM(a.bandwidth_cached) AS cached_bandwidth,
		       SUM(a.status_4xx) AS total_4xx,
		       SUM(a.status_5xx) AS total_5xx
		FROM cdn_analytics a
		JOIN cdn_zones z ON a.zone_id = z.id
		WHERE a.date > CURRENT_DATE - INTERVAL '30 days'
		GROUP BY z.source_account_id, z.name, z.domain;
	`

	_, err := d.Pool.Exec(ctx, schema)
	return err
}
