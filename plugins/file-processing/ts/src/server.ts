/**
 * File Processing Plugin - HTTP Server
 */

import { createLogger, getAppContext } from '@nself/plugin-utils';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { loadConfig, validateConfig, getDatabaseConfig } from './config.js';
import { Database } from './database.js';
import type { CreateJobRequest, ProcessingStatus } from './types.js';

const logger = createLogger('file-processing:server');

async function startServer() {
  const config = loadConfig();
  validateConfig(config);

  const db = new Database(getDatabaseConfig());

  // Run multi-app migration to add source_account_id columns if missing
  await db.migrateMultiApp();

  const fastify = Fastify({
    logger: {
      level: config.logLevel,
    },
  });

  // CORS
  await fastify.register(cors, {
    origin: true,
  });

  // Multi-app context: resolve source_account_id per request and create scoped DB
  fastify.decorateRequest('scopedDb', null);
  fastify.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped Database from request */
  function scopedDb(request: unknown): Database {
    return (request as Record<string, unknown>).scopedDb as Database;
  }

  // Health check
  fastify.get('/health', async () => {
    return { status: 'ok', timestamp: new Date().toISOString() };
  });

  // Create processing job
  fastify.post<{ Body: CreateJobRequest }>('/api/jobs', async (request, reply) => {
    try {
      const sdb = scopedDb(request);
      const jobId = await sdb.createJob(request.body);

      return {
        jobId,
        status: 'pending',
        estimatedDuration: 3000, // 3 seconds estimate
      };
    } catch (error) {
      reply.code(500);
      return { error: error instanceof Error ? error.message : 'Failed to create job' };
    }
  });

  // Get job status
  fastify.get<{ Params: { jobId: string } }>('/api/jobs/:jobId', async (request, reply) => {
    const { jobId } = request.params;

    try {
      const sdb = scopedDb(request);
      const job = await sdb.getJob(jobId);
      if (!job) {
        reply.code(404);
        return { error: 'Job not found' };
      }

      const thumbnails = await sdb.getThumbnails(jobId);
      const metadata = await sdb.getMetadata(jobId);
      const scan = await sdb.getScan(jobId);

      return {
        job,
        thumbnails,
        metadata,
        scan,
      };
    } catch (error) {
      reply.code(500);
      return { error: error instanceof Error ? error.message : 'Failed to get job' };
    }
  });

  // List jobs
  fastify.get<{
    Querystring: { status?: string; limit?: string; offset?: string };
  }>('/api/jobs', async (request) => {
    const { status, limit = '50', offset = '0' } = request.query;

    try {
      const sdb = scopedDb(request);
      const jobs = await sdb.listJobs(
        status as ProcessingStatus | undefined,
        parseInt(limit, 10),
        parseInt(offset, 10)
      );

      return { jobs };
    } catch (error) {
      return { error: error instanceof Error ? error.message : 'Failed to list jobs' };
    }
  });

  // Get statistics
  fastify.get('/api/stats', async (request) => {
    try {
      const sdb = scopedDb(request);
      const stats = await sdb.getStats();
      return stats;
    } catch (error) {
      return { error: error instanceof Error ? error.message : 'Failed to get stats' };
    }
  });

  // Start server
  try {
    await fastify.listen({ port: config.port, host: config.host });
    logger.info(`Server listening on ${config.host}:${config.port}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    await db.close();
    process.exit(1);
  }

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    await fastify.close();
    await db.close();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}

startServer().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
