/**
 * mDNS Plugin Server
 * HTTP server for mDNS service discovery API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { MdnsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  AdvertiseServiceRequest,
  UpdateServiceRequest,
  ListServicesQuery,
  DiscoverRequest,
  ListDiscoveryQuery,
} from './types.js';
import { discoverServices } from './discovery.js';

const logger = createLogger('mdns:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new MdnsDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
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

  function scopedDb(request: unknown): MdnsDatabase {
    return (request as Record<string, unknown>).scopedDb as MdnsDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'mdns', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'mdns', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'mdns',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'mdns',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalServices: stats.total_services,
        advertisedServices: stats.advertised_services,
        totalDiscovered: stats.total_discovered,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Service Endpoints (advertised services)
  // =========================================================================

  app.post<{ Body: AdvertiseServiceRequest }>('/api/services', async (request, reply) => {
    try {
      const service = await scopedDb(request).createService({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        service_name: request.body.service_name,
        service_type: request.body.service_type ?? fullConfig.defaultServiceType,
        port: request.body.port,
        host: request.body.host ?? 'localhost',
        domain: request.body.domain ?? fullConfig.domain,
        txt_records: request.body.txt_records ?? {},
        is_advertised: false,
        is_active: true,
        last_seen_at: new Date(),
        metadata: {},
      });

      return reply.status(201).send(service);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create service', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListServicesQuery }>('/api/services', async (request) => {
    const services = await scopedDb(request).listServices({
      serviceType: request.query.service_type,
      isAdvertised: request.query.is_advertised === 'true' ? true : request.query.is_advertised === 'false' ? false : undefined,
      isActive: request.query.is_active === 'true' ? true : request.query.is_active === 'false' ? false : undefined,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 200,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { services, count: services.length };
  });

  app.get<{ Params: { id: string } }>('/api/services/:id', async (request, reply) => {
    const service = await scopedDb(request).getService(request.params.id);
    if (!service) {
      return reply.status(404).send({ error: 'Service not found' });
    }
    return service;
  });

  app.put<{ Params: { id: string }; Body: UpdateServiceRequest }>('/api/services/:id', async (request, reply) => {
    const service = await scopedDb(request).updateService(
      request.params.id,
      request.body as Partial<Record<string, unknown>>
    );
    if (!service) {
      return reply.status(404).send({ error: 'Service not found' });
    }
    return service;
  });

  app.delete<{ Params: { id: string } }>('/api/services/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteService(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Service not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Advertise Endpoints
  // =========================================================================

  app.post<{ Params: { id: string } }>('/api/services/:id/advertise', async (request, reply) => {
    try {
      const service = await scopedDb(request).setAdvertised(request.params.id, true);
      if (!service) {
        return reply.status(404).send({ error: 'Service not found' });
      }

      logger.info('Service advertising started', {
        serviceId: service.id,
        serviceName: service.service_name,
        serviceType: service.service_type,
        port: service.port,
      });

      return { success: true, service };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start advertising', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Params: { id: string } }>('/api/services/:id/stop', async (request, reply) => {
    try {
      const service = await scopedDb(request).setAdvertised(request.params.id, false);
      if (!service) {
        return reply.status(404).send({ error: 'Service not found' });
      }

      logger.info('Service advertising stopped', {
        serviceId: service.id,
        serviceName: service.service_name,
      });

      return { success: true, service };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to stop advertising', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Discovery Endpoints
  // =========================================================================

  app.post<{ Body: DiscoverRequest }>('/api/discover', async (request, reply) => {
    try {
      const serviceType = request.body.service_type ?? fullConfig.defaultServiceType;
      const startTime = Date.now();

      logger.info('Discovery scan initiated', {
        serviceType,
        domain: request.body.domain ?? fullConfig.domain,
      });

      // Perform real mDNS discovery on the network
      const discovered = await discoverServices({
        serviceType,
        timeout: request.body.timeout ?? 5000,
        domain: request.body.domain ?? fullConfig.domain,
      });

      // Store discovered services in database
      for (const service of discovered) {
        try {
          await scopedDb(request).upsertDiscovery({
            service_type: service.service_type,
            service_name: service.service_name,
            host: service.host,
            port: service.port,
            addresses: service.addresses,
            txt_records: service.txt_records,
          });
        } catch (error) {
          logger.warn('Failed to store discovery', {
            service: service.service_name,
            error: error instanceof Error ? error.message : 'Unknown error',
          });
        }
      }

      const scanDuration = Date.now() - startTime;

      // Convert to database record format for response
      const services = await scopedDb(request).listDiscoveries({
        serviceType,
        isAvailable: true,
      });

      return {
        services: services.filter(s =>
          discovered.some(d => d.service_name === s.service_name && d.host === s.host)
        ),
        count: discovered.length,
        scan_duration_ms: scanDuration,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Discovery scan failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListDiscoveryQuery }>('/api/discovered', async (request) => {
    const discoveries = await scopedDb(request).listDiscoveries({
      serviceType: request.query.service_type,
      isAvailable: request.query.is_available === 'true' ? true : request.query.is_available === 'false' ? false : undefined,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 200,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { discoveries, count: discoveries.length };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'mdns',
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
        logger.info(`mDNS server listening on ${fullConfig.host}:${fullConfig.port}`);
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
