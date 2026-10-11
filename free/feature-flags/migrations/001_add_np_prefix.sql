-- Migration 001: add the np_flags_ prefix to the feature-flags tables (guarded renames only).
-- Renames the tables (and their indexes) an older release created without the np_ prefix.
-- Old names: ff_*. New names: np_flags_*. The TS-era ff_* tables (flag_type, tags, rules in their own table) do not match the Go tables np_feature_flags_*, so their data is parked under np_flags_* (preserved, not read by the Go plugin).
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
      ('ff_flags', 'np_flags_flags')
      , ('ff_rules', 'np_flags_rules')
      , ('ff_segments', 'np_flags_segments')
      , ('ff_evaluations', 'np_flags_evaluations')
      , ('ff_webhook_events', 'np_flags_webhook_events')
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
      AND tablename IN ('np_flags_flags', 'np_flags_rules', 'np_flags_segments', 'np_flags_evaluations', 'np_flags_webhook_events')
      AND starts_with(indexname, 'idx_ff_')
  LOOP
    new_idx := 'idx_np_flags_' || substr(i.indexname, length('idx_ff_') + 1);
    IF to_regclass('public.' || quote_ident(new_idx)) IS NULL THEN
      EXECUTE format('ALTER INDEX public.%I RENAME TO %I', i.indexname, new_idx);
    END IF;
  END LOOP;
END $$;
