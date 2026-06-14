-- Rollback: Remove source_plugin and target_plugin columns from np_auditlog_events
-- Reverse of 002_add_source_target_plugin.sql

DROP INDEX IF EXISTS idx_np_auditlog_target_plugin;
DROP INDEX IF EXISTS idx_np_auditlog_source_plugin;

ALTER TABLE np_auditlog_events
    DROP COLUMN IF EXISTS target_plugin;

ALTER TABLE np_auditlog_events
    DROP COLUMN IF EXISTS source_plugin;
