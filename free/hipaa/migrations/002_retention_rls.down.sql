-- 002_retention_rls.down.sql
-- Reverse of 002_retention_rls.sql
-- Removes RLS policies and the hipaa_admin role from np_phi_audit_log.

-- 1. Drop all policies on the table (must be done before DISABLE ROW LEVEL SECURITY).
DROP POLICY IF EXISTS phi_audit_log_select        ON np_phi_audit_log;
DROP POLICY IF EXISTS phi_audit_log_insert        ON np_phi_audit_log;
DROP POLICY IF EXISTS phi_audit_log_no_update     ON np_phi_audit_log;
DROP POLICY IF EXISTS phi_audit_log_retention_delete ON np_phi_audit_log;

-- 2. Remove the FORCE and disable RLS.
ALTER TABLE np_phi_audit_log NO FORCE ROW LEVEL SECURITY;
ALTER TABLE np_phi_audit_log DISABLE ROW LEVEL SECURITY;

-- 3. Revoke grants from hipaa_admin.
REVOKE SELECT, INSERT, DELETE ON np_phi_audit_log FROM hipaa_admin;

-- 4. Drop role only if it has no other dependencies.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hipaa_admin') THEN
        DROP ROLE hipaa_admin;
    END IF;
END $$;
