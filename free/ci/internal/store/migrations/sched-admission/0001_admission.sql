-- Expand-only. The three indexes at the end are on core tables; core always applies first (registry.go migrationList).
CREATE TABLE sched_admitted (attempt_id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, project TEXT NOT NULL, user_id TEXT NOT NULL, team_id TEXT NOT NULL, provider TEXT NOT NULL, runner_id TEXT NOT NULL, world_digest TEXT NOT NULL, admitted_at INTEGER NOT NULL);
CREATE INDEX sched_admitted_user ON sched_admitted(user_id);
CREATE INDEX sched_admitted_team ON sched_admitted(team_id);
CREATE INDEX sched_admitted_provider ON sched_admitted(provider);
CREATE TABLE sched_refusal (attempt_id TEXT PRIMARY KEY, reason TEXT NOT NULL, detail TEXT NOT NULL, at INTEGER NOT NULL);
CREATE INDEX sched_attempt_active ON attempt(state, job_id);
CREATE INDEX sched_lease_runner ON lease(runner_id);
CREATE INDEX sched_job_pipeline ON job(pipeline_id, name);
