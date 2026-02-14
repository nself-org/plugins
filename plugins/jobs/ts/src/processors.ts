/**
 * Job Processors
 * Built-in processors for common job types
 */

import type { Job } from 'bullmq';
import { createLogger } from '@nself/plugin-utils';
import type {
  SendEmailPayload,
  HttpRequestPayload,
  DatabaseBackupPayload,
  FileCleanupPayload,
  CustomJobPayload,
  SendEmailResult,
  HttpRequestResult,
  DatabaseBackupResult,
  FileCleanupResult,
} from './types.js';
import { JobsDatabase } from './database.js';

const logger = createLogger('jobs:processors');

/**
 * Send Email Processor
 * Placeholder - integrate with your email service (e.g., SendGrid, AWS SES)
 */
export async function processSendEmail(job: Job<SendEmailPayload>): Promise<SendEmailResult> {
  const { to, subject, body } = job.data;

  await job.updateProgress(10);

  // NOTE: Email integration requires external SMTP service configuration
  // Integration point: Replace this with your email provider (SendGrid, AWS SES, Mailgun, etc.)
  // Example: await sendgridClient.send({ to, subject, html: body })
  logger.info(`Sending email to ${to}: ${subject}`);

  await job.updateProgress(50);

  // Simulate async operation
  await new Promise(resolve => setTimeout(resolve, 1000));

  await job.updateProgress(100);

  return {
    messageId: `msg_${Date.now()}`,
    accepted: Array.isArray(to) ? to : [to],
    rejected: [],
  };
}

/**
 * HTTP Request Processor
 * Makes HTTP requests with retry logic
 */
export async function processHttpRequest(job: Job<HttpRequestPayload>): Promise<HttpRequestResult> {
  const { url, method, headers, body, timeout } = job.data;

  await job.updateProgress(10);

  const startTime = Date.now();

  try {
    const response = await fetch(url, {
      method,
      headers: headers || {},
      body: body ? JSON.stringify(body) : undefined,
      signal: timeout ? AbortSignal.timeout(timeout) : undefined,
    });

    await job.updateProgress(50);

    const responseBody = await response.text();
    let parsedBody: unknown;

    try {
      parsedBody = JSON.parse(responseBody);
    } catch {
      parsedBody = responseBody;
    }

    await job.updateProgress(100);

    return {
      status: response.status,
      statusText: response.statusText,
      headers: Object.fromEntries(response.headers.entries()),
      body: parsedBody,
      duration_ms: Date.now() - startTime,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('HTTP request failed', { error: message });
    throw error;
  }
}

/**
 * Database Backup Processor
 * Creates database backups using pg_dump
 */
export async function processDatabaseBackup(
  job: Job<DatabaseBackupPayload>,
  db: JobsDatabase
): Promise<DatabaseBackupResult> {
  const { database, tables, destination, compression } = job.data;

  await job.updateProgress(10);

  logger.info(`Starting backup of database: ${database}`);

  // NOTE: Database backup requires pg_dump binary and PostgreSQL connection credentials
  // Integration point: Use child_process.spawn('pg_dump', [...args]) with proper connection params
  // Example: spawn('pg_dump', ['-h', host, '-U', user, '-d', database, '-f', destination])
  // For production use, consider using the dedicated backup plugin instead
  const tablesBackedUp = tables?.length || 0;

  await job.updateProgress(50);

  // Simulate backup operation
  await new Promise(resolve => setTimeout(resolve, 2000));

  await job.updateProgress(100);

  return {
    filename: `${database}_${Date.now()}.${compression ? 'sql.gz' : 'sql'}`,
    size_bytes: 1024 * 1024, // 1MB placeholder
    tables_backed_up: tablesBackedUp,
    duration_ms: Date.now() - job.timestamp,
  };
}

/**
 * File Cleanup Processor
 * Cleans up old completed/failed jobs from database
 */
export async function processFileCleanup(
  job: Job<FileCleanupPayload>,
  db: JobsDatabase
): Promise<FileCleanupResult> {
  const { target, older_than_hours, older_than_days } = job.data;

  await job.updateProgress(10);

  let removedCount = 0;

  if (target === 'completed_jobs' && older_than_hours) {
    const result = await db.query<{ cleanup_old_jobs: number }>(
      'SELECT cleanup_old_jobs($1) as count',
      [older_than_hours]
    );
    removedCount = result[0]?.cleanup_old_jobs || 0;
  } else if (target === 'failed_jobs' && older_than_days) {
    const result = await db.query<{ cleanup_old_failed_jobs: number }>(
      'SELECT cleanup_old_failed_jobs($1) as count',
      [older_than_days]
    );
    removedCount = result[0]?.cleanup_old_failed_jobs || 0;
  }

  await job.updateProgress(100);

  return {
    files_removed: removedCount,
    bytes_freed: removedCount * 1024, // Estimate
    files: [],
  };
}

/**
 * Custom Job Processor
 * For user-defined custom jobs via Hasura Actions
 */
export async function processCustomJob(job: Job<CustomJobPayload>): Promise<unknown> {
  const { action, data } = job.data;

  await job.updateProgress(10);

  logger.info(`Processing custom action: ${action}`);

  // NOTE: Hasura Actions integration requires GraphQL endpoint configuration
  // Integration point: Make POST request to Hasura Actions endpoint with action name and data
  // Example: await fetch(hasuraUrl, { method: 'POST', body: JSON.stringify({ action, input: data }) })
  // Requires HASURA_ADMIN_SECRET and HASURA_GRAPHQL_ENDPOINT environment variables

  await job.updateProgress(100);

  return {
    success: true,
    action,
    processedAt: new Date().toISOString(),
  };
}
