/**
 * Database Module
 * PostgreSQL integration for job persistence and tracking
 */

import pkg from 'pg';
const { Pool } = pkg;
import { createLogger } from '@nself/plugin-utils';
import type { JobsConfig, JobRecord, JobResultRecord, JobFailureRecord, JobScheduleRecord } from './types.js';

const logger = createLogger('jobs:database');

export class JobsDatabase {
  private pool: InstanceType<typeof Pool>;
  private config: JobsConfig;
  private readonly sourceAccountId: string;

  constructor(config: JobsConfig, sourceAccountId = 'primary') {
    this.config = config;
    this.sourceAccountId = sourceAccountId;
    this.pool = new Pool({
      host: config.database.host,
      port: config.database.port,
      database: config.database.database,
      user: config.database.user,
      password: config.database.password,
      ssl: config.database.ssl ? { rejectUnauthorized: false } : false,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });
  }

  /**
   * Create a new JobsDatabase instance scoped to a specific source account.
   * Shares the same underlying pool and config.
   */
  forSourceAccount(accountId: string): JobsDatabase {
    const scoped = new JobsDatabase(this.config, accountId);
    // Share the same pool instance
    scoped.pool = this.pool;
    return scoped;
  }

  /**
   * Get the current source account ID
   */
  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    try {
      await this.pool.query('SELECT 1');
    } catch (error) {
      throw new Error(`Database connection failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  async disconnect(): Promise<void> {
    await this.pool.end();
  }

  async query<T = unknown>(sql: string, params?: unknown[]): Promise<T[]> {
    const result = await this.pool.query(sql, params);
    return result.rows as T[];
  }

  /**
   * Initialize database schema
   */
  async initializeSchema(): Promise<void> {
    logger.info('Initializing jobs schema...');

    // Check if schema exists
    const schemaExists = await this.query<{ exists: boolean }>(`
      SELECT EXISTS (
        SELECT FROM information_schema.tables
        WHERE table_name = 'np_jobs_tasks'
      )
    `);

    if (!schemaExists[0]?.exists) {
      logger.info('Creating initial schema...');
      await this.createInitialSchema();
    } else {
      logger.info('Schema already exists, checking for migrations...');
    }

    // Run migrations for multi-app support
    await this.migrateMultiApp();

    logger.success('Schema initialization complete');
  }

  /**
   * Create initial database schema
   */
  private async createInitialSchema(): Promise<void> {
    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Jobs/Tasks
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_jobs_tasks (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bullmq_id VARCHAR(255) UNIQUE,
        queue_name VARCHAR(128) NOT NULL,
        job_type VARCHAR(128) NOT NULL,
        priority INTEGER DEFAULT 0,
        status VARCHAR(32) NOT NULL DEFAULT 'waiting',
        payload JSONB DEFAULT '{}',
        options JSONB DEFAULT '{}',
        scheduled_for TIMESTAMPTZ,
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        progress INTEGER DEFAULT 0,
        worker_id VARCHAR(255),
        process_id VARCHAR(255),
        retry_count INTEGER DEFAULT 0,
        max_retries INTEGER DEFAULT 3,
        retry_delay INTEGER DEFAULT 5000,
        metadata JSONB DEFAULT '{}',
        tags TEXT[] DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_source_account
        ON np_jobs_tasks(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_queue
        ON np_jobs_tasks(queue_name);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_status
        ON np_jobs_tasks(status);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_type
        ON np_jobs_tasks(job_type);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_scheduled
        ON np_jobs_tasks(scheduled_for) WHERE scheduled_for IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_np_jobs_tasks_bullmq
        ON np_jobs_tasks(bullmq_id);

      -- =====================================================================
      -- Job Results
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_jobs_job_results (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES np_jobs_tasks(id) ON DELETE CASCADE,
        result JSONB,
        duration_ms INTEGER,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_jobs_results_source_account
        ON np_jobs_job_results(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_results_job
        ON np_jobs_job_results(job_id);

      -- =====================================================================
      -- Job Failures
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_jobs_job_failures (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES np_jobs_tasks(id) ON DELETE CASCADE,
        error_message TEXT,
        error_stack TEXT,
        attempt_number INTEGER,
        will_retry BOOLEAN DEFAULT false,
        retry_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_jobs_failures_source_account
        ON np_jobs_job_failures(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_failures_job
        ON np_jobs_job_failures(job_id);

      -- =====================================================================
      -- Job Schedules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_jobs_job_schedules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        job_type VARCHAR(128) NOT NULL,
        queue_name VARCHAR(128) DEFAULT 'default',
        payload JSONB DEFAULT '{}',
        options JSONB DEFAULT '{}',
        cron_expression VARCHAR(255) NOT NULL,
        timezone VARCHAR(64) DEFAULT 'UTC',
        enabled BOOLEAN DEFAULT true,
        last_run_at TIMESTAMPTZ,
        next_run_at TIMESTAMPTZ,
        last_job_id UUID REFERENCES np_jobs_tasks(id),
        total_runs INTEGER DEFAULT 0,
        successful_runs INTEGER DEFAULT 0,
        failed_runs INTEGER DEFAULT 0,
        max_runs INTEGER,
        end_date TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}',
        tags TEXT[] DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_np_jobs_schedules_source_account
        ON np_jobs_job_schedules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_jobs_schedules_enabled
        ON np_jobs_job_schedules(enabled) WHERE enabled = true;
      CREATE INDEX IF NOT EXISTS idx_np_jobs_schedules_next_run
        ON np_jobs_job_schedules(next_run_at) WHERE enabled = true;
    `;

    await this.pool.query(schema);
    logger.success('Initial schema created');
  }

  /**
   * Run schema migrations to add source_account_id column if missing
   */
  private async migrateMultiApp(): Promise<void> {
    const migResult = await this.query<{ exists: boolean }>(`SELECT EXISTS (
      SELECT 1 FROM information_schema.columns WHERE table_name = 'np_jobs_tasks' AND column_name = 'source_account_id'
    )`);
    if (!migResult[0]?.exists) {
      logger.info('Running multi-app migration: adding source_account_id columns...');
      for (const table of ['np_jobs_tasks', 'np_jobs_job_results', 'np_jobs_job_failures', 'np_jobs_job_schedules']) {
        await this.query(`ALTER TABLE ${table} ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`);
      }
      logger.info('Multi-app migration complete');
    }
  }

  // Job operations
  async createJob(job: Partial<JobRecord>): Promise<JobRecord> {
    const result = await this.query<JobRecord>(
      `INSERT INTO np_jobs_tasks (
        bullmq_id, queue_name, job_type, priority, status, payload, options,
        scheduled_for, retry_count, max_retries, retry_delay, metadata, tags, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        job.bullmq_id,
        job.queue_name,
        job.job_type,
        job.priority,
        job.status || 'waiting',
        JSON.stringify(job.payload || {}),
        JSON.stringify(job.options || {}),
        job.scheduled_for,
        job.retry_count || 0,
        job.max_retries || this.config.retryAttempts,
        job.retry_delay || this.config.retryDelay,
        JSON.stringify(job.metadata || {}),
        job.tags || [],
        this.sourceAccountId,
      ]
    );
    return result[0];
  }

  async updateJobStatus(jobId: string, status: string, extra?: Partial<JobRecord>): Promise<void> {
    const updates: string[] = ['status = $2', 'updated_at = NOW()'];
    const params: unknown[] = [jobId, status];
    let paramIndex = 3;

    if (extra?.progress !== undefined) {
      updates.push(`progress = $${paramIndex++}`);
      params.push(extra.progress);
    }

    if (extra?.worker_id !== undefined) {
      updates.push(`worker_id = $${paramIndex++}`);
      params.push(extra.worker_id);
    }

    if (extra?.process_id !== undefined) {
      updates.push(`process_id = $${paramIndex++}`);
      params.push(extra.process_id);
    }

    params.push(this.sourceAccountId);
    await this.query(
      `UPDATE np_jobs_tasks SET ${updates.join(', ')} WHERE id = $1 AND source_account_id = $${paramIndex}`,
      params
    );
  }

  async getJobByBullMQId(bullmqId: string): Promise<JobRecord | null> {
    const result = await this.query<JobRecord>(
      'SELECT * FROM np_jobs_tasks WHERE bullmq_id = $1 AND source_account_id = $2',
      [bullmqId, this.sourceAccountId]
    );
    return result[0] || null;
  }

  async saveJobResult(jobId: string, result: unknown, durationMs: number): Promise<void> {
    await this.query(
      `INSERT INTO np_jobs_job_results (job_id, result, duration_ms, source_account_id)
       VALUES ($1, $2, $3, $4)`,
      [jobId, JSON.stringify(result), durationMs, this.sourceAccountId]
    );
  }

  async saveJobFailure(
    jobId: string,
    error: Error,
    attemptNumber: number,
    willRetry: boolean,
    retryAt?: Date
  ): Promise<void> {
    await this.query(
      `INSERT INTO np_jobs_job_failures (
        job_id, error_message, error_stack, attempt_number, will_retry, retry_at, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
      [jobId, error.message, error.stack || null, attemptNumber, willRetry, retryAt || null, this.sourceAccountId]
    );
  }

  async incrementRetryCount(jobId: string): Promise<void> {
    await this.query(
      'UPDATE np_jobs_tasks SET retry_count = retry_count + 1, updated_at = NOW() WHERE id = $1 AND source_account_id = $2',
      [jobId, this.sourceAccountId]
    );
  }

  // Schedule operations
  async createSchedule(schedule: Partial<JobScheduleRecord>): Promise<JobScheduleRecord> {
    const result = await this.query<JobScheduleRecord>(
      `INSERT INTO np_jobs_job_schedules (
        name, description, job_type, queue_name, payload, options,
        cron_expression, timezone, enabled, max_runs, end_date, metadata, tags, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        schedule.name,
        schedule.description || null,
        schedule.job_type,
        schedule.queue_name || 'default',
        JSON.stringify(schedule.payload || {}),
        JSON.stringify(schedule.options || {}),
        schedule.cron_expression,
        schedule.timezone || 'UTC',
        schedule.enabled !== false,
        schedule.max_runs || null,
        schedule.end_date || null,
        JSON.stringify(schedule.metadata || {}),
        schedule.tags || [],
        this.sourceAccountId,
      ]
    );
    return result[0];
  }

  async getSchedules(enabled?: boolean): Promise<JobScheduleRecord[]> {
    if (enabled !== undefined) {
      return this.query<JobScheduleRecord>(
        'SELECT * FROM np_jobs_job_schedules WHERE enabled = $1 AND source_account_id = $2 ORDER BY next_run_at',
        [enabled, this.sourceAccountId]
      );
    }
    return this.query<JobScheduleRecord>(
      'SELECT * FROM np_jobs_job_schedules WHERE source_account_id = $1 ORDER BY next_run_at',
      [this.sourceAccountId]
    );
  }

  async updateScheduleRun(scheduleId: string, jobId: string): Promise<void> {
    await this.query(
      `UPDATE np_jobs_job_schedules SET
        last_run_at = NOW(),
        last_job_id = $2,
        total_runs = total_runs + 1,
        updated_at = NOW()
       WHERE id = $1 AND source_account_id = $3`,
      [scheduleId, jobId, this.sourceAccountId]
    );
  }

  async updateScheduleSuccess(scheduleId: string): Promise<void> {
    await this.query(
      'UPDATE np_jobs_job_schedules SET successful_runs = successful_runs + 1 WHERE id = $1 AND source_account_id = $2',
      [scheduleId, this.sourceAccountId]
    );
  }

  async updateScheduleFailure(scheduleId: string): Promise<void> {
    await this.query(
      'UPDATE np_jobs_job_schedules SET failed_runs = failed_runs + 1 WHERE id = $1 AND source_account_id = $2',
      [scheduleId, this.sourceAccountId]
    );
  }

  async updateNextRun(scheduleId: string, nextRun: Date): Promise<void> {
    await this.query(
      'UPDATE np_jobs_job_schedules SET next_run_at = $2 WHERE id = $1 AND source_account_id = $3',
      [scheduleId, nextRun, this.sourceAccountId]
    );
  }

  // Stats
  async getStats() {
    const [queueStats, typeStats, counts] = await Promise.all([
      this.query(
        `SELECT queue_name,
          COUNT(*) FILTER (WHERE status = 'waiting') AS waiting,
          COUNT(*) FILTER (WHERE status = 'active') AS active,
          COUNT(*) FILTER (WHERE status = 'completed') AS completed,
          COUNT(*) FILTER (WHERE status = 'failed') AS failed,
          COUNT(*) FILTER (WHERE status = 'delayed') AS delayed,
          COUNT(*) FILTER (WHERE status = 'stuck') AS stuck,
          COUNT(*) AS total,
          AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) FILTER (WHERE status = 'completed' AND completed_at IS NOT NULL) AS avg_duration_seconds,
          MAX(created_at) AS last_job_at
        FROM np_jobs_tasks
        WHERE source_account_id = $1
        GROUP BY queue_name
        ORDER BY queue_name`,
        [this.sourceAccountId]
      ),
      this.query(
        `SELECT job_type,
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
        FROM np_jobs_tasks
        WHERE source_account_id = $1
        GROUP BY job_type
        ORDER BY total_jobs DESC
        LIMIT 20`,
        [this.sourceAccountId]
      ),
      this.query<{
        waiting: number;
        active: number;
        completed: number;
        failed: number;
        total: number;
      }>(`SELECT
        COUNT(*) FILTER (WHERE status = 'waiting') as waiting,
        COUNT(*) FILTER (WHERE status = 'active') as active,
        COUNT(*) FILTER (WHERE status = 'completed') as completed,
        COUNT(*) FILTER (WHERE status = 'failed') as failed,
        COUNT(*) as total
      FROM np_jobs_tasks
      WHERE source_account_id = $1`,
        [this.sourceAccountId]
      ),
    ]);

    return {
      ...counts[0],
      queues: queueStats,
      job_types: typeStats,
    };
  }

  /**
   * Delete all data for a specific source account across all tables.
   */
  async cleanupForAccount(sourceAccountId: string): Promise<{ tables: Record<string, number> }> {
    const tables = ['np_jobs_job_results', 'np_jobs_job_failures', 'np_jobs_tasks', 'np_jobs_job_schedules'];
    const result: Record<string, number> = {};

    for (const table of tables) {
      const rows = await this.query<{ count: string }>(
        `WITH deleted AS (DELETE FROM ${table} WHERE source_account_id = $1 RETURNING 1)
         SELECT COUNT(*)::text AS count FROM deleted`,
        [sourceAccountId]
      );
      result[table] = parseInt(rows[0]?.count ?? '0', 10);
    }

    logger.info(`Cleaned up data for account "${sourceAccountId}"`, result);
    return { tables: result };
  }
}
