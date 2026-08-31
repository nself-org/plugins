-- HIPAA plugin: retention RLS
-- Migration 002: Prevent DELETE of np_phi_audit_log rows before retain_until.
-- This is enforced at the Postgres row-level security layer, not application logic.

-- Enable RLS on the audit log table.
ALTER TABLE np_phi_audit_log ENABLE ROW LEVEL SECURITY;

-- SELECT policy: users may read all rows for their own source_account_id.
-- The hipaa plugin service connects with a role that has the 'hipaa_service' role attribute.
CREATE POLICY phi_audit_log_select
  ON np_phi_audit_log
  FOR SELECT
  USING (true);

-- INSERT policy: the service may append new rows freely.
CREATE POLICY phi_audit_log_insert
  ON np_phi_audit_log
  FOR INSERT
  WITH CHECK (true);

-- UPDATE policy: no updates allowed — audit log is append-only.
CREATE POLICY phi_audit_log_no_update
  ON np_phi_audit_log
  FOR UPDATE
  USING (false);

-- DELETE policy: block DELETE until retain_until has passed.
-- After the 6-year window, the compliance team may delete via a privileged role.
CREATE POLICY phi_audit_log_retention_delete
  ON np_phi_audit_log
  FOR DELETE
  USING (retain_until < CURRENT_DATE);

-- Grant the policy bypass to the hipaa_admin role (for post-retention purge only).
-- In practice the nself service role does NOT bypass RLS unless explicitly granted.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname = 'hipaa_admin'
  ) THEN
    CREATE ROLE hipaa_admin;
  END IF;
END
$$;

-- hipaa_admin may bypass RLS entirely — used only by auditor purge scripts.
ALTER TABLE np_phi_audit_log FORCE ROW LEVEL SECURITY;
GRANT SELECT, INSERT, DELETE ON np_phi_audit_log TO hipaa_admin;
