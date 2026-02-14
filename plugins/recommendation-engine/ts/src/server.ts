#!/usr/bin/env node
/**
 * HTTP server for recommendation-engine API
 * Multi-app aware: each request is scoped to a source_account_id
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  loadSecurityConfig,
  ApiRateLimiter,
  createAuthHook,
  createRateLimitHook,
  getAppContext,
} from '@nself/plugin-utils';
import { config } from './config.js';
import { db, RecommendationDatabase } from './database.js';
import { RecommendationEngine } from './engine.js';
import type { RecommendationQuery, SimilarQuery } from './types.js';

const logger = createLogger('recommendation:server');

const fastify = Fastify({ logger: false });

// CORS
fastify.register(cors, { origin: true });

// Security middleware
const securityConfig = loadSecurityConfig('RECOMMENDATION');
const rateLimiter = new ApiRateLimiter(
  securityConfig.rateLimitMax ?? 100,
  securityConfig.rateLimitWindowMs ?? 60000
);
fastify.addHook('preHandler', createAuthHook(securityConfig.apiKey));
fastify.addHook('preHandler', createRateLimitHook(rateLimiter));

// Multi-app context: scope DB per request
fastify.decorateRequest('scopedDb', null);
fastify.decorateRequest('scopedEngine', null);

// Map of engines per source_account_id
const engines = new Map<string, RecommendationEngine>();

function getOrCreateEngine(scopedDb: RecommendationDatabase, accountId: string): RecommendationEngine {
  let engine = engines.get(accountId);
  if (!engine) {
    engine = new RecommendationEngine(scopedDb);
    engines.set(accountId, engine);
  }
  return engine;
}

fastify.addHook('onRequest', async (request) => {
  const ctx = getAppContext(request);
  const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  const engine = getOrCreateEngine(scopedDb, ctx.sourceAccountId);
  (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
  (request as unknown as Record<string, unknown>).scopedEngine = engine;
});

function scopedDb(request: unknown): RecommendationDatabase {
  return (request as Record<string, unknown>).scopedDb as RecommendationDatabase;
}

function scopedEngine(request: unknown): RecommendationEngine {
  return (request as Record<string, unknown>).scopedEngine as RecommendationEngine;
}

// =============================================================================
// Health
// =============================================================================

fastify.get('/health', async () => ({
  status: 'ok',
  timestamp: new Date().toISOString(),
  service: 'recommendation-engine',
}));

fastify.get('/ready', async (_request) => {
  try {
    const stats = await db.getStats();
    const modelState = await db.getModelState();
    return {
      status: modelState?.model_ready ? 'ready' : 'warming_up',
      ...stats,
      model_ready: modelState?.model_ready ?? false,
    };
  } catch {
    return { status: 'not_ready' };
  }
});

// =============================================================================
// Recommendations
// =============================================================================

fastify.get<{ Params: { userId: string }; Querystring: RecommendationQuery }>(
  '/v1/recommendations/:userId',
  async (request, reply) => {
    try {
      const { userId } = request.params;
      const limit = parseInt(request.query.limit ?? '20', 10);
      const mediaType = request.query.type;

      const engine = scopedEngine(request);
      const recommendations = await engine.getRecommendations(userId, limit, mediaType);

      return { data: recommendations, total: recommendations.length };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get recommendations', { error: msg });
      return reply.code(500).send({ error: msg });
    }
  }
);

// =============================================================================
// Similar Content
// =============================================================================

fastify.get<{ Params: { mediaId: string }; Querystring: SimilarQuery }>(
  '/v1/similar/:mediaId',
  async (request, reply) => {
    try {
      const { mediaId } = request.params;
      const limit = parseInt(request.query.limit ?? '10', 10);

      const engine = scopedEngine(request);
      const similar = await engine.getSimilarItems(mediaId, limit);

      return { data: similar, total: similar.length };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get similar items', { error: msg });
      return reply.code(500).send({ error: msg });
    }
  }
);

// =============================================================================
// Model Management
// =============================================================================

fastify.post('/v1/rebuild', async (request, reply) => {
  try {
    const engine = scopedEngine(request);
    const result = await engine.rebuild();
    return result;
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to trigger rebuild', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get('/v1/status', async (request, reply) => {
  try {
    const engine = scopedEngine(request);
    const status = await engine.getStatus();
    return status;
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to get status', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Data Management (for populating profiles)
// =============================================================================

fastify.post<{
  Body: {
    media_id: string;
    title: string;
    media_type?: string;
    genres?: string[];
    cast_members?: string[];
    director?: string;
    description?: string;
    view_count?: number;
    avg_rating?: number;
  };
}>('/v1/items', async (request, reply) => {
  try {
    const item = await scopedDb(request).upsertItemProfile({
      media_id: request.body.media_id,
      title: request.body.title,
      media_type: request.body.media_type ?? null,
      genres: request.body.genres ?? [],
      cast_members: request.body.cast_members ?? [],
      director: request.body.director ?? null,
      description: request.body.description ?? null,
      tfidf_vector: null,
      view_count: request.body.view_count ?? 0,
      avg_rating: request.body.avg_rating ?? 0,
    });
    return reply.code(201).send({ item });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to upsert item', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{
  Body: {
    user_id: string;
    interaction_count?: number;
    preferred_genres?: string[];
    avg_rating?: number;
  };
}>('/v1/users', async (request, reply) => {
  try {
    const profile = await scopedDb(request).upsertUserProfile({
      user_id: request.body.user_id,
      interaction_count: request.body.interaction_count ?? 0,
      preferred_genres: request.body.preferred_genres ?? [],
      avg_rating: request.body.avg_rating ?? null,
      last_interaction_at: new Date(),
      profile_vector: null,
    });
    return reply.code(201).send({ profile });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to upsert user profile', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get('/v1/stats', async (request, reply) => {
  try {
    const stats = await scopedDb(request).getStats();
    return { stats };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Server Lifecycle
// =============================================================================

const start = async () => {
  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    // Initialize the default engine
    const defaultEngine = getOrCreateEngine(db, 'primary');
    await defaultEngine.initialize();
    logger.info('Recommendation engine initialized');

    await fastify.listen({ port: config.server.port, host: config.server.host });
    logger.info(`Recommendation engine server running on http://${config.server.host}:${config.server.port}`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Failed to start server', { error: msg });
    process.exit(1);
  }
};

const gracefulShutdown = async () => {
  logger.info('Shutting down gracefully...');
  try {
    // Shut down all engines
    for (const [accountId, engine] of engines) {
      await engine.shutdown();
      logger.debug('Engine shut down', { accountId });
    }
    engines.clear();

    await fastify.close();
    await db.close();
    process.exit(0);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Error during shutdown', { error: msg });
    process.exit(1);
  }
};

process.on('SIGTERM', gracefulShutdown);
process.on('SIGINT', gracefulShutdown);

start();

export { fastify };
