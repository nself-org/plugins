/**
 * DDNS Plugin Server
 * HTTP server for dynamic DNS updater API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { DdnsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateDdnsConfigRequest,
  UpdateDdnsConfigRequest,
  ForceUpdateRequest,
  ListConfigsQuery,
  ListHistoryQuery,
  ProviderInfo,
} from './types.js';
import { createDDNSProvider, IPDetector } from './providers.js';

const logger = createLogger('ddns:server');

// =========================================================================
// Provider Definitions
// =========================================================================

const PROVIDERS: ProviderInfo[] = [
  {
    name: 'duckdns',
    display_name: 'DuckDNS',
    website: 'https://www.duckdns.org',
    requires_api_key: false,
    requires_zone_id: false,
    supports_ipv6: true,
  },
  {
    name: 'cloudflare',
    display_name: 'Cloudflare',
    website: 'https://www.cloudflare.com',
    requires_api_key: true,
    requires_zone_id: true,
    supports_ipv6: true,
  },
  {
    name: 'noip',
    display_name: 'No-IP',
    website: 'https://www.noip.com',
    requires_api_key: false,
    requires_zone_id: false,
    supports_ipv6: false,
  },
  {
    name: 'dynu',
    display_name: 'Dynu',
    website: 'https://www.dynu.com',
    requires_api_key: false,
    requires_zone_id: false,
    supports_ipv6: true,
  },
];

// =========================================================================
// External IP Detection
// =========================================================================

async function getExternalIp(): Promise<string | null> {
  const services = [
    'https://api.ipify.org?format=json',
    'https://httpbin.org/ip',
    'https://api.my-ip.io/v2/ip.json',
  ];

  for (const url of services) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(5000) });
      if (!response.ok) continue;

      const data = await response.json() as Record<string, string>;
      const ip = data.ip ?? data.origin;
      if (ip) return ip.trim();
    } catch {
      continue;
    }
  }

  return null;
}

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new DdnsDatabase();
  await db.connect();
  await db.initializeSchema();

  // Initialize DNS provider (Cloudflare or Route53)
  const provider = fullConfig.provider || 'cloudflare';
  let ddnsProvider: ReturnType<typeof createDDNSProvider> | null = null;
  try {
    ddnsProvider = createDDNSProvider(provider, process.env as Record<string, string>);
    logger.info('DNS provider initialized', { provider });
  } catch (error) {
    logger.warn('DNS provider not configured', {
      provider,
      error: error instanceof Error ? error.message : 'Unknown error',
    });
  }

  // Initialize IP detector
  const ipDetector = new IPDetector();

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

  function scopedDb(request: unknown): DdnsDatabase {
    return (request as Record<string, unknown>).scopedDb as DdnsDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'ddns', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'ddns', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'ddns',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'ddns',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalConfigs: stats.total_configs,
        enabledConfigs: stats.enabled_configs,
        totalUpdates: stats.total_updates,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // IP Detection Endpoint
  // =========================================================================

  app.get('/api/ip', async (_request, reply) => {
    try {
      const ipv4 = await ipDetector.getIPv4();
      const ipv6 = await ipDetector.getIPv6();

      return {
        ipv4,
        ipv6,
        timestamp: new Date().toISOString(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('IP detection failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Config Endpoints
  // =========================================================================

  app.post<{ Body: CreateDdnsConfigRequest }>('/api/configs', async (request, reply) => {
    try {
      const providerName = request.body.provider.toLowerCase();
      const provider = PROVIDERS.find(p => p.name === providerName);
      if (!provider) {
        return reply.status(400).send({
          error: `Unknown provider: ${request.body.provider}. Available: ${PROVIDERS.map(p => p.name).join(', ')}`,
        });
      }

      if (provider.requires_api_key && !request.body.api_key) {
        return reply.status(400).send({ error: `Provider "${providerName}" requires api_key` });
      }

      if (provider.requires_zone_id && !request.body.zone_id) {
        return reply.status(400).send({ error: `Provider "${providerName}" requires zone_id` });
      }

      const ddnsConfig = await scopedDb(request).createConfig({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        provider: providerName,
        domain: request.body.domain,
        hostname: request.body.hostname ?? '@',
        token: request.body.token,
        api_key: request.body.api_key ?? null,
        zone_id: request.body.zone_id ?? null,
        record_type: request.body.record_type ?? 'A',
        current_ip: null,
        last_check_at: null,
        last_update_at: null,
        check_interval: request.body.check_interval ?? fullConfig.checkInterval,
        is_enabled: true,
        metadata: {},
      });

      return reply.status(201).send(ddnsConfig);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create DDNS config', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListConfigsQuery }>('/api/configs', async (request) => {
    const configs = await scopedDb(request).listConfigs({
      provider: request.query.provider,
      isEnabled: request.query.is_enabled === 'true' ? true : request.query.is_enabled === 'false' ? false : undefined,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 200,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { configs, count: configs.length };
  });

  app.get<{ Params: { id: string } }>('/api/configs/:id', async (request, reply) => {
    const ddnsConfig = await scopedDb(request).getConfig(request.params.id);
    if (!ddnsConfig) {
      return reply.status(404).send({ error: 'Config not found' });
    }
    return ddnsConfig;
  });

  app.put<{ Params: { id: string }; Body: UpdateDdnsConfigRequest }>('/api/configs/:id', async (request, reply) => {
    const ddnsConfig = await scopedDb(request).updateConfig(
      request.params.id,
      request.body as Partial<Record<string, unknown>>
    );
    if (!ddnsConfig) {
      return reply.status(404).send({ error: 'Config not found' });
    }
    return ddnsConfig;
  });

  app.delete<{ Params: { id: string } }>('/api/configs/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteConfig(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Config not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/api/status', async (request, reply) => {
    try {
      const configs = await scopedDb(request).listConfigs({ isEnabled: true });
      const externalIp = await getExternalIp();

      return {
        configs: configs.map(c => ({
          id: c.id,
          provider: c.provider,
          domain: c.domain,
          current_ip: c.current_ip,
          last_check_at: c.last_check_at,
          last_update_at: c.last_update_at,
          is_enabled: c.is_enabled,
        })),
        external_ip: externalIp,
        timestamp: new Date().toISOString(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Update Endpoint
  // =========================================================================

  app.post<{ Body: ForceUpdateRequest }>('/api/update', async (request, reply) => {
    try {
      const externalIp = await getExternalIp();
      if (!externalIp) {
        return reply.status(502).send({ error: 'Failed to determine external IP address' });
      }

      const configs = request.body.config_id
        ? [await scopedDb(request).getConfig(request.body.config_id)].filter(Boolean)
        : await scopedDb(request).listConfigs({ isEnabled: true });

      if (configs.length === 0) {
        return reply.status(404).send({ error: 'No DDNS configs found' });
      }

      const results = [];

      for (const cfg of configs) {
        if (!cfg) continue;

        const startTime = Date.now();
        const oldIp = cfg.current_ip;

        if (oldIp === externalIp) {
          // IP hasn't changed, skip update
          await scopedDb(request).updateLastCheck(cfg.id);
          await scopedDb(request).createUpdateLog({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'skipped',
            response_code: null,
            response_message: 'IP unchanged',
            error: null,
            duration_ms: Date.now() - startTime,
          });

          results.push({
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'skipped' as const,
            message: 'IP unchanged',
          });
          continue;
        }

        // Real DNS provider API integration
        try {
          if (!ddnsProvider) {
            throw new Error(`DNS provider ${provider} not configured`);
          }

          logger.info('DNS update triggered', {
            provider: cfg.provider,
            domain: cfg.domain,
            oldIp,
            newIp: externalIp,
          });

          // Update DNS record via provider API
          // Note: cfg.token is used as record_id for Cloudflare/Route53
          // cfg.zone_id should be set when creating the config
          if (!cfg.zone_id || !cfg.token) {
            throw new Error('zone_id and record_id (token) are required for DNS provider');
          }

          await ddnsProvider.updateRecord({
            zone_id: cfg.zone_id,
            record_id: cfg.token,
            name: cfg.domain,
            type: 'A',
            content: externalIp,
            ttl: 300,
          });

          // Update database
          await scopedDb(request).updateCurrentIp(cfg.id, externalIp);
          await scopedDb(request).createUpdateLog({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'success',
            response_code: 200,
            response_message: 'DNS record updated',
            error: null,
            duration_ms: Date.now() - startTime,
          });

          // Verify DNS propagation
          const verified = await ddnsProvider.verifyRecord(cfg.domain, externalIp);

          results.push({
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'success' as const,
            message: `DNS record updated${verified ? ' and verified' : ' (verification pending)'}`,
          });
        } catch (updateError) {
          // Handle DNS update failure
          const errorMessage = updateError instanceof Error ? updateError.message : 'Unknown error';
          logger.error('DNS update failed', {
            provider: cfg.provider,
            domain: cfg.domain,
            error: errorMessage,
          });

          await scopedDb(request).createUpdateLog({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'failed',
            response_code: null,
            response_message: null,
            error: [errorMessage],
            duration_ms: Date.now() - startTime,
          });

          results.push({
            config_id: cfg.id,
            provider: cfg.provider,
            domain: cfg.domain,
            old_ip: oldIp,
            new_ip: externalIp,
            status: 'failed' as const,
            message: errorMessage,
          });
        }
      }

      return { results, count: results.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DNS update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Providers Endpoint
  // =========================================================================

  app.get('/api/providers', async () => {
    return { providers: PROVIDERS, count: PROVIDERS.length };
  });

  // =========================================================================
  // History Endpoint
  // =========================================================================

  app.get<{ Querystring: ListHistoryQuery }>('/api/history', async (request) => {
    const logs = await scopedDb(request).listUpdateLogs({
      configId: request.query.config_id,
      status: request.query.status,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 50,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { history: logs, count: logs.length };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'ddns',
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
        logger.info(`DDNS server listening on ${fullConfig.host}:${fullConfig.port}`);
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
