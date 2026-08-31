-- 001_add_source_account_id.down.sql
-- Reverse of 001_add_source_account_id.sql

DROP INDEX IF EXISTS idx_idme_verifications_sac;
DROP INDEX IF EXISTS idx_idme_groups_sac;
DROP INDEX IF EXISTS idx_idme_badges_sac;
DROP INDEX IF EXISTS idx_idme_attributes_sac;
DROP INDEX IF EXISTS idx_idme_webhook_events_sac;

ALTER TABLE idme_verifications
  DROP COLUMN IF EXISTS source_account_id;

ALTER TABLE idme_groups
  DROP COLUMN IF EXISTS source_account_id;

ALTER TABLE idme_badges
  DROP COLUMN IF EXISTS source_account_id;

ALTER TABLE idme_attributes
  DROP COLUMN IF EXISTS source_account_id;

ALTER TABLE idme_webhook_events
  DROP COLUMN IF EXISTS source_account_id;
