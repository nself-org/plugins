/**
 * DLNA Plugin Server
 * Fastify HTTP server for UPnP services, REST API, and media streaming
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook } from '@nself/plugin-utils';
import { DlnaDatabase } from './database.js';
import { loadConfig, getLocalIpAddress, type Config } from './config.js';
import { SSDPServer } from './ssdp.js';
import {
  generateDeviceDescription,
  generateContentDirectorySCPD,
  generateConnectionManagerSCPD,
} from './upnp.js';
import {
  parseSOAPAction,
  handleContentDirectoryAction,
  handleConnectionManagerAction,
  getSystemUpdateId,
} from './content-directory.js';
import { MediaScanner } from './media-scanner.js';
import { handleMediaStream, handleThumbnailStream } from './streaming.js';

const logger = createLogger('dlna:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);
  const localIp = getLocalIpAddress();
  const baseUrl = `http://${localIp}:${fullConfig.dlnaPort}`;

  // Initialize database
  const db = new DlnaDatabase(fullConfig.sourceAccountId);
  await db.connect();
  await db.initializeSchema();

  // Initialize SSDP server
  const ssdp = new SSDPServer({
    address: '239.255.255.250',
    port: fullConfig.ssdpPort,
    uuid: fullConfig.uuid,
    friendlyName: fullConfig.friendlyName,
    httpPort: fullConfig.dlnaPort,
    httpHost: localIp,
    advertiseInterval: fullConfig.advertiseInterval,
  });

  // Track discovered renderers in the database
  ssdp.setRendererCallback(async (renderer) => {
    try {
      await db.upsertRenderer({
        source_account_id: fullConfig.sourceAccountId,
        usn: renderer.usn,
        friendly_name: null,
        location: renderer.location,
        ip_address: renderer.ipAddress,
        device_type: renderer.deviceType,
        manufacturer: null,
        model_name: null,
        last_seen_at: renderer.lastSeen,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Failed to persist renderer', { error: message });
    }
  });

  // Initialize media scanner
  const scanner = new MediaScanner(db, fullConfig.sourceAccountId);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 1 * 1024 * 1024,
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

  // ---------------------------------------------------------------------------
  // UPnP Device Description Endpoints
  // ---------------------------------------------------------------------------

  app.get('/description.xml', async (_request, reply) => {
    const xml = generateDeviceDescription(
      fullConfig.uuid,
      fullConfig.friendlyName,
      fullConfig.dlnaPort,
      localIp
    );
    reply.header('Content-Type', 'text/xml; charset="utf-8"');
    return xml;
  });

  app.get('/ContentDirectory.xml', async (_request, reply) => {
    reply.header('Content-Type', 'text/xml; charset="utf-8"');
    return generateContentDirectorySCPD();
  });

  app.get('/ConnectionManager.xml', async (_request, reply) => {
    reply.header('Content-Type', 'text/xml; charset="utf-8"');
    return generateConnectionManagerSCPD();
  });

  // ---------------------------------------------------------------------------
  // UPnP SOAP Control Endpoints
  // ---------------------------------------------------------------------------

  // Raw body parser for SOAP requests
  app.addContentTypeParser('text/xml', { parseAs: 'string' }, (_req, body, done) => {
    done(null, body as string);
  });

  // Also handle text/xml with charset
  app.addContentTypeParser('text/xml; charset="utf-8"', { parseAs: 'string' }, (_req, body, done) => {
    done(null, body as string);
  });

  app.post('/control/ContentDirectory', async (request, reply) => {
    const soapActionHeader = request.headers.soapaction as string ?? '';
    const xmlBody = request.body as string;

    const action = parseSOAPAction(xmlBody, soapActionHeader);
    logger.debug('ContentDirectory action', { action: action.actionName });

    const response = await handleContentDirectoryAction(action, db, baseUrl);

    reply.header('Content-Type', 'text/xml; charset="utf-8"');
    reply.header('EXT', '');
    return response;
  });

  app.post('/control/ConnectionManager', async (request, reply) => {
    const soapActionHeader = request.headers.soapaction as string ?? '';
    const xmlBody = request.body as string;

    const action = parseSOAPAction(xmlBody, soapActionHeader);
    logger.debug('ConnectionManager action', { action: action.actionName });

    const response = await handleConnectionManagerAction(action);

    reply.header('Content-Type', 'text/xml; charset="utf-8"');
    reply.header('EXT', '');
    return response;
  });

  // ---------------------------------------------------------------------------
  // UPnP Event Subscription Endpoints (stub - minimal compliance)
  // ---------------------------------------------------------------------------

  app.route({
    method: 'SUBSCRIBE' as never,
    url: '/event/ContentDirectory',
    handler: async (_request, reply) => {
      reply.status(200);
      reply.header('SID', `uuid:${fullConfig.uuid}-event-cd`);
      reply.header('TIMEOUT', 'Second-1800');
      return '';
    },
  });

  app.route({
    method: 'SUBSCRIBE' as never,
    url: '/event/ConnectionManager',
    handler: async (_request, reply) => {
      reply.status(200);
      reply.header('SID', `uuid:${fullConfig.uuid}-event-cm`);
      reply.header('TIMEOUT', 'Second-1800');
      return '';
    },
  });

  // ---------------------------------------------------------------------------
  // Media Streaming Endpoints
  // ---------------------------------------------------------------------------

  app.get<{ Params: { id: string } }>('/media/:id', async (request, reply) => {
    await handleMediaStream(request, reply, db);
  });

  app.get<{ Params: { id: string } }>('/thumbnails/:id', async (request, reply) => {
    await handleThumbnailStream(request, reply, db);
  });

  // ---------------------------------------------------------------------------
  // REST API Endpoints (for nself management)
  // ---------------------------------------------------------------------------

  // Health check
  app.get('/health', async () => {
    return { status: 'ok', plugin: 'dlna', timestamp: new Date().toISOString() };
  });

  // Readiness check
  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'dlna', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'dlna',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // Server status
  app.get('/v1/status', async () => {
    const stats = await db.getStats();
    const activeRenderers = await db.getActiveRenderers();

    return {
      plugin: 'dlna',
      version: '1.0.0',
      status: 'running',
      friendlyName: fullConfig.friendlyName,
      uuid: fullConfig.uuid,
      httpPort: fullConfig.dlnaPort,
      ssdpPort: fullConfig.ssdpPort,
      baseUrl,
      systemUpdateId: getSystemUpdateId(),
      stats,
      activeRenderers: activeRenderers.length,
      mediaPaths: fullConfig.mediaPaths,
      uptime: process.uptime(),
      timestamp: new Date().toISOString(),
    };
  });

  // List discovered renderers
  app.get('/v1/renderers', async (request) => {
    const { limit = 100, offset = 0, active } = request.query as {
      limit?: number;
      offset?: number;
      active?: string;
    };

    if (active === 'true') {
      const renderers = await db.getActiveRenderers();
      return { data: renderers, total: renderers.length };
    }

    const renderers = await db.listRenderers(limit, offset);
    const total = await db.countRenderers();
    return { data: renderers, total, limit, offset };
  });

  // List media items
  app.get('/v1/media', async (request) => {
    const { limit = 100, offset = 0, parent } = request.query as {
      limit?: number;
      offset?: number;
      parent?: string;
    };

    if (parent) {
      const parentId = parent === '0' ? null : parent;
      const { items, totalCount } = await db.listChildren(parentId, offset, limit);
      return { data: items, total: totalCount, limit, offset };
    }

    const items = await db.listMediaItems(limit, offset);
    const total = await db.countMediaItems();
    return { data: items, total, limit, offset };
  });

  // Get single media item
  app.get<{ Params: { id: string } }>('/v1/media/:id', async (request, reply) => {
    const { id } = request.params;
    const item = await db.getMediaItem(id);
    if (!item) {
      return reply.status(404).send({ error: 'Media item not found' });
    }
    return item;
  });

  // Scan media directories
  app.post('/v1/scan', async (_request, reply) => {
    try {
      const result = await scanner.scan(fullConfig.mediaPaths);
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Media scan failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Stats endpoint
  app.get('/v1/stats', async () => {
    return await db.getStats();
  });

  // ---------------------------------------------------------------------------
  // Graceful shutdown
  // ---------------------------------------------------------------------------

  const shutdown = async () => {
    logger.info('Shutting down...');
    await ssdp.stop();
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    ssdp,
    scanner,
    start: async () => {
      // Start HTTP server
      await app.listen({ port: fullConfig.dlnaPort, host: fullConfig.host });
      logger.success(`DLNA HTTP server running on http://${fullConfig.host}:${fullConfig.dlnaPort}`);
      logger.info(`Device description: ${baseUrl}/description.xml`);

      // Start SSDP discovery
      try {
        await ssdp.start();
        logger.success(`SSDP server running on ${fullConfig.ssdpPort}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn(`SSDP failed to start (may need elevated permissions for port ${fullConfig.ssdpPort})`, { error: message });
        logger.info('HTTP server is still running; DLNA clients can connect directly via description.xml URL');
      }

      // Run initial media scan
      logger.info('Running initial media scan...');
      try {
        const result = await scanner.scan(fullConfig.mediaPaths);
        logger.success(`Initial scan complete: ${result.totalFiles} files indexed`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Initial media scan failed', { error: message });
      }

      // Search for renderers on the network
      ssdp.searchForRenderers();

      logger.info(`Friendly name: ${fullConfig.friendlyName}`);
      logger.info(`UUID: ${fullConfig.uuid}`);
      logger.info(`Media paths: ${fullConfig.mediaPaths.join(', ')}`);
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
