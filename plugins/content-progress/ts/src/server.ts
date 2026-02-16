/**
 * Content Progress Plugin Server
 * HTTP server for progress tracking API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { ZodError } from 'zod';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ProgressDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type { ContentType } from './types.js';
import {
  UpdateProgressSchema,
  AddToWatchlistSchema,
  UpdateWatchlistSchema,
  AddToFavoritesSchema,
  formatZodError,
} from './schemas.js';

const logger = createLogger('progress:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new ProgressDatabase(
    undefined,
    'primary',
    fullConfig.completeThreshold,
    fullConfig.historySampleSeconds
  );

  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 5 * 1024 * 1024, // 5MB
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

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
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

  /** Extract scoped ProgressDatabase from request */
  function scopedDb(request: unknown): ProgressDatabase {
    return (request as Record<string, unknown>).scopedDb as ProgressDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'content-progress', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'content-progress', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'content-progress',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return {
      alive: true,
      plugin: 'content-progress',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalUsers: stats.total_users,
        totalPositions: stats.total_positions,
        totalCompleted: stats.total_completed,
        lastActivity: stats.last_activity,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return {
      plugin: 'content-progress',
      version: '1.0.0',
      status: 'running',
      config: {
        completeThreshold: fullConfig.completeThreshold,
        historySampleSeconds: fullConfig.historySampleSeconds,
      },
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Progress Endpoints
  // =========================================================================

  // Update progress
  app.post('/v1/progress', async (request, reply) => {
    try {
      const body = UpdateProgressSchema.parse(request.body);
      const position = await scopedDb(request).updateProgress(body);
      return position;
    } catch (error) {
      if (error instanceof ZodError) {
        return reply.status(400).send({ error: formatZodError(error) });
      }

      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Update progress failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Get all progress for user
  app.get('/v1/progress/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const positions = await scopedDb(request).getUserProgress(userId, limit, offset);
    return { data: positions, limit, offset };
  });

  // Get specific progress
  app.get('/v1/progress/:userId/:contentType/:contentId', async (request, reply) => {
    const { userId, contentType, contentId } = request.params as {
      userId: string;
      contentType: ContentType;
      contentId: string;
    };

    const position = await scopedDb(request).getProgress(userId, contentType, contentId);
    if (!position) {
      return reply.status(404).send({ error: 'Progress not found' });
    }

    return position;
  });

  // Delete progress
  app.delete('/v1/progress/:userId/:contentType/:contentId', async (request, reply) => {
    const { userId, contentType, contentId } = request.params as {
      userId: string;
      contentType: ContentType;
      contentId: string;
    };

    const deleted = await scopedDb(request).deleteProgress(userId, contentType, contentId);
    if (!deleted) {
      return reply.status(404).send({ error: 'Progress not found' });
    }

    return { deleted: true };
  });

  // Mark as completed
  app.post('/v1/progress/:userId/:contentType/:contentId/complete', async (request, reply) => {
    const { userId, contentType, contentId } = request.params as {
      userId: string;
      contentType: ContentType;
      contentId: string;
    };

    const position = await scopedDb(request).markCompleted(userId, contentType, contentId);
    if (!position) {
      return reply.status(404).send({ error: 'Progress not found' });
    }

    return position;
  });

  // Continue watching
  app.get('/v1/continue-watching/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 20 } = request.query as { limit?: number };
    const items = await scopedDb(request).getContinueWatching(userId, limit);
    return { data: items };
  });

  // Recently watched
  app.get('/v1/recently-watched/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 50 } = request.query as { limit?: number };
    const items = await scopedDb(request).getRecentlyWatched(userId, limit);
    return { data: items };
  });

  // =========================================================================
  // History Endpoints
  // =========================================================================

  app.get('/v1/history/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const history = await scopedDb(request).getUserHistory(userId, limit, offset);
    return { data: history, limit, offset };
  });

  // =========================================================================
  // Watchlist Endpoints
  // =========================================================================

  app.post('/v1/watchlist', async (request, reply) => {
    try {
      const body = AddToWatchlistSchema.parse(request.body);
      const item = await scopedDb(request).addToWatchlist(body);
      return item;
    } catch (error) {
      if (error instanceof ZodError) {
        return reply.status(400).send({ error: formatZodError(error) });
      }

      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Add to watchlist failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/watchlist/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const items = await scopedDb(request).getWatchlist(userId, limit, offset);
    return { data: items, limit, offset };
  });

  app.put('/v1/watchlist/:userId/:contentType/:contentId', async (request, reply) => {
    try {
      const { userId, contentType, contentId } = request.params as {
        userId: string;
        contentType: ContentType;
        contentId: string;
      };
      const body = UpdateWatchlistSchema.parse(request.body);

      const item = await scopedDb(request).updateWatchlistItem(userId, contentType, contentId, body);
      if (!item) {
        return reply.status(404).send({ error: 'Watchlist item not found' });
      }

      return item;
    } catch (error) {
      if (error instanceof ZodError) {
        return reply.status(400).send({ error: formatZodError(error) });
      }

      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Update watchlist failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/watchlist/:userId/:contentType/:contentId', async (request, reply) => {
    const { userId, contentType, contentId } = request.params as {
      userId: string;
      contentType: ContentType;
      contentId: string;
    };

    const deleted = await scopedDb(request).removeFromWatchlist(userId, contentType, contentId);
    if (!deleted) {
      return reply.status(404).send({ error: 'Watchlist item not found' });
    }

    return { deleted: true };
  });

  // =========================================================================
  // Favorites Endpoints
  // =========================================================================

  app.post('/v1/favorites', async (request, reply) => {
    try {
      const body = AddToFavoritesSchema.parse(request.body);
      const item = await scopedDb(request).addToFavorites(body);
      return item;
    } catch (error) {
      if (error instanceof ZodError) {
        return reply.status(400).send({ error: formatZodError(error) });
      }

      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Add to favorites failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/favorites/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const items = await scopedDb(request).getFavorites(userId, limit, offset);
    return { data: items, limit, offset };
  });

  app.delete('/v1/favorites/:userId/:contentType/:contentId', async (request, reply) => {
    const { userId, contentType, contentId } = request.params as {
      userId: string;
      contentType: ContentType;
      contentId: string;
    };

    const deleted = await scopedDb(request).removeFromFavorites(userId, contentType, contentId);
    if (!deleted) {
      return reply.status(404).send({ error: 'Favorite not found' });
    }

    return { deleted: true };
  });

  // =========================================================================
  // Stats Endpoints
  // =========================================================================

  app.get('/v1/stats/:userId', async (request) => {
    const { userId } = request.params as { userId: string };
    const stats = await scopedDb(request).getUserStats(userId);
    return stats;
  });

  // =========================================================================
  // Webhook Events (for debugging)
  // =========================================================================

  app.get('/v1/events', async (request) => {
    const { type, limit = 100, offset = 0 } = request.query as {
      type?: string;
      limit?: number;
      offset?: number;
    };
    const events = await scopedDb(request).listWebhookEvents(type, limit, offset);
    return { data: events, limit, offset };
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
      logger.success(`Content Progress plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Complete threshold: ${fullConfig.completeThreshold}%`);
      logger.info(`History sampling: ${fullConfig.historySampleSeconds}s`);
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
