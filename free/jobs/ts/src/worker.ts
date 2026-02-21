/**
 * Job Worker
 * BullMQ worker that processes jobs from queues
 */

import { Worker, Job } from 'bullmq';
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const IORedis = require('ioredis');
import { createLogger } from '@nself/plugin-utils';
import { getConfig } from './config.js';
import { JobsDatabase } from './database.js';
import {
  processSendEmail,
  processHttpRequest,
  processDatabaseBackup,
  processFileCleanup,
  processCustomJob,
} from './processors.js';
import type {
  JobPayload,
  JobPriority,
  SendEmailPayload,
  HttpRequestPayload,
  DatabaseBackupPayload,
  FileCleanupPayload,
  CustomJobPayload,
} from './types.js';

const logger = createLogger('jobs:worker');

const config = getConfig();
const db = new JobsDatabase(config);

// Redis connection
const connection = new IORedis(config.redisUrl, {
  maxRetriesPerRequest: null,
});

// Worker ID
const WORKER_ID = `worker_${process.pid}_${Date.now()}`;

// Get queue name from env or default
const QUEUE_NAME = process.env.WORKER_QUEUE || 'default';
const CONCURRENCY = parseInt(process.env.WORKER_CONCURRENCY || String(config.defaultConcurrency), 10);

logger.info(`Starting worker for queue: ${QUEUE_NAME}`);
logger.info(`Concurrency: ${CONCURRENCY}`);
logger.info(`Worker ID: ${WORKER_ID}`);

/**
 * Main job processor function
 */
async function processJob(job: Job<JobPayload>): Promise<unknown> {
  const startTime = Date.now();
  const jobType = job.name;

  logger.info(`Processing job ${job.id} (${jobType})`);

  try {
    // Find or create job record in database
    let jobRecord = await db.getJobByBullMQId(job.id!);

    if (!jobRecord) {
      jobRecord = await db.createJob({
        bullmq_id: job.id!,
        queue_name: job.queueName,
        job_type: jobType,
        priority: (job.opts.priority || 0) >= 5 ? 'high' : 'normal' as JobPriority,
        status: 'active',
        payload: job.data as Record<string, unknown>,
        options: job.opts,
        worker_id: WORKER_ID,
        process_id: process.pid,
      });
    } else {
      await db.updateJobStatus(jobRecord.id, 'active', {
        worker_id: WORKER_ID,
        process_id: process.pid,
      });
    }

    // Process based on job type
    let result: unknown;

    switch (jobType) {
      case 'send-email':
        result = await processSendEmail(job as Job<SendEmailPayload>);
        break;

      case 'http-request':
        result = await processHttpRequest(job as Job<HttpRequestPayload>);
        break;

      case 'database-backup':
        result = await processDatabaseBackup(job as Job<DatabaseBackupPayload>, db);
        break;

      case 'file-cleanup':
        result = await processFileCleanup(job as Job<FileCleanupPayload>, db);
        break;

      case 'custom':
        result = await processCustomJob(job as Job<CustomJobPayload>);
        break;

      default:
        throw new Error(`Unknown job type: ${jobType}`);
    }

    // Save result
    const duration = Date.now() - startTime;
    await db.saveJobResult(jobRecord.id, result, duration);
    await db.updateJobStatus(jobRecord.id, 'completed');

    logger.info(`Job ${job.id} completed in ${duration}ms`);

    return result;
  } catch (error) {
    const duration = Date.now() - startTime;
    const err = error as Error;

    logger.error(`Job ${job.id} failed after ${duration}ms: ${err.message}`);

    // Get job record
    const jobRecord = await db.getJobByBullMQId(job.id!);

    if (jobRecord) {
      const attemptNumber = (job.attemptsMade || 0) + 1;
      const willRetry = attemptNumber < (job.opts.attempts || config.retryAttempts);

      await db.saveJobFailure(jobRecord.id, err, attemptNumber, willRetry);

      if (willRetry) {
        await db.incrementRetryCount(jobRecord.id);
      } else {
        await db.updateJobStatus(jobRecord.id, 'failed');
      }
    }

    throw error;
  }
}

/**
 * Create and start worker
 */
async function startWorker() {
  // Connect to database
  await db.connect();
  logger.info('Database connected');

  const worker = new Worker(QUEUE_NAME, processJob, {
    connection,
    concurrency: CONCURRENCY,
    autorun: true,
    removeOnComplete: { count: 1000 },
    removeOnFail: { count: 5000 },
  });

  // Event handlers
  worker.on('ready', () => {
    logger.info(`Worker ready and listening for jobs on queue: ${QUEUE_NAME}`);
  });

  worker.on('active', (job: Job) => {
    logger.info(`Job ${job.id} is now active`);
  });

  worker.on('completed', (job: Job, result: unknown) => {
    logger.info(`Job ${job.id} completed successfully`);
  });

  worker.on('failed', (job: Job | undefined, error: Error) => {
    if (job) {
      logger.error(`Job ${job.id} failed: ${error.message}`);
    } else {
      logger.error(`Job failed: ${error.message}`);
    }
  });

  worker.on('error', (error: Error) => {
    logger.error(`Worker error: ${error.message}`);
  });

  worker.on('stalled', (jobId: string) => {
    logger.warn(`Job ${jobId} stalled`);
  });

  // Graceful shutdown
  async function shutdown() {
    logger.info('Shutting down...');
    await worker.close();
    await connection.quit();
    await db.disconnect();
    process.exit(0);
  }

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return worker;
}

// Start worker
startWorker().catch((error) => {
  logger.error(`Failed to start worker: ${error instanceof Error ? error.message : 'Unknown error'}`);
  process.exit(1);
});
