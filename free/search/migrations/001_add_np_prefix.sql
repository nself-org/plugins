-- Migration: Add np_ prefix to search tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `search_` prefix without the required `np_` namespace wrapper.

BEGIN;

ALTER TABLE IF EXISTS search_indexes RENAME TO np_search_indexes;
ALTER TABLE IF EXISTS search_documents RENAME TO np_search_documents;
ALTER TABLE IF EXISTS search_synonyms RENAME TO np_search_synonyms;
ALTER TABLE IF EXISTS search_queries RENAME TO np_search_queries;
ALTER TABLE IF EXISTS search_webhook_events RENAME TO np_search_webhook_events;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_search_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_search_', 'idx_np_search_'));
  END LOOP;
END $$;

COMMIT;
