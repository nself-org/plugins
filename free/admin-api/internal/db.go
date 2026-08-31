package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with source-account scoping.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a connection pool from the given DATABASE_URL.
func NewDB(ctx context.Context, cfg *Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolCfg.MinConns = cfg.DBPoolMin
	poolCfg.MaxConns = cfg.DBPoolMax

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &DB{pool: pool, sourceAccountID: "primary"}, nil
}

// ForAccount returns a copy scoped to the given source_account_id.
func (d *DB) ForAccount(id string) *DB {
	return &DB{pool: d.pool, sourceAccountID: normalizeAccountID(id)}
}

// Close closes the underlying pool.
func (d *DB) Close() { d.pool.Close() }

func normalizeAccountID(v string) string {
	out := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			out = append(out, c)
		} else if c >= 'A' && c <= 'Z' {
			out = append(out, c+32)
		} else {
			out = append(out, '-')
		}
	}
	// trim leading/trailing dashes
	s := string(out)
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return "primary"
	}
	return s
}

// InitSchema creates tables and indexes idempotently.
func (d *DB) InitSchema(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_admin_metrics_snapshots (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			metric_type VARCHAR(50) NOT NULL DEFAULT 'system',
			cpu_usage_percent DOUBLE PRECISION,
			memory_used_bytes BIGINT,
			memory_total_bytes BIGINT,
			disk_used_bytes BIGINT,
			disk_total_bytes BIGINT,
			active_connections INTEGER,
			request_count INTEGER,
			error_count INTEGER,
			avg_response_time_ms DOUBLE PRECISION,
			active_sessions INTEGER,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_source_account
			ON np_admin_metrics_snapshots(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_type
			ON np_admin_metrics_snapshots(source_account_id, metric_type);
		CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_created
			ON np_admin_metrics_snapshots(source_account_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS np_admin_dashboard_config (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			config_key VARCHAR(255) NOT NULL,
			config_value JSONB NOT NULL DEFAULT '{}',
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, config_key)
		);

		CREATE INDEX IF NOT EXISTS idx_np_admin_dashboard_config_source_account
			ON np_admin_dashboard_config(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_admin_dashboard_config_key
			ON np_admin_dashboard_config(source_account_id, config_key);

		CREATE TABLE IF NOT EXISTS np_admin_audit_log (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			action VARCHAR(255) NOT NULL,
			resource_type VARCHAR(255) NOT NULL DEFAULT '',
			resource_id VARCHAR(255) NOT NULL DEFAULT '',
			user_id VARCHAR(255) NOT NULL DEFAULT '',
			details JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_admin_audit_created
			ON np_admin_audit_log(created_at DESC);
	`)
	return err
}

// CreateSnapshot inserts a new metrics snapshot and returns it.
func (d *DB) CreateSnapshot(ctx context.Context, r *CreateSnapshotRequest) (*MetricsSnapshot, error) {
	metricType := r.MetricType
	if metricType == "" {
		metricType = "system"
	}
	meta := r.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaJSON, _ := json.Marshal(meta)

	row := d.pool.QueryRow(ctx, `
		INSERT INTO np_admin_metrics_snapshots (
			source_account_id, metric_type, cpu_usage_percent,
			memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			active_connections, request_count, error_count,
			avg_response_time_ms, active_sessions, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, source_account_id, metric_type,
			cpu_usage_percent, memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			active_connections, request_count, error_count,
			avg_response_time_ms, active_sessions, metadata, created_at`,
		d.sourceAccountID, metricType,
		r.CPUUsagePercent, r.MemoryUsedBytes, r.MemoryTotalBytes,
		r.DiskUsedBytes, r.DiskTotalBytes,
		r.ActiveConnections, r.RequestCount, r.ErrorCount,
		r.AvgResponseTimeMS, r.ActiveSessions, metaJSON,
	)
	return scanSnapshot(row)
}

// ListSnapshots returns snapshots filtered by period keyword.
func (d *DB) ListSnapshots(ctx context.Context, period string) ([]*MetricsSnapshot, error) {
	interval := periodToInterval(period)
	rows, err := d.pool.Query(ctx, `
		SELECT id, source_account_id, metric_type,
			cpu_usage_percent, memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			active_connections, request_count, error_count,
			avg_response_time_ms, active_sessions, metadata, created_at
		FROM np_admin_metrics_snapshots
		WHERE source_account_id = $1
		  AND created_at >= NOW() - INTERVAL '`+interval+`'
		ORDER BY created_at DESC
		LIMIT 1000`,
		d.sourceAccountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectSnapshots(rows)
}

func periodToInterval(p string) string {
	switch p {
	case "1h":
		return "1 hour"
	case "24h":
		return "24 hours"
	case "7d":
		return "7 days"
	case "30d":
		return "30 days"
	default:
		return "24 hours"
	}
}

// GetDashboardConfig returns the dashboard config (config_key = 'dashboard').
func (d *DB) GetDashboardConfig(ctx context.Context) (*DashboardConfig, error) {
	row := d.pool.QueryRow(ctx, `
		SELECT id, source_account_id, config_key, config_value, description, created_at, updated_at
		FROM np_admin_dashboard_config
		WHERE source_account_id = $1 AND config_key = 'dashboard'`,
		d.sourceAccountID,
	)
	return scanDashboard(row)
}

// UpsertDashboardConfig saves widgets + layout under key 'dashboard'.
func (d *DB) UpsertDashboardConfig(ctx context.Context, widgets, layout map[string]any) (*DashboardConfig, error) {
	val := map[string]any{"widgets": widgets, "layout": layout}
	valJSON, _ := json.Marshal(val)

	row := d.pool.QueryRow(ctx, `
		INSERT INTO np_admin_dashboard_config (source_account_id, config_key, config_value)
		VALUES ($1, 'dashboard', $2)
		ON CONFLICT (source_account_id, config_key) DO UPDATE SET
			config_value = EXCLUDED.config_value,
			updated_at = NOW()
		RETURNING id, source_account_id, config_key, config_value, description, created_at, updated_at`,
		d.sourceAccountID, valJSON,
	)
	return scanDashboard(row)
}

// HealthCheck pings the DB and returns latency + connection info.
func (d *DB) HealthCheck(ctx context.Context) (latencyMS float64, connCount, maxConn int, version string, err error) {
	start := time.Now()
	var ver string
	if err = d.pool.QueryRow(ctx, "SELECT version()").Scan(&ver); err != nil {
		latencyMS = float64(time.Since(start).Milliseconds())
		return
	}
	latencyMS = float64(time.Since(start).Milliseconds())
	version = ver

	d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database()`).Scan(&connCount) //nolint:errcheck
	d.pool.QueryRow(ctx, `SELECT setting::int FROM pg_settings WHERE name = 'max_connections'`).Scan(&maxConn)        //nolint:errcheck
	return
}

// GetStats returns aggregate dashboard statistics.
func (d *DB) GetStats(ctx context.Context) (*DashboardStats, error) {
	row := d.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM np_admin_metrics_snapshots WHERE source_account_id = $1),
			(SELECT COUNT(*) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= CURRENT_DATE),
			(SELECT MIN(created_at) FROM np_admin_metrics_snapshots WHERE source_account_id = $1),
			(SELECT MAX(created_at) FROM np_admin_metrics_snapshots WHERE source_account_id = $1),
			(SELECT COUNT(*) FROM np_admin_dashboard_config WHERE source_account_id = $1),
			(SELECT AVG(cpu_usage_percent) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours' AND cpu_usage_percent IS NOT NULL),
			(SELECT AVG(memory_used_bytes * 100.0 / NULLIF(memory_total_bytes,0)) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours' AND memory_used_bytes IS NOT NULL),
			(SELECT MAX(active_connections) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours'),
			(SELECT SUM(request_count) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours'),
			(SELECT SUM(error_count) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours')`,
		d.sourceAccountID,
	)

	var s DashboardStats
	var oldestT, newestT *time.Time
	if err := row.Scan(
		&s.SnapshotsTotal, &s.SnapshotsToday,
		&oldestT, &newestT,
		&s.ConfigEntries,
		&s.AvgCPU24h, &s.AvgMemory24h,
		&s.PeakConnections24h, &s.TotalRequests24h, &s.TotalErrors24h,
	); err != nil {
		return nil, err
	}
	if oldestT != nil {
		ts := oldestT.UTC().Format(time.RFC3339)
		s.OldestSnapshot = &ts
	}
	if newestT != nil {
		ts := newestT.UTC().Format(time.RFC3339)
		s.NewestSnapshot = &ts
	}
	return &s, nil
}

// GetAuditLog returns recent audit events.
func (d *DB) GetAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.pool.Query(ctx,
		`SELECT id, action, resource_type, resource_id, user_id, details, created_at
		 FROM np_admin_audit_log
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.Action, &e.ResourceType, &e.ResourceID, &e.UserID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(detailsJSON, &e.Details) //nolint:errcheck
		out = append(out, &e)
	}
	return out, rows.Err()
}

// CreateAuditEntry inserts an audit event.
func (d *DB) CreateAuditEntry(ctx context.Context, r *AuditRequest) (*AuditEntry, error) {
	details := r.Details
	if details == nil {
		details = map[string]any{}
	}
	detailsJSON, _ := json.Marshal(details)

	var e AuditEntry
	var detailsBack []byte
	err := d.pool.QueryRow(ctx, `
		INSERT INTO np_admin_audit_log (action, resource_type, resource_id, user_id, details)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, action, resource_type, resource_id, user_id, details, created_at`,
		r.Action, r.ResourceType, r.ResourceID, r.UserID, detailsJSON,
	).Scan(&e.ID, &e.Action, &e.ResourceType, &e.ResourceID, &e.UserID, &detailsBack, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(detailsBack, &e.Details) //nolint:errcheck
	return &e, nil
}

// helpers

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(row rowScanner) (*MetricsSnapshot, error) {
	var s MetricsSnapshot
	var metaJSON []byte
	err := row.Scan(
		&s.ID, &s.SourceAccountID, &s.MetricType,
		&s.CPUUsagePercent, &s.MemoryUsedBytes, &s.MemoryTotalBytes,
		&s.DiskUsedBytes, &s.DiskTotalBytes,
		&s.ActiveConnections, &s.RequestCount, &s.ErrorCount,
		&s.AvgResponseTimeMS, &s.ActiveSessions,
		&metaJSON, &s.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(metaJSON, &s.Metadata) //nolint:errcheck
	return &s, nil
}

func collectSnapshots(rows pgx.Rows) ([]*MetricsSnapshot, error) {
	var out []*MetricsSnapshot
	for rows.Next() {
		var s MetricsSnapshot
		var metaJSON []byte
		if err := rows.Scan(
			&s.ID, &s.SourceAccountID, &s.MetricType,
			&s.CPUUsagePercent, &s.MemoryUsedBytes, &s.MemoryTotalBytes,
			&s.DiskUsedBytes, &s.DiskTotalBytes,
			&s.ActiveConnections, &s.RequestCount, &s.ErrorCount,
			&s.AvgResponseTimeMS, &s.ActiveSessions,
			&metaJSON, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		json.Unmarshal(metaJSON, &s.Metadata) //nolint:errcheck
		out = append(out, &s)
	}
	return out, rows.Err()
}

func scanDashboard(row rowScanner) (*DashboardConfig, error) {
	var c DashboardConfig
	var valJSON []byte
	err := row.Scan(&c.ID, &c.SourceAccountID, &c.ConfigKey, &valJSON, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(valJSON, &c.ConfigValue) //nolint:errcheck
	return &c, nil
}
