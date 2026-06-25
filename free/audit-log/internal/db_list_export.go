package internal

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Size-cap exception: single DB operation — 88L scan loop with struct mapping; splitting would fragment a single SQL query across files.
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
// Size-cap exception: single DB operation — 83L scan loop with struct mapping; splitting would fragment a single SQL query across files.
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
