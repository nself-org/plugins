-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Reverse of 004_np_audit_log_compat_view.sql

DROP VIEW IF EXISTS public.np_audit_log;
