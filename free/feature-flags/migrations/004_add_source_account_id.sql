-- Migration 004: Add source_account_id for multi-app isolation
-- Required by the Multi-Tenant Convention Wall (Hard Rule).
-- Additive only — safe to apply on running instances.
-- Feature flags are admin-managed resources; all rows default to 'primary'.

ALTER TABLE np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE np_feature_flags_segments
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE np_feature_flags_audit
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_ff_flags_source_account ON np_feature_flags_flags(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_ff_segments_source_account ON np_feature_flags_segments(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_ff_audit_source_account ON np_feature_flags_audit(source_account_id);
