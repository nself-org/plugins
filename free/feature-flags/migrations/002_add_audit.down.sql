-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback: Drop audit log table for feature flags
-- Reverse of 002_add_audit.sql

DROP INDEX IF EXISTS idx_np_ff_audit_actor;
DROP INDEX IF EXISTS idx_np_ff_audit_ts;
DROP INDEX IF EXISTS idx_np_ff_audit_flag_key;
DROP TABLE IF EXISTS np_feature_flags_audit;
