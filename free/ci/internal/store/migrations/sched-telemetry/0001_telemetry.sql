-- Expand-only. No foreign key crosses the set: attempt, job and pipeline are core tables, read by store code.
CREATE TABLE schedtel_decision (id TEXT PRIMARY KEY, run TEXT NOT NULL, job TEXT NOT NULL, job_name TEXT NOT NULL, project TEXT NOT NULL, attempt INTEGER NOT NULL, world_digest TEXT NOT NULL, chosen TEXT NOT NULL, runner_class TEXT NOT NULL, outcome TEXT NOT NULL, cost_micro INTEGER NOT NULL, wait_ms INTEGER NOT NULL, doc_json TEXT NOT NULL, ts INTEGER NOT NULL);
CREATE INDEX schedtel_decision_run ON schedtel_decision(run, ts);
CREATE INDEX schedtel_decision_project ON schedtel_decision(project, ts);
CREATE TABLE schedtel_outcome (attempt_id TEXT PRIMARY KEY, project TEXT NOT NULL, job_key TEXT NOT NULL, runner TEXT NOT NULL, runner_class TEXT NOT NULL, wait_ms INTEGER NOT NULL, runtime_ms INTEGER NOT NULL, cold_start_ms INTEGER NOT NULL, cost_micro INTEGER NOT NULL, result TEXT NOT NULL, failure_class TEXT NOT NULL, ts INTEGER NOT NULL);
CREATE INDEX schedtel_outcome_runner ON schedtel_outcome(runner, ts);
CREATE INDEX schedtel_outcome_project ON schedtel_outcome(project, ts);
CREATE TABLE schedtel_estimator (project TEXT NOT NULL, job_key TEXT NOT NULL, runner_class TEXT NOT NULL, runtime_ms INTEGER NOT NULL, n INTEGER NOT NULL, cold_start_ms INTEGER NOT NULL, cold_n INTEGER NOT NULL, updated INTEGER NOT NULL, PRIMARY KEY (project, job_key, runner_class));
