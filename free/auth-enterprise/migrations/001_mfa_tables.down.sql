-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_mfa_tables.down.sql
-- Reverse of 001_mfa_tables.sql
-- Drops MFA enrollment, recovery codes, and policy tables.
-- Policies are dropped automatically when their table is dropped.

-- Drop in declaration order (no FK dependencies between these three tables).
DROP TABLE IF EXISTS np_mfa_policies;
DROP TABLE IF EXISTS np_mfa_recovery_codes;
DROP TABLE IF EXISTS np_mfa_enrollments;
