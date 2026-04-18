-- DOWN Migration: 001_audit_log_init
-- Plugin: audit-log
-- Description: Removes all audit-log database objects in reverse order.
--              WARNING: This permanently destroys all audit event data.
--              Only run during plugin uninstallation after confirming with
--              the operator that the data is no longer needed.

-- Drop RLS policies first (they reference the table).
DROP POLICY IF EXISTS np_auditlog_no_update  ON np_auditlog_events;
DROP POLICY IF EXISTS np_auditlog_no_delete  ON np_auditlog_events;
DROP POLICY IF EXISTS np_auditlog_insert     ON np_auditlog_events;
DROP POLICY IF EXISTS np_auditlog_select     ON np_auditlog_events;

-- Drop indexes (dropped automatically with the table, but listed for clarity).
DROP INDEX IF EXISTS idx_np_auditlog_actor_user;
DROP INDEX IF EXISTS idx_np_auditlog_event_type;
DROP INDEX IF EXISTS idx_np_auditlog_account_time;

-- Drop the default partition, then the parent table (CASCADE covers any
-- additional month partitions that were created after initial install).
DROP TABLE IF EXISTS np_auditlog_events_default;
DROP TABLE IF EXISTS np_auditlog_events CASCADE;
