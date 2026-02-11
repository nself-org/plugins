/**
 * CDN Plugin Server
 * HTTP server for CDN management, cache purging, signed URLs, and analytics
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import crypto from 'node:crypto';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { CdnDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateZoneRequest,
  PurgeRequest,
  PurgeAllRequest,
  SignUrlRequest,
  BatchSignRequest,
} from './types.js';

const logger = createLogger('cdn:server');

function generateSignedUrl(url: string, signingKey: string, ttl: number, ip?: string): string {
  const expires = Math.floor(Date.now() / 1000) + ttl;
  const dataToSign = `${url}${expires}${ip ?? ''}`;
  const signature = crypto.createHmac('sha256', signingKey).update(dataToSign).digest('hex');

  const separator = url.includes('?') ? '&' : '?';
  let signedUrl = `${url}${separator}expires=${expires}&sig=${signature}`;
  if (ip) {
    signedUrl += `&ip=${ip}`;
  }

  return signedUrl;
}

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new CdnDatabase(undefined, 'primary');

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
    fullConfig.security.rateLimitMax ?? 500,
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

  function scopedDb(request: unknown): CdnDatabase {
    return (request as Record<string, unknown>).scopedDb as CdnDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'cdn', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'cdn', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'cdn',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return {
      alive: true,
      plugin: 'cdn',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        zones: stats.total_zones,
        pendingPurges: stats.pending_purges,
        activeSignedUrls: stats.active_signed_urls,
        totalRequestsTracked: stats.total_requests_tracked,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Zone Endpoints
  // =========================================================================

  app.get('/api/zones', async (request) => {
    const { provider } = request.query as { provider?: string };
    const zones = await scopedDb(request).listZones(provider);
    return { data: zones, total: zones.length };
  });

  app.post('/api/zones', async (request, reply) => {
    try {
      const body = request.body as CreateZoneRequest;

      if (!body.provider || !body.zone_id || !body.name || !body.domain) {
        return reply.status(400).send({ error: 'provider, zone_id, name, and domain are required' });
      }

      const zone = await scopedDb(request).createZone(body);
      return reply.status(201).send(zone);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create zone failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/zones/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const zone = await scopedDb(request).getZone(id);
    if (!zone) {
      return reply.status(404).send({ error: 'Zone not found' });
    }
    return zone;
  });

  // =========================================================================
  // Purge Endpoints
  // =========================================================================

  app.post('/api/purge', async (request, reply) => {
    try {
      const body = request.body as PurgeRequest;

      if (!body.zone_id) {
        return reply.status(400).send({ error: 'zone_id is required' });
      }

      // Validate zone exists
      const zone = await scopedDb(request).getZone(body.zone_id);
      if (!zone) {
        return reply.status(404).send({ error: 'Zone not found' });
      }

      let purgeType = body.purge_type;
      if (!purgeType) {
        if (body.urls && body.urls.length > 0) purgeType = 'urls';
        else if (body.tags && body.tags.length > 0) purgeType = 'tags';
        else if (body.prefixes && body.prefixes.length > 0) purgeType = 'prefixes';
        else return reply.status(400).send({ error: 'Specify urls, tags, or prefixes to purge' });
      }

      // Validate batch size
      if (purgeType === 'urls' && body.urls && body.urls.length > fullConfig.purgeBatchSize) {
        return reply.status(400).send({
          error: `URL count exceeds maximum batch size of ${fullConfig.purgeBatchSize}`,
        });
      }

      const purgeRecord = await scopedDb(request).createPurgeRequest(body.zone_id, purgeType, {
        urls: body.urls,
        tags: body.tags,
        prefixes: body.prefixes,
        requested_by: body.requested_by,
      });

      // Mark as completed (actual CDN provider integration would be here)
      await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'completed');

      return reply.status(201).send({
        purge_request_id: purgeRecord.id,
        status: 'completed',
        purge_type: purgeType,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Purge failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/purge/all', async (request, reply) => {
    try {
      const body = request.body as PurgeAllRequest;

      if (!body.zone_id) {
        return reply.status(400).send({ error: 'zone_id is required' });
      }

      if (!body.confirm) {
        return reply.status(400).send({ error: 'Set confirm: true to purge entire zone cache' });
      }

      const zone = await scopedDb(request).getZone(body.zone_id);
      if (!zone) {
        return reply.status(404).send({ error: 'Zone not found' });
      }

      const purgeRecord = await scopedDb(request).createPurgeRequest(body.zone_id, 'all', {
        requested_by: body.requested_by,
      });

      await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'completed');

      return reply.status(201).send({
        purge_request_id: purgeRecord.id,
        status: 'completed',
        purge_type: 'all',
        zone_name: zone.name,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Purge all failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/purge/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const purge = await scopedDb(request).getPurgeRequest(id);
    if (!purge) {
      return reply.status(404).send({ error: 'Purge request not found' });
    }
    return purge;
  });

  // =========================================================================
  // Signed URL Endpoints
  // =========================================================================

  app.post('/api/sign', async (request, reply) => {
    try {
      const body = request.body as SignUrlRequest;

      if (!body.zone_id || !body.url) {
        return reply.status(400).send({ error: 'zone_id and url are required' });
      }

      if (!fullConfig.signingKey) {
        return reply.status(500).send({ error: 'CDN_SIGNING_KEY is not configured' });
      }

      const zone = await scopedDb(request).getZone(body.zone_id);
      if (!zone) {
        return reply.status(404).send({ error: 'Zone not found' });
      }

      const ttl = body.ttl ?? fullConfig.signedUrlTtl;
      const signedUrl = generateSignedUrl(body.url, fullConfig.signingKey, ttl, body.ip_restriction);
      const expiresAt = new Date(Date.now() + ttl * 1000);

      const record = await scopedDb(request).createSignedUrl(body.zone_id, body.url, signedUrl, expiresAt, {
        ip_restriction: body.ip_restriction,
        max_access: body.max_access,
      });

      return {
        id: record.id,
        signed_url: signedUrl,
        original_url: body.url,
        expires_at: expiresAt.toISOString(),
        ttl,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sign URL failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/sign/batch', async (request, reply) => {
    try {
      const body = request.body as BatchSignRequest;

      if (!body.zone_id || !body.urls || !Array.isArray(body.urls)) {
        return reply.status(400).send({ error: 'zone_id and urls array are required' });
      }

      if (!fullConfig.signingKey) {
        return reply.status(500).send({ error: 'CDN_SIGNING_KEY is not configured' });
      }

      const zone = await scopedDb(request).getZone(body.zone_id);
      if (!zone) {
        return reply.status(404).send({ error: 'Zone not found' });
      }

      const ttl = body.ttl ?? fullConfig.signedUrlTtl;
      const results: { original_url: string; signed_url: string; expires_at: string }[] = [];

      for (const url of body.urls) {
        const signedUrl = generateSignedUrl(url, fullConfig.signingKey, ttl);
        const expiresAt = new Date(Date.now() + ttl * 1000);

        await scopedDb(request).createSignedUrl(body.zone_id, url, signedUrl, expiresAt);

        results.push({
          original_url: url,
          signed_url: signedUrl,
          expires_at: expiresAt.toISOString(),
        });
      }

      return { data: results, total: results.length, ttl };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch sign failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Analytics Endpoints
  // =========================================================================

  app.get('/api/analytics', async (request) => {
    const query = request.query as {
      zone_id?: string;
      from?: string;
      to?: string;
      limit?: string;
      offset?: string;
    };

    const result = await scopedDb(request).getAnalytics({
      zone_id: query.zone_id,
      from: query.from,
      to: query.to,
      limit: query.limit ? parseInt(query.limit, 10) : undefined,
      offset: query.offset ? parseInt(query.offset, 10) : undefined,
    });

    return result;
  });

  app.get('/api/analytics/summary', async (request) => {
    const { zone_id } = request.query as { zone_id?: string };
    const summary = await scopedDb(request).getAnalyticsSummary(zone_id);
    return { data: summary };
  });

  app.post('/sync', async (request, reply) => {
    try {

      logger.info('Starting analytics sync...');

      // Placeholder for actual CDN provider analytics sync
      const zones = await scopedDb(request).listZones();

      return {
        success: true,
        zones_synced: zones.length,
        message: 'Analytics sync completed (provider integration pending)',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analytics sync failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return stats;
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
      logger.success(`CDN plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Provider: ${fullConfig.provider}`);
      logger.info(`Signed URL TTL: ${fullConfig.signedUrlTtl}s`);
      logger.info(`Purge batch size: ${fullConfig.purgeBatchSize}`);
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
