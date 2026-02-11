/**
 * Activity Feed Plugin Server
 * HTTP server for activity feed API endpoints
 */

import Fastify, { type FastifyInstance } from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { FeedDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateActivityInput,
  CreateSubscriptionInput,
  FeedQuery,
  ActivityQuery,
  EntityFeedQuery,
} from './types.js';

const logger = createLogger('feed:server');

export interface FeedServer extends FastifyInstance {
  start: () => Promise<void>;
}

export async function createServer(config?: Partial<Config>): Promise<FeedServer> {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new FeedDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
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

  /** Extract scoped FeedDatabase from request */
  function scopedDb(request: unknown): FeedDatabase {
    return (request as Record<string, unknown>).scopedDb as FeedDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'activity-feed', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'activity-feed', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'activity-feed',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'activity-feed',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      config: {
        strategy: fullConfig.strategy,
        maxFeedSize: fullConfig.maxFeedSize,
        aggregationWindowMinutes: fullConfig.aggregationWindowMinutes,
      },
      stats: {
        totalActivities: stats.totalActivities,
        totalSubscriptions: stats.totalSubscriptions,
        unreadFeedItems: stats.unreadFeedItems,
        lastActivity: stats.lastActivityAt,
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
      plugin: 'activity-feed',
      version: '1.0.0',
      status: 'running',
      config: {
        strategy: fullConfig.strategy,
        maxFeedSize: fullConfig.maxFeedSize,
        aggregationWindowMinutes: fullConfig.aggregationWindowMinutes,
        retentionDays: fullConfig.retentionDays,
      },
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Activity Endpoints
  // =========================================================================

  app.post('/v1/activities', async (request, reply) => {
    const input = request.body as CreateActivityInput;

    try {
      const activity = await scopedDb(request).createActivity(input);

      // Fan-out-on-write: create feed items for subscribers if strategy is 'write'
      if (fullConfig.strategy === 'write') {
        const subscribers = await scopedDb(request).getSubscribersForActor(activity.actor_id);
        for (const userId of subscribers) {
          await scopedDb(request).createFeedItem(userId, activity.id);
        }
      }

      return activity;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create activity', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/activities', async (request, reply) => {
    const query: ActivityQuery = {
      actorId: (request.query as Record<string, string>).actorId,
      verb: (request.query as Record<string, string>).verb as ActivityQuery['verb'],
      objectType: (request.query as Record<string, string>).objectType,
      objectId: (request.query as Record<string, string>).objectId,
      targetType: (request.query as Record<string, string>).targetType,
      targetId: (request.query as Record<string, string>).targetId,
      limit: parseInt((request.query as Record<string, string>).limit ?? '100', 10),
      offset: parseInt((request.query as Record<string, string>).offset ?? '0', 10),
    };

    try {
      const [activities, total] = await Promise.all([
        scopedDb(request).listActivities(query),
        scopedDb(request).countActivities(query),
      ]);

      return {
        data: activities,
        total,
        limit: query.limit,
        offset: query.offset,
        hasMore: (query.offset ?? 0) + activities.length < total,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list activities', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/activities/:id', async (request, reply) => {
    const { id } = request.params as { id: string };

    try {
      const activity = await scopedDb(request).getActivity(id);
      if (!activity) {
        return reply.status(404).send({ error: 'Activity not found' });
      }
      return activity;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get activity', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/activities/:id', async (request, reply) => {
    const { id } = request.params as { id: string };

    try {
      await scopedDb(request).deleteActivity(id);
      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete activity', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Feed Endpoints
  // =========================================================================

  app.get('/v1/feed/:userId', async (request, reply) => {
    const { userId } = request.params as { userId: string };
    const queryParams = request.query as Record<string, string>;

    const query: FeedQuery = {
      userId,
      limit: Math.min(parseInt(queryParams.limit ?? '100', 10), fullConfig.maxFeedSize),
      offset: parseInt(queryParams.offset ?? '0', 10),
      includeRead: queryParams.includeRead !== 'false',
      includeHidden: queryParams.includeHidden === 'true',
    };

    try {
      let feedItems: Array<{ activity: unknown }>;
      let total: number;
      let unreadCount: number;

      if (fullConfig.strategy === 'write') {
        // Fan-out-on-write: read from pre-materialized feed
        const items = await scopedDb(request).getUserFeed(query);
        feedItems = items as Array<{ activity: unknown }>;
        [total, unreadCount] = await Promise.all([
          scopedDb(request).countUserFeedItems(userId, query.includeRead ?? true, query.includeHidden ?? false),
          scopedDb(request).getUnreadCount(userId),
        ]);
      } else {
        // Fan-out-on-read: query activities from subscribed sources
        const activities = await scopedDb(request).getUserFeedUsingSubscriptions(query);
        feedItems = activities.map(activity => ({ activity }));
        [total, unreadCount] = await Promise.all([
          scopedDb(request).countActivities({ actorId: undefined }),
          Promise.resolve(0), // No unread count for fan-out-on-read
        ]);
      }

      return {
        data: feedItems,
        total,
        unreadCount,
        limit: query.limit,
        offset: query.offset,
        hasMore: (query.offset ?? 0) + feedItems.length < total,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get user feed', { error: message, userId });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/feed/:userId/unread', async (request, reply) => {
    const { userId } = request.params as { userId: string };

    try {
      const unreadCount = await scopedDb(request).getUnreadCount(userId);
      return { userId, unreadCount };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get unread count', { error: message, userId });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/v1/feed/:userId/read', async (request, reply) => {
    const { userId } = request.params as { userId: string };
    const { activityIds } = request.body as { activityIds?: string[] };

    try {
      const updated = await scopedDb(request).markFeedItemsAsRead(userId, activityIds);
      return { success: true, updated };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to mark items as read', { error: message, userId });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/v1/feed/:userId/hide', async (request, reply) => {
    const { userId } = request.params as { userId: string };
    const { activityId } = request.body as { activityId: string };

    try {
      await scopedDb(request).hideFeedItem(userId, activityId);
      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to hide feed item', { error: message, userId, activityId });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/feed/:userId/stats', async (request, reply) => {
    const { userId } = request.params as { userId: string };

    try {
      const stats = await scopedDb(request).getUserFeedStats(userId);
      return stats;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get user feed stats', { error: message, userId });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Entity Feed Endpoints
  // =========================================================================

  app.get('/v1/entity/:type/:id/feed', async (request, reply) => {
    const { type, id } = request.params as { type: string; id: string };
    const queryParams = request.query as Record<string, string>;

    const query: EntityFeedQuery = {
      entityType: type,
      entityId: id,
      limit: parseInt(queryParams.limit ?? '100', 10),
      offset: parseInt(queryParams.offset ?? '0', 10),
    };

    try {
      const activities = await scopedDb(request).getEntityFeed(query);
      const total = await scopedDb(request).countActivities({
        objectType: type,
        objectId: id,
      });

      return {
        data: activities,
        total,
        limit: query.limit,
        offset: query.offset,
        hasMore: (query.offset ?? 0) + activities.length < total,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get entity feed', { error: message, type, id });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Subscription Endpoints
  // =========================================================================

  app.post('/v1/subscriptions', async (request, reply) => {
    const input = request.body as CreateSubscriptionInput;

    try {
      const subscription = await scopedDb(request).createSubscription(input);
      return subscription;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create subscription', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/subscriptions/:userId', async (request, reply) => {
    const { userId } = request.params as { userId: string };

    try {
      const subscriptions = await scopedDb(request).listUserSubscriptions(userId);
      return {
        data: subscriptions,
        total: subscriptions.length,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list subscriptions', { error: message, userId });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/subscriptions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };

    try {
      await scopedDb(request).deleteSubscription(id);
      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete subscription', { error: message, id });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Fan-out Endpoint
  // =========================================================================

  app.post('/v1/fanout', async (request, reply) => {
    const { activityId, forceRefresh } = request.body as {
      activityId: string;
      forceRefresh?: boolean;
    };

    try {
      const startTime = Date.now();

      // Get activity
      const activity = await scopedDb(request).getActivity(activityId);
      if (!activity) {
        return reply.status(404).send({ error: 'Activity not found' });
      }

      // Get subscribers
      const subscribers = await scopedDb(request).getSubscribersForActor(activity.actor_id);

      // Create feed items for all subscribers
      let feedItemsCreated = 0;
      for (const userId of subscribers) {
        if (forceRefresh) {
          await scopedDb(request).createFeedItem(userId, activityId);
          feedItemsCreated++;
        } else {
          try {
            await scopedDb(request).createFeedItem(userId, activityId);
            feedItemsCreated++;
          } catch {
            // Item already exists, skip
          }
        }
      }

      const duration = Date.now() - startTime;

      return {
        activityId,
        subscribersCount: subscribers.length,
        feedItemsCreated,
        duration,
        success: true,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to fan-out activity', { error: message, activityId });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Statistics Endpoint
  // =========================================================================

  app.get('/v1/stats', async (request, reply) => {
    try {
      const stats = await scopedDb(request).getStats();
      return stats;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Start Server
  // =========================================================================

  const startServer = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.info(`Activity Feed server listening on ${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Strategy: ${fullConfig.strategy}`);
      logger.info(`Max feed size: ${fullConfig.maxFeedSize}`);
      logger.info(`Aggregation window: ${fullConfig.aggregationWindowMinutes} minutes`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
  };

  return Object.assign(app, { start: startServer }) as FeedServer;
}
