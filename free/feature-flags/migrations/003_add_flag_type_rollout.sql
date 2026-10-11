-- Migration 003: create the Go flag and segment tables, and add type, rollout_pct and stale_after_days to
-- np_feature_flags_flags when an older Go install created it without them.
--
-- Why creation lives here: migration 001 only parks the TS-era ff_* tables as np_flags_* (they do not match the Go
-- tables), so np_feature_flags_flags and np_feature_flags_segments are first created by this file. Idempotent and
-- additive (no DROP): a fresh database gets the full tables, an install whose plugin created them at boot keeps its rows.
--
-- type:             flag category ('boolean', 'percentage', 'kill_switch', 'experiment', 'config')
-- rollout_pct:      0-100 integer, top-level rollout percentage (complement to per-rule percentage)
-- stale_after_days: if set, flags unmodified for this many days appear in the prune report

CREATE TABLE IF NOT EXISTS public.np_feature_flags_flags (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    key             VARCHAR(255) NOT NULL UNIQUE,
    name            VARCHAR(255),
    description     TEXT,
    type            VARCHAR(50)  NOT NULL DEFAULT 'boolean'
                    CHECK (type IN ('boolean', 'percentage', 'kill_switch', 'experiment', 'config')),
    enabled         BOOLEAN      DEFAULT false,
    rollout_pct     INTEGER      CHECK (rollout_pct IS NULL OR (rollout_pct >= 0 AND rollout_pct <= 100)),
    stale_after_days INTEGER     CHECK (stale_after_days IS NULL OR stale_after_days > 0),
    default_value   JSONB        DEFAULT 'false',
    rules           JSONB        DEFAULT '[]',
    created_at      TIMESTAMPTZ  DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  DEFAULT NOW()
);

ALTER TABLE public.np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS type VARCHAR(50) NOT NULL DEFAULT 'boolean'
        CHECK (type IN ('boolean', 'percentage', 'kill_switch', 'experiment', 'config'));

ALTER TABLE public.np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS rollout_pct INTEGER
        CHECK (rollout_pct IS NULL OR (rollout_pct >= 0 AND rollout_pct <= 100));

ALTER TABLE public.np_feature_flags_flags
    ADD COLUMN IF NOT EXISTS stale_after_days INTEGER
        CHECK (stale_after_days IS NULL OR stale_after_days > 0);

CREATE INDEX IF NOT EXISTS idx_np_ff_flags_key ON public.np_feature_flags_flags(key);
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_enabled ON public.np_feature_flags_flags(enabled);
-- Index for type-filtered list queries
CREATE INDEX IF NOT EXISTS idx_np_ff_flags_type ON public.np_feature_flags_flags(type);

CREATE TABLE IF NOT EXISTS public.np_feature_flags_segments (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL UNIQUE,
    rules      JSONB        DEFAULT '[]',
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    updated_at TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_ff_segments_name ON public.np_feature_flags_segments(name);
