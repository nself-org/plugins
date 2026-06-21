package internal

import (
	"context"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_webhooks_endpoints (
			id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			url             TEXT NOT NULL,
			description     TEXT,
			secret          VARCHAR(255) NOT NULL,
			events          TEXT[] NOT NULL,
			headers         JSONB DEFAULT '{}',
			enabled         BOOLEAN DEFAULT TRUE,
			failure_count   INTEGER DEFAULT 0,
			last_success_at TIMESTAMPTZ,
			last_failure_at TIMESTAMPTZ,
			disabled_at     TIMESTAMPTZ,
			disabled_reason TEXT,
			metadata        JSONB DEFAULT '{}',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_enabled
			ON np_webhooks_endpoints (enabled);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_events
			ON np_webhooks_endpoints USING GIN(events);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_created
			ON np_webhooks_endpoints (created_at DESC);

		CREATE TABLE IF NOT EXISTS np_webhooks_deliveries (
			id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			endpoint_id      UUID NOT NULL REFERENCES np_webhooks_endpoints(id) ON DELETE CASCADE,
			event_type       VARCHAR(128) NOT NULL,
			payload          JSONB NOT NULL,
			status           VARCHAR(32) DEFAULT 'pending',
			response_status  INTEGER,
			response_body    TEXT,
			response_time_ms INTEGER,
			attempt_count    INTEGER DEFAULT 0,
			max_attempts     INTEGER DEFAULT 5,
			next_retry_at    TIMESTAMPTZ,
			error_message    TEXT,
			signature        VARCHAR(255),
			delivered_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_endpoint
			ON np_webhooks_deliveries (endpoint_id);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_status
			ON np_webhooks_deliveries (status);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_event_type
			ON np_webhooks_deliveries (event_type);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_next_retry
			ON np_webhooks_deliveries (next_retry_at) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_created
			ON np_webhooks_deliveries (created_at DESC);
	`)
	return err
}

// --- Helpers -----------------------------------------------------------------

// GenerateSecret returns a random webhook secret prefixed with "whsec_".
