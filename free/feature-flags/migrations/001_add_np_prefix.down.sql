-- Rollback: Rename np_ prefix back to ff_ for feature-flags tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_flags_flags RENAME TO ff_flags;
ALTER TABLE IF EXISTS np_flags_rules RENAME TO ff_rules;
ALTER TABLE IF EXISTS np_flags_segments RENAME TO ff_segments;
ALTER TABLE IF EXISTS np_flags_evaluations RENAME TO ff_evaluations;
ALTER TABLE IF EXISTS np_flags_webhook_events RENAME TO ff_webhook_events;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_flags_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_flags_', 'idx_ff_'));
  END LOOP;
END $$;

COMMIT;
