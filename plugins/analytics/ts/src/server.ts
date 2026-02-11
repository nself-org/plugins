/**
 * Analytics Plugin Server
 * HTTP server for event tracking and analytics API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { AnalyticsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  TrackEventRequest,
  TrackEventBatchRequest,
  IncrementCounterRequest,
  CreateFunnelRequest,
  UpdateFunnelRequest,
  CreateQuotaRequest,
  UpdateQuotaRequest,
  QuotaCheckRequest,
  CounterPeriod,
} from './types.js';

const logger = createLogger('analytics:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new AnalyticsDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large batch requests
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

  /** Extract scoped AnalyticsDatabase from request */
  function scopedDb(request: unknown): AnalyticsDatabase {
    return (request as Record<string, unknown>).scopedDb as AnalyticsDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'analytics', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'analytics', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'analytics',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'analytics',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        events: stats.events,
        counters: stats.counters,
        funnels: stats.funnels,
        quotas: stats.quotas,
        lastEvent: stats.lastEventAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Event Tracking
  // =========================================================================

  app.post('/v1/events', async (request, reply) => {
    const event = request.body as TrackEventRequest;

    if (!event.event_name) {
      return reply.status(400).send({ error: 'event_name is required' });
    }

    try {
      const sdb = scopedDb(request);
      const eventId = await sdb.trackEvent(event);

      // Auto-increment matching counters
      await sdb.incrementCounter(event.event_name, event.user_id ?? 'total', 1);

      return { success: true, event_id: eventId };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Event tracking failed', { error: message });
      return reply.status(500).send({ error: 'Failed to track event' });
    }
  });

  app.post('/v1/events/batch', async (request, reply) => {
    const batch = request.body as TrackEventBatchRequest;

    if (!Array.isArray(batch.events)) {
      return reply.status(400).send({ error: 'events array is required' });
    }

    if (batch.events.length === 0) {
      return reply.status(400).send({ error: 'events array cannot be empty' });
    }

    if (batch.events.length > fullConfig.batchSize) {
      return reply.status(400).send({
        error: `Batch size exceeds maximum of ${fullConfig.batchSize}`,
      });
    }

    try {
      const sdb = scopedDb(request);
      const count = await sdb.trackEventBatch(batch.events);

      // Auto-increment counters for each event
      for (const event of batch.events) {
        await sdb.incrementCounter(event.event_name, event.user_id ?? 'total', 1);
      }

      return { success: true, tracked: count };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch tracking failed', { error: message });
      return reply.status(500).send({ error: 'Failed to track batch' });
    }
  });

  app.get('/v1/events', async (request) => {
    const sdb = scopedDb(request);
    const {
      limit = 100,
      offset = 0,
      event_name,
      user_id,
      session_id,
      start_date,
      end_date,
    } = request.query as {
      limit?: number;
      offset?: number;
      event_name?: string;
      user_id?: string;
      session_id?: string;
      start_date?: string;
      end_date?: string;
    };

    const filters = {
      event_name,
      user_id,
      session_id,
      start_date: start_date ? new Date(start_date) : undefined,
      end_date: end_date ? new Date(end_date) : undefined,
    };

    const events = await sdb.listEvents(Number(limit), Number(offset), filters);
    const total = await sdb.countEvents({ event_name, user_id });

    return { data: events, total, limit, offset };
  });

  // =========================================================================
  // Counter Operations
  // =========================================================================

  app.post('/v1/counters/increment', async (request, reply) => {
    const req = request.body as IncrementCounterRequest;

    if (!req.counter_name) {
      return reply.status(400).send({ error: 'counter_name is required' });
    }

    try {
      const sdb = scopedDb(request);
      await sdb.incrementCounter(
        req.counter_name,
        req.dimension ?? 'total',
        req.increment ?? 1,
        req.metadata
      );

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Counter increment failed', { error: message });
      return reply.status(500).send({ error: 'Failed to increment counter' });
    }
  });

  app.get('/v1/counters', async (request, reply) => {
    const sdb = scopedDb(request);
    const { counter_name, dimension, period } = request.query as {
      counter_name?: string;
      dimension?: string;
      period?: CounterPeriod;
    };

    if (!counter_name) {
      return reply.status(400).send({ error: 'counter_name query parameter is required' });
    }

    try {
      const value = await sdb.getCounterValue(
        counter_name,
        dimension ?? 'total',
        period ?? 'all_time'
      );

      if (!value) {
        return reply.status(404).send({ error: 'Counter not found' });
      }

      return value;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Counter query failed', { error: message });
      return reply.status(500).send({ error: 'Failed to query counter' });
    }
  });

  app.get('/v1/counters/:name/timeseries', async (request) => {
    const sdb = scopedDb(request);
    const { name } = request.params as { name: string };
    const { dimension, period, start_date, end_date } = request.query as {
      dimension?: string;
      period?: CounterPeriod;
      start_date?: string;
      end_date?: string;
    };

    const timeseries = await sdb.getCounterTimeseries(
      name,
      dimension ?? 'total',
      period ?? 'daily',
      start_date ? new Date(start_date) : undefined,
      end_date ? new Date(end_date) : undefined
    );

    return { counter_name: name, dimension: dimension ?? 'total', period: period ?? 'daily', data: timeseries };
  });

  app.post('/v1/counters/rollup', async (request) => {
    const sdb = scopedDb(request);
    const result = await sdb.rollupCounters();
    return { success: true, rolled_up: result };
  });

  // =========================================================================
  // Funnel Operations
  // =========================================================================

  app.post('/v1/funnels', async (request, reply) => {
    const funnel = request.body as CreateFunnelRequest;

    if (!funnel.name || !funnel.steps || funnel.steps.length === 0) {
      return reply.status(400).send({ error: 'name and steps are required' });
    }

    try {
      const sdb = scopedDb(request);
      const id = await sdb.createFunnel(funnel);
      return { success: true, funnel_id: id };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Funnel creation failed', { error: message });
      return reply.status(500).send({ error: 'Failed to create funnel' });
    }
  });

  app.get('/v1/funnels', async (request) => {
    const sdb = scopedDb(request);
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const funnels = await sdb.listFunnels(Number(limit), Number(offset));
    return { data: funnels, limit, offset };
  });

  app.get('/v1/funnels/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const funnel = await scopedDb(request).getFunnel(id);

    if (!funnel) {
      return reply.status(404).send({ error: 'Funnel not found' });
    }

    return funnel;
  });

  app.get('/v1/funnels/:id/analyze', async (request, reply) => {
    const { id } = request.params as { id: string };
    const sdb = scopedDb(request);

    try {
      const analysis = await sdb.analyzeFunnel(id);

      if (!analysis) {
        return reply.status(404).send({ error: 'Funnel not found' });
      }

      const funnel = await sdb.getFunnel(id);
      if (!funnel) {
        return reply.status(404).send({ error: 'Funnel not found' });
      }

      const steps = analysis.steps;
      const totalEntered = steps[0]?.users ?? 0;
      const totalCompleted = steps[steps.length - 1]?.users ?? 0;

      const stepsWithRates = steps.map((step, index) => {
        const prevUsers = index > 0 ? steps[index - 1].users : totalEntered;
        const conversionRate = prevUsers > 0 ? (step.users / prevUsers) * 100 : 0;
        const dropOffRate = prevUsers > 0 ? ((prevUsers - step.users) / prevUsers) * 100 : 0;

        return {
          step_number: step.step_number,
          step_name: step.step_name,
          event_name: funnel.steps[index]?.event_name ?? '',
          users: step.users,
          conversion_rate: Math.round(conversionRate * 100) / 100,
          drop_off_rate: Math.round(dropOffRate * 100) / 100,
        };
      });

      const overallConversionRate = totalEntered > 0 ? (totalCompleted / totalEntered) * 100 : 0;

      return {
        funnel_id: id,
        funnel_name: funnel.name,
        steps: stepsWithRates,
        total_entered: totalEntered,
        total_completed: totalCompleted,
        overall_conversion_rate: Math.round(overallConversionRate * 100) / 100,
        analysis_timestamp: new Date(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Funnel analysis failed', { error: message });
      return reply.status(500).send({ error: 'Failed to analyze funnel' });
    }
  });

  app.put('/v1/funnels/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updates = request.body as UpdateFunnelRequest;

    try {
      const updated = await scopedDb(request).updateFunnel(id, updates);

      if (!updated) {
        return reply.status(404).send({ error: 'Funnel not found' });
      }

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Funnel update failed', { error: message });
      return reply.status(500).send({ error: 'Failed to update funnel' });
    }
  });

  app.delete('/v1/funnels/:id', async (request, reply) => {
    const { id } = request.params as { id: string };

    try {
      const deleted = await scopedDb(request).deleteFunnel(id);

      if (!deleted) {
        return reply.status(404).send({ error: 'Funnel not found' });
      }

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Funnel deletion failed', { error: message });
      return reply.status(500).send({ error: 'Failed to delete funnel' });
    }
  });

  // =========================================================================
  // Quota Operations
  // =========================================================================

  app.post('/v1/quotas', async (request, reply) => {
    const quota = request.body as CreateQuotaRequest;

    if (!quota.name || !quota.counter_name || !quota.max_value || !quota.period) {
      return reply.status(400).send({
        error: 'name, counter_name, max_value, and period are required',
      });
    }

    try {
      const sdb = scopedDb(request);
      const id = await sdb.createQuota(quota);
      return { success: true, quota_id: id };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Quota creation failed', { error: message });
      return reply.status(500).send({ error: 'Failed to create quota' });
    }
  });

  app.get('/v1/quotas', async (request) => {
    const sdb = scopedDb(request);
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const quotas = await sdb.listQuotas(Number(limit), Number(offset));
    return { data: quotas, limit, offset };
  });

  app.put('/v1/quotas/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updates = request.body as UpdateQuotaRequest;

    try {
      const updated = await scopedDb(request).updateQuota(id, updates);

      if (!updated) {
        return reply.status(404).send({ error: 'Quota not found' });
      }

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Quota update failed', { error: message });
      return reply.status(500).send({ error: 'Failed to update quota' });
    }
  });

  app.delete('/v1/quotas/:id', async (request, reply) => {
    const { id } = request.params as { id: string };

    try {
      const deleted = await scopedDb(request).deleteQuota(id);

      if (!deleted) {
        return reply.status(404).send({ error: 'Quota not found' });
      }

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Quota deletion failed', { error: message });
      return reply.status(500).send({ error: 'Failed to delete quota' });
    }
  });

  app.post('/v1/quotas/check', async (request, reply) => {
    const req = request.body as QuotaCheckRequest;

    if (!req.counter_name) {
      return reply.status(400).send({ error: 'counter_name is required' });
    }

    try {
      const sdb = scopedDb(request);
      const result = await sdb.checkQuota(
        req.counter_name,
        req.scope_id ?? null,
        req.increment ?? 1
      );

      if (!result.quota) {
        return {
          allowed: true,
          quota_name: 'none',
          current_value: result.currentValue,
          max_value: 0,
          remaining: Infinity,
          action: 'none' as const,
        };
      }

      return {
        allowed: result.allowed,
        quota_name: result.quota.name,
        current_value: result.currentValue,
        max_value: Number(result.quota.max_value),
        remaining: Number(result.quota.max_value) - result.currentValue,
        action: result.quota.action_on_exceed,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Quota check failed', { error: message });
      return reply.status(500).send({ error: 'Failed to check quota' });
    }
  });

  app.get('/v1/violations', async (request) => {
    const sdb = scopedDb(request);
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const violations = await sdb.listViolations(Number(limit), Number(offset));
    return { data: violations, limit, offset };
  });

  // =========================================================================
  // Dashboard & Status
  // =========================================================================

  app.get('/v1/dashboard', async (request) => {
    const stats = await scopedDb(request).getDashboardStats();
    return stats;
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'analytics',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  // Start server
  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.success(`Analytics plugin listening on ${fullConfig.host}:${fullConfig.port}`);

  return {
    app,
    db,
    start: async () => {
      logger.info('Server already started');
    },
    stop: shutdown,
  };
}
