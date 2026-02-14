/**
 * Stream Gateway Plugin Server
 * HTTP server for stream admission, session management, and analytics endpoints
 */

import crypto from 'node:crypto';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { StreamGatewayDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type { FastifyRequest } from 'fastify';
import type {
  AdmitRequest,
  HeartbeatRequest,
  EndSessionRequest,
  CreateStreamRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
  SessionStatus,
  StreamStatus,
  V1AdmitRequest,
  V1HeartbeatRequest,
} from './types.js';

const logger = createLogger('stream-gateway:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new StreamGatewayDatabase(undefined, 'primary');
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 5 * 1024 * 1024,
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

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): StreamGatewayDatabase {
    return (request as Record<string, unknown>).scopedDb as StreamGatewayDatabase;
  }
  function getAppId(request: unknown): string {
    const ctx = getAppContext(request as FastifyRequest);
    return ctx.sourceAccountId ?? 'default';
  }
  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'stream-gateway', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'stream-gateway', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'stream-gateway',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getGatewayStats();
    return {
      alive: true,
      plugin: 'stream-gateway',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        activeStreams: stats.active_streams,
        activeSessions: stats.active_sessions,
        totalRules: stats.total_rules,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Admission Endpoints
  // =========================================================================

  app.post('/api/admit', async (request, reply) => {
    try {
      const body = request.body as AdmitRequest;
      const appId = getAppId(request);
      const sdb = scopedDb(request);

      if (!body.stream_id || !body.user_id) {
        return reply.status(400).send({ error: 'stream_id and user_id are required' });
      }

      // Check stream exists
      const stream = await sdb.getStream(body.stream_id, appId);

      // Check concurrent user sessions
      const userSessions = await sdb.getUserActiveSessions(body.user_id, appId);
      if (userSessions.length >= fullConfig.defaultMaxConcurrent) {
        const denied = await sdb.createSession(appId, body, 'denied', 'concurrent_limit');
        return reply.status(403).send({
          admitted: false,
          session_id: denied.id,
          reason: 'concurrent_limit',
          message: `Maximum ${fullConfig.defaultMaxConcurrent} concurrent np_streamgw_streams reached. Please stop another stream first.`,
          current_sessions: userSessions.length,
          max_sessions: fullConfig.defaultMaxConcurrent,
        });
      }

      // Check device sessions if device_id provided
      if (body.device_id) {
        const deviceSessions = await sdb.getDeviceActiveSessions(body.device_id, appId);
        if (deviceSessions.length >= fullConfig.defaultMaxDeviceStreams) {
          const denied = await sdb.createSession(appId, body, 'denied', 'device_limit');
          return reply.status(403).send({
            admitted: false,
            session_id: denied.id,
            reason: 'device_limit',
            message: `Device already streaming. Maximum ${fullConfig.defaultMaxDeviceStreams} stream(s) per device.`,
            current_sessions: deviceSessions.length,
            max_sessions: fullConfig.defaultMaxDeviceStreams,
          });
        }
      }

      // Check stream max viewers
      if (stream && stream.max_viewers !== null && stream.current_viewers >= stream.max_viewers) {
        const denied = await sdb.createSession(appId, body, 'denied', 'stream_full');
        return reply.status(403).send({
          admitted: false,
          session_id: denied.id,
          reason: 'stream_full',
          message: `Stream has reached maximum viewer capacity of ${stream.max_viewers}.`,
        });
      }

      // Check admission rules
      const rules = await sdb.listRules(appId, true);
      for (const rule of rules) {
        if (rule.action === 'deny') {
          const conditions = rule.conditions as Record<string, unknown>;
          if (rule.rule_type === 'concurrent_limit') {
            const max = (conditions.max as number) ?? fullConfig.defaultMaxConcurrent;
            if (userSessions.length >= max) {
              const denied = await sdb.createSession(appId, body, 'denied', 'concurrent_limit');
              return reply.status(403).send({
                admitted: false,
                session_id: denied.id,
                reason: 'concurrent_limit',
                message: `Rule "${rule.name}": Maximum ${max} concurrent streams.`,
                current_sessions: userSessions.length,
                max_sessions: max,
              });
            }
          }
        }
      }

      // Admit the user
      const session = await sdb.createSession(appId, body, 'active');

      // Update stream viewer count
      if (stream) {
        await sdb.incrementStreamViewers(body.stream_id, appId);
      }

      return {
        admitted: true,
        session_id: session.id,
        playback_url: stream?.playback_url ?? null,
        heartbeat_interval_seconds: fullConfig.heartbeatInterval,
        max_quality: body.quality ?? 'auto',
        expires_at: new Date(Date.now() + fullConfig.sessionMaxDurationHours * 3600 * 1000).toISOString(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Admission failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/heartbeat', async (request, reply) => {
    try {
      const body = request.body as HeartbeatRequest;

      if (!body.session_id) {
        return reply.status(400).send({ error: 'session_id is required' });
      }

      const session = await scopedDb(request).heartbeatSession(
        body.session_id,
        body.bytes_transferred,
        body.quality
      );

      if (!session) {
        return reply.status(404).send({ error: 'Active session not found' });
      }

      return { ok: true, session_id: session.id, next_heartbeat_seconds: fullConfig.heartbeatInterval };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Heartbeat failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/end', async (request, reply) => {
    try {
      const body = request.body as EndSessionRequest;

      if (!body.session_id) {
        return reply.status(400).send({ error: 'session_id is required' });
      }

      const session = await scopedDb(request).endSession(body.session_id, body.bytes_transferred);

      if (!session) {
        return reply.status(404).send({ error: 'Active session not found' });
      }

      // Decrement stream viewers
      const appId = getAppId(request);
      await scopedDb(request).decrementStreamViewers(session.stream_id, appId);

      return { ok: true, session };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('End session failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stream Endpoints
  // =========================================================================

  app.get('/api/streams', async (request) => {
    const { status, limit = 100, offset = 0 } = request.query as {
      status?: StreamStatus;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const np_streamgw_streams = await scopedDb(request).listStreams(appId, status, limit, offset);
    return { data: streams, limit, offset };
  });

  app.post('/api/streams', async (request, reply) => {
    try {
      const body = request.body as CreateStreamRequest;
      const appId = getAppId(request);

      if (!body.stream_id) {
        return reply.status(400).send({ error: 'stream_id is required' });
      }

      const stream = await scopedDb(request).createStream(appId, body);
      return reply.status(201).send(stream);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create stream failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/streams/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const appId = getAppId(request);
    const stream = await scopedDb(request).getStream(id, appId);

    if (!stream) {
      return reply.status(404).send({ error: 'Stream not found' });
    }

    return stream;
  });

  app.get('/api/streams/:id/viewers', async (request, _reply) => {
    const { id } = request.params as { id: string };
    const viewers = await scopedDb(request).getStreamViewers(id);
    return { data: viewers, count: viewers.length };
  });

  app.post('/api/streams/:id/evict/:userId', async (request, reply) => {
    try {
      const { id, userId } = request.params as { id: string; userId: string };
      const evicted = await scopedDb(request).evictUserFromStream(id, userId);

      if (evicted.length === 0) {
        return reply.status(404).send({ error: 'No active sessions found for user on this stream' });
      }

      // Decrement viewer count
      const appId = getAppId(request);
      for (const _session of evicted) {
        await scopedDb(request).decrementStreamViewers(id, appId);
      }

      return { ok: true, evicted_count: evicted.length, sessions: evicted };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evict failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Session Endpoints
  // =========================================================================

  app.get('/api/sessions', async (request) => {
    const { status, limit = 100, offset = 0 } = request.query as {
      status?: SessionStatus;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const sessions = await scopedDb(request).listSessions(appId, status, limit, offset);
    return { data: sessions, limit, offset };
  });

  app.get('/api/sessions/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const sessions = await scopedDb(request).getUserActiveSessions(userId);
    return { data: sessions, count: sessions.length };
  });

  // =========================================================================
  // Rule Endpoints
  // =========================================================================

  app.get('/api/rules', async (request) => {
    const { active } = request.query as { active?: string };
    const appId = getAppId(request);
    const activeOnly = active === 'true';
    const rules = await scopedDb(request).listRules(appId, activeOnly);
    return { data: rules };
  });

  app.post('/api/rules', async (request, reply) => {
    try {
      const body = request.body as CreateRuleRequest;
      const appId = getAppId(request);

      if (!body.name || !body.rule_type || !body.conditions) {
        return reply.status(400).send({ error: 'name, rule_type, and conditions are required' });
      }

      const rule = await scopedDb(request).createRule(appId, body);
      return reply.status(201).send(rule);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create rule failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.put('/api/rules/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateRuleRequest;
    const rule = await scopedDb(request).updateRule(id, body);

    if (!rule) {
      return reply.status(404).send({ error: 'Rule not found' });
    }

    return rule;
  });

  app.delete('/api/rules/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteRule(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Rule not found' });
    }

    return { deleted: true };
  });

  // =========================================================================
  // Analytics Endpoints
  // =========================================================================

  app.get('/api/analytics/:streamId', async (request) => {
    const { streamId } = request.params as { streamId: string };
    const { limit = 100 } = request.query as { limit?: number };
    const analytics = await scopedDb(request).getStreamAnalytics(streamId, limit);
    return { data: analytics };
  });

  app.get('/api/analytics/summary', async (request) => {
    const summary = await scopedDb(request).getAnalyticsSummary();
    return summary;
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getGatewayStats();
    return {
      plugin: 'stream-gateway',
      version: '1.0.0',
      stats,
      config: {
        heartbeatInterval: fullConfig.heartbeatInterval,
        heartbeatTimeout: fullConfig.heartbeatTimeout,
        defaultMaxConcurrent: fullConfig.defaultMaxConcurrent,
        defaultMaxDeviceStreams: fullConfig.defaultMaxDeviceStreams,
        sessionMaxDurationHours: fullConfig.sessionMaxDurationHours,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // nTV v1 API Endpoints
  // =========================================================================

  /**
   * Sign a playback URL with HMAC-SHA256 for secure, time-limited access.
   */
  function signPlaybackUrl(url: string, sessionId: string, secret: string, expirySeconds: number): string {
    const expires = Math.floor(Date.now() / 1000) + expirySeconds;
    const payload = `${url}|${sessionId}|${expires}`;
    const signature = crypto.createHmac('sha256', secret).update(payload).digest('hex');
    return `${url}?token=${signature}&expires=${expires}&session=${sessionId}`;
  }

  // POST /v1/admit - Admit user to stream (nTV frontend)
  app.post('/v1/admit', async (request, reply) => {
    try {
      const body = request.body as V1AdmitRequest;
      const appId = getAppId(request);
      const sdb = scopedDb(request);

      if (!body.user_id || !body.content_id) {
        return reply.status(400).send({ admitted: false, reason: 'user_id and content_id are required' });
      }

      // Map content_id to stream_id for the existing admission logic
      const streamId = body.content_id;

      // Check stream exists
      const stream = await sdb.getStream(streamId, appId);

      // Check concurrent user sessions
      const userSessions = await sdb.getUserActiveSessions(body.user_id, appId);
      if (userSessions.length >= fullConfig.defaultMaxConcurrent) {
        await sdb.createSession(appId, {
          stream_id: streamId,
          user_id: body.user_id,
          device_id: body.device_id,
          metadata: body.content_rating ? { content_rating: body.content_rating } : {},
        }, 'denied', 'concurrent_limit');
        return reply.status(403).send({
          admitted: false,
          reason: `Maximum ${fullConfig.defaultMaxConcurrent} concurrent np_streamgw_streams reached`,
        });
      }

      // Check device sessions if device_id provided
      if (body.device_id) {
        const deviceSessions = await sdb.getDeviceActiveSessions(body.device_id, appId);
        if (deviceSessions.length >= fullConfig.defaultMaxDeviceStreams) {
          await sdb.createSession(appId, {
            stream_id: streamId,
            user_id: body.user_id,
            device_id: body.device_id,
            metadata: body.content_rating ? { content_rating: body.content_rating } : {},
          }, 'denied', 'device_limit');
          return reply.status(403).send({
            admitted: false,
            reason: `Device already streaming (max ${fullConfig.defaultMaxDeviceStreams})`,
          });
        }
      }

      // Check stream capacity
      if (stream && stream.max_viewers !== null && stream.current_viewers >= stream.max_viewers) {
        await sdb.createSession(appId, {
          stream_id: streamId,
          user_id: body.user_id,
          device_id: body.device_id,
          metadata: body.content_rating ? { content_rating: body.content_rating } : {},
        }, 'denied', 'stream_full');
        return reply.status(403).send({
          admitted: false,
          reason: 'Stream at maximum capacity',
        });
      }

      // Check admission rules
      const rules = await sdb.listRules(appId, true);
      for (const rule of rules) {
        if (rule.action === 'deny') {
          const conditions = rule.conditions as Record<string, unknown>;
          if (rule.rule_type === 'concurrent_limit') {
            const max = (conditions.max as number) ?? fullConfig.defaultMaxConcurrent;
            if (userSessions.length >= max) {
              await sdb.createSession(appId, {
                stream_id: streamId,
                user_id: body.user_id,
                device_id: body.device_id,
                metadata: body.content_rating ? { content_rating: body.content_rating } : {},
              }, 'denied', 'concurrent_limit');
              return reply.status(403).send({
                admitted: false,
                reason: `Rule "${rule.name}": maximum ${max} concurrent streams`,
              });
            }
          }
        }
      }

      // Admit the user
      const session = await sdb.createSession(appId, {
        stream_id: streamId,
        user_id: body.user_id,
        device_id: body.device_id,
        metadata: body.content_rating ? { content_rating: body.content_rating } : {},
      }, 'active');

      // Update stream viewer count
      if (stream) {
        await sdb.incrementStreamViewers(streamId, appId);
      }

      // Build signed playback URL
      const playbackUrl = stream?.playback_url ?? '';
      const expirySeconds = fullConfig.signedUrlExpirySeconds;
      const signingSecret = fullConfig.signingSecret;
      const signedUrl = signingSecret
        ? signPlaybackUrl(playbackUrl, session.id, signingSecret, expirySeconds)
        : playbackUrl;

      const expiresAt = new Date(Date.now() + expirySeconds * 1000).toISOString();

      // Generate a token from the signature for the client to hold
      const token = signingSecret
        ? crypto.createHmac('sha256', signingSecret).update(`${session.id}|${expiresAt}`).digest('hex')
        : session.id;

      return {
        admitted: true,
        session_id: session.id,
        signed_url: signedUrl,
        token,
        expires_at: expiresAt,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 admission failed', { error: message });
      return reply.status(500).send({ admitted: false, reason: message });
    }
  });

  // POST /v1/heartbeat - Session heartbeat (nTV frontend)
  app.post('/v1/heartbeat', async (request, reply) => {
    try {
      const body = request.body as V1HeartbeatRequest;

      if (!body.session_id) {
        return reply.status(400).send({ active: false, error: 'session_id is required' });
      }

      const session = await scopedDb(request).heartbeatSession(body.session_id);

      if (!session) {
        return reply.status(404).send({ active: false, error: 'Active session not found' });
      }

      const durationSeconds = session.started_at
        ? Math.floor((Date.now() - new Date(session.started_at).getTime()) / 1000)
        : 0;

      return { active: true, duration_seconds: durationSeconds };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 heartbeat failed', { error: message });
      return reply.status(500).send({ active: false, error: message });
    }
  });

  // POST /v1/sessions/:id/end - End session (nTV frontend)
  app.post('/v1/sessions/:id/end', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const sdb = scopedDb(request);

      const session = await sdb.endSession(id);

      if (!session) {
        return reply.status(404).send({ ended: false, error: 'Active session not found' });
      }

      // Decrement stream viewers
      const appId = getAppId(request);
      await sdb.decrementStreamViewers(session.stream_id, appId);

      return { ended: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 end session failed', { error: message });
      return reply.status(500).send({ ended: false, error: message });
    }
  });

  // GET /v1/sessions/active - List active sessions for account (nTV frontend)
  app.get('/v1/sessions/active', async (request) => {
    const appId = getAppId(request);
    const sessions = await scopedDb(request).getActiveSessions(appId);

    return sessions.map(s => ({
      session_id: s.id,
      user_id: s.user_id,
      content_id: s.stream_id,
      device_id: s.device_id,
      started_at: s.started_at,
      last_heartbeat: s.last_heartbeat_at,
    }));
  });

  // GET /v1/sessions/family/:family_id - Get sessions for a family group (nTV frontend)
  app.get('/v1/sessions/family/:family_id', async (request, reply) => {
    try {
      const { family_id } = request.params as { family_id: string };
      const sdb = scopedDb(request);

      const sessions = await sdb.getFamilySessions(family_id);

      return sessions.map(s => ({
        session_id: s.id,
        user_id: s.user_id,
        content_id: s.stream_id,
        device_id: s.device_id,
        started_at: s.started_at,
        last_heartbeat: s.last_heartbeat_at,
      }));
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 family sessions failed', { error: message });
      return reply.status(500).send({ error: message });
    }
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
      logger.success(`Stream Gateway plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Max concurrent: ${fullConfig.defaultMaxConcurrent}, Heartbeat: ${fullConfig.heartbeatInterval}s`);
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
