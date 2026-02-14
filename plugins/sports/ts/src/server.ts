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
  AddFavoriteRequest,
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

  /** Resolve user ID from X-User-Id header, userId query param, or fall back to source account */
  function resolveUserId(request: { headers: Record<string, string | string[] | undefined>; query: unknown }): string {
    const headerValue = request.headers['x-user-id'];
    const header = Array.isArray(headerValue) ? headerValue[0] : headerValue;
    if (header) return header;

    const query = request.query as Record<string, string | undefined>;
    if (query?.userId) return query.userId;

    const ctx = getAppContext(request as Parameters<typeof getAppContext>[0]);
    return ctx.sourceAccountId;
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
  // nTV v1 API Endpoints
  // Versioned routes consumed by nTV and other frontends. Existing /api/*
  // routes remain for backwards compatibility.
  // =========================================================================

  // POST /api/v1/sync - delegates to existing sync logic
  app.post('/api/v1/sync', async (request, reply) => {
    try {
      const body = (request.body as SyncRequest) ?? {};
      const startTime = Date.now();

      const providers = body.providers ?? fullConfig.providers;
      const syncErrors: string[] = [];
      let totalSynced = 0;

      for (const provider of providers) {
        const syncRecord = await scopedDb(request).createSyncRecord(provider, 'full');
        try {
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
      logger.error('v1 sync failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // GET /api/v1/leagues - delegates to existing leagues list
  app.get('/api/v1/leagues', async (request) => {
    const { sport } = request.query as { sport?: string };
    const leagues = await scopedDb(request).listLeagues(sport);
    return { data: leagues, total: leagues.length };
  });

  // GET /api/v1/teams - delegates to existing teams list (with optional leagueId query param)
  app.get('/api/v1/teams', async (request) => {
    const { leagueId, sport, search } = request.query as {
      leagueId?: string;
      sport?: string;
      search?: string;
    };
    const teams = await scopedDb(request).listTeams({
      league_id: leagueId,
      sport,
      search,
    });
    return { data: teams, total: teams.length };
  });

  // GET /api/v1/games - games listing with optional leagueId, status, date filters
  app.get('/api/v1/games', async (request) => {
    const query = request.query as {
      leagueId?: string;
      status?: string;
      date?: string;
      limit?: string;
      offset?: string;
    };

    // If a date is provided, use it as a day range (from start-of-day to end-of-day)
    let from: string | undefined;
    let to: string | undefined;

    if (query.date) {
      from = `${query.date}T00:00:00Z`;
      to = `${query.date}T23:59:59Z`;
    }

    const result = await scopedDb(request).listEvents({
      league_id: query.leagueId,
      status: query.status,
      from,
      to,
      limit: query.limit ? parseInt(query.limit, 10) : undefined,
      offset: query.offset ? parseInt(query.offset, 10) : undefined,
    });

    return result;
  });

  // GET /api/v1/standings/:leagueId - team standings for a league sorted by points/wins
  app.get('/api/v1/standings/:leagueId', async (request, reply) => {
    try {
      const { leagueId } = request.params as { leagueId: string };
      const { season } = request.query as { season?: string };

      const league = await scopedDb(request).getLeague(leagueId);
      if (!league) {
        return reply.status(404).send({ error: 'League not found' });
      }

      const standings = await scopedDb(request).getStandings(leagueId, season);

      return {
        data: standings,
        total: standings.length,
        league: {
          id: league.id,
          name: league.name,
          sport: league.sport,
        },
        season: season ?? null,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Get standings failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // POST /api/v1/favorites - add a favorite team
  app.post('/api/v1/favorites', async (request, reply) => {
    try {
      const body = request.body as AddFavoriteRequest;

      if (!body.team_id) {
        return reply.status(400).send({ error: 'team_id is required' });
      }

      // Verify the team exists
      const team = await scopedDb(request).getTeam(body.team_id);
      if (!team) {
        return reply.status(404).send({ error: 'Team not found' });
      }

      // Extract user ID from X-User-Id header, query param, or fallback to source account
      const userId = resolveUserId(request);

      const favorite = await scopedDb(request).addFavorite(
        userId,
        body.team_id,
        body.notify_live ?? true,
        body.auto_record ?? false
      );

      return reply.status(201).send(favorite);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Add favorite failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // GET /api/v1/favorites - list favorite teams for current user
  app.get('/api/v1/favorites', async (request) => {
    const userId = resolveUserId(request);
    const favorites = await scopedDb(request).listFavorites(userId);
    return { data: favorites, total: favorites.length };
  });

  // DELETE /api/v1/favorites/:teamId - remove a favorite team
  app.delete('/api/v1/favorites/:teamId', async (request, reply) => {
    try {
      const { teamId } = request.params as { teamId: string };
      const userId = resolveUserId(request);

      const removed = await scopedDb(request).removeFavorite(userId, teamId);
      if (!removed) {
        return reply.status(404).send({ error: 'Favorite not found' });
      }

      return { removed: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Remove favorite failed', { error: message });
      return reply.status(500).send({ error: message });
    }
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
