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

// InitSchema creates all analytics tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS analytics_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			event_name VARCHAR(255) NOT NULL,
			event_category VARCHAR(128),
			user_id VARCHAR(255),
			session_id VARCHAR(255),
			properties JSONB DEFAULT '{}',
			context JSONB DEFAULT '{}',
			source_plugin VARCHAR(128),
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_events_source_account ON analytics_events(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_events_name ON analytics_events(event_name);
		CREATE INDEX IF NOT EXISTS idx_analytics_events_user ON analytics_events(user_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_events_session ON analytics_events(session_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_events_timestamp ON analytics_events(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_analytics_events_category ON analytics_events(event_category);

		CREATE TABLE IF NOT EXISTS analytics_counters (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			counter_name VARCHAR(255) NOT NULL,
			dimension VARCHAR(255) DEFAULT 'total',
			period VARCHAR(32) NOT NULL,
			period_start TIMESTAMPTZ NOT NULL,
			value BIGINT DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, counter_name, dimension, period, period_start)
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_counters_source_account ON analytics_counters(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_counters_name ON analytics_counters(counter_name);
		CREATE INDEX IF NOT EXISTS idx_analytics_counters_period ON analytics_counters(period, period_start);

		CREATE TABLE IF NOT EXISTS analytics_funnels (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			name VARCHAR(255) NOT NULL,
			description TEXT,
			steps JSONB NOT NULL,
			window_hours INTEGER DEFAULT 24,
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_funnels_source_account ON analytics_funnels(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_funnels_enabled ON analytics_funnels(enabled);

		CREATE TABLE IF NOT EXISTS analytics_quotas (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			name VARCHAR(255) NOT NULL,
			scope VARCHAR(32) DEFAULT 'app',
			scope_id VARCHAR(255),
			counter_name VARCHAR(255) NOT NULL,
			max_value BIGINT NOT NULL,
			period VARCHAR(32) NOT NULL,
			action_on_exceed VARCHAR(32) DEFAULT 'warn',
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_quotas_source_account ON analytics_quotas(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_quotas_counter ON analytics_quotas(counter_name);
		CREATE INDEX IF NOT EXISTS idx_analytics_quotas_enabled ON analytics_quotas(enabled);

		CREATE TABLE IF NOT EXISTS analytics_quota_violations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			quota_id UUID REFERENCES analytics_quotas(id) ON DELETE CASCADE,
			scope_id VARCHAR(255),
			current_value BIGINT,
			max_value BIGINT,
			action_taken VARCHAR(32),
			notified BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_source_account ON analytics_quota_violations(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_quota ON analytics_quota_violations(quota_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_created ON analytics_quota_violations(created_at DESC);

		CREATE TABLE IF NOT EXISTS analytics_webhook_events (
			id VARCHAR(255) PRIMARY KEY,
			source_account_id VARCHAR(128) DEFAULT 'primary',
			event_type VARCHAR(128),
			payload JSONB,
			processed BOOLEAN DEFAULT false,
			processed_at TIMESTAMPTZ,
			error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_source_account ON analytics_webhook_events(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_processed ON analytics_webhook_events(processed);
		CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_type ON analytics_webhook_events(event_type);
	`

	_, err := d.Pool.Exec(ctx, schema)
	return err
}
