-- Migration: Add np_ prefix to link-preview tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `lp_` prefix which did not include the required `np_` namespace.

BEGIN;

ALTER TABLE IF EXISTS lp_link_previews RENAME TO np_linkprev_link_previews;
ALTER TABLE IF EXISTS lp_link_preview_usage RENAME TO np_linkprev_link_preview_usage;
ALTER TABLE IF EXISTS lp_preview_templates RENAME TO np_linkprev_preview_templates;
ALTER TABLE IF EXISTS lp_oembed_providers RENAME TO np_linkprev_oembed_providers;
ALTER TABLE IF EXISTS lp_url_blocklist RENAME TO np_linkprev_url_blocklist;
ALTER TABLE IF EXISTS lp_preview_settings RENAME TO np_linkprev_preview_settings;
ALTER TABLE IF EXISTS lp_preview_analytics RENAME TO np_linkprev_preview_analytics;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_lp_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_lp_', 'idx_np_linkprev_'));
  END LOOP;
END $$;

COMMIT;
