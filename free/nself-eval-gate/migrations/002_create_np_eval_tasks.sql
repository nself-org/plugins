-- Migration 002: Create np_eval_tasks table
-- nself-eval-gate plugin — eval harness foundation
-- Idempotent: uses IF NOT EXISTS throughout

CREATE TABLE IF NOT EXISTS np_eval_tasks (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    suite_id                UUID        NOT NULL REFERENCES np_eval_suites(id) ON DELETE CASCADE,
    task_ref                TEXT        NOT NULL,
    query                   TEXT        NOT NULL,
    scoring_mode            TEXT        NOT NULL CHECK (scoring_mode IN ('exact', 'semantic', 'rubric')),
    expected_output         TEXT,
    rubric                  JSONB,
    golden_memories         JSONB,
    expected_output_contains TEXT[],
    metrics                 TEXT[]      NOT NULL DEFAULT '{}',
    threshold               FLOAT       NOT NULL DEFAULT 0.80,
    tier                    TEXT        CHECK (tier IS NULL OR tier IN ('supervised', 'semi-auto', 'full-auto')),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_account_id       TEXT        NOT NULL DEFAULT 'primary'
);

CREATE INDEX IF NOT EXISTS np_eval_tasks_suite_id_idx ON np_eval_tasks (suite_id);
CREATE INDEX IF NOT EXISTS np_eval_tasks_tier_idx ON np_eval_tasks (tier);
