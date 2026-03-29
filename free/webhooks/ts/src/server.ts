/**
 * Webhooks Plugin Server
 * HTTP server for webhook management and delivery API
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { WebhooksDatabase } from './database.js';
import { WebhookDeliveryService } from './delivery.js';
import { loadConfig, type Config } from './config.js';
import type { CreateEndpointInput, UpdateEndpointInput, DispatchEventInput, RegisterEventTypeInput } from './types.js';

const logger = createLogger('webhooks:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new WebhooksDatabase();
  await db.connect();
  await db.initializeSchema();

  const deliveryService = new WebhookDeliveryService(db, fullConfig);

  // Start background delivery processing
  deliveryService.startProcessing();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: fullConfig.maxPayloadSize * 2, // Allow some overhead
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

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.decorateRequest('scopedDeliveryService', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
    (request as unknown as Record<string, unknown>).scopedDeliveryService = new WebhookDeliveryService(scopedDb, fullConfig);
  });

  /** Extract scoped database from request */
  function scopedDb(request: unknown): WebhooksDatabase {
    return (request as Record<string, unknown>).scopedDb as WebhooksDatabase;
  }

  /** Extract scoped delivery service from request */
  function scopedDeliveryService(request: unknown): WebhookDeliveryService {
    return (request as Record<string, unknown>).scopedDeliveryService as WebhookDeliveryService;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'webhooks', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'webhooks', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'webhooks',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'webhooks',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Webhook Endpoints Management
  // =========================================================================

  app.post<{ Body: CreateEndpointInput }>('/v1/endpoints', async (request, reply) => {
    try {
      const endpoint = await scopedDb(request).createEndpoint(request.body);
      return reply.status(201).send(endpoint);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create endpoint', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/endpoints', async (request) => {
    const { enabled } = request.query as { enabled?: string };
    const filters = enabled !== undefined ? { enabled: enabled === 'true' } : undefined;
    const endpoints = await scopedDb(request).listEndpoints(filters);
    return { endpoints };
  });

  app.get<{ Params: { id: string } }>('/v1/endpoints/:id', async (request, reply) => {
    const endpoint = await scopedDb(request).getEndpoint(request.params.id);
    if (!endpoint) {
      return reply.status(404).send({ error: 'Endpoint not found' });
    }
    return endpoint;
  });

  app.put<{ Params: { id: string }; Body: UpdateEndpointInput }>('/v1/endpoints/:id', async (request, reply) => {
    try {
      const endpoint = await scopedDb(request).updateEndpoint(request.params.id, request.body);
      if (!endpoint) {
        return reply.status(404).send({ error: 'Endpoint not found' });
      }
      return endpoint;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update endpoint', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/v1/endpoints/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteEndpoint(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Endpoint not found' });
    }
    return { deleted: true };
  });

  app.post<{ Params: { id: string } }>('/v1/endpoints/:id/test', async (request, reply) => {
    const result = await scopedDeliveryService(request).testEndpoint(request.params.id);
    if (!result.success) {
      return reply.status(400).send(result);
    }
    return result;
  });

  app.post<{ Params: { id: string } }>('/v1/endpoints/:id/rotate-secret', async (request, reply) => {
    const newSecret = await scopedDb(request).rotateEndpointSecret(request.params.id);
    if (!newSecret) {
      return reply.status(404).send({ error: 'Endpoint not found' });
    }
    return { secret: newSecret };
  });

  app.post<{ Params: { id: string } }>('/v1/endpoints/:id/enable', async (request, reply) => {
    const enabled = await scopedDb(request).enableEndpoint(request.params.id);
    if (!enabled) {
      return reply.status(404).send({ error: 'Endpoint not found' });
    }
    return { enabled: true };
  });

  // =========================================================================
  // Event Dispatch
  // =========================================================================

  app.post<{ Body: DispatchEventInput }>('/v1/dispatch', async (request, reply) => {
    try {
      const result = await scopedDeliveryService(request).dispatchEvent(request.body);
      return reply.status(202).send(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to dispatch event', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Deliveries
  // =========================================================================

  app.get('/v1/deliveries', async (request) => {
    const { endpoint_id, event_type, status, limit } = request.query as {
      endpoint_id?: string;
      event_type?: string;
      status?: string;
      limit?: string;
    };

    const deliveries = await scopedDb(request).listDeliveries({
      endpointId: endpoint_id,
      eventType: event_type,
      status: status as 'pending' | 'delivering' | 'delivered' | 'failed' | 'dead_letter' | undefined,
      limit: limit ? parseInt(limit, 10) : undefined,
    });

    return { deliveries };
  });

  app.get<{ Params: { id: string } }>('/v1/deliveries/:id', async (request, reply) => {
    const delivery = await scopedDb(request).getDelivery(request.params.id);
    if (!delivery) {
      return reply.status(404).send({ error: 'Delivery not found' });
    }
    return delivery;
  });

  app.post<{ Params: { id: string } }>('/v1/deliveries/:id/retry', async (request, reply) => {
    const retried = await scopedDb(request).retryDelivery(request.params.id);
    if (!retried) {
      return reply.status(404).send({ error: 'Delivery not found or cannot be retried' });
    }
    return { retried: true };
  });

  // =========================================================================
  // Event Types
  // =========================================================================

  app.get('/v1/event-types', async (request) => {
    const eventTypes = await scopedDb(request).listEventTypes();
    return { event_types: eventTypes };
  });

  app.post<{ Body: RegisterEventTypeInput }>('/v1/event-types', async (request, reply) => {
    try {
      const eventType = await scopedDb(request).registerEventType(request.body);
      return reply.status(201).send(eventType);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to register event type', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Dead Letter Queue
  // =========================================================================

  app.get('/v1/dead-letter', async (request) => {
    const { resolved } = request.query as { resolved?: string };
    const resolvedFilter = resolved !== undefined ? resolved === 'true' : undefined;
    const deadLetters = await scopedDb(request).listDeadLetters(resolvedFilter);
    return { dead_letters: deadLetters };
  });

  app.post<{ Params: { id: string } }>('/v1/dead-letter/:id/retry', async (request, reply) => {
    const currentDb = scopedDb(request);
    const deadLetter = await currentDb.query<{ delivery_id: string }>(
      'SELECT delivery_id FROM np_webhooks_dead_letters WHERE id = $1 AND source_account_id = $2',
      [request.params.id, currentDb.getCurrentSourceAccountId()]
    );

    if (deadLetter.rows.length === 0) {
      return reply.status(404).send({ error: 'Dead letter not found' });
    }

    const deliveryId = deadLetter.rows[0].delivery_id;
    const retried = await currentDb.retryDelivery(deliveryId);

    if (!retried) {
      return reply.status(400).send({ error: 'Failed to retry delivery' });
    }

    return { retried: true };
  });

  app.post<{ Params: { id: string } }>('/v1/dead-letter/:id/resolve', async (request, reply) => {
    const resolved = await scopedDb(request).resolveDeadLetter(request.params.id);
    if (!resolved) {
      return reply.status(404).send({ error: 'Dead letter not found' });
    }
    return { resolved: true };
  });

  // =========================================================================
  // Statistics
  // =========================================================================

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    const byEndpoint = await scopedDb(request).getDeliveryStatsByEndpoint();
    const byEventType = await scopedDb(request).getDeliveryStatsByEventType();

    return {
      stats,
      by_endpoint: byEndpoint,
      by_event_type: byEventType,
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    deliveryService.stopProcessing();
    await app.close();
    await db.disconnect();
    logger.info('Server shutdown complete');
  };

  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  const config = loadConfig();
  const app = await createServer();

  try {
    await app.listen({ port: config.port, host: config.host });
    logger.success(`Webhooks plugin server listening on ${config.host}:${config.port}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    logger.error('Failed to start server:', { error: message });
    process.exit(1);
  }
}
