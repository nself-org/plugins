-- Migration: Add np_ prefix to feature-flags tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `ff_` prefix which did not include the required `np_` namespace.

BEGIN;

ALTER TABLE IF EXISTS ff_flags RENAME TO np_flags_flags;
ALTER TABLE IF EXISTS ff_rules RENAME TO np_flags_rules;
ALTER TABLE IF EXISTS ff_segments RENAME TO np_flags_segments;
ALTER TABLE IF EXISTS ff_evaluations RENAME TO np_flags_evaluations;
ALTER TABLE IF EXISTS ff_webhook_events RENAME TO np_flags_webhook_events;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_ff_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_ff_', 'idx_np_flags_'));
  END LOOP;
END $$;

COMMIT;
