-- Migration 001: add the np_ prefix to the search tables (guarded renames only).
-- Renames the tables (and their indexes) an older release created without the np_ prefix.
-- Old names: search_*. New names: np_search_*.
-- Guarded, idempotent, no transaction control (sdk/go/migrate refuses it, and applies each file itself):
--   * a table is renamed only when the old name exists in public AND the new name is free;
--   * when both names exist, both are left untouched (nothing merged, nothing dropped); the plugin log reports it;
--   * an index is renamed only when it sits on a renamed np_ table and the new index name is free.
-- A fresh database has none of the old tables, so this file is a no-op there; the tables themselves are created by the next migration file.

DO $$
DECLARE
  t RECORD;
  i RECORD;
  new_idx TEXT;
BEGIN
  FOR t IN
    SELECT old_name, new_name FROM (VALUES
      ('search_indexes', 'np_search_indexes')
      , ('search_documents', 'np_search_documents')
      , ('search_synonyms', 'np_search_synonyms')
      , ('search_queries', 'np_search_queries')
      , ('search_webhook_events', 'np_search_webhook_events')
    ) AS v(old_name, new_name)
  LOOP
    IF to_regclass('public.' || quote_ident(t.old_name)) IS NOT NULL
       AND to_regclass('public.' || quote_ident(t.new_name)) IS NULL THEN
      EXECUTE format('ALTER TABLE public.%I RENAME TO %I', t.old_name, t.new_name);
    END IF;
  END LOOP;

  FOR i IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND tablename IN ('np_search_indexes', 'np_search_documents', 'np_search_synonyms', 'np_search_queries', 'np_search_webhook_events')
      AND starts_with(indexname, 'idx_search_')
  LOOP
    new_idx := 'idx_np_search_' || substr(i.indexname, length('idx_search_') + 1);
    IF to_regclass('public.' || quote_ident(new_idx)) IS NULL THEN
      EXECUTE format('ALTER INDEX public.%I RENAME TO %I', i.indexname, new_idx);
    END IF;
  END LOOP;
END $$;
