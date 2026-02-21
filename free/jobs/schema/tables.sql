-- =============================================================================
-- Jobs Plugin Schema
-- BullMQ-based background job queue with scheduling and monitoring
-- =============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Jobs Tasks Table
-- Core job metadata and status tracking
-- =============================================================================

CREATE TYPE job_status AS ENUM (
    'waiting',      -- Job is queued and waiting to be processed
    'active',       -- Job is currently being processed
    'completed',    -- Job completed successfully
    'failed',       -- Job failed (after all retries)
    'delayed',      -- Job is scheduled for future execution
    'stuck',        -- Job appears to be stuck
    'paused'        -- Job is paused
);

CREATE TYPE job_priority AS ENUM (
    'critical',     -- Highest priority (10)
    'high',         -- High priority (5)
    'normal',       -- Normal priority (0)
    'low'           -- Low priority (-5)
);

CREATE TABLE IF NOT EXISTS jobs_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bullmq_id VARCHAR(255) UNIQUE,              -- BullMQ job ID
    queue_name VARCHAR(100) NOT NULL DEFAULT 'default',
    job_type VARCHAR(100) NOT NULL,             -- send-email, http-request, etc.
    priority job_priority NOT NULL DEFAULT 'normal',
    status job_status NOT NULL DEFAULT 'waiting',

    -- Job data
    payload JSONB NOT NULL DEFAULT '{}',        -- Job input data
    options JSONB DEFAULT '{}',                 -- BullMQ options

    -- Timing
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    scheduled_for TIMESTAMP WITH TIME ZONE,     -- For delayed jobs

    -- Progress tracking
    progress INTEGER DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),

    -- Retry logic
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    retry_delay INTEGER DEFAULT 5000,           -- Milliseconds

    -- Metadata
    metadata JSONB DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',

    -- Execution context
    worker_id VARCHAR(255),                     -- ID of worker processing the job
    process_id INTEGER,                         -- Process ID of worker

    -- Timestamps
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_queue_name ON jobs_tasks(queue_name);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_job_type ON jobs_tasks(job_type);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_status ON jobs_tasks(status);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_priority ON jobs_tasks(priority);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_created_at ON jobs_tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_scheduled_for ON jobs_tasks(scheduled_for) WHERE scheduled_for IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_bullmq_id ON jobs_tasks(bullmq_id);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_tags ON jobs_tasks USING gin(tags);
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_metadata ON jobs_tasks USING gin(metadata);

-- Composite index for queue processing
CREATE INDEX IF NOT EXISTS idx_jobs_tasks_queue_status ON jobs_tasks(queue_name, status);

-- =============================================================================
-- Job Results Table
-- Stores successful job outputs
-- =============================================================================

CREATE TABLE IF NOT EXISTS job_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES jobs_tasks(id) ON DELETE CASCADE,

    -- Result data
    result JSONB NOT NULL,                      -- Job output/result

    -- Execution metrics
    duration_ms INTEGER NOT NULL,               -- Execution time in milliseconds
    memory_mb NUMERIC(10,2),                    -- Memory used (MB)
    cpu_percent NUMERIC(5,2),                   -- CPU utilization

    -- Metadata
    metadata JSONB DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_results_job_id ON job_results(job_id);
CREATE INDEX IF NOT EXISTS idx_job_results_created_at ON job_results(created_at);

-- =============================================================================
-- Job Failures Table
-- Tracks all failure attempts with stack traces
-- =============================================================================

CREATE TABLE IF NOT EXISTS job_failures (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES jobs_tasks(id) ON DELETE CASCADE,

    -- Error details
    error_message TEXT NOT NULL,
    error_stack TEXT,
    error_code VARCHAR(50),

    -- Failure context
    attempt_number INTEGER NOT NULL,            -- Which retry attempt failed
    failed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Worker context
    worker_id VARCHAR(255),
    process_id INTEGER,

    -- Error metadata
    metadata JSONB DEFAULT '{}',

    -- Recovery
    will_retry BOOLEAN DEFAULT FALSE,
    retry_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_job_failures_job_id ON job_failures(job_id);
CREATE INDEX IF NOT EXISTS idx_job_failures_failed_at ON job_failures(failed_at);
CREATE INDEX IF NOT EXISTS idx_job_failures_will_retry ON job_failures(will_retry);

-- =============================================================================
-- Job Schedules Table
-- Manages cron-based recurring jobs
-- =============================================================================

CREATE TABLE IF NOT EXISTS job_schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Schedule identification
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,

    -- Job definition
    job_type VARCHAR(100) NOT NULL,
    queue_name VARCHAR(100) NOT NULL DEFAULT 'default',
    payload JSONB NOT NULL DEFAULT '{}',
    options JSONB DEFAULT '{}',

    -- Cron schedule
    cron_expression VARCHAR(100) NOT NULL,      -- Standard cron format
    timezone VARCHAR(100) DEFAULT 'UTC',

    -- Status
    enabled BOOLEAN DEFAULT TRUE,

    -- Execution tracking
    last_run_at TIMESTAMP WITH TIME ZONE,
    last_job_id UUID REFERENCES jobs_tasks(id),
    next_run_at TIMESTAMP WITH TIME ZONE,

    -- Stats
    total_runs INTEGER DEFAULT 0,
    successful_runs INTEGER DEFAULT 0,
    failed_runs INTEGER DEFAULT 0,

    -- Limits
    max_runs INTEGER,                           -- NULL = unlimited
    end_date TIMESTAMP WITH TIME ZONE,          -- NULL = no end

    -- Metadata
    metadata JSONB DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_job_schedules_name ON job_schedules(name);
CREATE INDEX IF NOT EXISTS idx_job_schedules_enabled ON job_schedules(enabled);
CREATE INDEX IF NOT EXISTS idx_job_schedules_next_run ON job_schedules(next_run_at) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_job_schedules_job_type ON job_schedules(job_type);

-- =============================================================================
-- Views for Common Queries
-- =============================================================================

-- Active jobs view
CREATE OR REPLACE VIEW jobs_active AS
SELECT
    j.id,
    j.queue_name,
    j.job_type,
    j.priority,
    j.status,
    j.payload,
    j.created_at,
    j.started_at,
    j.progress,
    j.worker_id,
    EXTRACT(EPOCH FROM (NOW() - j.started_at))::INTEGER AS running_seconds
FROM jobs_tasks j
WHERE j.status = 'active';

-- Failed jobs view with last error
CREATE OR REPLACE VIEW jobs_failed_details AS
SELECT
    j.id,
    j.queue_name,
    j.job_type,
    j.priority,
    j.payload,
    j.retry_count,
    j.max_retries,
    j.failed_at,
    f.error_message,
    f.error_stack,
    f.attempt_number
FROM jobs_tasks j
LEFT JOIN LATERAL (
    SELECT error_message, error_stack, attempt_number
    FROM job_failures
    WHERE job_id = j.id
    ORDER BY failed_at DESC
    LIMIT 1
) f ON TRUE
WHERE j.status = 'failed';

-- Queue statistics view
CREATE OR REPLACE VIEW queue_stats AS
SELECT
    queue_name,
    COUNT(*) FILTER (WHERE status = 'waiting') AS waiting,
    COUNT(*) FILTER (WHERE status = 'active') AS active,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    COUNT(*) FILTER (WHERE status = 'delayed') AS delayed,
    COUNT(*) FILTER (WHERE status = 'stuck') AS stuck,
    COUNT(*) AS total,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) FILTER (WHERE status = 'completed' AND completed_at IS NOT NULL) AS avg_duration_seconds,
    MAX(created_at) AS last_job_at
FROM jobs_tasks
GROUP BY queue_name;

-- Job type statistics
CREATE OR REPLACE VIEW job_type_stats AS
SELECT
    job_type,
    COUNT(*) AS total_jobs,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    ROUND(
        COUNT(*) FILTER (WHERE status = 'completed')::NUMERIC /
        NULLIF(COUNT(*) FILTER (WHERE status IN ('completed', 'failed')), 0) * 100,
        2
    ) AS success_rate,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) FILTER (WHERE status = 'completed') AS avg_duration_seconds,
    MIN(created_at) AS first_job_at,
    MAX(created_at) AS last_job_at
FROM jobs_tasks
GROUP BY job_type;

-- Recent failures view
CREATE OR REPLACE VIEW recent_failures AS
SELECT
    j.id AS job_id,
    j.job_type,
    j.queue_name,
    f.error_message,
    f.error_code,
    f.attempt_number,
    f.failed_at,
    f.will_retry,
    f.retry_at,
    j.payload
FROM job_failures f
JOIN jobs_tasks j ON f.job_id = j.id
WHERE f.failed_at > NOW() - INTERVAL '24 hours'
ORDER BY f.failed_at DESC;

-- Scheduled jobs overview
CREATE OR REPLACE VIEW scheduled_jobs_overview AS
SELECT
    s.id,
    s.name,
    s.job_type,
    s.cron_expression,
    s.enabled,
    s.next_run_at,
    s.last_run_at,
    s.total_runs,
    s.successful_runs,
    s.failed_runs,
    CASE
        WHEN s.total_runs > 0 THEN
            ROUND(s.successful_runs::NUMERIC / s.total_runs * 100, 2)
        ELSE 0
    END AS success_rate,
    EXTRACT(EPOCH FROM (s.next_run_at - NOW()))::INTEGER AS seconds_until_next_run
FROM job_schedules s
WHERE s.enabled = TRUE
ORDER BY s.next_run_at ASC;

-- =============================================================================
-- Functions
-- =============================================================================

-- Function to update job status
CREATE OR REPLACE FUNCTION update_job_status()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();

    -- Set timestamps based on status changes
    IF NEW.status = 'active' AND OLD.status != 'active' THEN
        NEW.started_at = NOW();
    END IF;

    IF NEW.status = 'completed' AND OLD.status != 'completed' THEN
        NEW.completed_at = NOW();
    END IF;

    IF NEW.status = 'failed' AND OLD.status != 'failed' THEN
        NEW.failed_at = NOW();
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_tasks_status_update
    BEFORE UPDATE ON jobs_tasks
    FOR EACH ROW
    EXECUTE FUNCTION update_job_status();

-- Function to clean up old completed jobs
CREATE OR REPLACE FUNCTION cleanup_old_jobs(
    p_older_than_hours INTEGER DEFAULT 24
) RETURNS INTEGER AS $$
DECLARE
    v_deleted_count INTEGER;
BEGIN
    DELETE FROM jobs_tasks
    WHERE status = 'completed'
      AND completed_at < NOW() - (p_older_than_hours || ' hours')::INTERVAL;

    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;
    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to clean up old failed jobs
CREATE OR REPLACE FUNCTION cleanup_old_failed_jobs(
    p_older_than_days INTEGER DEFAULT 7
) RETURNS INTEGER AS $$
DECLARE
    v_deleted_count INTEGER;
BEGIN
    DELETE FROM jobs_tasks
    WHERE status = 'failed'
      AND failed_at < NOW() - (p_older_than_days || ' days')::INTERVAL;

    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;
    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to get job statistics
CREATE OR REPLACE FUNCTION get_job_stats(
    p_queue_name VARCHAR DEFAULT NULL,
    p_hours INTEGER DEFAULT 24
) RETURNS TABLE (
    metric VARCHAR,
    value BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 'total_jobs'::VARCHAR, COUNT(*)
    FROM jobs_tasks
    WHERE (p_queue_name IS NULL OR queue_name = p_queue_name)
      AND created_at > NOW() - (p_hours || ' hours')::INTERVAL

    UNION ALL

    SELECT 'waiting'::VARCHAR, COUNT(*)
    FROM jobs_tasks
    WHERE status = 'waiting'
      AND (p_queue_name IS NULL OR queue_name = p_queue_name)

    UNION ALL

    SELECT 'active'::VARCHAR, COUNT(*)
    FROM jobs_tasks
    WHERE status = 'active'
      AND (p_queue_name IS NULL OR queue_name = p_queue_name)

    UNION ALL

    SELECT 'completed'::VARCHAR, COUNT(*)
    FROM jobs_tasks
    WHERE status = 'completed'
      AND (p_queue_name IS NULL OR queue_name = p_queue_name)
      AND created_at > NOW() - (p_hours || ' hours')::INTERVAL

    UNION ALL

    SELECT 'failed'::VARCHAR, COUNT(*)
    FROM jobs_tasks
    WHERE status = 'failed'
      AND (p_queue_name IS NULL OR queue_name = p_queue_name)
      AND created_at > NOW() - (p_hours || ' hours')::INTERVAL;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- Initial Data
-- =============================================================================

-- Insert default scheduled jobs (examples)
INSERT INTO job_schedules (name, description, job_type, cron_expression, payload, metadata)
VALUES
    (
        'cleanup-completed-jobs',
        'Clean up completed jobs older than 24 hours',
        'file-cleanup',
        '0 2 * * *',  -- Daily at 2 AM
        '{"target": "completed_jobs", "older_than_hours": 24}'::JSONB,
        '{"system": true, "automated": true}'::JSONB
    ),
    (
        'cleanup-failed-jobs',
        'Clean up failed jobs older than 7 days',
        'file-cleanup',
        '0 3 * * 0',  -- Weekly on Sunday at 3 AM
        '{"target": "failed_jobs", "older_than_days": 7}'::JSONB,
        '{"system": true, "automated": true}'::JSONB
    )
ON CONFLICT (name) DO NOTHING;
