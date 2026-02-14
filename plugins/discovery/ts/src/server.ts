/**
 * Discovery Plugin REST API Server
 * Fastify-based HTTP server for content discovery feeds
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, loadSecurityConfig } from '@nself/plugin-utils';
import { DiscoveryDatabase } from './database.js';
import { DiscoveryCache } from './cache.js';
import { config } from './config.js';
import type {
  TrendingQuery,
  PopularQuery,
  RecentQuery,
  ContinueWatchingQuery,
  FeedResponse,
  TrendingItem,
  PopularItem,
  RecentItem,
  ContinueWatchingItem,
  HealthResponse,
  StatusResponse,
} from './types.js';

const logger = createLogger('discovery:server');

export async function createServer(db: DiscoveryDatabase, cache: DiscoveryCache) {
  const fastify = Fastify({
    logger: config.log_level === 'debug',
  });

  // Register CORS
  await fastify.register(cors, {
    origin: true,
  });

  // Register authentication and rate limiting
  const securityConfig = loadSecurityConfig('DISCOVERY');
  const rateLimiter = new ApiRateLimiter(
    securityConfig.rateLimitMax ?? 100,
    securityConfig.rateLimitWindowMs ?? 60000
  );

  fastify.addHook('preHandler', createAuthHook(securityConfig.apiKey));
  fastify.addHook('preHandler', createRateLimitHook(rateLimiter));

  // ============================================================================
  // Health Check
  // ============================================================================

  fastify.get('/health', async (): Promise<HealthResponse> => {
    const dbConnected = await db.isConnected();
    const redisConnected = cache.isConnected();

    const status = dbConnected ? (redisConnected ? 'ok' : 'degraded') : 'error';

    return {
      status,
      timestamp: new Date().toISOString(),
      database: dbConnected,
      redis: redisConnected,
      version: '1.0.0',
    };
  });

  // ============================================================================
  // GET /v1/trending
  // Returns trending content by computed score within a time window.
  // ============================================================================

  fastify.get<{
    Querystring: TrendingQuery;
  }>('/v1/trending', {
    schema: {
      querystring: {
        type: 'object',
        properties: {
          limit: { type: 'integer', minimum: 1, maximum: 100, default: config.default_limit },
          window_hours: { type: 'integer', minimum: 1, maximum: 720, default: config.trending_window_hours },
          source_account_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request): Promise<FeedResponse<TrendingItem>> => {
    const limit = request.query.limit ?? config.default_limit;
    const windowHours = request.query.window_hours ?? config.trending_window_hours;
    const sourceAccountId = request.query.source_account_id;

    try {
      const { items, cached, cached_at } = await cache.getTrending(limit, windowHours, sourceAccountId);

      return {
        items,
        count: items.length,
        cached,
        cached_at,
        generated_at: new Date().toISOString(),
      };
    } catch (error) {
      logger.error('Failed to fetch trending', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  });

  // ============================================================================
  // GET /v1/popular
  // Returns popular content by total view count weighted by ratings.
  // ============================================================================

  fastify.get<{
    Querystring: PopularQuery;
  }>('/v1/popular', {
    schema: {
      querystring: {
        type: 'object',
        properties: {
          limit: { type: 'integer', minimum: 1, maximum: 100, default: config.default_limit },
          source_account_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request): Promise<FeedResponse<PopularItem>> => {
    const limit = request.query.limit ?? config.default_limit;
    const sourceAccountId = request.query.source_account_id;

    try {
      const { items, cached, cached_at } = await cache.getPopular(limit, sourceAccountId);

      return {
        items,
        count: items.length,
        cached,
        cached_at,
        generated_at: new Date().toISOString(),
      };
    } catch (error) {
      logger.error('Failed to fetch popular', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  });

  // ============================================================================
  // GET /v1/recent
  // Returns recently added content ordered by creation date.
  // ============================================================================

  fastify.get<{
    Querystring: RecentQuery;
  }>('/v1/recent', {
    schema: {
      querystring: {
        type: 'object',
        properties: {
          limit: { type: 'integer', minimum: 1, maximum: 100, default: config.default_limit },
          source_account_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request): Promise<FeedResponse<RecentItem>> => {
    const limit = request.query.limit ?? config.default_limit;
    const sourceAccountId = request.query.source_account_id;

    try {
      const { items, cached, cached_at } = await cache.getRecent(limit, sourceAccountId);

      return {
        items,
        count: items.length,
        cached,
        cached_at,
        generated_at: new Date().toISOString(),
      };
    } catch (error) {
      logger.error('Failed to fetch recent', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  });

  // ============================================================================
  // GET /v1/continue/:userId
  // Returns continue watching items for a specific user (5% < progress < 95%).
  // ============================================================================

  fastify.get<{
    Params: { userId: string };
    Querystring: ContinueWatchingQuery;
  }>('/v1/continue/:userId', {
    schema: {
      params: {
        type: 'object',
        required: ['userId'],
        properties: {
          userId: { type: 'string', minLength: 1 },
        },
      },
      querystring: {
        type: 'object',
        properties: {
          limit: { type: 'integer', minimum: 1, maximum: 100, default: 10 },
          source_account_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request): Promise<FeedResponse<ContinueWatchingItem>> => {
    const { userId } = request.params;
    const limit = request.query.limit ?? 10;
    const sourceAccountId = request.query.source_account_id;

    try {
      const { items, cached, cached_at } = await cache.getContinueWatching(userId, limit, sourceAccountId);

      return {
        items,
        count: items.length,
        cached,
        cached_at,
        generated_at: new Date().toISOString(),
      };
    } catch (error) {
      logger.error('Failed to fetch continue watching', {
        userId,
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  });

  // ============================================================================
  // POST /v1/cache/invalidate
  // Invalidate caches. Accepts optional feed type filter.
  // ============================================================================

  fastify.post<{
    Body: { feed?: 'trending' | 'popular' | 'recent' | 'continue'; user_id?: string };
  }>('/v1/cache/invalidate', {
    schema: {
      body: {
        type: 'object',
        properties: {
          feed: { type: 'string', enum: ['trending', 'popular', 'recent', 'continue'] },
          user_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request) => {
    const { feed, user_id } = request.body || {};
    let deleted = 0;

    if (feed === 'trending') {
      deleted = await cache.invalidateTrending();
    } else if (feed === 'popular') {
      deleted = await cache.invalidatePopular();
    } else if (feed === 'recent') {
      deleted = await cache.invalidateRecent();
    } else if (feed === 'continue' && user_id) {
      deleted = await cache.invalidateContinueWatching(user_id);
    } else {
      deleted = await cache.invalidateAll();
    }

    return {
      success: true,
      deleted,
      message: feed ? `${feed} cache invalidated` : 'All caches invalidated',
    };
  });

  // ============================================================================
  // POST /v1/cache/refresh
  // Refresh precomputed cache tables (trending and popular).
  // ============================================================================

  fastify.post<{
    Body: { source_account_id?: string };
  }>('/v1/cache/refresh', {
    schema: {
      body: {
        type: 'object',
        properties: {
          source_account_id: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request) => {
    const sourceAccountId = request.body?.source_account_id || 'primary';

    try {
      const [trendingCount, popularCount] = await Promise.all([
        db.refreshTrendingCache(config.trending_window_hours, sourceAccountId),
        db.refreshPopularCache(sourceAccountId),
      ]);

      // Invalidate Redis caches so next request picks up fresh data
      await cache.invalidateTrending();
      await cache.invalidatePopular();

      return {
        success: true,
        trending_entries: trendingCount,
        popular_entries: popularCount,
        source_account_id: sourceAccountId,
      };
    } catch (error) {
      logger.error('Cache refresh failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  });

  // ============================================================================
  // GET /v1/status
  // Returns detailed status of all feeds and infrastructure.
  // ============================================================================

  fastify.get('/v1/status', async (): Promise<StatusResponse> => {
    const stats = await db.getStatistics();
    const cacheKeys = await cache.getCacheKeyCount();
    const dbConnected = await db.isConnected();
    const trendingLastComputed = await db.getTrendingLastComputed();
    const popularLastComputed = await db.getPopularLastComputed();

    return {
      feeds: {
        trending: {
          cached: cache.isConnected(),
          item_count: stats.trending_cache_entries,
          last_computed: trendingLastComputed?.toISOString() ?? null,
        },
        popular: {
          cached: cache.isConnected(),
          item_count: stats.popular_cache_entries,
          last_computed: popularLastComputed?.toISOString() ?? null,
        },
        recent: {
          cached: cache.isConnected(),
          item_count: stats.total_media_items,
        },
        continue_watching: {
          cached_users: 0, // Would require scanning Redis keys
        },
      },
      cache: {
        connected: cache.isConnected(),
        keys: cacheKeys,
      },
      database: {
        connected: dbConnected,
        media_items: stats.total_media_items,
        watch_progress: stats.total_watch_progress,
        user_ratings: stats.total_user_ratings,
      },
    };
  });

  // ============================================================================
  // Error Handler
  // ============================================================================

  fastify.setErrorHandler((error, request, reply) => {
    logger.error('Request error', {
      method: request.method,
      url: request.url,
      error: error.message,
    });

    reply.code(500).send({
      error: 'Internal server error',
      message: error.message,
    });
  });

  return fastify;
}

/**
 * Start server
 */
export async function startServer(db: DiscoveryDatabase, cache: DiscoveryCache) {
  const server = await createServer(db, cache);

  try {
    await server.listen({
      port: config.port,
      host: '0.0.0.0',
    });

    logger.info(`Discovery Plugin API server listening on port ${config.port}`);
    logger.info(`Health check: http://localhost:${config.port}/health`);

    return server;
  } catch (error) {
    logger.error('Failed to start server', { error: error instanceof Error ? error.message : String(error) });
    throw error;
  }
}
