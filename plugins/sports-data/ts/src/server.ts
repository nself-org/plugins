/**
 * Sports Data Plugin Server
 * HTTP server for sports data API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { SportsDataDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  ListGamesQuery,
  ListTeamsQuery,
  ListLeaguesQuery,
  ListStandingsQuery,
  ListPlayersQuery,
  SearchPlayersQuery,
  CreateFavoriteRequest,
  ListFavoritesQuery,
  TriggerSyncRequest,
  ScoresQuery,
  UpcomingGamesQuery,
} from './types.js';

const logger = createLogger('sports-data:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new SportsDataDatabase();
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
    fullConfig.security.rateLimitMax ?? 200,
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

  function scopedDb(request: unknown): SportsDataDatabase {
    return (request as Record<string, unknown>).scopedDb as SportsDataDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'sports-data', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'sports-data', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'sports-data',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'sports-data',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalLeagues: stats.total_leagues,
        totalTeams: stats.total_teams,
        liveGames: stats.live_games,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // League Endpoints
  // =========================================================================

  app.get<{ Querystring: ListLeaguesQuery }>('/api/leagues', async (request) => {
    const leagues = await scopedDb(request).listLeagues({
      sport: request.query.sport,
      active: request.query.active === 'true' ? true : request.query.active === 'false' ? false : undefined,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : undefined,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { leagues, count: leagues.length };
  });

  app.get<{ Params: { id: string } }>('/api/leagues/:id', async (request, reply) => {
    const league = await scopedDb(request).getLeague(request.params.id);
    if (!league) {
      return reply.status(404).send({ error: 'League not found' });
    }
    return league;
  });

  // =========================================================================
  // Team Endpoints
  // =========================================================================

  app.get<{ Querystring: ListTeamsQuery }>('/api/teams', async (request) => {
    const teams = await scopedDb(request).listTeams({
      leagueId: request.query.league_id,
      conference: request.query.conference,
      division: request.query.division,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : undefined,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { teams, count: teams.length };
  });

  app.get<{ Params: { id: string } }>('/api/teams/:id', async (request, reply) => {
    const team = await scopedDb(request).getTeam(request.params.id);
    if (!team) {
      return reply.status(404).send({ error: 'Team not found' });
    }

    // Also fetch roster
    const roster = await scopedDb(request).listPlayers({ teamId: request.params.id });
    return { ...team, roster };
  });

  // =========================================================================
  // Game Endpoints
  // =========================================================================

  app.get<{ Querystring: ListGamesQuery }>('/api/games', async (request) => {
    const games = await scopedDb(request).listGames({
      leagueId: request.query.league_id,
      teamId: request.query.team_id,
      status: request.query.status,
      from: request.query.from ? new Date(request.query.from) : undefined,
      to: request.query.to ? new Date(request.query.to) : undefined,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 50,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { games, count: games.length };
  });

  app.get<{ Params: { id: string } }>('/api/games/:id', async (request, reply) => {
    const game = await scopedDb(request).getGame(request.params.id);
    if (!game) {
      return reply.status(404).send({ error: 'Game not found' });
    }
    return game;
  });

  app.get('/api/games/live', async (request) => {
    const games = await scopedDb(request).getLiveGames();
    return { games, count: games.length };
  });

  app.get('/api/games/today', async (request) => {
    const games = await scopedDb(request).getTodayGames();
    return { games, count: games.length };
  });

  app.get<{ Querystring: UpcomingGamesQuery }>('/api/games/upcoming', async (request, reply) => {
    const userId = request.query.user_id;
    if (!userId) {
      return reply.status(400).send({ error: 'user_id query parameter required' });
    }

    const days = request.query.days ? parseInt(String(request.query.days), 10) : 7;
    const games = await scopedDb(request).getUpcomingGamesForUser(userId, days);
    return { games, count: games.length };
  });

  // =========================================================================
  // Scores Endpoint
  // =========================================================================

  app.get<{ Querystring: ScoresQuery }>('/api/scores', async (request) => {
    const games = await scopedDb(request).getScores({
      leagueId: request.query.league_id,
      date: request.query.date ? new Date(request.query.date) : undefined,
    });

    return {
      games: games.map(g => ({
        id: g.id,
        homeTeam: { name: g.home_team_name, abbreviation: g.home_team_abbreviation, logoUrl: g.home_team_logo_url },
        awayTeam: { name: g.away_team_name, abbreviation: g.away_team_abbreviation, logoUrl: g.away_team_logo_url },
        homeScore: g.home_score,
        awayScore: g.away_score,
        status: g.status,
        period: g.period,
        clock: g.clock,
        league: g.league_name,
      })),
      count: games.length,
    };
  });

  // =========================================================================
  // Standings Endpoint
  // =========================================================================

  app.get<{ Querystring: ListStandingsQuery }>('/api/standings', async (request) => {
    const standings = await scopedDb(request).listStandings({
      leagueId: request.query.league_id,
      seasonYear: request.query.season_year ? parseInt(String(request.query.season_year), 10) : undefined,
      seasonType: request.query.season_type,
      conference: request.query.conference,
    });

    return { standings, count: standings.length };
  });

  // =========================================================================
  // Player Endpoints
  // =========================================================================

  app.get<{ Querystring: ListPlayersQuery }>('/api/players', async (request) => {
    const players = await scopedDb(request).listPlayers({
      teamId: request.query.team_id,
      position: request.query.position,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : undefined,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { players, count: players.length };
  });

  app.get<{ Params: { id: string } }>('/api/players/:id', async (request, reply) => {
    const player = await scopedDb(request).getPlayer(request.params.id);
    if (!player) {
      return reply.status(404).send({ error: 'Player not found' });
    }
    return player;
  });

  app.get<{ Querystring: SearchPlayersQuery }>('/api/players/search', async (request, reply) => {
    if (!request.query.query) {
      return reply.status(400).send({ error: 'query parameter required' });
    }

    const limit = request.query.limit ? parseInt(String(request.query.limit), 10) : 20;
    const players = await scopedDb(request).searchPlayers(request.query.query, limit);
    return { players, count: players.length };
  });

  // =========================================================================
  // Favorites Endpoints
  // =========================================================================

  app.post<{ Body: CreateFavoriteRequest }>('/api/favorites', async (request, reply) => {
    try {
      const favorite = await scopedDb(request).addFavorite({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        user_id: request.body.user_id,
        favorite_type: request.body.favorite_type,
        favorite_id: request.body.favorite_id,
      });

      return reply.status(201).send(favorite);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to add favorite', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListFavoritesQuery }>('/api/favorites', async (request, reply) => {
    if (!request.query.user_id) {
      return reply.status(400).send({ error: 'user_id query parameter required' });
    }

    const favorites = await scopedDb(request).listFavorites(request.query.user_id);
    return { favorites, count: favorites.length };
  });

  app.delete<{ Params: { id: string } }>('/api/favorites/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteFavorite(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Favorite not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Sync Endpoints
  // =========================================================================

  app.post<{ Body: TriggerSyncRequest }>('/api/sync', async (request, reply) => {
    try {
      const provider = request.body.provider ?? fullConfig.provider;
      const resources = request.body.resources ?? ['games', 'standings', 'teams'];

      for (const resource of resources) {
        await scopedDb(request).updateSyncState(provider, resource, 'syncing');
      }

      logger.info('Sync triggered', { provider, resources });

      // In a real implementation, this would trigger actual API calls
      // For now, mark as completed
      for (const resource of resources) {
        await scopedDb(request).updateSyncState(provider, resource, 'idle');
      }

      return reply.status(202).send({
        message: 'Sync triggered',
        provider,
        resources,
        timestamp: new Date().toISOString(),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync trigger failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/sync/status', async (request) => {
    const syncStates = await scopedDb(request).getSyncStatus();
    return { sync_states: syncStates, count: syncStates.length };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'sports-data',
      version: '1.0.0',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    async start() {
      try {
        await app.listen({ port: fullConfig.port, host: fullConfig.host });
        logger.info(`Sports-data server listening on ${fullConfig.host}:${fullConfig.port}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      await app.close();
      await db.disconnect();
      logger.info('Server stopped');
    },
  };

  return server;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const server = await createServer(config);
  await server.start();

  process.on('SIGTERM', async () => {
    logger.info('SIGTERM received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });

  process.on('SIGINT', async () => {
    logger.info('SIGINT received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
