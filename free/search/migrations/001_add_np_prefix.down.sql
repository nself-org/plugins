-- Rollback: Rename np_ prefix back to search_ for search tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_search_webhook_events RENAME TO search_webhook_events;
ALTER TABLE IF EXISTS np_search_queries RENAME TO search_queries;
ALTER TABLE IF EXISTS np_search_synonyms RENAME TO search_synonyms;
ALTER TABLE IF EXISTS np_search_documents RENAME TO search_documents;
ALTER TABLE IF EXISTS np_search_indexes RENAME TO search_indexes;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_search_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_search_', 'idx_search_'));
  END LOOP;
END $$;

COMMIT;
