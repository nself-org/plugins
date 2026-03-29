-- Migration: Add np_ prefix to webhooks tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `webhook_` prefix without the required `np_` namespace wrapper.

BEGIN;

ALTER TABLE IF EXISTS webhook_endpoints RENAME TO np_webhooks_endpoints;
ALTER TABLE IF EXISTS webhook_deliveries RENAME TO np_webhooks_deliveries;
ALTER TABLE IF EXISTS webhook_event_types RENAME TO np_webhooks_event_types;
ALTER TABLE IF EXISTS webhook_dead_letters RENAME TO np_webhooks_dead_letters;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_webhook_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_webhook_', 'idx_np_webhooks_'));
  END LOOP;
END $$;

COMMIT;
