-- Migration 001: Create np_eval_suites table
-- nself-eval-gate plugin — eval harness foundation
-- Idempotent: uses IF NOT EXISTS throughout

CREATE TABLE IF NOT EXISTS np_eval_suites (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             TEXT        NOT NULL,
    version          TEXT        NOT NULL,
    suite_type       TEXT        NOT NULL CHECK (suite_type IN ('recall', 'generation', 'mixed')),
    description      TEXT,
    repo             TEXT        NOT NULL,
    schema_ver       TEXT        NOT NULL DEFAULT '1',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_account_id TEXT       NOT NULL DEFAULT 'primary',
    CONSTRAINT np_eval_suites_slug_source_uniq UNIQUE (slug, source_account_id)
);

CREATE INDEX IF NOT EXISTS np_eval_suites_slug_idx ON np_eval_suites (slug);
CREATE INDEX IF NOT EXISTS np_eval_suites_repo_idx ON np_eval_suites (repo);
