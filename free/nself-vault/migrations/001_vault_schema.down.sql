-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback of 001_vault_schema (not applied at boot).
DROP TABLE IF EXISTS public.np_vault_audit;
DROP TABLE IF EXISTS public.np_vault_envelopes;
DROP TABLE IF EXISTS public.np_vault_records;
DROP TABLE IF EXISTS public.np_vault_devices;
