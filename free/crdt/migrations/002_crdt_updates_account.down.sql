-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 002_crdt_updates_account.down.sql
-- Reverse of 002_crdt_updates_account.sql

DROP INDEX IF EXISTS public.idx_np_crdt_updates_account;
ALTER TABLE public.np_crdt_updates DROP COLUMN IF EXISTS source_account_id;
