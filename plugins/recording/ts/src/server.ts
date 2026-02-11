/**
 * Recording Plugin Server
 * HTTP server for recording orchestration, scheduling, and archive management
 */

import Fastify, { type FastifyRequest } from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { RecordingDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateRecordingRequest,
  UpdateRecordingRequest,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  TriggerEncodeRequest,
  SportsWebhookPayload,
  DeviceWebhookPayload,
  RecordingStatus,
  PublishStatus,
  EncodeStatus,
} from './types.js';

const logger = createLogger('recording:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new RecordingDatabase(undefined, 'primary');
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
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

  function scopedDb(request: unknown): RecordingDatabase {
    return (request as Record<string, unknown>).scopedDb as RecordingDatabase;
  }

  function getAppId(request: unknown): string {
    const ctx = getAppContext(request as FastifyRequest);
    return ctx.sourceAccountId ?? 'default';
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'recording', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'recording', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'recording',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getRecordingStats();
    return {
      alive: true,
      plugin: 'recording',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalRecordings: stats.total_recordings,
        recordingNow: stats.recording_now,
        published: stats.published,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Recording Endpoints
  // =========================================================================

  app.get('/api/recordings', async (request) => {
    const { status, publish_status, category, limit = 100, offset = 0 } = request.query as {
      status?: RecordingStatus;
      publish_status?: PublishStatus;
      category?: string;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const recordings = await scopedDb(request).listRecordings(appId, status, publish_status, category, limit, offset);
    return { data: recordings, limit, offset };
  });

  app.post('/api/recordings', async (request, reply) => {
    try {
      const body = request.body as CreateRecordingRequest;
      const appId = getAppId(request);

      if (!body.title || !body.source_type || !body.scheduled_start || !body.scheduled_end) {
        return reply.status(400).send({ error: 'title, source_type, scheduled_start, and scheduled_end are required' });
      }

      const recording = await scopedDb(request).createRecording(appId, body);
      return reply.status(201).send(recording);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create recording failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/recordings/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).getRecording(id);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.put('/api/recordings/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateRecordingRequest;
    const recording = await scopedDb(request).updateRecording(id, body);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.delete('/api/recordings/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteRecording(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return { deleted: true };
  });

  app.post('/api/recordings/:id/start', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).updateRecordingStatus(id, 'recording');

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.post('/api/recordings/:id/stop', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).updateRecordingStatus(id, 'finalizing');

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.post('/api/recordings/:id/cancel', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).cancelRecording(id);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found or cannot be cancelled' });
    }

    return recording;
  });

  app.post('/api/recordings/:id/encode', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = (request.body as TriggerEncodeRequest) ?? {};

      const recording = await scopedDb(request).getRecording(id);
      if (!recording) {
        return reply.status(404).send({ error: 'Recording not found' });
      }

      if (!recording.file_path) {
        return reply.status(400).send({ error: 'Recording has no file to encode' });
      }

      const profile = body.profile ?? fullConfig.defaultEncodeProfile;
      const job = await scopedDb(request).createEncodeJob(id, profile, recording.file_path as string, body.settings);

      // Update recording encode status
      await scopedDb(request).updateRecordingStatus(id, 'encoding', {
        encode_status: 'pending',
        encode_progress: 0,
      });

      return reply.status(201).send(job);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Trigger encode failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/recordings/:id/publish', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).publishRecording(id);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.post('/api/recordings/:id/unpublish', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).unpublishRecording(id);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    return recording;
  });

  app.post('/api/recordings/:id/enrich', async (request, reply) => {
    const { id } = request.params as { id: string };
    const existing = await scopedDb(request).getRecording(id);

    if (!existing) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    const recording = await scopedDb(request).updateRecordingStatus(id, existing.status ?? 'published' as RecordingStatus, {
      enrichment_status: 'enriching',
    });

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    // In production, this would call the media-metadata plugin
    return { ok: true, recording_id: id, enrichment_status: 'enriching' };
  });

  app.get('/api/recordings/:id/stream-url', async (request, reply) => {
    const { id } = request.params as { id: string };
    const recording = await scopedDb(request).getRecording(id);

    if (!recording) {
      return reply.status(404).send({ error: 'Recording not found' });
    }

    if (!recording.file_path && !recording.storage_object_id) {
      return reply.status(400).send({ error: 'Recording has no playback file' });
    }

    // In production, this would generate a signed URL
    return {
      recording_id: id,
      stream_url: recording.file_path ?? `${fullConfig.storageUrl}/objects/${recording.storage_object_id}`,
      expires_at: new Date(Date.now() + 3600 * 1000).toISOString(),
    };
  });

  // =========================================================================
  // Schedule Endpoints
  // =========================================================================

  app.get('/api/schedules', async (request) => {
    const { active, limit = 100, offset = 0 } = request.query as {
      active?: string;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const activeOnly = active === 'true';
    const schedules = await scopedDb(request).listSchedules(appId, activeOnly, limit, offset);
    return { data: schedules, limit, offset };
  });

  app.post('/api/schedules', async (request, reply) => {
    try {
      const body = request.body as CreateScheduleRequest;
      const appId = getAppId(request);

      if (!body.name || !body.schedule_type || !body.duration_minutes) {
        return reply.status(400).send({ error: 'name, schedule_type, and duration_minutes are required' });
      }

      const schedule = await scopedDb(request).createSchedule(appId, body);
      return reply.status(201).send(schedule);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create schedule failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.put('/api/schedules/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateScheduleRequest;
    const schedule = await scopedDb(request).updateSchedule(id, body);

    if (!schedule) {
      return reply.status(404).send({ error: 'Schedule not found' });
    }

    return schedule;
  });

  app.delete('/api/schedules/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteSchedule(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Schedule not found' });
    }

    return { deleted: true };
  });

  // =========================================================================
  // Encode Job Endpoints
  // =========================================================================

  app.get('/api/encode-jobs', async (request) => {
    const { recording_id, status, limit = 100, offset = 0 } = request.query as {
      recording_id?: string;
      status?: EncodeStatus;
      limit?: number;
      offset?: number;
    };
    const jobs = await scopedDb(request).listEncodeJobs(recording_id, status, limit, offset);
    return { data: jobs, limit, offset };
  });

  app.get('/api/encode-jobs/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const job = await scopedDb(request).getEncodeJob(id);

    if (!job) {
      return reply.status(404).send({ error: 'Encode job not found' });
    }

    return job;
  });

  // =========================================================================
  // Webhook Endpoints
  // =========================================================================

  app.post('/webhooks/sports', async (request, reply) => {
    try {
      const body = request.body as SportsWebhookPayload;
      const appId = getAppId(request);

      if (!body.event_id || !body.start_time || !body.end_time) {
        return reply.status(400).send({ error: 'event_id, start_time, and end_time are required' });
      }

      // Create recording from sports event
      const leadMs = fullConfig.defaultLeadTimeMinutes * 60 * 1000;
      const trailMs = fullConfig.defaultTrailTimeMinutes * 60 * 1000;
      const start = new Date(new Date(body.start_time).getTime() - leadMs);
      const end = new Date(new Date(body.end_time).getTime() + trailMs);

      const recording = await scopedDb(request).createRecording(appId, {
        title: body.title ?? `${body.sport} - ${body.league} - ${body.event_id}`,
        source_type: 'live_tv',
        source_channel: body.channel,
        scheduled_start: start.toISOString(),
        scheduled_end: end.toISOString(),
        sports_event_id: body.event_id,
        tags: [body.sport, body.league],
        category: 'sports',
        metadata: body.metadata,
      });

      logger.info(`Sports recording scheduled: ${recording.id} for event ${body.event_id}`);
      return reply.status(201).send({ ok: true, recording });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sports webhook failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/webhooks/device', async (request, reply) => {
    try {
      const body = request.body as DeviceWebhookPayload;

      if (!body.device_id || !body.action) {
        return reply.status(400).send({ error: 'device_id and action are required' });
      }

      if (body.action === 'recording_completed' && body.recording_id) {
        // Update recording with file info from device
        await scopedDb(request).updateRecordingStatus(body.recording_id, 'finalizing', {
          file_path: body.file_path ?? null,
          file_size: body.file_size ?? null,
          duration_seconds: body.duration_seconds ?? null,
        });

        logger.info(`Device recording completed: ${body.recording_id}`);
      }

      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Device webhook failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getRecordingStats();
    return {
      plugin: 'recording',
      version: '1.0.0',
      stats,
      config: {
        maxConcurrentRecordings: fullConfig.maxConcurrentRecordings,
        maxConcurrentEncodes: fullConfig.maxConcurrentEncodes,
        defaultEncodeProfile: fullConfig.defaultEncodeProfile,
        autoEncode: fullConfig.autoEncode,
        autoEnrich: fullConfig.autoEnrich,
        autoPublish: fullConfig.autoPublish,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Recording plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Auto-encode: ${fullConfig.autoEncode}, Auto-publish: ${fullConfig.autoPublish}`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
