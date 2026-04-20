package internal

import (
	"context"
	"fmt"
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
func InsertEvent(ctx context.Context, pool *pgxpool.Pool, e *AuditEvent) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_auditlog_events
			(id, source_account_id, actor_user_id, actor_type, event_type,
			 resource_type, resource_id, ip_address, user_agent, metadata,
			 severity, source_plugin, target_plugin, created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		e.ID,
		e.SourceAccountID,
		e.ActorUserID,
		e.ActorType,
		e.EventType,
		e.ResourceType,
		e.ResourceID,
		e.IPAddress,
		e.UserAgent,
		e.Metadata,
		e.Severity,
		e.SourcePlugin,
		e.TargetPlugin,
		e.CreatedAt,
	)
	return err
}

// QueryFilter holds the optional filter parameters for ListEvents and ExportEvents.
type QueryFilter struct {
	EventType       string
	ActorUserID     string
	Severity        string
	SourceAccountID string
	From            *time.Time
	To              *time.Time
	Limit           int
	Offset          int
}

// ListEvents returns audit events matching the given filter, ordered by
// created_at DESC. Pagination is via limit/offset.
func ListEvents(ctx context.Context, pool *pgxpool.Pool, f QueryFilter) ([]*AuditEvent, int64, error) {
	args := []any{}
	argIdx := 1

	where := " WHERE 1=1"

	if f.EventType != "" {
		where += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, f.EventType)
		argIdx++
	}
	if f.ActorUserID != "" {
		where += fmt.Sprintf(" AND actor_user_id = $%d", argIdx)
		args = append(args, f.ActorUserID)
		argIdx++
	}
	if f.Severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, f.Severity)
		argIdx++
	}
	if f.SourceAccountID != "" {
		where += fmt.Sprintf(" AND source_account_id = $%d", argIdx)
		args = append(args, f.SourceAccountID)
		argIdx++
	}
	if f.From != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *f.From)
		argIdx++
	}
	if f.To != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *f.To)
		argIdx++
	}

	// Count total matching rows for pagination metadata.
	var total int64
	countQuery := "SELECT COUNT(*) FROM np_auditlog_events" + where
	if err := pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query: %w", err)
	}

	// Fetch the requested page.
	dataQuery := `SELECT id, source_account_id, actor_user_id, actor_type, event_type,
		resource_type, resource_id, ip_address, user_agent, metadata, severity,
		source_plugin, target_plugin, created_at
		FROM np_auditlog_events` +
		where +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, f.Limit, f.Offset)

	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list query: %w", err)
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		e := &AuditEvent{}
		if err := rows.Scan(
			&e.ID,
			&e.SourceAccountID,
			&e.ActorUserID,
			&e.ActorType,
			&e.EventType,
			&e.ResourceType,
			&e.ResourceID,
			&e.IPAddress,
			&e.UserAgent,
			&e.Metadata,
			&e.Severity,
			&e.SourcePlugin,
			&e.TargetPlugin,
			&e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// ExportEvents returns all audit events matching the given filter with no
// pagination limit. It is intended for compliance exports (CSV). The caller is
// responsible for streaming the result; avoid calling this on very large
// datasets without appropriate time bounds in the filter.
func ExportEvents(ctx context.Context, pool *pgxpool.Pool, f QueryFilter) ([]*AuditEvent, error) {
	args := []any{}
	argIdx := 1

	where := " WHERE 1=1"

	if f.EventType != "" {
		where += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, f.EventType)
		argIdx++
	}
	if f.ActorUserID != "" {
		where += fmt.Sprintf(" AND actor_user_id = $%d", argIdx)
		args = append(args, f.ActorUserID)
		argIdx++
	}
	if f.Severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, f.Severity)
		argIdx++
	}
	if f.SourceAccountID != "" {
		where += fmt.Sprintf(" AND source_account_id = $%d", argIdx)
		args = append(args, f.SourceAccountID)
		argIdx++
	}
	if f.From != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *f.From)
		argIdx++
	}
	if f.To != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *f.To)
		argIdx++
	}

	// Suppress unused variable warning — argIdx is incremented through the
	// loop above but not used after the last conditional.
	_ = argIdx

	dataQuery := `SELECT id, source_account_id, actor_user_id, actor_type, event_type,
		resource_type, resource_id, ip_address, user_agent, metadata, severity,
		source_plugin, target_plugin, created_at
		FROM np_auditlog_events` +
		where +
		" ORDER BY created_at ASC"

	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("export query: %w", err)
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		e := &AuditEvent{}
		if err := rows.Scan(
			&e.ID,
			&e.SourceAccountID,
			&e.ActorUserID,
			&e.ActorType,
			&e.EventType,
			&e.ResourceType,
			&e.ResourceID,
			&e.IPAddress,
			&e.UserAgent,
			&e.Metadata,
			&e.Severity,
			&e.SourcePlugin,
			&e.TargetPlugin,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// GetEvent fetches a single audit event by its ID.
func GetEvent(ctx context.Context, pool *pgxpool.Pool, id string) (*AuditEvent, error) {
	e := &AuditEvent{}
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, actor_user_id, actor_type, event_type,
		       resource_type, resource_id, ip_address, user_agent, metadata,
		       severity, source_plugin, target_plugin, created_at
		FROM np_auditlog_events
		WHERE id = $1
		LIMIT 1
	`, id).Scan(
		&e.ID,
		&e.SourceAccountID,
		&e.ActorUserID,
		&e.ActorType,
		&e.EventType,
		&e.ResourceType,
		&e.ResourceID,
		&e.IPAddress,
		&e.UserAgent,
		&e.Metadata,
		&e.Severity,
		&e.SourcePlugin,
		&e.TargetPlugin,
		&e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}
