-- Rollback: Remove tenant_id column from np_auditlog_events
-- Reverse of 003_add_tenant_id_forward_compat.sql

DROP INDEX IF EXISTS idx_np_auditlog_tenant_id;

ALTER TABLE np_auditlog_events
    DROP COLUMN IF EXISTS tenant_id;
