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

const initSchema = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_observability_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    url VARCHAR(512),
    check_type VARCHAR(50) DEFAULT 'http',
    check_interval_sec INTEGER DEFAULT 30,
    timeout_sec INTEGER DEFAULT 10,
    status VARCHAR(20) DEFAULT 'unknown',
    last_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, name)
);

CREATE INDEX IF NOT EXISTS idx_np_obs_services_source
    ON np_observability_services(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_obs_services_status
    ON np_observability_services(source_account_id, status);

CREATE TABLE IF NOT EXISTS np_observability_health_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_id UUID NOT NULL REFERENCES np_observability_services(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,
    response_time_ms INTEGER,
    status_code INTEGER,
    error_message TEXT,
    checked_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_obs_health_source
    ON np_observability_health_history(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_obs_health_service
    ON np_observability_health_history(service_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_obs_health_time
    ON np_observability_health_history(source_account_id, checked_at DESC);

CREATE TABLE IF NOT EXISTS np_observability_watchdog_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_id UUID REFERENCES np_observability_services(id) ON DELETE SET NULL,
    event_type VARCHAR(50) NOT NULL,
    details TEXT,
    occurred_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_obs_events_source
    ON np_observability_watchdog_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_obs_events_service
    ON np_observability_watchdog_events(service_id) WHERE service_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_obs_events_time
    ON np_observability_watchdog_events(source_account_id, occurred_at DESC);
`

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, initSchema)
	return err
}
