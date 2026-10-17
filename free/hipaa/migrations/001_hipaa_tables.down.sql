-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_hipaa_tables.down.sql
-- Reverse of 001_hipaa_tables.sql
-- NOTE: Run 002_retention_rls.down.sql BEFORE this file to drop RLS policies
-- and roles that reference np_phi_audit_log prior to dropping the table.

-- Drop in declaration order (no FK references between these three tables).
DROP TABLE IF EXISTS np_phi_audit_log;
DROP TABLE IF EXISTS np_phi_columns;
DROP TABLE IF EXISTS np_baa_records;
