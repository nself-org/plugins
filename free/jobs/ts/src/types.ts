/**
 * Jobs Plugin Types
 * Type definitions for BullMQ job queue system
 */

import type { Job as BullJob, JobsOptions, Queue as BullQueue, Worker as BullWorker } from 'bullmq';

// =============================================================================
// Configuration
// =============================================================================

export interface JobsConfig {
  redisUrl: string;
  dashboardEnabled: boolean;
  dashboardPort: number;
  dashboardPath: string;
  defaultConcurrency: number;
  retryAttempts: number;
  retryDelay: number;
  jobTimeout: number;
  enableTelemetry: boolean;
  cleanCompletedAfter: number;
  cleanFailedAfter: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Job Types and Statuses
// =============================================================================

export type JobType = 'send-email' | 'http-request' | 'database-backup' | 'file-cleanup' | 'custom';

export type JobStatus = 'waiting' | 'active' | 'completed' | 'failed' | 'delayed' | 'stuck' | 'paused';

export type JobPriority = 'critical' | 'high' | 'normal' | 'low';

export const JobPriorityValue: Record<JobPriority, number> = {
  critical: 10,
  high: 5,
  normal: 0,
  low: -5,
};

// =============================================================================
// Job Records (Database)
// =============================================================================

export interface JobRecord {
  id: string;
  source_account_id: string;
  bullmq_id: string | null;
  queue_name: string;
  job_type: string;
  priority: JobPriority;
  status: JobStatus;
  payload: Record<string, unknown>;
  options: JobsOptions;
  created_at: Date;
  started_at: Date | null;
  completed_at: Date | null;
  failed_at: Date | null;
  scheduled_for: Date | null;
  progress: number;
  retry_count: number;
  max_retries: number;
  retry_delay: number;
  metadata: Record<string, unknown>;
  tags: string[];
  worker_id: string | null;
  process_id: number | null;
  updated_at: Date;
}

export interface JobResultRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  result: Record<string, unknown>;
  duration_ms: number;
  memory_mb: number | null;
  cpu_percent: number | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface JobFailureRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  error_message: string;
  error_stack: string | null;
  error_code: string | null;
  attempt_number: number;
  failed_at: Date;
  worker_id: string | null;
  process_id: number | null;
  metadata: Record<string, unknown>;
  will_retry: boolean;
  retry_at: Date | null;
}

export interface JobScheduleRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  job_type: string;
  queue_name: string;
  payload: Record<string, unknown>;
  options: JobsOptions;
  cron_expression: string;
  timezone: string;
  enabled: boolean;
  last_run_at: Date | null;
  last_job_id: string | null;
  next_run_at: Date | null;
  total_runs: number;
  successful_runs: number;
  failed_runs: number;
  max_runs: number | null;
  end_date: Date | null;
  metadata: Record<string, unknown>;
  tags: string[];
  created_at: Date;
  updated_at: Date;
  created_by: string | null;
  updated_by: string | null;
}

// =============================================================================
// Job Payloads (Type-specific)
// =============================================================================

export interface SendEmailPayload {
  to: string | string[];
  from?: string;
  subject: string;
  body: string;
  html?: string;
  attachments?: Array<{
    filename: string;
    content: string | Buffer;
    contentType?: string;
  }>;
  cc?: string[];
  bcc?: string[];
}

export interface HttpRequestPayload {
  url: string;
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  headers?: Record<string, string>;
  body?: unknown;
  timeout?: number;
  retryOn?: number[];
}

export interface DatabaseBackupPayload {
  database: string;
  tables?: string[];
  destination: string;
  compression?: boolean;
  encryption?: boolean;
}

export interface FileCleanupPayload {
  target: 'completed_jobs' | 'failed_jobs' | 'old_files';
  older_than_hours?: number;
  older_than_days?: number;
  path?: string;
  pattern?: string;
}

export interface CustomJobPayload {
  action: string;
  data: Record<string, unknown>;
}

export type JobPayload =
  | SendEmailPayload
  | HttpRequestPayload
  | DatabaseBackupPayload
  | FileCleanupPayload
  | CustomJobPayload
  | Record<string, unknown>;

// =============================================================================
// Job Results
// =============================================================================

export interface JobResult<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
  metadata?: Record<string, unknown>;
}

export interface SendEmailResult {
  messageId: string;
  accepted: string[];
  rejected: string[];
}

export interface HttpRequestResult {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: unknown;
  duration_ms: number;
}

export interface DatabaseBackupResult {
  filename: string;
  size_bytes: number;
  tables_backed_up: number;
  duration_ms: number;
}

export interface FileCleanupResult {
  files_removed: number;
  bytes_freed: number;
  files: string[];
}

// =============================================================================
// Job Options
// =============================================================================

export interface CreateJobOptions extends Omit<JobsOptions, 'priority'> {
  queue?: string;
  priority?: JobPriority;
  delay?: number;
  maxRetries?: number;
  retryDelay?: number;
  timeout?: number;
  metadata?: Record<string, unknown>;
  tags?: string[];
}

// =============================================================================
// Statistics
// =============================================================================

export interface QueueStats {
  queue_name: string;
  waiting: number;
  active: number;
  completed: number;
  failed: number;
  delayed: number;
  stuck: number;
  total: number;
  avg_duration_seconds: number | null;
  last_job_at: Date | null;
}

export interface JobTypeStats {
  job_type: string;
  total_jobs: number;
  completed: number;
  failed: number;
  success_rate: number | null;
  avg_duration_seconds: number | null;
  first_job_at: Date | null;
  last_job_at: Date | null;
}

export interface GlobalStats {
  total_jobs: number;
  waiting: number;
  active: number;
  completed: number;
  failed: number;
  queues: QueueStats[];
  job_types: JobTypeStats[];
}

// =============================================================================
// Job Processor
// =============================================================================

export type JobProcessor<T = unknown, R = unknown> = (
  job: BullJob<T>,
  token?: string
) => Promise<R>;

export interface RegisteredProcessor {
  type: JobType | string;
  processor: JobProcessor;
  concurrency?: number;
}

// =============================================================================
// Events
// =============================================================================

export interface JobEvent {
  type: 'created' | 'started' | 'progress' | 'completed' | 'failed' | 'retry';
  jobId: string;
  data?: unknown;
  timestamp: Date;
}

// =============================================================================
// Scheduler
// =============================================================================

export interface ScheduleOptions {
  name: string;
  description?: string;
  jobType: string;
  queueName?: string;
  payload: JobPayload;
  cronExpression: string;
  timezone?: string;
  enabled?: boolean;
  maxRuns?: number;
  endDate?: Date;
  metadata?: Record<string, unknown>;
  tags?: string[];
}

// =============================================================================
// Exports
// =============================================================================

export type {
  BullJob,
  JobsOptions,
  BullQueue,
  BullWorker,
};
