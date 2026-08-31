-- Migration 004: Create np_eval_thresholds table
-- nself-eval-gate plugin — eval harness foundation
-- INTENTIONALLY has NO source_account_id — global config, not per-tenant.
-- Threshold updates require admin role (see spec §8 permissions matrix).
-- Idempotent: uses IF NOT EXISTS throughout

CREATE TABLE IF NOT EXISTS np_eval_thresholds (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    autonomy_tier  TEXT        NOT NULL UNIQUE CHECK (autonomy_tier IN ('supervised', 'semi-auto', 'full-auto')),
    min_pass_rate  FLOAT       NOT NULL,
    min_suite_score FLOAT      NOT NULL,
    applies_to     TEXT[]      NOT NULL DEFAULT '{}',
    enforced       BOOLEAN     NOT NULL DEFAULT true,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
