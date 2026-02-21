/**
 * Search Plugin Server
 * HTTP server for search API endpoints and webhooks
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { SearchDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateIndexRequest,
  UpdateIndexRequest,
  IndexDocumentsRequest,
  SearchRequest,
  SuggestRequest,
  CreateSynonymRequest,
  ReindexOptions,
} from './types.js';

const logger = createLogger('search:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new SearchDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large document batches
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
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped SearchDatabase from request */
  function scopedDb(request: unknown): SearchDatabase {
    return (request as Record<string, unknown>).scopedDb as SearchDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'search', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'search', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'search',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const indexes = await scopedDb(request).listIndexes();
    return {
      alive: true,
      plugin: 'search',
      version: '1.0.0',
      engine: fullConfig.engine,
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        indexes: indexes.length,
        documents: indexes.reduce((sum, idx) => sum + idx.document_count, 0),
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const indexes = await scopedDb(request).listIndexes();
    const stats = fullConfig.analyticsEnabled
      ? await scopedDb(request).getSearchStats(30)
      : undefined;

    return {
      plugin: 'search',
      version: '1.0.0',
      engine: fullConfig.engine,
      status: 'running',
      indexes: indexes.map(idx => ({
        name: idx.name,
        enabled: idx.enabled,
        document_count: idx.document_count,
        last_indexed_at: idx.last_indexed_at,
      })),
      stats: stats
        ? {
            total_queries: stats.total_queries,
            avg_time_ms: stats.avg_time_ms,
          }
        : undefined,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Index Management
  // =========================================================================

  app.post<{ Body: CreateIndexRequest }>('/v1/indexes', async (request, reply) => {
    try {
      const index = await scopedDb(request).createIndex(request.body);
      return reply.status(201).send(index);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create index', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/indexes', async (request) => {
    const indexes = await scopedDb(request).listIndexes();
    return { indexes, count: indexes.length };
  });

  app.get<{ Params: { name: string } }>('/v1/indexes/:name', async (request, reply) => {
    const index = await scopedDb(request).getIndex(request.params.name);
    if (!index) {
      return reply.status(404).send({ error: 'Index not found' });
    }
    return index;
  });

  app.put<{ Params: { name: string }; Body: UpdateIndexRequest }>(
    '/v1/indexes/:name',
    async (request, reply) => {
      try {
        const index = await scopedDb(request).updateIndex(request.params.name, request.body);
        if (!index) {
          return reply.status(404).send({ error: 'Index not found' });
        }
        return index;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update index', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { name: string } }>('/v1/indexes/:name', async (request, reply) => {
    const deleted = await scopedDb(request).deleteIndex(request.params.name);
    if (!deleted) {
      return reply.status(404).send({ error: 'Index not found' });
    }
    return { deleted: true };
  });

  // =========================================================================
  // Document Management
  // =========================================================================

  app.post<{ Params: { name: string }; Body: IndexDocumentsRequest }>(
    '/v1/indexes/:name/documents',
    async (request, reply) => {
      try {
        const { documents } = request.body;

        if (!Array.isArray(documents) || documents.length === 0) {
          return reply.status(400).send({ error: 'documents array is required' });
        }

        if (documents.length > 1000) {
          return reply.status(400).send({ error: 'Maximum 1000 documents per batch' });
        }

        const indexed = await scopedDb(request).indexDocuments(request.params.name, documents);

        return {
          indexed,
          total: documents.length,
          failed: documents.length - indexed,
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to index documents', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.put<{ Params: { name: string; id: string }; Body: Record<string, unknown> }>(
    '/v1/indexes/:name/documents/:id',
    async (request, reply) => {
      try {
        const document = await scopedDb(request).indexDocument(request.params.name, {
          id: request.params.id,
          ...request.body,
        });
        return document;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update document', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { name: string; id: string } }>(
    '/v1/indexes/:name/documents/:id',
    async (request, reply) => {
      const deleted = await scopedDb(request).deleteDocument(request.params.name, request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Document not found' });
      }
      return { deleted: true };
    }
  );

  // =========================================================================
  // Reindex
  // =========================================================================

  app.post<{ Params: { name: string }; Body: ReindexOptions }>(
    '/v1/indexes/:name/reindex',
    async (request, reply) => {
      try {
        const result = await scopedDb(request).reindexFromSource(request.params.name, request.body);
        return result;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Reindex failed', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Search
  // =========================================================================

  app.post<{ Body: SearchRequest }>('/v1/search', async (request, reply) => {
    try {
      const searchRequest = request.body;

      if (!searchRequest.q || searchRequest.q.trim().length === 0) {
        return reply.status(400).send({ error: 'Query parameter "q" is required' });
      }

      const result = await scopedDb(request).search(searchRequest);

      // Record analytics
      if (fullConfig.analyticsEnabled) {
        await scopedDb(request).recordQuery(
          searchRequest.indexes?.[0] ?? null,
          searchRequest.q,
          result.total,
          result.processingTimeMs,
          searchRequest.filter
        );
      }

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Autocomplete / Suggestions
  // =========================================================================

  app.get<{ Querystring: SuggestRequest }>('/v1/suggest', async (request, reply) => {
    try {
      const { q, indexes, limit } = request.query;

      if (!q || q.trim().length === 0) {
        return reply.status(400).send({ error: 'Query parameter "q" is required' });
      }

      const indexArray = indexes ? (Array.isArray(indexes) ? indexes : [indexes]) : undefined;
      const limitNum = limit ? parseInt(String(limit), 10) : 10;

      const result = await scopedDb(request).suggest(q, indexArray, limitNum);

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Suggest failed', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Synonym Management
  // =========================================================================

  app.post<{ Params: { name: string }; Body: CreateSynonymRequest }>(
    '/v1/indexes/:name/synonyms',
    async (request, reply) => {
      try {
        const synonym = await scopedDb(request).addSynonym(
          request.params.name,
          request.body.word,
          request.body.synonyms
        );
        return reply.status(201).send(synonym);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to add synonym', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { name: string } }>('/v1/indexes/:name/synonyms', async (request, reply) => {
    try {
      const synonyms = await scopedDb(request).getSynonyms(request.params.name);
      return { synonyms, count: synonyms.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get synonyms', { error: message });
      return reply.status(400).send({ error: message });
      }
  });

  app.delete<{ Params: { name: string; id: string } }>(
    '/v1/indexes/:name/synonyms/:id',
    async (request, reply) => {
      const deleted = await scopedDb(request).deleteSynonym(request.params.name, request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Synonym not found' });
      }
      return { deleted: true };
    }
  );

  // =========================================================================
  // Analytics
  // =========================================================================

  app.get<{ Querystring: { limit?: number; days?: number } }>(
    '/v1/analytics/top-queries',
    async (request, reply) => {
      if (!fullConfig.analyticsEnabled) {
        return reply.status(404).send({ error: 'Analytics disabled' });
      }

      const limit = request.query.limit ? parseInt(String(request.query.limit), 10) : 20;
      const days = request.query.days ? parseInt(String(request.query.days), 10) : 30;

      const queries = await scopedDb(request).getTopQueries(limit, days);
      return { queries, count: queries.length };
    }
  );

  app.get<{ Querystring: { limit?: number; days?: number } }>(
    '/v1/analytics/no-results',
    async (request, reply) => {
      if (!fullConfig.analyticsEnabled) {
        return reply.status(404).send({ error: 'Analytics disabled' });
      }

      const limit = request.query.limit ? parseInt(String(request.query.limit), 10) : 20;
      const days = request.query.days ? parseInt(String(request.query.days), 10) : 30;

      const queries = await scopedDb(request).getNoResultQueries(limit, days);
      return { queries, count: queries.length };
    }
  );

  // =========================================================================
  // Sync Endpoint (for cleanup)
  // =========================================================================

  app.post('/v1/sync', async (request, reply) => {
    if (!fullConfig.analyticsEnabled) {
      return { message: 'Analytics disabled, nothing to sync' };
    }

    try {
      const cleaned = await scopedDb(request).cleanupOldAnalytics(fullConfig.analyticsRetentionDays);
      return {
        message: 'Analytics cleanup completed',
        records_deleted: cleaned,
        retention_days: fullConfig.analyticsRetentionDays,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhook', async (request, reply) => {
    try {
      const payload = request.body as Record<string, unknown>;
      const eventType = payload.type as string;

      if (!eventType) {
        return reply.status(400).send({ error: 'Missing event type' });
      }

      await scopedDb(request).insertWebhookEvent(eventType, payload);

      return { received: true, type: eventType };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  return app;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const fullConfig = loadConfig(config);
  const app = await createServer(config);

  try {
    await app.listen({
      port: fullConfig.port,
      host: fullConfig.host,
    });

    logger.info(`Search plugin server running`, {
      port: fullConfig.port,
      host: fullConfig.host,
      engine: fullConfig.engine,
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
