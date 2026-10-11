-- Migration 004: Add source_account_id for multi-app isolation
-- Required by the Multi-Tenant Convention Wall (Hard Rule).
-- Additive only — safe to apply on running instances.
-- Feature flags are admin-managed resources; all rows default to 'primary'.
-- Every object is public-qualified (sdk/go/migrate pins search_path to np_feature_flags, public).

ALTER TABLE public.np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_feature_flags_segments
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE public.np_feature_flags_audit
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_ff_flags_source_account ON public.np_feature_flags_flags(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_ff_segments_source_account ON public.np_feature_flags_segments(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_ff_audit_source_account ON public.np_feature_flags_audit(source_account_id);
