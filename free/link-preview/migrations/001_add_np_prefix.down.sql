-- Rollback: Rename np_ prefix back to lp_ for link-preview tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_linkprev_preview_analytics RENAME TO lp_preview_analytics;
ALTER TABLE IF EXISTS np_linkprev_preview_settings RENAME TO lp_preview_settings;
ALTER TABLE IF EXISTS np_linkprev_url_blocklist RENAME TO lp_url_blocklist;
ALTER TABLE IF EXISTS np_linkprev_oembed_providers RENAME TO lp_oembed_providers;
ALTER TABLE IF EXISTS np_linkprev_preview_templates RENAME TO lp_preview_templates;
ALTER TABLE IF EXISTS np_linkprev_link_preview_usage RENAME TO lp_link_preview_usage;
ALTER TABLE IF EXISTS np_linkprev_link_previews RENAME TO lp_link_previews;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_linkprev_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_linkprev_', 'idx_lp_'));
  END LOOP;
END $$;

COMMIT;
