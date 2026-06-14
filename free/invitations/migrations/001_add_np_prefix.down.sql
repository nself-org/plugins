-- Rollback: Rename np_ prefix back to inv_ for invitations tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_invites_webhook_events RENAME TO inv_webhook_events;
ALTER TABLE IF EXISTS np_invites_bulk_sends RENAME TO inv_bulk_sends;
ALTER TABLE IF EXISTS np_invites_templates RENAME TO inv_templates;
ALTER TABLE IF EXISTS np_invites_invitations RENAME TO inv_invitations;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_invites_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_invites_', 'idx_inv_'));
  END LOOP;
END $$;

COMMIT;
