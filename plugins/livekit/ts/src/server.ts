/**
 * LiveKit Plugin Server
 * HTTP server for LiveKit voice/video infrastructure management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { LiveKitDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateRoomRequest,
  CreateParticipantRequest,
  UpdateParticipantRequest,
  CreateTokenRequest,
  CreateQualityMetricRequest,
  StartRoomCompositeRequest,
  StartTrackEgressRequest,
  StartStreamEgressRequest,
  RoomStatus,
  EgressStatus,
} from './types.js';
import crypto from 'node:crypto';

const logger = createLogger('livekit:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new LiveKitDatabase();

  // Connect to database
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
    fullConfig.security.rateLimitMax ?? 100,
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

  function scopedDb(request: unknown): LiveKitDatabase {
    return (request as Record<string, unknown>).scopedDb as LiveKitDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'livekit', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'livekit', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({ ready: false, plugin: 'livekit', error: 'Database unavailable', timestamp: new Date().toISOString() });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'livekit',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'livekit',
      version: '1.0.0',
      status: 'running',
      livekitUrl: fullConfig.livekitUrl,
      egressEnabled: fullConfig.egressEnabled,
      qualityMonitoringEnabled: fullConfig.qualityMonitoringEnabled,
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Room Management
  // =========================================================================

  app.post<{ Body: CreateRoomRequest }>('/api/livekit/rooms', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.roomName || !body.roomType) {
        return reply.status(400).send({ error: 'roomName and roomType are required' });
      }

      const room = await scopedDb(request).createRoom(body);
      // Activate the room
      const activated = await scopedDb(request).updateRoom(room.id, {
        status: 'active',
        activatedAt: new Date().toISOString(),
      });

      return reply.status(201).send({
        success: true,
        room: activated,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create room', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{ Params: { roomId: string } }>('/api/livekit/rooms/:roomId', async (request, reply) => {
    const room = await scopedDb(request).getRoom(request.params.roomId);
    if (!room) {
      return reply.status(404).send({ error: 'Room not found' });
    }
    return { success: true, room };
  });

  app.get<{ Querystring: { status?: RoomStatus; roomType?: string; limit?: string; offset?: string } }>(
    '/api/livekit/rooms',
    async (request) => {
      const rooms = await scopedDb(request).listRooms({
        status: request.query.status,
        roomType: request.query.roomType,
        limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
        offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
      });
      return { success: true, rooms, count: rooms.length };
    }
  );

  app.delete<{ Params: { roomId: string } }>('/api/livekit/rooms/:roomId', async (request, reply) => {
    const room = await scopedDb(request).closeRoom(request.params.roomId);
    if (!room) {
      return reply.status(404).send({ error: 'Room not found' });
    }
    return { success: true, room };
  });

  // =========================================================================
  // Token Management
  // =========================================================================

  app.post<{ Body: CreateTokenRequest }>('/api/livekit/tokens', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.roomName || !body.participantIdentity) {
        return reply.status(400).send({ error: 'roomName and participantIdentity are required' });
      }

      // Find or create room
      let room = await scopedDb(request).getRoomByName(body.roomName);
      if (!room) {
        return reply.status(404).send({ error: `Room not found: ${body.roomName}` });
      }

      const ttl = Math.min(body.ttl ?? fullConfig.tokenDefaultTtl, fullConfig.tokenMaxTtl);
      const expiresAt = new Date(Date.now() + ttl * 1000);

      const grants = body.grants ?? {
        canPublish: true,
        canSubscribe: true,
        canPublishData: true,
      };

      // Generate a placeholder token (real implementation would use livekit-server-sdk)
      const tokenPayload = JSON.stringify({
        roomName: body.roomName,
        identity: body.participantIdentity,
        name: body.participantName,
        grants,
        exp: Math.floor(expiresAt.getTime() / 1000),
      });
      const token = Buffer.from(tokenPayload).toString('base64url');
      const tokenHash = crypto.createHash('sha256').update(token).digest('hex');

      const tokenRecord = await scopedDb(request).createToken(
        room.id,
        body.participantIdentity,
        tokenHash,
        grants,
        expiresAt,
      );

      return reply.status(201).send({
        success: true,
        token,
        tokenId: tokenRecord.id,
        livekitUrl: fullConfig.livekitUrl,
        expiresAt: expiresAt.toISOString(),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate token', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.post<{ Params: { tokenId: string }; Body: { revokedBy: string; reason?: string } }>(
    '/api/livekit/tokens/:tokenId/revoke',
    async (request, reply) => {
      try {
        const body = request.body;
        if (!body.revokedBy) {
          return reply.status(400).send({ error: 'revokedBy is required' });
        }

        const token = await scopedDb(request).revokeToken(
          request.params.tokenId,
          body.revokedBy,
          body.reason,
        );

        if (!token) {
          return reply.status(404).send({ error: 'Token not found or already revoked' });
        }

        return { success: true, token };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to revoke token', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Querystring: { roomId?: string; limit?: string; offset?: string } }>(
    '/api/livekit/tokens',
    async (request) => {
      const tokens = await scopedDb(request).listTokens({
        roomId: request.query.roomId,
        limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
        offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
      });
      return { success: true, tokens, count: tokens.length };
    }
  );

  // =========================================================================
  // Recording & Egress
  // =========================================================================

  app.post<{ Body: StartRoomCompositeRequest }>('/api/livekit/egress/room-composite', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.roomName) {
        return reply.status(400).send({ error: 'roomName is required' });
      }

      const room = await scopedDb(request).getRoomByName(body.roomName);
      if (!room) {
        return reply.status(404).send({ error: `Room not found: ${body.roomName}` });
      }

      // Generate egress ID (real implementation would call LiveKit API)
      const egressId = `EG_${crypto.randomBytes(12).toString('hex')}`;

      const job = await scopedDb(request).createEgressJob({
        roomId: room.id,
        livekitEgressId: egressId,
        egressType: 'room',
        outputType: body.fileOutput ? 'file' : 'stream',
        config: {
          layout: body.layout ?? 'grid',
          audioOnly: body.audioOnly ?? false,
          videoOptions: body.videoOptions,
          fileOutput: body.fileOutput,
        },
      });

      // Mark as active
      const activeJob = await scopedDb(request).updateEgressJob(job.id, { status: 'active' });

      return reply.status(201).send({
        success: true,
        egressId,
        jobId: activeJob?.id,
        status: 'active',
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start room composite egress', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.post<{ Body: StartTrackEgressRequest }>('/api/livekit/egress/track', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.roomName || !body.trackSid) {
        return reply.status(400).send({ error: 'roomName and trackSid are required' });
      }

      const room = await scopedDb(request).getRoomByName(body.roomName);
      if (!room) {
        return reply.status(404).send({ error: `Room not found: ${body.roomName}` });
      }

      const egressId = `EG_${crypto.randomBytes(12).toString('hex')}`;

      const job = await scopedDb(request).createEgressJob({
        roomId: room.id,
        livekitEgressId: egressId,
        egressType: 'track',
        outputType: 'file',
        config: { trackSid: body.trackSid, fileOutput: body.fileOutput },
      });

      const activeJob = await scopedDb(request).updateEgressJob(job.id, { status: 'active' });

      return reply.status(201).send({
        success: true,
        egressId,
        jobId: activeJob?.id,
        status: 'active',
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start track egress', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.post<{ Body: StartStreamEgressRequest }>('/api/livekit/egress/stream', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.roomName || !body.urls || body.urls.length === 0) {
        return reply.status(400).send({ error: 'roomName and urls are required' });
      }

      const room = await scopedDb(request).getRoomByName(body.roomName);
      if (!room) {
        return reply.status(404).send({ error: `Room not found: ${body.roomName}` });
      }

      const egressId = `EG_${crypto.randomBytes(12).toString('hex')}`;

      const job = await scopedDb(request).createEgressJob({
        roomId: room.id,
        livekitEgressId: egressId,
        egressType: 'stream',
        outputType: 'stream',
        config: { urls: body.urls, protocol: body.streamProtocol ?? 'rtmp' },
      });

      const activeJob = await scopedDb(request).updateEgressJob(job.id, { status: 'active' });

      return reply.status(201).send({
        success: true,
        egressId,
        jobId: activeJob?.id,
        status: 'active',
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start stream egress', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{ Params: { egressId: string } }>('/api/livekit/egress/:egressId', async (request, reply) => {
    const job = await scopedDb(request).getEgressJobByEgressId(request.params.egressId);
    if (!job) {
      return reply.status(404).send({ error: 'Egress job not found' });
    }
    return { success: true, job };
  });

  app.get<{ Querystring: { roomId?: string; status?: EgressStatus; egressType?: string; limit?: string; offset?: string } }>(
    '/api/livekit/egress',
    async (request) => {
      const jobs = await scopedDb(request).listEgressJobs({
        roomId: request.query.roomId,
        status: request.query.status,
        egressType: request.query.egressType,
        limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
        offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
      });
      return { success: true, jobs, count: jobs.length };
    }
  );

  app.delete<{ Params: { egressId: string } }>('/api/livekit/egress/:egressId', async (request, reply) => {
    const job = await scopedDb(request).stopEgressJob(request.params.egressId);
    if (!job) {
      return reply.status(404).send({ error: 'Egress job not found' });
    }
    return { success: true, job };
  });

  // =========================================================================
  // Participant Management
  // =========================================================================

  app.post<{ Params: { roomId: string }; Body: CreateParticipantRequest }>(
    '/api/livekit/rooms/:roomId/participants',
    async (request, reply) => {
      try {
        const body = { ...request.body, roomId: request.params.roomId };
        if (!body.userId || !body.livekitIdentity) {
          return reply.status(400).send({ error: 'userId and livekitIdentity are required' });
        }

        const room = await scopedDb(request).getRoom(request.params.roomId);
        if (!room) {
          return reply.status(404).send({ error: 'Room not found' });
        }

        const participant = await scopedDb(request).createParticipant(body);
        return reply.status(201).send({ success: true, participant });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to add participant', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { roomId: string }; Querystring: { status?: string } }>(
    '/api/livekit/rooms/:roomId/participants',
    async (request) => {
      const participants = await scopedDb(request).listParticipants(
        request.params.roomId,
        request.query.status as 'joining' | 'joined' | 'reconnecting' | 'disconnected' | undefined,
      );
      return { success: true, participants, count: participants.length };
    }
  );

  app.delete<{ Params: { roomId: string; participantId: string } }>(
    '/api/livekit/rooms/:roomId/participants/:participantId',
    async (request, reply) => {
      const removed = await scopedDb(request).removeParticipant(request.params.participantId);
      if (!removed) {
        return reply.status(404).send({ error: 'Participant not found' });
      }
      return { success: true, removed: true };
    }
  );

  app.post<{ Params: { roomId: string; participantId: string }; Body: { trackType?: string } }>(
    '/api/livekit/rooms/:roomId/participants/:participantId/mute',
    async (request, reply) => {
      try {
        const trackType = request.body.trackType ?? 'microphone';
        const updates: UpdateParticipantRequest = {};

        if (trackType === 'microphone') updates.microphoneEnabled = false;
        else if (trackType === 'camera') updates.cameraEnabled = false;
        else if (trackType === 'screen_share') updates.screenShareEnabled = false;
        else return reply.status(400).send({ error: `Invalid trackType: ${trackType}` });

        const participant = await scopedDb(request).updateParticipant(request.params.participantId, updates);
        if (!participant) {
          return reply.status(404).send({ error: 'Participant not found' });
        }

        return { success: true, participant };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to mute participant', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Quality Monitoring
  // =========================================================================

  app.get<{ Params: { roomId: string } }>('/api/livekit/rooms/:roomId/quality', async (request, reply) => {
    try {
      const roomMetrics = await scopedDb(request).getRoomQualityMetrics(request.params.roomId);
      const participantMetrics = await scopedDb(request).getParticipantQualityMetrics(request.params.roomId);

      return {
        success: true,
        room: roomMetrics,
        participants: participantMetrics,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get quality metrics', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.post<{ Body: CreateQualityMetricRequest }>('/api/livekit/quality-metrics', async (request, reply) => {
    try {
      if (!request.body.roomId || !request.body.metricType) {
        return reply.status(400).send({ error: 'roomId and metricType are required' });
      }

      const metric = await scopedDb(request).recordQualityMetric(request.body);
      return reply.status(201).send({ success: true, metric });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to record quality metric', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhook', async (request, reply) => {
    try {
      const payload = request.body as Record<string, unknown>;
      const eventType = payload.type as string ?? payload.event as string;

      if (!eventType) {
        return reply.status(400).send({ error: 'Missing event type' });
      }

      await scopedDb(request).insertWebhookEvent(eventType, payload);

      return { received: true, type: eventType };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  return app;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const fullConfig = loadConfig(config);
  const app = await createServer(config);

  try {
    await app.listen({
      port: fullConfig.port,
      host: fullConfig.host,
    });

    logger.info('LiveKit plugin server running', {
      port: fullConfig.port,
      host: fullConfig.host,
      livekitUrl: fullConfig.livekitUrl,
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
