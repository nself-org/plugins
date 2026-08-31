// Package clickhouse implements the warehouse Driver for ClickHouse.
// Uses the official clickhouse-go/v2 driver for native TCP protocol (low latency).
package clickhouse

import (
	"context"
	"fmt"
	"strings"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
)

// Driver writes rows to ClickHouse via the native TCP protocol.
type Driver struct {
	conn chdriver.Conn
}

// New opens a ClickHouse connection using the provided DSN.
// DSN format: clickhouse://user:pass@host:9000/database?dial_timeout=5s
func New(ctx context.Context, dsn string) (*Driver, error) {
	if dsn == "" {
		return nil, fmt.Errorf("WAREHOUSE_CLICKHOUSE_DSN is required")
	}
	opts, err := chdriver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse DSN: %w", err)
	}
	conn, err := chdriver.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}
	return &Driver{conn: conn}, nil
}

// EnsureTable creates a ReplacingMergeTree table if it does not already exist.
// This is idempotent — re-running on an existing table is a no-op.
func (d *Driver) EnsureTable(ctx context.Context, table string, columns map[string]string) error {
	cols := buildColumnDDL(columns)
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			%s
		) ENGINE = ReplacingMergeTree()
		ORDER BY tuple()
	`, table, cols)
	return d.conn.Exec(ctx, ddl)
}

// WriteBatch inserts rows into ClickHouse using a batch insert (async insert
// disabled; synchronous for <2s lag guarantee).
func (d *Driver) WriteBatch(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	// Derive column order from the first row
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}

	// #nosec G201 — table name is validated against np_* allowlist upstream
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	batch, err := d.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, row := range rows {
		vals := make([]interface{}, len(cols))
		for i, c := range cols {
			vals[i] = row[c]
		}
		if err := batch.Append(vals...); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batch.Send()
}

// Close releases the ClickHouse connection.
func (d *Driver) Close() error {
	return d.conn.Close()
}

// buildColumnDDL converts a map of col→type into a DDL column list.
func buildColumnDDL(columns map[string]string) string {
	parts := make([]string, 0, len(columns))
	for name, typ := range columns {
		parts = append(parts, fmt.Sprintf("`%s` %s", name, typ))
	}
	return strings.Join(parts, ",\n\t\t\t")
}
