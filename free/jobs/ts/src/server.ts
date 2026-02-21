/**
 * Jobs Server
 * BullBoard dashboard and API server
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createBullBoard } from '@bull-board/api';
import { BullMQAdapter } from '@bull-board/api/bullMQAdapter.js';
import { FastifyAdapter } from '@bull-board/fastify';
import { Queue } from 'bullmq';
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const IORedis = require('ioredis');
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { getConfig } from './config.js';
import { JobsDatabase } from './database.js';
import { JobPriorityValue } from './types.js';
import type { JobPayload, CreateJobOptions, JobPriority } from './types.js';

const logger = createLogger('jobs:server');

const config = getConfig();
const db = new JobsDatabase(config);

// Redis connection
const connection = new IORedis(config.redisUrl, {
  maxRetriesPerRequest: null,
});

// Create queues
const queues = {
  default: new Queue('default', { connection }),
  'high-priority': new Queue('high-priority', { connection }),
  'low-priority': new Queue('low-priority', { connection }),
};

logger.info('Starting Jobs server...');
logger.info(`Dashboard: http://localhost:${config.dashboardPort}${config.dashboardPath}`);

/**
 * Helper to get the scoped database from a request
 */
function scopedDb(request: unknown): JobsDatabase {
  return (request as Record<string, unknown>).scopedDb as JobsDatabase;
}

/**
 * Create and start server
 */
async function startServer() {
  // Connect to database
  await db.connect();
  logger.info('Database connected');

  // Initialize schema (creates tables if missing, runs migrations if needed)
  await db.initializeSchema();

  // Create Fastify app
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
  });

  // CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // App context middleware: scope database per-request based on X-App-Name header or ?app= query
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  // BullBoard dashboard
  if (config.dashboardEnabled) {
    const serverAdapter = new FastifyAdapter();
    serverAdapter.setBasePath(config.dashboardPath);

    createBullBoard({
      queues: Object.values(queues).map(q => new BullMQAdapter(q)) as any,
      serverAdapter,
    });

    await app.register(serverAdapter.registerPlugin(), {
      basePath: config.dashboardPath,
      prefix: config.dashboardPath,
    });

    logger.info(`BullBoard dashboard enabled at ${config.dashboardPath}`);
  }

  // Health endpoints
  app.get('/health', async () => {
    return { status: 'ok', plugin: 'jobs', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      await connection.ping();
      return { ready: true, plugin: 'jobs', timestamp: new Date().toISOString() };
    } catch (error) {
      return reply.status(503).send({
        ready: false,
        plugin: 'jobs',
        error: error instanceof Error ? error.message : 'Service unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // Stats endpoint
  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return stats;
  });

  // Create job endpoint
  app.post<{
    Body: {
      type: string;
      queue?: string;
      payload: JobPayload;
      options?: CreateJobOptions;
    };
  }>('/api/jobs', async (request, reply) => {
    const { type, queue = 'default', payload, options = {} } = request.body;

    if (!type || !payload) {
      return reply.status(400).send({ error: 'Type and payload are required' });
    }

    const queueName = queue in queues ? queue : 'default';
    const targetQueue = queues[queueName as keyof typeof queues];

    // Map priority to numeric value
    const priority = options.priority && options.priority in JobPriorityValue
      ? JobPriorityValue[options.priority as JobPriority]
      : 0;

    try {
      const job = await targetQueue.add(type, payload, {
        ...options,
        priority,
        attempts: options.maxRetries || config.retryAttempts,
        backoff: {
          type: 'exponential',
          delay: options.retryDelay || config.retryDelay,
        },
      });

      // Create database record
      await scopedDb(request).createJob({
        bullmq_id: job.id!,
        queue_name: queueName,
        job_type: type,
        priority: options.priority || 'normal',
        status: options.delay ? 'delayed' : 'waiting',
        payload: payload as Record<string, unknown>,
        options: job.opts,
        scheduled_for: options.delay ? new Date(Date.now() + options.delay) : undefined,
        max_retries: options.maxRetries || config.retryAttempts,
        retry_delay: options.retryDelay || config.retryDelay,
        metadata: options.metadata || {},
        tags: options.tags || [],
      });

      return {
        success: true,
        jobId: job.id,
        queue: queueName,
        type,
      };
    } catch (error) {
      return reply.status(500).send({
        error: error instanceof Error ? error.message : 'Failed to create job',
      });
    }
  });

  // Get job endpoint
  app.get<{ Params: { id: string } }>('/api/jobs/:id', async (request, reply) => {
    const { id } = request.params;

    const job = await scopedDb(request).getJobByBullMQId(id);

    if (!job) {
      return reply.status(404).send({ error: 'Job not found' });
    }

    return job;
  });

  // Graceful shutdown
  async function shutdown() {
    logger.info('Shutting down...');
    await app.close();
    await Promise.all(Object.values(queues).map(q => q.close()));
    await connection.quit();
    await db.disconnect();
    process.exit(0);
  }

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  // Start server
  await app.listen({ port: config.dashboardPort, host: '0.0.0.0' });

  logger.info(`Server running on http://localhost:${config.dashboardPort}`);
  logger.info(`Dashboard: http://localhost:${config.dashboardPort}${config.dashboardPath}`);
  logger.info('Ready to accept jobs!');

  return app;
}

// Start server
startServer().catch((error) => {
  logger.error(`Failed to start server: ${error instanceof Error ? error.message : 'Unknown error'}`);
  process.exit(1);
});
