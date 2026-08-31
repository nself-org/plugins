-- Migration 003: Create np_eval_runs table
-- nself-eval-gate plugin — eval harness foundation
-- Idempotent: uses IF NOT EXISTS throughout

CREATE TABLE IF NOT EXISTS np_eval_runs (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    suite_id         UUID        NOT NULL REFERENCES np_eval_suites(id),
    triggered_by     TEXT        NOT NULL CHECK (triggered_by IN ('ci', 'manual', 'pre-tier-promotion')),
    commit_sha       TEXT,
    branch           TEXT,
    pass_rate        FLOAT       NOT NULL,
    suite_score      FLOAT       NOT NULL,
    passed           BOOLEAN     NOT NULL,
    results          JSONB       NOT NULL DEFAULT '[]',
    duration_ms      INT,
    model_judge      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_account_id TEXT       NOT NULL DEFAULT 'primary'
);

CREATE INDEX IF NOT EXISTS np_eval_runs_suite_created_idx ON np_eval_runs (suite_id, created_at DESC);
CREATE INDEX IF NOT EXISTS np_eval_runs_passed_idx ON np_eval_runs (passed);
