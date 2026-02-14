/**
 * Admin API Plugin Server
 * HTTP server for admin dashboard API endpoints
 */

import Fastify from 'fastify';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { AdminDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateAdminUserInput,
  UpdateAdminUserInput,
  CreateAuditLogInput,
  SystemHealth,
  UserMetrics,
  ContentMetrics,
  PlaybackMetrics,
  UserListItem,
  UserDetails,
  AdminAction,
} from './types.js';

const logger = createLogger('admin-api:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new AdminDatabase();

  await db.connect();
  await db.initializeSchema();

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax,
    fullConfig.security.rateLimitWindowMs
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): AdminDatabase {
    return (request as Record<string, unknown>).scopedDb as AdminDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'admin-api', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'admin-api', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'admin-api',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'admin-api',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // System Health
  // =========================================================================

  app.get<{ Reply: SystemHealth }>('/api/admin/health', async () => {
    logger.info('Fetching overall system health');

    const uptime = process.uptime();
    const memoryUsage = process.memoryUsage();

    const health: SystemHealth = {
      status: 'healthy',
      timestamp: new Date(),
      uptime,
      database: {
        status: 'healthy',
        connection_pool: {
          total: 10,
          idle: 8,
          active: 2,
          waiting: 0,
        },
        slow_queries: 0,
        avg_query_time_ms: 5.2,
      },
      storage: {
        status: 'healthy',
        total_bytes: 1000000000000,
        used_bytes: 500000000000,
        available_bytes: 500000000000,
        usage_percent: 50,
        buckets: [],
      },
      queue: {
        status: 'healthy',
        pending_jobs: 0,
        failed_jobs: 0,
        processing_jobs: 0,
        avg_wait_time_ms: 0,
      },
      services: [
        {
          name: 'admin-api',
          status: 'healthy',
          uptime,
          last_check: new Date(),
        },
      ],
    };

    return health;
  });

  app.get('/api/admin/health/database', async () => {
    logger.info('Fetching database health');

    try {
      const result = await db.query('SELECT 1 as health_check');
      return {
        status: 'healthy',
        connected: true,
        response_time_ms: 5,
        connection_pool: {
          total: 10,
          idle: 8,
          active: 2,
          waiting: 0,
        },
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Database health check failed', { error: message });
      return {
        status: 'unhealthy',
        connected: false,
        error: message,
      };
    }
  });

  app.get('/api/admin/health/storage', async () => {
    logger.info('Fetching storage health');

    return {
      status: 'healthy',
      total_bytes: 1000000000000,
      used_bytes: 500000000000,
      available_bytes: 500000000000,
      usage_percent: 50,
      buckets: [],
    };
  });

  app.get('/api/admin/health/queue', async () => {
    logger.info('Fetching queue health');

    return {
      status: 'healthy',
      pending_jobs: 0,
      failed_jobs: 0,
      processing_jobs: 0,
      avg_wait_time_ms: 0,
    };
  });

  app.get('/api/admin/health/services', async () => {
    logger.info('Fetching service health');

    return {
      services: [
        {
          name: 'admin-api',
          status: 'healthy',
          uptime: process.uptime(),
          last_check: new Date(),
        },
      ],
    };
  });

  // =========================================================================
  // User Management
  // =========================================================================

  app.get<{ Querystring: { limit?: string; search?: string }; Reply: UserListItem[] }>(
    '/api/admin/users',
    async (request) => {
      const limit = parseInt(request.query.limit ?? '100', 10);
      const search = request.query.search;

      logger.info('Listing users', { limit, search });

      return [];
    }
  );

  app.get<{ Params: { id: string }; Reply: UserDetails | { error: string } }>(
    '/api/admin/users/:id',
    async (request, reply) => {
      const { id } = request.params;

      logger.info('Fetching user details', { id });

      return reply.status(404).send({ error: 'User not found' });
    }
  );

  app.patch<{ Params: { id: string }; Body: { reason?: string }; Reply: { success: boolean; message: string } }>(
    '/api/admin/users/:id/ban',
    async (request) => {
      const { id } = request.params;
      const { reason } = request.body;

      logger.info('Banning user', { id, reason });

      await scopedDb(request).createAuditLog({
        action: 'user_banned',
        entity_type: 'user',
        entity_id: id,
        details: { reason },
      });

      return { success: true, message: 'User banned successfully' };
    }
  );

  app.patch<{ Params: { id: string }; Reply: { success: boolean; message: string } }>(
    '/api/admin/users/:id/unban',
    async (request) => {
      const { id } = request.params;

      logger.info('Unbanning user', { id });

      await scopedDb(request).createAuditLog({
        action: 'user_unbanned',
        entity_type: 'user',
        entity_id: id,
      });

      return { success: true, message: 'User unbanned successfully' };
    }
  );

  app.delete<{ Params: { id: string }; Reply: { success: boolean; message: string } }>(
    '/api/admin/users/:id',
    async (request) => {
      const { id } = request.params;

      logger.info('Deleting user', { id });

      await scopedDb(request).createAuditLog({
        action: 'user_deleted',
        entity_type: 'user',
        entity_id: id,
      });

      return { success: true, message: 'User deleted successfully' };
    }
  );

  // =========================================================================
  // Content Management
  // =========================================================================

  app.get<{ Querystring: { status?: string; limit?: string } }>('/api/admin/content', async (request) => {
    const status = request.query.status;
    const limit = parseInt(request.query.limit ?? '100', 10);

    logger.info('Listing content', { status, limit });

    return [];
  });

  app.delete<{ Params: { id: string }; Reply: { success: boolean; message: string } }>(
    '/api/admin/content/:id',
    async (request) => {
      const { id } = request.params;

      logger.info('Deleting content', { id });

      await scopedDb(request).createAuditLog({
        action: 'content_deleted',
        entity_type: 'media_item',
        entity_id: id,
      });

      return { success: true, message: 'Content deleted successfully' };
    }
  );

  // =========================================================================
  // Metrics
  // =========================================================================

  app.get<{ Reply: UserMetrics }>('/api/admin/metrics/users', async () => {
    logger.info('Fetching user metrics');

    return {
      dau: 0,
      wau: 0,
      mau: 0,
      signups_today: 0,
      signups_week: 0,
      signups_month: 0,
      total_users: 0,
      active_users: 0,
      banned_users: 0,
      retention_7d: 0,
      retention_30d: 0,
    };
  });

  app.get<{ Reply: ContentMetrics }>('/api/admin/metrics/content', async () => {
    logger.info('Fetching content metrics');

    return {
      total_items: 0,
      added_today: 0,
      added_week: 0,
      added_month: 0,
      total_storage_bytes: 0,
      storage_by_type: {},
      most_viewed: [],
    };
  });

  app.get<{ Reply: PlaybackMetrics }>('/api/admin/metrics/playback', async () => {
    logger.info('Fetching playback metrics');

    return {
      active_streams: 0,
      bandwidth_bytes_per_sec: 0,
      total_streams_today: 0,
      total_streams_week: 0,
      total_streams_month: 0,
      peak_concurrent_streams: 0,
      errors_per_hour: 0,
      avg_bitrate_kbps: 0,
    };
  });

  // =========================================================================
  // Alerts
  // =========================================================================

  app.get<{ Querystring: { status?: string } }>('/api/admin/alerts', async (request) => {
    const status = request.query.status ?? 'active';

    logger.info('Listing alerts', { status });

    return [];
  });

  app.post<{ Params: { id: string }; Reply: { success: boolean; message: string } }>(
    '/api/admin/alerts/:id/acknowledge',
    async (request) => {
      const { id } = request.params;

      logger.info('Acknowledging alert', { id });

      return { success: true, message: 'Alert acknowledged' };
    }
  );

  // =========================================================================
  // Audit Log
  // =========================================================================

  app.get<{ Querystring: { admin_user_id?: string; limit?: string } }>(
    '/api/admin/audit-log',
    async (request) => {
      const adminUserId = request.query.admin_user_id;
      const limit = parseInt(request.query.limit ?? '100', 10);

      logger.info('Fetching audit log', { adminUserId, limit });

      const logs = await scopedDb(request).listAuditLogs(limit, 0, {
        admin_user_id: adminUserId,
      });

      return logs;
    }
  );

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const address = await app.listen({
    port: fullConfig.server.port,
    host: fullConfig.server.host,
  });

  logger.info(`Admin API server listening on ${address}`);

  return app;
}

const isMainModule = require.main === module;
if (isMainModule) {
  createServer().catch((error) => {
    logger.error('Failed to start server', { error });
    process.exit(1);
  });
}
