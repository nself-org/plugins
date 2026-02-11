/**
 * Content Policy Plugin Server
 * HTTP server for content evaluation and policy management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ContentPolicyDatabase } from './database.js';
import { ContentPolicyEvaluator } from './evaluator.js';
import { loadConfig, type ContentPolicyConfig } from './config.js';
import type {
  EvaluateRequest,
  BatchEvaluateRequest,
  CreatePolicyRequest,
  UpdatePolicyRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
  CreateWordListRequest,
  UpdateWordListRequest,
  CreateOverrideRequest,
  TestRuleRequest,
  RuleConfig,
} from './types.js';

const logger = createLogger('content-policy:server');

export async function createServer(config?: Partial<ContentPolicyConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ContentPolicyDatabase();
  await db.connect();
  await db.initializeSchema();

  const evaluator = new ContentPolicyEvaluator(db);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large content evaluations
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(fullConfig.rateLimitMax, fullConfig.rateLimitWindowMs);

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request
  app.decorateRequest('scopedDb', null);
  app.decorateRequest('scopedEvaluator', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
    (request as unknown as Record<string, unknown>).scopedEvaluator = new ContentPolicyEvaluator(scopedDb);
  });

  /** Extract scoped database from request */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function scopedDb(request: any): ContentPolicyDatabase {
    return (request as Record<string, unknown>).scopedDb as ContentPolicyDatabase;
  }

  /** Extract scoped evaluator from request */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function scopedEvaluator(request: any): ContentPolicyEvaluator {
    return (request as Record<string, unknown>).scopedEvaluator as ContentPolicyEvaluator;
  }

  // =========================================================================
  // Health & Status Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'content-policy', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'content-policy', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'content-policy',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'content-policy',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        total_evaluations: stats.total_evaluations,
        flagged: stats.flagged,
        denied: stats.denied,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'content-policy',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Evaluation Endpoints
  // =========================================================================

  app.post<{ Body: EvaluateRequest }>('/v1/evaluate', async (request, reply) => {
    const { content_type, content_text, content_id, submitter_id, policy_ids } = request.body;

    if (!content_type || !content_text) {
      return reply.status(400).send({ error: 'content_type and content_text are required' });
    }

    if (content_text.length > fullConfig.maxContentLength) {
      return reply.status(400).send({
        error: `Content exceeds maximum length of ${fullConfig.maxContentLength} characters`,
      });
    }

    try {
      const result = await scopedEvaluator(request).evaluate({
        content_type,
        content_text,
        content_id,
        submitter_id,
        policy_ids,
      });
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evaluation failed', { error: message });
      return reply.status(500).send({ error: 'Evaluation failed', message });
    }
  });

  app.post<{ Body: BatchEvaluateRequest }>('/v1/evaluate/batch', async (request, reply) => {
    const { items } = request.body;

    if (!Array.isArray(items) || items.length === 0) {
      return reply.status(400).send({ error: 'items array is required and must not be empty' });
    }

    if (items.length > 100) {
      return reply.status(400).send({ error: 'Maximum 100 items per batch' });
    }

    const results = [];
    let processed = 0;
    let failed = 0;

    for (const item of items) {
      try {
        const result = await scopedEvaluator(request).evaluate(item);
        results.push(result);
        processed++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Batch item evaluation failed', { error: message });
        failed++;
      }
    }

    return {
      results,
      total: items.length,
      processed,
      failed,
    };
  });

  app.get('/v1/evaluations', async (request) => {
    const { limit = 100, offset = 0, result, content_type, submitter_id, since } = request.query as {
      limit?: number;
      offset?: number;
      result?: string;
      content_type?: string;
      submitter_id?: string;
      since?: string;
    };

    const evaluations = await scopedDb(request).listEvaluations(limit, offset, {
      result: result as 'allowed' | 'denied' | 'flagged' | 'quarantined' | undefined,
      content_type,
      submitter_id,
      since: since ? new Date(since) : undefined,
    });

    const total = await scopedDb(request).countEvaluations({
      result: result as 'allowed' | 'denied' | 'flagged' | 'quarantined' | undefined,
      content_type,
    });

    return { data: evaluations, total, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/v1/evaluations/:id', async (request, reply) => {
    const { id } = request.params;
    const evaluation = await scopedDb(request).getEvaluation(id);

    if (!evaluation) {
      return reply.status(404).send({ error: 'Evaluation not found' });
    }

    return evaluation;
  });

  // =========================================================================
  // Override Endpoints
  // =========================================================================

  app.post<{ Body: CreateOverrideRequest }>('/v1/overrides', async (request, reply) => {
    const { evaluation_id, override_result, moderator_id, reason } = request.body;

    if (!evaluation_id || !override_result || !moderator_id) {
      return reply.status(400).send({
        error: 'evaluation_id, override_result, and moderator_id are required',
      });
    }

    try {
      const override = await scopedDb(request).createOverride({
        evaluation_id,
        override_result,
        moderator_id,
        reason,
      });
      return override;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Override creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/overrides', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const overrides = await scopedDb(request).listOverrides(limit, offset);
    return { data: overrides, limit, offset };
  });

  // =========================================================================
  // Policy Endpoints
  // =========================================================================

  app.post<{ Body: CreatePolicyRequest }>('/v1/policies', async (request, reply) => {
    const data = request.body;

    if (!data.name) {
      return reply.status(400).send({ error: 'name is required' });
    }

    try {
      const policy = await scopedDb(request).createPolicy(data);
      return policy;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policy creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/policies', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const policies = await scopedDb(request).listPolicies(limit, offset);
    return { data: policies, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/v1/policies/:id', async (request, reply) => {
    const { id } = request.params;
    const policy = await scopedDb(request).getPolicy(id);

    if (!policy) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    const rules = await scopedDb(request).listRules(id);
    return { ...policy, rules };
  });

  app.put<{ Params: { id: string }; Body: UpdatePolicyRequest }>('/v1/policies/:id', async (request, reply) => {
    const { id } = request.params;
    const data = request.body;

    try {
      const policy = await scopedDb(request).updatePolicy(id, data);
      if (!policy) {
        return reply.status(404).send({ error: 'Policy not found' });
      }
      return policy;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policy update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/v1/policies/:id', async (request, reply) => {
    const { id } = request.params;
    const deleted = await scopedDb(request).deletePolicy(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Rule Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: CreateRuleRequest }>('/v1/policies/:id/rules', async (request, reply) => {
    const { id } = request.params;
    const data = request.body;

    if (!data.name || !data.rule_type || !data.config) {
      return reply.status(400).send({ error: 'name, rule_type, and config are required' });
    }

    // Verify policy exists
    const policy = await scopedDb(request).getPolicy(id);
    if (!policy) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    try {
      const rule = await scopedDb(request).createRule(id, data);
      return rule;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Rule creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/v1/policies/:id/rules', async (request, reply) => {
    const { id } = request.params;

    // Verify policy exists
    const policy = await scopedDb(request).getPolicy(id);
    if (!policy) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    const rules = await scopedDb(request).listRules(id);
    return { data: rules };
  });

  app.put<{ Params: { id: string; ruleId: string }; Body: UpdateRuleRequest }>(
    '/v1/policies/:id/rules/:ruleId',
    async (request, reply) => {
      const { ruleId } = request.params;
      const data = request.body;

      try {
        const rule = await scopedDb(request).updateRule(ruleId, data);
        if (!rule) {
          return reply.status(404).send({ error: 'Rule not found' });
        }
        return rule;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Rule update failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { id: string; ruleId: string } }>('/v1/policies/:id/rules/:ruleId', async (request, reply) => {
    const { ruleId } = request.params;
    const deleted = await scopedDb(request).deleteRule(ruleId);

    if (!deleted) {
      return reply.status(404).send({ error: 'Rule not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Word List Endpoints
  // =========================================================================

  app.post<{ Body: CreateWordListRequest }>('/v1/word-lists', async (request, reply) => {
    const data = request.body;

    if (!data.name || !data.list_type || !Array.isArray(data.words)) {
      return reply.status(400).send({ error: 'name, list_type, and words are required' });
    }

    try {
      const wordList = await scopedDb(request).createWordList(data);
      return wordList;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Word list creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/word-lists', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const wordLists = await scopedDb(request).listWordLists(limit, offset);
    return { data: wordLists, limit, offset };
  });

  app.put<{ Params: { id: string }; Body: UpdateWordListRequest }>('/v1/word-lists/:id', async (request, reply) => {
    const { id } = request.params;
    const data = request.body;

    try {
      const wordList = await scopedDb(request).updateWordList(id, data);
      if (!wordList) {
        return reply.status(404).send({ error: 'Word list not found' });
      }
      return wordList;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Word list update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/v1/word-lists/:id', async (request, reply) => {
    const { id } = request.params;
    const deleted = await scopedDb(request).deleteWordList(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Word list not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Test & Utility Endpoints
  // =========================================================================

  app.post<{ Body: TestRuleRequest }>('/v1/test', async (request, reply) => {
    const { content_text, rule_type, config } = request.body;

    if (!content_text || !rule_type || !config) {
      return reply.status(400).send({ error: 'content_text, rule_type, and config are required' });
    }

    try {
      // Add type to config
      const configWithType = { ...config, type: rule_type } as RuleConfig;
      const result = await scopedEvaluator(request).testRule(content_text, configWithType);
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Rule test failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/queue', async (request) => {
    const { result, limit = 100 } = request.query as { result?: 'flagged' | 'quarantined'; limit?: number };
    const queue = await scopedDb(request).getQueue(result, limit);
    return { data: queue, count: queue.length };
  });

  app.get('/v1/stats', async (request) => {
    const { since } = request.query as { since?: string };
    const stats = await scopedDb(request).getStats(since ? new Date(since) : undefined);
    return stats;
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (err) {
      const error = err instanceof Error ? err.message : 'Unknown error';
      logger.error('Server failed to start', { error });
      process.exit(1);
    }
  };

  return { app, start, db, evaluator };
}
