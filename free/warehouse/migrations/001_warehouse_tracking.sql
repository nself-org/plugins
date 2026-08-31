-- G01: Data Warehouse Export — tracking tables
-- Uses np_ prefix per nSelf convention.

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

-- Index for surfacing recent errors in Admin UI
CREATE INDEX IF NOT EXISTS np_warehouse_errors_occurred_at_idx
  ON np_warehouse_errors (occurred_at DESC);
