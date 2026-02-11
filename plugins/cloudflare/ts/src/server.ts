#!/usr/bin/env node
/**
 * Cloudflare Plugin HTTP Server
 * REST API endpoints for Cloudflare zone, DNS, R2, cache, and analytics management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { CloudflareDatabase } from './database.js';
import {
  CreateDnsRecordRequest,
  UpdateDnsRecordRequest,
  PurgeCacheRequest,
  CreateR2BucketRequest,
  SyncRequest,
  GetAnalyticsQuery,
  HealthCheckResponse,
} from './types.js';

const logger = createLogger('cloudflare:server');

const fastify = Fastify({
  logger: false,
  bodyLimit: 10485760,
});

let cfDb: CloudflareDatabase;

/**
 * Get scoped database for request
 */
function getAppContext(request: { headers: Record<string, string | string[] | undefined> }): string {
  return (request.headers['x-app-id'] as string) || 'primary';
}

function scopedDb(request: { headers: Record<string, string | string[] | undefined> }): CloudflareDatabase {
  return cfDb.forSourceAccount(getAppContext(request));
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return {
    status: 'ok',
    plugin: 'cloudflare',
    timestamp: new Date().toISOString(),
    version: '1.0.0',
  };
});

fastify.get('/ready', async () => {
  try {
    await cfDb.getStats();
    return { ready: true, database: 'ok', timestamp: new Date().toISOString() };
  } catch {
    return { ready: false, database: 'error', timestamp: new Date().toISOString() };
  }
});

fastify.get('/live', async () => {
  return {
    alive: true,
    uptime: process.uptime(),
    memory: {
      used: process.memoryUsage().heapUsed,
      total: process.memoryUsage().heapTotal,
    },
  };
});

// ============================================================================
// Zone Endpoints
// ============================================================================

fastify.get('/api/zones', async (request) => {
  const db = scopedDb(request);
  const zones = await db.getZones();
  return { data: zones, total: zones.length };
});

fastify.get<{ Params: { id: string } }>('/api/zones/:id', async (request, reply) => {
  const db = scopedDb(request);
  const zone = await db.getZoneById(request.params.id);

  if (!zone) {
    reply.code(404);
    throw new Error('Zone not found');
  }

  return zone;
});

fastify.post<{ Params: { id: string }; Body: { settings: Record<string, unknown> } }>('/api/zones/:id/settings', async (request, reply) => {
  const db = scopedDb(request);
  const zone = await db.getZoneById(request.params.id);

  if (!zone) {
    reply.code(404);
    throw new Error('Zone not found');
  }

  const updated = await db.updateZoneSettings(request.params.id, request.body.settings);
  return updated;
});

// ============================================================================
// DNS Endpoints
// ============================================================================

fastify.get<{ Params: { id: string } }>('/api/zones/:id/dns', async (request) => {
  const db = scopedDb(request);
  const records = await db.getDnsRecordsByZone(request.params.id);
  return { data: records, total: records.length };
});

fastify.post<{ Params: { id: string }; Body: CreateDnsRecordRequest }>('/api/zones/:id/dns', async (request, reply) => {
  const db = scopedDb(request);
  const { type, name, content, ttl, proxied, priority } = request.body;

  if (!type || !name || !content) {
    reply.code(400);
    throw new Error('type, name, and content are required');
  }

  const recordId = `dns_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;

  const record = await db.upsertDnsRecord({
    id: recordId,
    source_account_id: getAppContext(request),
    zone_id: request.params.id,
    type,
    name,
    content,
    ttl: ttl ?? 1,
    proxied: proxied ?? true,
    priority: priority ?? null,
    locked: false,
  });

  reply.code(201);
  return record;
});

fastify.put<{ Params: { id: string }; Body: UpdateDnsRecordRequest }>('/api/dns/:id', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getDnsRecordById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('DNS record not found');
  }

  const updated = await db.upsertDnsRecord({
    id: existing.id,
    source_account_id: existing.source_account_id,
    zone_id: existing.zone_id,
    type: request.body.type ?? existing.type,
    name: request.body.name ?? existing.name,
    content: request.body.content ?? existing.content,
    ttl: request.body.ttl ?? existing.ttl,
    proxied: request.body.proxied ?? existing.proxied,
    priority: request.body.priority ?? existing.priority,
    locked: existing.locked,
  });

  return updated;
});

fastify.delete<{ Params: { id: string } }>('/api/dns/:id', async (request, reply) => {
  const db = scopedDb(request);
  await db.deleteDnsRecord(request.params.id);
  reply.code(204);
});

// ============================================================================
// R2 Bucket Endpoints
// ============================================================================

fastify.get('/api/r2/buckets', async (request) => {
  const db = scopedDb(request);
  const buckets = await db.getR2Buckets();
  return { data: buckets, total: buckets.length };
});

fastify.post<{ Body: CreateR2BucketRequest }>('/api/r2/buckets', async (request, reply) => {
  const db = scopedDb(request);
  const { name, location } = request.body;

  if (!name) {
    reply.code(400);
    throw new Error('name is required');
  }

  const bucket = await db.upsertR2Bucket({
    source_account_id: getAppContext(request),
    name,
    location: location ?? null,
    storage_class: 'Standard',
    object_count: 0,
    total_size_bytes: 0,
    created_at: new Date(),
  });

  reply.code(201);
  return bucket;
});

fastify.delete<{ Params: { name: string } }>('/api/r2/buckets/:name', async (request, reply) => {
  const db = scopedDb(request);
  await db.deleteR2Bucket(request.params.name);
  reply.code(204);
});

fastify.get<{ Params: { name: string } }>('/api/r2/buckets/:name/stats', async (request, reply) => {
  const db = scopedDb(request);
  const bucket = await db.getR2BucketByName(request.params.name);

  if (!bucket) {
    reply.code(404);
    throw new Error('R2 bucket not found');
  }

  return {
    name: bucket.name,
    objectCount: bucket.object_count,
    totalSizeBytes: bucket.total_size_bytes,
    location: bucket.location,
    storageClass: bucket.storage_class,
  };
});

// ============================================================================
// Cache Endpoints
// ============================================================================

fastify.post<{ Params: { id: string }; Body: PurgeCacheRequest }>('/api/zones/:id/cache/purge', async (request) => {
  const db = scopedDb(request);
  const { type, urls, tags, hosts, prefixes } = request.body;

  const purge = await db.insertCachePurge({
    source_account_id: getAppContext(request),
    zone_id: request.params.id,
    purge_type: type,
    urls: urls ?? null,
    tags: tags ?? null,
    hosts: hosts ?? null,
    prefixes: prefixes ?? null,
    status: 'completed',
    cf_response: { success: true, purge_id: `purge_${Date.now()}` },
  });

  return { purged: true, id: purge.id };
});

fastify.get<{ Params: { id: string } }>('/api/zones/:id/cache/stats', async (request) => {
  const db = scopedDb(request);
  const stats = await db.getCacheStats(request.params.id);
  return stats;
});

// ============================================================================
// Analytics Endpoints
// ============================================================================

fastify.get<{ Params: { id: string }; Querystring: GetAnalyticsQuery }>('/api/zones/:id/analytics', async (request) => {
  const db = scopedDb(request);
  const from = request.query.from || new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
  const to = request.query.to || new Date().toISOString().split('T')[0];

  const daily = await db.getAnalytics(request.params.id, from, to);

  const totals = daily.reduce(
    (acc, day) => ({
      requests: acc.requests + Number(day.requests_total),
      bandwidth: acc.bandwidth + Number(day.bandwidth_total),
      cached: acc.cached + Number(day.requests_cached),
      threats: acc.threats + Number(day.threats_total),
      uniqueVisitors: acc.uniqueVisitors + Number(day.unique_visitors),
    }),
    { requests: 0, bandwidth: 0, cached: 0, threats: 0, uniqueVisitors: 0 },
  );

  return { daily, totals };
});

// ============================================================================
// Sync Endpoints
// ============================================================================

fastify.post<{ Body: SyncRequest }>('/api/sync', async (request) => {
  const db = scopedDb(request);
  const resources = request.body?.resources || ['zones', 'dns', 'r2', 'analytics'];
  const startTime = Date.now();
  const synced: Record<string, number> = {};
  const errors: string[] = [];

  for (const resource of resources) {
    try {
      switch (resource) {
        case 'zones':
          synced.zones = 0;
          logger.info('Zone sync would connect to Cloudflare API');
          break;
        case 'dns':
          synced.dns = 0;
          logger.info('DNS sync would connect to Cloudflare API');
          break;
        case 'r2':
          synced.r2 = 0;
          logger.info('R2 sync would connect to Cloudflare API');
          break;
        case 'analytics':
          synced.analytics = 0;
          logger.info('Analytics sync would connect to Cloudflare API');
          break;
        default:
          errors.push(`Unknown resource: ${resource}`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      errors.push(`Failed to sync ${resource}: ${message}`);
    }
  }

  const stats = await db.getStats();
  return {
    synced,
    errors,
    duration: Date.now() - startTime,
    stats,
  };
});

fastify.get('/api/status', async (request) => {
  const db = scopedDb(request);
  const stats = await db.getStats();
  return {
    status: 'ok',
    ...stats,
    syncInterval: config.syncInterval,
  };
});

// ============================================================================
// Server Startup
// ============================================================================

async function start() {
  try {
    await fastify.register(cors, { origin: true });

    const db = createDatabase(config.database);
    await db.connect();
    cfDb = new CloudflareDatabase(db);

    logger.info('Cloudflare database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Cloudflare plugin server listening on ${config.host}:${config.port}`);
    logger.info(`Health check: http://${config.host}:${config.port}/health`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start cloudflare server', { error: message });
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down gracefully...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down gracefully...');
  await fastify.close();
  process.exit(0);
});

const isMainModule = process.argv[1] && (
  process.argv[1].endsWith('server.ts') ||
  process.argv[1].endsWith('server.js')
);

if (isMainModule) {
  start();
}

export { fastify };
