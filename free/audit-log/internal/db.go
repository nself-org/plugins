package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate creates the np_auditlog_events table and its supporting indexes
// if they do not already exist. Partitioning and RLS are applied once on
// first run; subsequent calls are idempotent.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Read the embedded migration SQL and execute it.
	_, err := pool.Exec(ctx, migrationSQL)
	return err
}

// migrationSQL is the full schema for the audit-log plugin.
// This mirrors migrations/001_audit_log_init.sql but is embedded here so
// the binary is self-contained and can run without filesystem access.
const migrationSQL = `
-- Ensure the table exists (partitioned by created_at month).
-- We use list partitioning by default here to stay compatible with
-- environments that may not have pg_partman; the parent table holds all
-- data when no child partitions are present.

CREATE TABLE IF NOT EXISTS np_auditlog_events (
    id                TEXT         NOT NULL,
    source_account_id TEXT         NOT NULL DEFAULT 'primary',
    actor_user_id     TEXT         NOT NULL DEFAULT '',
    actor_type        TEXT         NOT NULL,
    event_type        TEXT         NOT NULL,
    resource_type     TEXT         NOT NULL DEFAULT '',
    resource_id       TEXT         NOT NULL DEFAULT '',
    ip_address        TEXT         NOT NULL DEFAULT '',
    user_agent        TEXT         NOT NULL DEFAULT '',
    metadata          JSONB        NOT NULL DEFAULT '{}',
    severity          TEXT         NOT NULL DEFAULT 'info',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Default catch-all partition for any rows that don't yet have a month partition.
CREATE TABLE IF NOT EXISTS np_auditlog_events_default
    PARTITION OF np_auditlog_events DEFAULT;

-- Composite index: account isolation + time-range scans (most common query).
CREATE INDEX IF NOT EXISTS idx_np_auditlog_account_time
    ON np_auditlog_events (source_account_id, created_at DESC);

-- Index for filtering by event type.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_event_type
    ON np_auditlog_events (event_type);

-- Index for filtering by actor.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_actor_user
    ON np_auditlog_events (actor_user_id);

-- Enable Row-Level Security so that multi-tenant deployments can enforce
-- account isolation at the database layer.
ALTER TABLE np_auditlog_events ENABLE ROW LEVEL SECURITY;

-- SELECT policy: a session may only see rows for its own account.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_select'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_select ON np_auditlog_events
                FOR SELECT
                USING (source_account_id = current_setting('app.source_account_id', true))
        $policy$;
    END IF;
END $$;

-- INSERT policy: a session may only insert rows for its own account.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_insert'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_insert ON np_auditlog_events
                FOR INSERT
                WITH CHECK (source_account_id = current_setting('app.source_account_id', true))
        $policy$;
    END IF;
END $$;

-- DELETE policy: always false — the table is append-only.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_no_delete'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_no_delete ON np_auditlog_events
                FOR DELETE
                USING (false)
        $policy$;
    END IF;
END $$;

-- UPDATE policy: always false — the table is append-only.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_no_update'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_no_update ON np_auditlog_events
                FOR UPDATE
                USING (false)
        $policy$;
    END IF;
END $$;

-- Migration 002: inter-plugin tracing columns (S43-T18).
-- ADD COLUMN IF NOT EXISTS is idempotent — safe to re-run on existing schemas.
ALTER TABLE np_auditlog_events
    ADD COLUMN IF NOT EXISTS source_plugin TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_plugin TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_np_auditlog_source_plugin
    ON np_auditlog_events (source_plugin)
    WHERE source_plugin != '';

CREATE INDEX IF NOT EXISTS idx_np_auditlog_target_plugin
    ON np_auditlog_events (target_plugin)
    WHERE target_plugin != '';
`

// InsertEvent writes a new audit event to np_auditlog_events.
// Only parameterized queries are used; no string interpolation of user data.
