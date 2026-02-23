-- Migration: Add np_ prefix to invitations tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `inv_` prefix which did not include the required `np_` namespace.

BEGIN;

ALTER TABLE IF EXISTS inv_invitations RENAME TO np_invites_invitations;
ALTER TABLE IF EXISTS inv_templates RENAME TO np_invites_templates;
ALTER TABLE IF EXISTS inv_bulk_sends RENAME TO np_invites_bulk_sends;
ALTER TABLE IF EXISTS inv_webhook_events RENAME TO np_invites_webhook_events;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_inv_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_inv_', 'idx_np_invites_'));
  END LOOP;
END $$;

COMMIT;
