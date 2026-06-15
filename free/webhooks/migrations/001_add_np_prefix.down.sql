-- Rollback: Rename np_ prefix back to webhook_ for webhooks tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_webhooks_dead_letters RENAME TO webhook_dead_letters;
ALTER TABLE IF EXISTS np_webhooks_event_types RENAME TO webhook_event_types;
ALTER TABLE IF EXISTS np_webhooks_deliveries RENAME TO webhook_deliveries;
ALTER TABLE IF EXISTS np_webhooks_endpoints RENAME TO webhook_endpoints;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_webhooks_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_webhooks_', 'idx_webhook_'));
  END LOOP;
END $$;

COMMIT;
