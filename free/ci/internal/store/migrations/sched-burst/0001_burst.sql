-- Expand-only. No foreign key crosses the set: attempt is a core table, read by store code. Times are unix milliseconds.
-- An intent is committed (with its lifetime reservation) before any provider call, so a crash at any later step leaves a row that names the resource by its token.
CREATE TABLE burst_intent (token TEXT PRIMARY KEY, coordinator_id TEXT NOT NULL, provider TEXT NOT NULL, pipeline_id TEXT NOT NULL, job_key TEXT NOT NULL, platform TEXT NOT NULL,
  reserved_micro INTEGER NOT NULL, lifetime_ms INTEGER NOT NULL, state TEXT NOT NULL CHECK (state IN ('live','tearing-down','terminal')),
  resource_id TEXT NOT NULL DEFAULT '', node_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE INDEX burst_intent_state ON burst_intent(coordinator_id, state);
CREATE INDEX burst_intent_node ON burst_intent(node_id);
CREATE INDEX burst_intent_job ON burst_intent(job_key, created_at);
CREATE TABLE burst_provisioning_lease (intent TEXT PRIMARY KEY, idle_until INTEGER NOT NULL, max_until INTEGER NOT NULL);
CREATE TABLE burst_breaker (provider TEXT PRIMARY KEY, failures INTEGER NOT NULL, open_until INTEGER NOT NULL);
CREATE TABLE burst_teardown_failure (intent TEXT PRIMARY KEY, provider TEXT NOT NULL, first_failed_at INTEGER NOT NULL, last_failed_at INTEGER NOT NULL, attempts INTEGER NOT NULL, alerted INTEGER NOT NULL DEFAULT 0);
CREATE INDEX burst_teardown_failure_provider ON burst_teardown_failure(provider);
-- One row per ephemeral node that has held a lease: a second lease request is refused (ephemeral.used), whatever the trust class.
CREATE TABLE burst_node_use (node_id TEXT PRIMARY KEY, intent TEXT NOT NULL, attempt_id TEXT NOT NULL, at INTEGER NOT NULL);
CREATE TABLE burst_decision (id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, provider TEXT NOT NULL, job_key TEXT NOT NULL, allowed INTEGER NOT NULL, reason TEXT NOT NULL, input TEXT NOT NULL);
