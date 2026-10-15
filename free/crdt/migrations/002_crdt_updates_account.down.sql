-- 002_crdt_updates_account.down.sql
-- Reverse of 002_crdt_updates_account.sql

DROP INDEX IF EXISTS public.idx_np_crdt_updates_account;
ALTER TABLE public.np_crdt_updates DROP COLUMN IF EXISTS source_account_id;
