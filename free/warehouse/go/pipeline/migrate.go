package pipeline

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// migrationSQL is inlined to keep the Go module self-contained.
// Canonical SQL lives at plugins-pro/paid/warehouse/migrations/001_warehouse_tracking.sql.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS np_warehouse_watermarks (
  table_name   TEXT PRIMARY KEY,
  last_lsn     TEXT,
  exported_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  row_count    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS np_warehouse_errors (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  table_name  TEXT NOT NULL,
  batch_id    TEXT NOT NULL,
  error       TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS np_warehouse_errors_occurred_at_idx
  ON np_warehouse_errors (occurred_at DESC);
`

// Migrate creates the np_warehouse_watermarks and np_warehouse_errors tables
// if they do not already exist.
func Migrate(ctx context.Context, databaseURL string) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect for migration: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, migrationSQL); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}
