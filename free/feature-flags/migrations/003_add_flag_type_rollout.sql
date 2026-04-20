-- Migration 003: Add type, rollout_pct, and stale_after_days columns to np_feature_flags_flags
-- Additive only (no DROP statements) — safe to apply on running instances.
--
-- type:             flag category ('boolean', 'percentage', 'kill_switch', 'experiment', 'config')
-- rollout_pct:      0-100 integer, top-level rollout percentage (complement to per-rule percentage)
-- stale_after_days: if set, flags unmodified for this many days appear in the prune report

ALTER TABLE np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS type VARCHAR(50) NOT NULL DEFAULT 'boolean'
        CHECK (type IN ('boolean', 'percentage', 'kill_switch', 'experiment', 'config'));

ALTER TABLE np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS rollout_pct INTEGER
        CHECK (rollout_pct IS NULL OR (rollout_pct >= 0 AND rollout_pct <= 100));

ALTER TABLE np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS stale_after_days INTEGER
        CHECK (stale_after_days IS NULL OR stale_after_days > 0);

-- Index for type-filtered list queries
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_type ON np_feature_flags_flags(type);
