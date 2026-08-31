-- H03 #176: Persistent Job Queue schema
-- Creates np_jobs (visibility snapshot) and np_job_dlq (dead-letter queue).
-- Redis is the primary execution store; Postgres provides durability and Admin UI visibility.

CREATE TABLE IF NOT EXISTS np_jobs (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  queue          TEXT NOT NULL,
  job_type       TEXT NOT NULL,
  payload        JSONB NOT NULL DEFAULT '{}',
  status         TEXT NOT NULL DEFAULT 'pending',
  -- status values: pending | active | completed | failed | dead
  attempt_count  SMALLINT NOT NULL DEFAULT 0,
  scheduled_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at     TIMESTAMPTZ,
  completed_at   TIMESTAMPTZ,
  error          TEXT,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_jobs_queue_status
  ON np_jobs (queue, status, scheduled_at);

CREATE INDEX IF NOT EXISTS idx_np_jobs_source_account
  ON np_jobs (source_account_id, status);

COMMENT ON TABLE np_jobs IS 'Visibility snapshot of Redis job queue state. Written by job-queue plugin workers.';
COMMENT ON COLUMN np_jobs.source_account_id IS 'Multi-app isolation key; default primary.';

-- Dead-letter queue: jobs that exhausted all retry attempts
CREATE TABLE IF NOT EXISTS np_job_dlq (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id      UUID NOT NULL REFERENCES np_jobs(id) ON DELETE CASCADE,
  final_error TEXT NOT NULL,
  dlq_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  source_account_id TEXT NOT NULL DEFAULT 'primary'
);

CREATE INDEX IF NOT EXISTS idx_np_job_dlq_dlq_at
  ON np_job_dlq (dlq_at DESC);

COMMENT ON TABLE np_job_dlq IS 'Jobs that exhausted MAX_ATTEMPTS. Visible in Admin UI DLQ panel.';

-- H03 #177: Circuit breaker state persistence
-- Allows circuit breakers to survive service restarts.
CREATE TABLE IF NOT EXISTS np_circuit_breakers (
  name           TEXT PRIMARY KEY,
  state          TEXT NOT NULL DEFAULT 'closed',
  -- state values: closed | open | half-open
  failure_count  INT NOT NULL DEFAULT 0,
  last_failure   TIMESTAMPTZ,
  opened_at      TIMESTAMPTZ,
  next_probe_at  TIMESTAMPTZ,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE np_circuit_breakers IS 'Persistent circuit breaker state, keyed by circuit name. Survives service restarts.';
