-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Down migration for 001_init.sql — nself-post initial schema
-- Drop in reverse dependency order (queue FK depends on accounts)
DROP TABLE IF EXISTS public.np_post_queue;
DROP TABLE IF EXISTS public.np_post_accounts;
