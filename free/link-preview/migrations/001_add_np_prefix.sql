-- Migration 001: add the np_ prefix to the link-preview tables (guarded renames only).
-- Renames the tables (and their indexes) an older (TypeScript-era) release created with the lp_ prefix.
-- Old names: lp_*. New names: np_linkprev_*.
-- Guarded, idempotent, no transaction control (sdk/go/migrate refuses it, and applies each file itself):
--   * a table is renamed only when the old name exists in public AND the new name is free;
--   * when both names exist, both are left untouched (nothing merged, nothing dropped);
--   * an index is renamed only when it sits on a renamed np_ table and the new index name is free.
-- A fresh database has none of the old tables, so this file is a no-op there; the table the Go code uses is created by 003.
-- The reverse is 001_add_np_prefix.down.sql (never applied at boot).

DO $$
DECLARE
  t RECORD;
  i RECORD;
  new_idx TEXT;
BEGIN
  FOR t IN
    SELECT old_name, new_name FROM (VALUES
      ('lp_link_previews', 'np_linkprev_link_previews')
      , ('lp_link_preview_usage', 'np_linkprev_link_preview_usage')
      , ('lp_preview_templates', 'np_linkprev_preview_templates')
      , ('lp_oembed_providers', 'np_linkprev_oembed_providers')
      , ('lp_url_blocklist', 'np_linkprev_url_blocklist')
      , ('lp_preview_settings', 'np_linkprev_preview_settings')
      , ('lp_preview_analytics', 'np_linkprev_preview_analytics')
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
      AND tablename IN ('np_linkprev_link_previews', 'np_linkprev_link_preview_usage', 'np_linkprev_preview_templates', 'np_linkprev_oembed_providers', 'np_linkprev_url_blocklist', 'np_linkprev_preview_settings', 'np_linkprev_preview_analytics')
      AND starts_with(indexname, 'idx_lp_')
  LOOP
    new_idx := 'idx_np_linkprev_' || substr(i.indexname, length('idx_lp_') + 1);
    IF to_regclass('public.' || quote_ident(new_idx)) IS NULL THEN
      EXECUTE format('ALTER INDEX public.%I RENAME TO %I', i.indexname, new_idx);
    END IF;
  END LOOP;
END $$;
