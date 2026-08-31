-- 002_add_source_account_id.sql
-- Adds multi-app isolation column to event-bus stats table.
-- Per nSelf Multi-Tenant Convention Wall Hard Rule: Convention A (source_account_id).

ALTER TABLE np_event_bus_stats
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_event_bus_stats_sac
  ON np_event_bus_stats (source_account_id);
