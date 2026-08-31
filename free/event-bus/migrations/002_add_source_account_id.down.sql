-- 002_add_source_account_id.down.sql
-- Reverse of 002_add_source_account_id.sql

DROP INDEX IF EXISTS idx_np_event_bus_stats_sac;

ALTER TABLE np_event_bus_stats
  DROP COLUMN IF EXISTS source_account_id;
