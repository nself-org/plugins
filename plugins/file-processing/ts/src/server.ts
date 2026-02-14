/**
 * File Processing Plugin - HTTP Server
 */

import { createLogger, getAppContext } from '@nself/plugin-utils';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { tmpdir } from 'os';
import { join } from 'path';
import { loadConfig, validateConfig, getDatabaseConfig } from './config.js';
import { Database } from './database.js';
import { generatePosters, generateSpriteSheet, optimizeImage } from './image-processor.js';
import type {
  CreateJobRequest,
  ProcessingStatus,
  PosterRequest,
  SpriteRequest,
  OptimizeRequest,
} from './types.js';

const logger = createLogger('file-processing:server');

async function startServer() {
  const config = loadConfig();
  validateConfig(config);

  const db = new Database(getDatabaseConfig());

  // Initialize schema (creates tables if missing, runs migrations if they exist)
  await db.initializeSchema();

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

  // =========================================================================
  // Existing /api/* routes (backwards compatible)
  // =========================================================================

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

  // =========================================================================
  // nTV /v1/* image processing endpoints
  // =========================================================================

  // POST /v1/poster - Generate poster thumbnails at multiple widths/formats
  fastify.post<{ Body: PosterRequest }>('/v1/poster', async (request, reply) => {
    const {
      input_path,
      widths = [100, 400, 1200],
      formats = ['webp', 'avif', 'jpeg'],
    } = request.body;

    if (!input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }

    try {
      const outputDir = join(tmpdir(), `poster-${Date.now()}`);
      const outputs = await generatePosters(input_path, widths, formats, outputDir);

      return { outputs };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Poster generation failed';
      logger.error('POST /v1/poster failed', { error: message });
      reply.code(500);
      return { error: message };
    }
  });

  // POST /v1/sprite - Generate sprite sheet for trickplay
  fastify.post<{ Body: SpriteRequest }>('/v1/sprite', async (request, reply) => {
    const {
      input_path,
      grid = '10x10',
      thumb_size = '320x180',
    } = request.body;

    if (!input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }

    try {
      const outputDir = join(tmpdir(), `sprite-${Date.now()}`);
      const result = await generateSpriteSheet(input_path, grid, thumb_size, outputDir);

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Sprite generation failed';
      logger.error('POST /v1/sprite failed', { error: message });
      reply.code(500);
      return { error: message };
    }
  });

  // POST /v1/optimize - Optimize an image file
  fastify.post<{ Body: OptimizeRequest }>('/v1/optimize', async (request, reply) => {
    const {
      input_path,
      format = 'webp',
      quality = 80,
      strip_exif = true,
    } = request.body;

    if (!input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }

    try {
      const outputDir = join(tmpdir(), `optimize-${Date.now()}`);
      const result = await optimizeImage(input_path, format, quality, strip_exif, outputDir);

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Image optimization failed';
      logger.error('POST /v1/optimize failed', { error: message });
      reply.code(500);
      return { error: message };
    }
  });

  // GET /v1/jobs/:id - Job status alias for /api/jobs/:jobId
  fastify.get<{ Params: { id: string } }>('/v1/jobs/:id', async (request, reply) => {
    const { id } = request.params;

    try {
      const sdb = scopedDb(request);
      const job = await sdb.getJob(id);
      if (!job) {
        reply.code(404);
        return { error: 'Job not found' };
      }

      return {
        state: job.status,
        progress: job.status === 'completed' ? 100 : job.status === 'processing' ? 50 : 0,
      };
    } catch (error) {
      reply.code(500);
      return { error: error instanceof Error ? error.message : 'Failed to get job' };
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
