/**
 * Media Processing Plugin Server
 * HTTP server for API endpoints and job management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { MediaProcessingDatabase } from './database.js';
import { MediaProcessor } from './processor.js';
import { DropFolderWatcher } from './watcher.js';
import { ContentIdentifier } from './identify.js';
import { QAValidator } from './qa-validator.js';
import { loadConfig, type Config } from './config.js';
import type { CreateEncodingProfileInput, CreateJobInput } from './types.js';

const logger = createLogger('media-processing:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new MediaProcessingDatabase();
  await db.connect();
  await db.initializeSchema();

  const processor = new MediaProcessor(fullConfig, db);
  await processor.initialize();

  const watcher = new DropFolderWatcher(fullConfig, db);
  const identifier = new ContentIdentifier(fullConfig);
  const qaValidator = new QAValidator();

  // Job processing queue
  const jobQueue: string[] = [];
  let processingActive = false;

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 100 * 1024 * 1024, // 100MB
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 50,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): MediaProcessingDatabase {
    return (request as Record<string, unknown>).scopedDb as MediaProcessingDatabase;
  }

  // Background job processor
  async function processJobQueue() {
    if (processingActive) return;
    processingActive = true;

    while (jobQueue.length > 0 && processor.getActiveJobCount() < fullConfig.maxConcurrentJobs) {
      const jobId = jobQueue.shift();
      if (!jobId) continue;

      // Process job in background
      processor.processJob(jobId).catch(error => {
        logger.error('Job processing error', { jobId, error: error.message });
      });
    }

    processingActive = false;
  }

  // Start queue processor
  setInterval(() => {
    processJobQueue().catch(err => {
      logger.error('Queue processor error', { error: err.message });
    });
  }, 5000);

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'media-processing', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'media-processing', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'media-processing',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'media-processing',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalJobs: stats.totalJobs,
        activeJobs: processor.getActiveJobCount(),
        queuedJobs: jobQueue.length,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'media-processing',
      version: '1.0.0',
      status: 'running',
      config: {
        maxConcurrentJobs: fullConfig.maxConcurrentJobs,
        hardwareAccel: fullConfig.hardwareAccel,
        outputBasePath: fullConfig.outputBasePath,
      },
      stats,
      queue: {
        pending: jobQueue.length,
        active: processor.getActiveJobCount(),
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Encoding Profile Endpoints
  // =========================================================================

  app.post<{ Body: CreateEncodingProfileInput }>('/v1/profiles', async (request, reply) => {
    try {
      const profile = await scopedDb(request).createEncodingProfile(request.body);
      return reply.status(201).send({ success: true, data: profile });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create profile', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.get('/v1/profiles', async (request) => {
    const profiles = await scopedDb(request).listEncodingProfiles();
    return { success: true, data: profiles };
  });

  app.get<{ Params: { id: string } }>('/v1/profiles/:id', async (request, reply) => {
    const profile = await scopedDb(request).getEncodingProfile(request.params.id);
    if (!profile) {
      return reply.status(404).send({ success: false, error: 'Profile not found' });
    }
    return { success: true, data: profile };
  });

  app.put<{ Params: { id: string }; Body: Partial<CreateEncodingProfileInput> }>('/v1/profiles/:id', async (request, reply) => {
    try {
      const profile = await scopedDb(request).updateEncodingProfile({
        id: request.params.id,
        ...request.body,
      });
      return { success: true, data: profile };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update profile', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/v1/profiles/:id', async (request, reply) => {
    try {
      await scopedDb(request).deleteEncodingProfile(request.params.id);
      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete profile', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // Job Endpoints
  // =========================================================================

  app.post<{ Body: CreateJobInput }>('/v1/jobs', async (request, reply) => {
    try {
      const job = await scopedDb(request).createJob(request.body);

      // Add to queue
      jobQueue.push(job.id);
      logger.info('Job queued', { jobId: job.id, queueLength: jobQueue.length });

      // Trigger queue processing
      processJobQueue().catch(err => {
        logger.error('Failed to start queue processing', { error: err.message });
      });

      return reply.status(201).send({ success: true, data: job });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create job', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.get<{ Querystring: { status?: string; limit?: string; offset?: string } }>('/v1/jobs', async (request) => {
    const { status, limit, offset } = request.query;
    const jobs = await scopedDb(request).listJobs(
      status,
      limit ? parseInt(limit, 10) : 50,
      offset ? parseInt(offset, 10) : 0
    );
    return { success: true, data: jobs };
  });

  app.get<{ Params: { id: string } }>('/v1/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getJobWithOutputs(request.params.id);
    if (!job) {
      return reply.status(404).send({ success: false, error: 'Job not found' });
    }
    return { success: true, data: job };
  });

  app.post<{ Params: { id: string } }>('/v1/jobs/:id/cancel', async (request, reply) => {
    try {
      await processor.cancelJob(request.params.id);
      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to cancel job', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.post<{ Params: { id: string } }>('/v1/jobs/:id/retry', async (request, reply) => {
    try {
      const job = await scopedDb(request).getJob(request.params.id);
      if (!job) {
        return reply.status(404).send({ success: false, error: 'Job not found' });
      }

      if (job.status !== 'failed' && job.status !== 'cancelled' && job.status !== 'qa_failed') {
        return reply.status(400).send({ success: false, error: 'Job is not in a retriable state' });
      }

      // Reset job status
      await scopedDb(request).updateJobStatus(request.params.id, 'pending', 0);

      // Add back to queue
      jobQueue.push(job.id);
      logger.info('Job requeued', { jobId: job.id });

      processJobQueue().catch(err => {
        logger.error('Failed to start queue processing', { error: err.message });
      });

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to retry job', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/outputs', async (request) => {
    const outputs = await scopedDb(request).getJobOutputs(request.params.id);
    return { success: true, data: outputs };
  });

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/hls', async (request, reply) => {
    const manifest = await scopedDb(request).getHlsManifest(request.params.id);
    if (!manifest) {
      return reply.status(404).send({ success: false, error: 'HLS manifest not found' });
    }
    return { success: true, data: manifest };
  });

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/subtitles', async (request) => {
    const subtitles = await scopedDb(request).getJobSubtitles(request.params.id);
    return { success: true, data: subtitles };
  });

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/trickplay', async (request, reply) => {
    const trickplay = await scopedDb(request).getTrickplay(request.params.id);
    if (!trickplay) {
      return reply.status(404).send({ success: false, error: 'Trickplay data not found' });
    }
    return { success: true, data: trickplay };
  });

  // =========================================================================
  // Media Analysis Endpoint
  // =========================================================================

  app.post<{ Body: { url: string } }>('/v1/analyze', async (request, reply) => {
    try {
      const { url } = request.body;
      if (!url) {
        return reply.status(400).send({ success: false, error: 'URL is required' });
      }

      const ffmpegClient = new (await import('./ffmpeg.js')).FFmpegClient(fullConfig);

      // Probe the file
      const metadata = await ffmpegClient.probe(url);

      return { success: true, data: metadata };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to analyze media', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // Thumbnail Generation Endpoint
  // =========================================================================

  app.post<{ Body: { url: string; count?: number } }>('/v1/thumbnail', async (request, reply) => {
    try {
      const { url, count = 5 } = request.body;
      if (!url) {
        return reply.status(400).send({ success: false, error: 'URL is required' });
      }

      const ffmpegClient = new (await import('./ffmpeg.js')).FFmpegClient(fullConfig);
      const outputDir = `${fullConfig.outputBasePath}/temp/thumbnails`;
      const { promises: fs } = await import('fs');
      await fs.mkdir(outputDir, { recursive: true });

      const pattern = `${outputDir}/thumb_%03d.jpg`;
      const paths = await ffmpegClient.extractThumbnails(url, pattern, count);

      return { success: true, data: { thumbnails: paths, count: paths.length } };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate thumbnails', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // Watcher Endpoints (UPGRADE 1c)
  // =========================================================================

  app.post<{ Body: { path?: string } }>('/v1/watcher/start', async (request, reply) => {
    try {
      await watcher.start(request.body?.path);
      return { success: true, data: watcher.getStatus() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start watcher', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  app.post('/v1/watcher/stop', async () => {
    watcher.stop();
    return { success: true, data: watcher.getStatus() };
  });

  app.get('/v1/watcher/status', async () => {
    return { success: true, data: watcher.getStatus() };
  });

  // =========================================================================
  // Content Identification Endpoint (UPGRADE 1d)
  // =========================================================================

  app.post<{ Body: { filename: string; duration?: number } }>('/v1/identify', async (request, reply) => {
    try {
      const { filename, duration } = request.body;
      if (!filename) {
        return reply.status(400).send({ success: false, error: 'filename is required' });
      }

      const result = await identifier.identifyContent(filename, duration);
      return { success: true, data: result };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to identify content', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // QA Validation Endpoint (UPGRADE 1f)
  // =========================================================================

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/qa', async (request, reply) => {
    try {
      const job = await scopedDb(request).getJob(request.params.id);
      if (!job) {
        return reply.status(404).send({ success: false, error: 'Job not found' });
      }

      const outputDir = job.output_base_path ?? `${fullConfig.outputBasePath}/${job.id}`;
      const result = await qaValidator.validateOutput(outputDir);
      return { success: true, data: result };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to run QA validation', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // Upload Records Endpoint (UPGRADE 1e)
  // =========================================================================

  app.get<{ Params: { id: string } }>('/v1/jobs/:id/uploads', async (request, reply) => {
    try {
      const uploads = await scopedDb(request).getJobUploads(request.params.id);
      return { success: true, data: uploads };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get uploads', { error: message });
      return reply.status(400).send({ success: false, error: message });
    }
  });

  // =========================================================================
  // Statistics Endpoint
  // =========================================================================

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      success: true,
      data: {
        ...stats,
        activeJobs: processor.getActiveJobCount(),
        queuedJobs: jobQueue.length,
      },
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    watcher.stop();
    await app.close();
    await db.disconnect();
    logger.info('Server shut down complete');
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return app;
}

export async function startServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);
  const app = await createServer(config);

  try {
    await app.listen({
      port: fullConfig.port,
      host: fullConfig.host,
    });

    logger.info(`Media Processing server listening on ${fullConfig.host}:${fullConfig.port}`);
    logger.info(`Hardware acceleration: ${fullConfig.hardwareAccel}`);
    logger.info(`Max concurrent jobs: ${fullConfig.maxConcurrentJobs}`);
    logger.info(`Output base path: ${fullConfig.outputBasePath}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    throw error;
  }

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer().catch(error => {
    logger.error('Fatal error', { error: error.message });
    process.exit(1);
  });
}
