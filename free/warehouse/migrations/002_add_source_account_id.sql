-- 002_add_source_account_id.sql
-- Adds multi-app isolation column to warehouse tracking tables.
-- Per nSelf Multi-Tenant Convention Wall Hard Rule: Convention A (source_account_id).

ALTER TABLE np_warehouse_watermarks
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE np_warehouse_errors
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_warehouse_watermarks_sac
  ON np_warehouse_watermarks (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_warehouse_errors_sac
  ON np_warehouse_errors (source_account_id);
