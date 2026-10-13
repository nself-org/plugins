-- 001_add_source_account_id.sql
-- Boot-applied by sdk/go/migrate with search_path pinned to np_idme, so the tables are public-qualified.
-- Adds multi-app isolation column to all idme plugin tables.
-- Per nSelf Multi-Tenant Convention Wall Hard Rule: Convention A (source_account_id).

ALTER TABLE public.np_idme_verifications
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_idme_groups
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_idme_badges
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_idme_attributes
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_idme_webhook_events
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_idme_verifications_sac
  ON public.np_idme_verifications (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_idme_groups_sac
  ON public.np_idme_groups (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_idme_badges_sac
  ON public.np_idme_badges (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_idme_attributes_sac
  ON public.np_idme_attributes (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_idme_webhook_events_sac
  ON public.np_idme_webhook_events (source_account_id);
