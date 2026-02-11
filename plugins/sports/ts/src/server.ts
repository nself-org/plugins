/**
 * Sports Plugin Server
 * HTTP server for sports schedule and metadata API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { SportsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  LockEventRequest,
  OverrideEventRequest,
  SyncRequest,
  ReconcileRequest,
} from './types.js';

const logger = createLogger('sports:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new SportsDatabase(undefined, 'primary');

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

  function scopedDb(request: unknown): SportsDatabase {
    return (request as Record<string, unknown>).scopedDb as SportsDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'sports', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'sports', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'sports',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return {
      alive: true,
      plugin: 'sports',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        leagues: stats.leagues,
        teams: stats.teams,
        events: stats.events,
        liveEvents: stats.live_events,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/status', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    const syncStats = await scopedDb(request).getSyncStats();
    return {
      plugin: 'sports',
      version: '1.0.0',
      status: 'running',
      providers: fullConfig.providers,
      stats,
      syncStats,
      config: {
        enabledSports: fullConfig.enabledSports,
        enabledLeagues: fullConfig.enabledLeagues,
        lockWindowMinutes: fullConfig.lockWindowMinutes,
        cacheEnabled: fullConfig.cacheEnabled,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // League Endpoints
  // =========================================================================

  app.get('/api/leagues', async (request) => {
    const { sport } = request.query as { sport?: string };
    const leagues = await scopedDb(request).listLeagues(sport);
    return { data: leagues, total: leagues.length };
  });

  app.get('/api/leagues/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const league = await scopedDb(request).getLeague(id);
    if (!league) {
      return reply.status(404).send({ error: 'League not found' });
    }
    return league;
  });

  // =========================================================================
  // Team Endpoints
  // =========================================================================

  app.get('/api/teams', async (request) => {
    const { league_id, sport, search } = request.query as {
      league_id?: string;
      sport?: string;
      search?: string;
    };
    const teams = await scopedDb(request).listTeams({ league_id, sport, search });
    return { data: teams, total: teams.length };
  });

  app.get('/api/teams/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const team = await scopedDb(request).getTeam(id);
    if (!team) {
      return reply.status(404).send({ error: 'Team not found' });
    }
    return team;
  });

  // =========================================================================
  // Event Endpoints
  // =========================================================================

  app.get('/api/events', async (request) => {
    const query = request.query as {
      league_id?: string;
      team_id?: string;
      status?: string;
      from?: string;
      to?: string;
      season?: string;
      week?: string;
      limit?: string;
      offset?: string;
    };

    const result = await scopedDb(request).listEvents({
      league_id: query.league_id,
      team_id: query.team_id,
      status: query.status,
      from: query.from,
      to: query.to,
      season: query.season,
      week: query.week ? parseInt(query.week, 10) : undefined,
      limit: query.limit ? parseInt(query.limit, 10) : undefined,
      offset: query.offset ? parseInt(query.offset, 10) : undefined,
    });

    return result;
  });

  app.get('/api/events/upcoming', async (request) => {
    const { league_id, team_id, limit } = request.query as {
      league_id?: string;
      team_id?: string;
      limit?: string;
    };
    const events = await scopedDb(request).getUpcomingEvents({
      league_id,
      team_id,
      limit: limit ? parseInt(limit, 10) : undefined,
    });
    return { data: events };
  });

  app.get('/api/events/live', async (request) => {
    const { league_id } = request.query as { league_id?: string };
    const events = await scopedDb(request).getLiveEvents(league_id);
    return { data: events };
  });

  app.get('/api/events/today', async (request) => {
    const { league_id, timezone } = request.query as { league_id?: string; timezone?: string };
    const events = await scopedDb(request).getTodayEvents({ league_id, timezone });
    return { data: events };
  });

  app.get('/api/events/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const event = await scopedDb(request).getEvent(id);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }
    return event;
  });

  app.post('/api/events/:id/lock', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as LockEventRequest;

      if (!body.reason) {
        return reply.status(400).send({ error: 'Reason is required' });
      }

      const event = await scopedDb(request).lockEvent(id, body.reason);
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }

      return { locked: true, event };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Lock event failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/events/:id/unlock', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const event = await scopedDb(request).unlockEvent(id);
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }
      return { locked: false, event };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Unlock event failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/events/:id/override', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as OverrideEventRequest;

      if (!body.notes) {
        return reply.status(400).send({ error: 'Notes are required for operator override' });
      }

      const event = await scopedDb(request).overrideEvent(
        id,
        body.scheduled_at,
        body.broadcast_channel,
        body.notes
      );
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }

      return event;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Override event failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/events/:id/trigger-recording', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };

      const event = await scopedDb(request).getEvent(id);
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }

      await scopedDb(request).markRecordingTriggered(id);

      return { triggered: true, event_id: id };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Trigger recording failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Sync Endpoints
  // =========================================================================

  app.post('/sync', async (request, reply) => {
    try {
      const body = (request.body as SyncRequest) ?? {};
      const startTime = Date.now();

      const providers = body.providers ?? fullConfig.providers;
      const syncErrors: string[] = [];
      let totalSynced = 0;

      for (const provider of providers) {
        const syncRecord = await scopedDb(request).createSyncRecord(provider, 'full');
        try {
          // Placeholder for actual provider sync logic
          logger.info(`Syncing from provider: ${provider}`);
          await scopedDb(request).updateSyncRecord(syncRecord.id, 'completed', 0);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          syncErrors.push(`${provider}: ${message}`);
          await scopedDb(request).updateSyncRecord(syncRecord.id, 'failed', 0, [message]);
        }
      }

      const duration = Date.now() - startTime;

      return {
        success: syncErrors.length === 0,
        stats: { total_synced: totalSynced },
        errors: syncErrors,
        duration_ms: duration,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/reconcile', async (request, reply) => {
    try {
      const body = (request.body as ReconcileRequest) ?? {};
      const lookbackDays = body.lookback_days ?? 7;

      logger.info(`Reconciling last ${lookbackDays} days`);

      return {
        success: true,
        stats: { lookback_days: lookbackDays },
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reconcile failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhooks/:provider', async (request, reply) => {
    try {
      const { provider } = request.params as { provider: string };
      const payload = request.body as Record<string, unknown>;

      await scopedDb(request).insertWebhookEvent(
        provider,
        (payload.type as string) ?? 'unknown',
        payload,
        payload.id as string | undefined
      );

      return { received: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stats and Cache Endpoints
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getSyncStats();
    return stats;
  });

  app.get('/api/cache/status', async (request) => {
    const cacheStats = await scopedDb(request).getCacheStats();
    return cacheStats;
  });

  // =========================================================================
  // Graceful Shutdown
  // =========================================================================

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
      logger.success(`Sports plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Providers: ${fullConfig.providers.join(', ')}`);
      logger.info(`Enabled sports: ${fullConfig.enabledSports.join(', ')}`);
      logger.info(`Lock window: ${fullConfig.lockWindowMinutes} minutes`);
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
