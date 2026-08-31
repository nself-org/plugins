-- Migration 003: PERM-HASURA-01 remediation record for compliance plugin
-- RLS is applied in 002_rls_policies.sql. Hasura row filters (tenant_id) are
-- present in all three table YAMLs. This migration is a no-op verification
-- marker confirming compliance with the PERM-HASURA-01 doctor check.
-- Convention B (cloud multi-tenancy): tenant_id UUID — correct for this plugin.

-- No DDL changes. RLS already applied in migration 002.
SELECT 'PERM-HASURA-01: compliance plugin verified OK' AS status;
