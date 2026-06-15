-- Rollback: Remove type, rollout_pct, and stale_after_days columns from np_feature_flags_flags
-- Reverse of 003_add_flag_type_rollout.sql

DROP INDEX IF EXISTS idx_np_ff_flags_type;

ALTER TABLE np_feature_flags_flags
    DROP COLUMN IF EXISTS stale_after_days;

ALTER TABLE np_feature_flags_flags
    DROP COLUMN IF EXISTS rollout_pct;

ALTER TABLE np_feature_flags_flags
    DROP COLUMN IF EXISTS type;
