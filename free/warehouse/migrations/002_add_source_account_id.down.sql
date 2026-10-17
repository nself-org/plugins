-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 002_add_source_account_id.down.sql
-- Reverse of 002_add_source_account_id.sql

DROP INDEX IF EXISTS idx_np_warehouse_watermarks_sac;
DROP INDEX IF EXISTS idx_np_warehouse_errors_sac;

ALTER TABLE np_warehouse_watermarks
  DROP COLUMN IF EXISTS source_account_id;

ALTER TABLE np_warehouse_errors
  DROP COLUMN IF EXISTS source_account_id;
