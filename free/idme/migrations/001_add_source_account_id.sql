-- 001_add_source_account_id.sql
-- Adds multi-app isolation column to all idme plugin tables.
-- Per nSelf Multi-Tenant Convention Wall Hard Rule: Convention A (source_account_id).

ALTER TABLE idme_verifications
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE idme_groups
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE idme_badges
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE idme_attributes
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE idme_webhook_events
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_idme_verifications_sac
  ON idme_verifications (source_account_id);

CREATE INDEX IF NOT EXISTS idx_idme_groups_sac
  ON idme_groups (source_account_id);

CREATE INDEX IF NOT EXISTS idx_idme_badges_sac
  ON idme_badges (source_account_id);

CREATE INDEX IF NOT EXISTS idx_idme_attributes_sac
  ON idme_attributes (source_account_id);

CREATE INDEX IF NOT EXISTS idx_idme_webhook_events_sac
  ON idme_webhook_events (source_account_id);
