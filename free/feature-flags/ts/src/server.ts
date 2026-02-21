/**
 * Feature Flags Plugin Server
 * HTTP server for flag management and evaluation API
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { FeatureFlagsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { evaluateFlag, evaluateFlags } from './evaluator.js';
import type {
  CreateFlagRequest,
  UpdateFlagRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
  CreateSegmentRequest,
  UpdateSegmentRequest,
  EvaluationRequest,
  BatchEvaluationRequest,
  ListFlagsOptions,
  ListEvaluationsOptions,
} from './types.js';

const logger = createLogger('feature-flags:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new FeatureFlagsDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
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

  /** Extract scoped FeatureFlagsDatabase from request */
  function scopedDb(request: unknown): FeatureFlagsDatabase {
    return (request as Record<string, unknown>).scopedDb as FeatureFlagsDatabase;
  }

  // Helper to determine if evaluation should be logged
  function shouldLogEvaluation(): boolean {
    if (!fullConfig.evaluationLogEnabled) {
      return false;
    }
    const sampleRate = fullConfig.evaluationLogSampleRate;
    return Math.random() * 100 < sampleRate;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'feature-flags', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'feature-flags', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'feature-flags',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'feature-flags',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        flags: stats.flags,
        evaluations: stats.evaluations,
        lastEvaluation: stats.lastEvaluatedAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'feature-flags',
      version: '1.0.0',
      status: 'running',
      stats,
      config: {
        evaluationLogEnabled: fullConfig.evaluationLogEnabled,
        evaluationLogSampleRate: fullConfig.evaluationLogSampleRate,
        cacheTtlSeconds: fullConfig.cacheTtlSeconds,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Flag Management Endpoints
  // =========================================================================

  app.post<{ Body: CreateFlagRequest }>('/v1/flags', async (request, reply) => {
    try {
      const flag = await scopedDb(request).createFlag(request.body);
      return reply.status(201).send(flag);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create flag', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListFlagsOptions }>('/v1/flags', async (request) => {
    const options: ListFlagsOptions = {
      flag_type: request.query.flag_type,
      tag: request.query.tag,
      enabled: request.query.enabled,
      limit: request.query.limit,
      offset: request.query.offset,
    };

    const flags = await scopedDb(request).listFlags(options);
    return { flags, count: flags.length };
  });

  app.get<{ Params: { key: string } }>('/v1/flags/:key', async (request, reply) => {
    const flagDetail = await scopedDb(request).getFlagDetail(request.params.key);
    if (!flagDetail) {
      return reply.status(404).send({ error: 'Flag not found' });
    }
    return flagDetail;
  });

  app.put<{ Params: { key: string }; Body: UpdateFlagRequest }>('/v1/flags/:key', async (request, reply) => {
    const flag = await scopedDb(request).updateFlag(request.params.key, request.body);
    if (!flag) {
      return reply.status(404).send({ error: 'Flag not found' });
    }
    return flag;
  });

  app.delete<{ Params: { key: string } }>('/v1/flags/:key', async (request, reply) => {
    const deleted = await scopedDb(request).deleteFlag(request.params.key);
    if (!deleted) {
      return reply.status(404).send({ error: 'Flag not found' });
    }
    return { success: true };
  });

  app.post<{ Params: { key: string } }>('/v1/flags/:key/enable', async (request, reply) => {
    const enabled = await scopedDb(request).enableFlag(request.params.key);
    if (!enabled) {
      return reply.status(404).send({ error: 'Flag not found' });
    }
    return { success: true, enabled: true };
  });

  app.post<{ Params: { key: string } }>('/v1/flags/:key/disable', async (request, reply) => {
    const disabled = await scopedDb(request).disableFlag(request.params.key);
    if (!disabled) {
      return reply.status(404).send({ error: 'Flag not found' });
    }
    return { success: true, enabled: false };
  });

  // =========================================================================
  // Rule Management Endpoints
  // =========================================================================

  app.post<{ Params: { key: string }; Body: CreateRuleRequest }>('/v1/flags/:key/rules', async (request, reply) => {
    try {
      const ruleRequest = { ...request.body, flag_key: request.params.key } as CreateRuleRequest;
      const rule = await scopedDb(request).createRule(ruleRequest);
      if (!rule) {
        return reply.status(404).send({ error: 'Flag not found' });
      }
      return reply.status(201).send(rule);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create rule', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { key: string } }>('/v1/flags/:key/rules', async (request, reply) => {
    const flag = await scopedDb(request).getFlag(request.params.key);
    if (!flag) {
      return reply.status(404).send({ error: 'Flag not found' });
    }

    const rules = await scopedDb(request).getRulesByFlagId(flag.id);
    return { rules, count: rules.length };
  });

  app.put<{ Params: { key: string; ruleId: string }; Body: UpdateRuleRequest }>(
    '/v1/flags/:key/rules/:ruleId',
    async (request, reply) => {
      const rule = await scopedDb(request).updateRule(request.params.ruleId, request.body);
      if (!rule) {
        return reply.status(404).send({ error: 'Rule not found' });
      }
      return rule;
    }
  );

  app.delete<{ Params: { key: string; ruleId: string } }>('/v1/flags/:key/rules/:ruleId', async (request, reply) => {
    const deleted = await scopedDb(request).deleteRule(request.params.ruleId);
    if (!deleted) {
      return reply.status(404).send({ error: 'Rule not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Segment Management Endpoints
  // =========================================================================

  app.post<{ Body: CreateSegmentRequest }>('/v1/segments', async (request, reply) => {
    try {
      const segment = await scopedDb(request).createSegment(request.body);
      return reply.status(201).send(segment);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create segment', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/segments', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const segments = await scopedDb(request).listSegments(limit, offset);
    return { segments, count: segments.length };
  });

  app.put<{ Params: { id: string }; Body: UpdateSegmentRequest }>('/v1/segments/:id', async (request, reply) => {
    const segment = await scopedDb(request).updateSegment(request.params.id, request.body);
    if (!segment) {
      return reply.status(404).send({ error: 'Segment not found' });
    }
    return segment;
  });

  app.delete<{ Params: { id: string } }>('/v1/segments/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteSegment(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Segment not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Evaluation Endpoints
  // =========================================================================

  app.post<{ Body: EvaluationRequest }>('/v1/evaluate', async (request, reply) => {
    try {
      const { flag_key, user_id, context = {} } = request.body;

      const result = await evaluateFlag(flag_key, user_id, context, scopedDb(request));

      // Update flag evaluation stats
      await scopedDb(request).updateFlagEvaluation(flag_key);

      // Optionally log evaluation
      if (shouldLogEvaluation()) {
        await scopedDb(request).recordEvaluation(flag_key, user_id, context, result);
      }

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evaluation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: BatchEvaluationRequest }>('/v1/evaluate/batch', async (request, reply) => {
    try {
      const { flag_keys, user_id, context = {} } = request.body;

      const results = await evaluateFlags(flag_keys, user_id, context, scopedDb(request));

      // Update stats for all flags
      for (const flagKey of flag_keys) {
        await scopedDb(request).updateFlagEvaluation(flagKey);
      }

      // Optionally log evaluations
      if (shouldLogEvaluation()) {
        for (let i = 0; i < flag_keys.length; i++) {
          await scopedDb(request).recordEvaluation(flag_keys[i], user_id, context, results[i]);
        }
      }

      return { results, count: results.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch evaluation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListEvaluationsOptions & { since?: string } }>('/v1/evaluations', async (request) => {
    const options: ListEvaluationsOptions = {
      flag_key: request.query.flag_key,
      user_id: request.query.user_id,
      reason: request.query.reason,
      since: request.query.since ? new Date(request.query.since) : undefined,
      limit: request.query.limit,
      offset: request.query.offset,
    };

    const evaluations = await scopedDb(request).listEvaluations(options);
    return { evaluations, count: evaluations.length };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return stats;
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const start = async () => {
    try {
      await app.listen({
        port: fullConfig.port,
        host: fullConfig.host,
      });

      logger.info(`Feature flags server running on ${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Evaluation logging: ${fullConfig.evaluationLogEnabled ? 'enabled' : 'disabled'} (sample rate: ${fullConfig.evaluationLogSampleRate}%)`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed to start', { error: message });
      process.exit(1);
    }
  };

  const stop = async () => {
    logger.info('Shutting down server...');
    await app.close();
    await db.disconnect();
    logger.info('Server stopped');
  };

  // Graceful shutdown
  process.on('SIGINT', stop);
  process.on('SIGTERM', stop);

  return {
    app,
    start,
    stop,
  };
}
