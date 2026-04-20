-- Migration 002: Add audit log table for feature flags state changes
-- Additive only (no DROP statements) — safe to apply on running instances.
--
-- Table: np_feature_flags_audit
-- Purpose: Immutable append-only log of every create/enable/disable/kill/set/delete.
--
-- Retention: Application-level (recommend 365 days; prune via cron on audit table).

CREATE TABLE IF NOT EXISTS np_feature_flags_audit (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    flag_key   VARCHAR(255) NOT NULL,
    actor      TEXT         NOT NULL,
    action     VARCHAR(50)  NOT NULL
                CHECK (action IN ('create', 'enable', 'disable', 'kill', 'set', 'delete')),
    before     JSONB,
    after      JSONB,
    reason     TEXT,
    ts         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Index for per-flag audit queries (most common access pattern)
CREATE INDEX IF NOT EXISTS idx_np_ff_audit_flag_key ON np_feature_flags_audit(flag_key);

-- Index for time-range queries (retention sweep, compliance export)
CREATE INDEX IF NOT EXISTS idx_np_ff_audit_ts ON np_feature_flags_audit(ts DESC);

-- Index for actor queries (who changed what)
CREATE INDEX IF NOT EXISTS idx_np_ff_audit_actor ON np_feature_flags_audit(actor);
