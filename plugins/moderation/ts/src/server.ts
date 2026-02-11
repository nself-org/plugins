/**
 * Moderation Plugin Server
 * HTTP server for content moderation API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ModerationDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateRuleRequest,
  UpdateRuleRequest,
  CreateWordlistRequest,
  UpdateWordlistRequest,
  CreateActionRequest,
  RevokeActionRequest,
  CreateFlagRequest,
  ReviewFlagRequest,
  CreateAppealRequest,
  ReviewAppealRequest,
  CreateReportRequest,
  AnalyzeContentRequest,
  CheckProfanityRequest,
} from './types.js';

const logger = createLogger('moderation:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new ModerationDatabase();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
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

  function scopedDb(request: unknown): ModerationDatabase {
    return (request as Record<string, unknown>).scopedDb as ModerationDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'moderation', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'moderation', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false, plugin: 'moderation', error: 'Database unavailable', timestamp: new Date().toISOString(),
      });
    }
  });

  // =========================================================================
  // Content Analysis
  // =========================================================================

  app.post<{ Body: AnalyzeContentRequest }>('/api/moderation/analyze', async (request, reply) => {
    try {
      const { content, content_type, channel_id } = request.body;
      if (!content || !content_type) {
        return reply.status(400).send({ error: 'content and content_type are required' });
      }

      const profanityResult = await scopedDb(request).checkProfanity(content);
      const rules = await scopedDb(request).listRules(true);

      const matchedRules = rules
        .filter(rule => {
          if (channel_id && rule.channel_id && rule.channel_id !== channel_id) return false;
          if (rule.filter_type === 'profanity' && profanityResult.matched_words.length > 0) return true;
          if (rule.filter_type === 'spam') {
            const conditions = rule.conditions;
            if (conditions.pattern) {
              try {
                const regex = new RegExp(conditions.pattern, conditions.regex ? '' : 'i');
                return regex.test(content);
              } catch { return false; }
            }
          }
          return false;
        })
        .map(rule => ({
          rule_id: rule.id,
          rule_name: rule.name,
          severity: rule.severity,
          matched_words: profanityResult.matched_words.length > 0 ? profanityResult.matched_words : undefined,
        }));

      const suggestedActions = matchedRules.flatMap(rule => {
        const fullRule = rules.find(r => r.id === rule.rule_id);
        if (!fullRule) return [];
        return fullRule.actions.map(a => ({ type: a.type, reason: `Matched rule: ${rule.rule_name}` }));
      });

      const isSafe = matchedRules.length === 0;

      return {
        is_safe: isSafe,
        toxicity_score: 0,
        matched_rules: matchedRules,
        suggested_actions: suggestedActions,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Content analysis failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: CheckProfanityRequest }>('/api/moderation/check-profanity', async (request, reply) => {
    try {
      const { content, language } = request.body;
      if (!content) {
        return reply.status(400).send({ error: 'content is required' });
      }

      const result = await scopedDb(request).checkProfanity(content, language);

      return {
        contains_profanity: result.matched_words.length > 0,
        matched_words: result.matched_words,
        severity: result.severity,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Profanity check failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Actions
  // =========================================================================

  app.post<{ Body: CreateActionRequest }>('/api/moderation/actions', async (request, reply) => {
    try {
      const { user_id, action_type, reason } = request.body;
      if (!user_id || !action_type || !reason) {
        return reply.status(400).send({ error: 'user_id, action_type, and reason are required' });
      }

      const action = await scopedDb(request).createAction(request.body);

      await scopedDb(request).createAuditLog({
        event_type: 'action.created',
        event_category: 'action',
        actor_id: request.body.moderator_id,
        actor_type: request.body.is_automated ? 'automation' : 'user',
        target_type: 'user',
        target_id: user_id,
        details: { action_type, reason, action_id: action.id },
      });

      return reply.status(201).send({
        action_id: action.id,
        expires_at: action.expires_at?.toISOString() ?? null,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create action failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { user_id: string } }>('/api/moderation/actions/:user_id', async (request) => {
    const actions = await scopedDb(request).listActionsForUser(request.params.user_id);
    return {
      actions: actions.map(a => ({
        id: a.id,
        action_type: a.action_type,
        severity: a.severity,
        reason: a.reason,
        created_at: a.created_at,
        expires_at: a.expires_at,
        is_active: a.is_active,
        is_automated: a.is_automated,
        moderator_id: a.moderator_id,
      })),
    };
  });

  app.delete<{ Params: { action_id: string }; Body: RevokeActionRequest }>('/api/moderation/actions/:action_id', async (request, reply) => {
    try {
      const { revoke_reason } = request.body;
      if (!revoke_reason) {
        return reply.status(400).send({ error: 'revoke_reason is required' });
      }

      const success = await scopedDb(request).revokeAction(request.params.action_id, request.body);
      if (!success) {
        return reply.status(404).send({ error: 'Action not found or already revoked' });
      }

      await scopedDb(request).createAuditLog({
        event_type: 'action.revoked',
        event_category: 'action',
        actor_id: request.body.revoked_by,
        target_type: 'action',
        target_id: request.params.action_id,
        details: { revoke_reason },
      });

      return { success: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Revoke action failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Flags & Queue
  // =========================================================================

  app.post<{ Body: CreateFlagRequest }>('/api/moderation/flags', async (request, reply) => {
    try {
      const { content_type, content_id, flag_reason } = request.body;
      if (!content_type || !content_id || !flag_reason) {
        return reply.status(400).send({ error: 'content_type, content_id, and flag_reason are required' });
      }

      const flag = await scopedDb(request).createFlag(request.body);

      await scopedDb(request).createAuditLog({
        event_type: 'flag.created',
        event_category: 'flag',
        actor_id: request.body.flagged_by_user_id,
        actor_type: request.body.is_automated ? 'automation' : 'user',
        target_type: content_type,
        target_id: content_id,
        details: { flag_id: flag.id, flag_reason },
      });

      return reply.status(201).send({ flag_id: flag.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create flag failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { status?: string; severity?: string; limit?: string; offset?: string } }>(
    '/api/moderation/queue',
    async (request) => {
      const { status, severity, limit, offset } = request.query;
      const result = await scopedDb(request).listFlags({
        status: status ?? 'pending',
        severity,
        limit: limit ? parseInt(limit, 10) : 50,
        offset: offset ? parseInt(offset, 10) : 0,
      });

      return {
        flags: result.flags.map(f => ({
          id: f.id,
          content_type: f.content_type,
          content_id: f.content_id,
          flag_reason: f.flag_reason,
          flag_category: f.flag_category,
          severity: f.severity,
          status: f.status,
          is_automated: f.is_automated,
          created_at: f.created_at,
        })),
        total: result.total,
      };
    }
  );

  app.post<{ Params: { flag_id: string }; Body: ReviewFlagRequest }>(
    '/api/moderation/flags/:flag_id/review',
    async (request, reply) => {
      try {
        const { status } = request.body;
        if (!status || !['approved', 'rejected'].includes(status)) {
          return reply.status(400).send({ error: 'status must be approved or rejected' });
        }

        const flag = await scopedDb(request).reviewFlag(request.params.flag_id, request.body);
        if (!flag) {
          return reply.status(404).send({ error: 'Flag not found' });
        }

        let actionId: string | undefined;
        if (request.body.action && flag.status === 'approved') {
          const flagData = await scopedDb(request).getFlag(request.params.flag_id);
          if (flagData) {
            const action = await scopedDb(request).createAction({
              user_id: flagData.flagged_by_user_id ?? 'unknown',
              action_type: request.body.action.type,
              reason: `Flag review: ${flagData.flag_reason}`,
              duration_minutes: request.body.action.duration_minutes,
            });
            actionId = action.id;
          }
        }

        await scopedDb(request).createAuditLog({
          event_type: 'flag.reviewed',
          event_category: 'flag',
          actor_id: request.body.reviewed_by,
          target_type: 'flag',
          target_id: request.params.flag_id,
          details: { status, action_id: actionId },
        });

        return { success: true, action_id: actionId };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Review flag failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Reports
  // =========================================================================

  app.post<{ Body: CreateReportRequest }>('/api/moderation/reports', async (request, reply) => {
    try {
      const { reporter_id, content_type, content_id, report_category, report_reason } = request.body;
      if (!reporter_id || !content_type || !content_id || !report_category || !report_reason) {
        return reply.status(400).send({ error: 'reporter_id, content_type, content_id, report_category, and report_reason are required' });
      }

      const report = await scopedDb(request).createReport(request.body);

      await scopedDb(request).createAuditLog({
        event_type: 'report.created',
        event_category: 'flag',
        actor_id: reporter_id,
        target_type: content_type,
        target_id: content_id,
        details: { report_id: report.id, report_category },
      });

      return reply.status(201).send({ report_id: report.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create report failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { status?: string; limit?: string; offset?: string } }>(
    '/api/moderation/reports',
    async (request) => {
      const { status, limit, offset } = request.query;
      const result = await scopedDb(request).listReports({
        status,
        limit: limit ? parseInt(limit, 10) : 50,
        offset: offset ? parseInt(offset, 10) : 0,
      });
      return { reports: result.reports, total: result.total };
    }
  );

  // =========================================================================
  // Appeals
  // =========================================================================

  app.post<{ Body: CreateAppealRequest }>('/api/moderation/appeals', async (request, reply) => {
    try {
      const { action_id, appellant_user_id, appeal_reason } = request.body;
      if (!action_id || !appellant_user_id || !appeal_reason) {
        return reply.status(400).send({ error: 'action_id, appellant_user_id, and appeal_reason are required' });
      }

      if (!fullConfig.appealsEnabled) {
        return reply.status(403).send({ error: 'Appeals are disabled' });
      }

      const appeal = await scopedDb(request).createAppeal(request.body);

      await scopedDb(request).createAuditLog({
        event_type: 'appeal.created',
        event_category: 'appeal',
        actor_id: appellant_user_id,
        target_type: 'action',
        target_id: action_id,
        details: { appeal_id: appeal.id },
      });

      return reply.status(201).send({ appeal_id: appeal.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create appeal failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { status?: string; limit?: string } }>('/api/moderation/appeals', async (request) => {
    const { status, limit } = request.query;
    const appeals = await scopedDb(request).listAppeals({
      status,
      limit: limit ? parseInt(limit, 10) : 50,
    });
    return {
      appeals: appeals.map(a => ({
        id: a.id,
        action_id: a.action_id,
        appellant_user_id: a.appellant_user_id,
        appeal_reason: a.appeal_reason,
        status: a.status,
        created_at: a.created_at,
      })),
    };
  });

  app.post<{ Params: { appeal_id: string }; Body: ReviewAppealRequest }>(
    '/api/moderation/appeals/:appeal_id/review',
    async (request, reply) => {
      try {
        const { status, review_decision } = request.body;
        if (!status || !review_decision) {
          return reply.status(400).send({ error: 'status and review_decision are required' });
        }

        const appeal = await scopedDb(request).reviewAppeal(request.params.appeal_id, request.body);
        if (!appeal) {
          return reply.status(404).send({ error: 'Appeal not found' });
        }

        await scopedDb(request).createAuditLog({
          event_type: 'appeal.reviewed',
          event_category: 'appeal',
          actor_id: request.body.reviewed_by,
          target_type: 'appeal',
          target_id: request.params.appeal_id,
          details: { status, was_successful: appeal.was_successful },
        });

        return { success: true, action_revoked: appeal.was_successful ?? false };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Review appeal failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Rules & Wordlists
  // =========================================================================

  app.get('/api/moderation/rules', async (request) => {
    const rules = await scopedDb(request).listRules();
    return {
      rules: rules.map(r => ({
        id: r.id,
        name: r.name,
        description: r.description,
        filter_type: r.filter_type,
        severity: r.severity,
        is_enabled: r.is_enabled,
        conditions: r.conditions,
        actions: r.actions,
      })),
    };
  });

  app.post<{ Body: CreateRuleRequest }>('/api/moderation/rules', async (request, reply) => {
    try {
      const { name, filter_type, severity, conditions, actions } = request.body;
      if (!name || !filter_type || !severity || !conditions || !actions) {
        return reply.status(400).send({ error: 'name, filter_type, severity, conditions, and actions are required' });
      }

      const rule = await scopedDb(request).createRule(request.body);

      await scopedDb(request).createAuditLog({
        event_type: 'rule.created',
        event_category: 'config',
        target_type: 'rule',
        target_id: rule.id,
        details: { name, filter_type, severity },
      });

      return reply.status(201).send({ rule_id: rule.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create rule failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.patch<{ Params: { rule_id: string }; Body: UpdateRuleRequest }>(
    '/api/moderation/rules/:rule_id',
    async (request, reply) => {
      try {
        const rule = await scopedDb(request).updateRule(request.params.rule_id, request.body);
        if (!rule) {
          return reply.status(404).send({ error: 'Rule not found' });
        }

        await scopedDb(request).createAuditLog({
          event_type: 'rule.updated',
          event_category: 'config',
          target_type: 'rule',
          target_id: request.params.rule_id,
          details: { updates: request.body },
        });

        return { success: true };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Update rule failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { rule_id: string } }>('/api/moderation/rules/:rule_id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteRule(request.params.rule_id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Rule not found' });
    }
    return { deleted: true };
  });

  // Wordlists
  app.get('/api/moderation/wordlists', async (request) => {
    const wordlists = await scopedDb(request).listWordlists();
    return { wordlists };
  });

  app.post<{ Body: CreateWordlistRequest }>('/api/moderation/wordlists', async (request, reply) => {
    try {
      const { name, words } = request.body;
      if (!name || !words || words.length === 0) {
        return reply.status(400).send({ error: 'name and words are required' });
      }

      const wordlist = await scopedDb(request).createWordlist(request.body);
      return reply.status(201).send({ wordlist_id: wordlist.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create wordlist failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.patch<{ Params: { wordlist_id: string }; Body: UpdateWordlistRequest }>(
    '/api/moderation/wordlists/:wordlist_id',
    async (request, reply) => {
      try {
        const wordlist = await scopedDb(request).updateWordlist(request.params.wordlist_id, request.body);
        if (!wordlist) {
          return reply.status(404).send({ error: 'Wordlist not found' });
        }
        return { success: true };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Update wordlist failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { wordlist_id: string } }>('/api/moderation/wordlists/:wordlist_id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteWordlist(request.params.wordlist_id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Wordlist not found' });
    }
    return { deleted: true };
  });

  // =========================================================================
  // Statistics
  // =========================================================================

  app.get<{ Params: { user_id: string } }>('/api/moderation/stats/user/:user_id', async (request, reply) => {
    const stats = await scopedDb(request).getUserStats(request.params.user_id);
    if (!stats) {
      return reply.status(404).send({ error: 'User stats not found' });
    }
    return {
      total_warnings: stats.total_warnings,
      total_mutes: stats.total_mutes,
      total_bans: stats.total_bans,
      total_flags: stats.total_flags,
      risk_level: stats.risk_level,
      risk_score: stats.risk_score,
      average_toxicity_score: stats.average_toxicity_score,
      is_muted: stats.is_muted,
      muted_until: stats.muted_until,
      is_banned: stats.is_banned,
      banned_until: stats.banned_until,
    };
  });

  app.get<{ Querystring: { timeframe?: string } }>('/api/moderation/stats/overview', async (request) => {
    const timeframeMap: Record<string, number> = { day: 1, week: 7, month: 30 };
    const days = timeframeMap[request.query.timeframe ?? 'month'] ?? 30;
    const stats = await scopedDb(request).getOverviewStats(days);
    return stats;
  });

  // =========================================================================
  // Audit Log
  // =========================================================================

  app.get<{ Querystring: { event_type?: string; event_category?: string; actor_id?: string; limit?: string; offset?: string } }>(
    '/api/moderation/audit-log',
    async (request) => {
      const { event_type, event_category, actor_id, limit, offset } = request.query;
      const result = await scopedDb(request).listAuditLogs({
        event_type,
        event_category,
        actor_id,
        limit: limit ? parseInt(limit, 10) : 50,
        offset: offset ? parseInt(offset, 10) : 0,
      });
      return { logs: result.logs, total: result.total };
    }
  );

  // =========================================================================
  // Cleanup
  // =========================================================================

  app.post('/api/moderation/cleanup/expired', async (request) => {
    const count = await scopedDb(request).expireActions();
    return { expired_count: count };
  });

  return { app, db, config: fullConfig };
}

export async function startServer(config?: Partial<Config>) {
  const { app, config: fullConfig } = await createServer(config);

  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.info(`Moderation server listening on ${fullConfig.host}:${fullConfig.port}`);

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
