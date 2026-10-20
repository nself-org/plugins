-- Expand-only. No foreign key crosses the set: attempt, job and pipeline are core tables, read by store code.
-- Money is integer micro-units. A rate snapshot (rate_micro per rate_unit_ms, billing increment, minimum charge, extras) is
-- kept on the row it priced, so finalize and teardown never need the policy that was in force at admission.
CREATE TABLE ledger (attempt_id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, project TEXT NOT NULL, provider TEXT NOT NULL, class TEXT NOT NULL, remote INTEGER NOT NULL,
  est_ms INTEGER NOT NULL, est_micro INTEGER NOT NULL, act_ms INTEGER, act_micro INTEGER, confidence TEXT NOT NULL, budget_refs TEXT NOT NULL, window_day TEXT NOT NULL,
  rate_micro INTEGER NOT NULL, rate_unit_ms INTEGER NOT NULL, inc_ms INTEGER NOT NULL, min_micro INTEGER NOT NULL, extras_micro INTEGER NOT NULL,
  admitted_at INTEGER NOT NULL, finalized_at INTEGER);
CREATE INDEX ledger_provider ON ledger(provider, admitted_at);
CREATE INDEX ledger_pipeline ON ledger(pipeline_id);
CREATE TABLE resource_ledger (intent TEXT PRIMARY KEY, provider TEXT NOT NULL, pipeline_id TEXT NOT NULL, reserved_micro INTEGER NOT NULL, window_day TEXT NOT NULL,
  rate_micro INTEGER NOT NULL, rate_unit_ms INTEGER NOT NULL, inc_ms INTEGER NOT NULL, min_micro INTEGER NOT NULL, extras_micro INTEGER NOT NULL,
  provisioned_at INTEGER NOT NULL, confirmed_teardown_at INTEGER, actual_micro INTEGER);
CREATE TABLE reservation (owner TEXT NOT NULL, scope TEXT NOT NULL, scope_key TEXT NOT NULL, window_key TEXT NOT NULL, micro INTEGER NOT NULL, actual_micro INTEGER, settled INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (owner, scope));
CREATE INDEX reservation_scope ON reservation(scope, scope_key, window_key);
CREATE TABLE window_high (kind TEXT PRIMARY KEY, key TEXT NOT NULL);
CREATE TABLE ledger_usage (key TEXT PRIMARY KEY, provider TEXT NOT NULL, period TEXT NOT NULL, day TEXT NOT NULL, series TEXT NOT NULL, minutes INTEGER NOT NULL, cost_micro INTEGER NOT NULL, balance_micro INTEGER, reported_at INTEGER NOT NULL);
CREATE INDEX ledger_usage_provider ON ledger_usage(provider, period);
CREATE TABLE approval (run TEXT PRIMARY KEY, sha TEXT NOT NULL, config_digest TEXT NOT NULL, estimate_micro INTEGER NOT NULL, actor TEXT NOT NULL, ts INTEGER NOT NULL);
CREATE TABLE approval_pending (run TEXT PRIMARY KEY, estimate_micro INTEGER NOT NULL, ts INTEGER NOT NULL);
CREATE TABLE provider_account (id TEXT PRIMARY KEY, owner_coordinator TEXT NOT NULL);
