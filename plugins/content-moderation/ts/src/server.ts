#!/usr/bin/env node
/**
 * Content Moderation Plugin HTTP Server
 * REST API endpoints for content moderation, review queues, appeals, and policies
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { ModerationDatabase } from './database.js';
import {
  SubmitReviewRequest,
  BatchReviewRequest,
  QueueQuery,
  ManualReviewRequest,
  ReviewListQuery,
  SubmitAppealRequest,
  ResolveAppealRequest,
  AppealListQuery,
  CreatePolicyRequest,
  UpdatePolicyRequest,
  AddStrikeRequest,
  StatsQuery,
  HealthCheckResponse,
} from './types.js';

const logger = createLogger('content-moderation:server');

const fastify = Fastify({
  logger: false,
  bodyLimit: 10485760,
});

let modDb: ModerationDatabase;

/**
 * Get scoped database for request
 */
function getAppContext(request: { headers: Record<string, string | string[] | undefined> }): string {
  return (request.headers['x-app-id'] as string) || 'primary';
}

function scopedDb(request: { headers: Record<string, string | string[] | undefined> }): ModerationDatabase {
  return modDb.forSourceAccount(getAppContext(request));
}

/**
 * Simple auto-moderation logic
 * In production, this would call external APIs (OpenAI, Google Vision, etc.)
 */
function autoModerate(contentText?: string | null, _contentUrl?: string | null): {
  autoResult: Record<string, unknown>;
  autoAction: string;
  autoConfidence: number;
  status: string;
} {
  // Default: content is safe
  let confidence = 0.0;
  const categories: Record<string, number> = {
    violence: 0.0,
    sexual: 0.0,
    hate: 0.0,
    harassment: 0.0,
    self_harm: 0.0,
    spam: 0.0,
  };

  // Simple keyword-based check (placeholder for real API integration)
  if (contentText) {
    const text = contentText.toLowerCase();
    // This is a simplified placeholder. Real implementation would use OpenAI, etc.
    const suspiciousPatterns = ['spam', 'buy now', 'click here', 'free money'];
    for (const pattern of suspiciousPatterns) {
      if (text.includes(pattern)) {
        categories.spam = 0.7;
        confidence = Math.max(confidence, 0.7);
      }
    }
  }

  const autoResult = {
    safe: confidence < config.flagThreshold,
    categories,
    provider: config.provider,
  };

  let autoAction: string;
  let status: string;

  if (confidence <= config.autoApproveBelow) {
    autoAction = 'approve';
    status = 'approved';
  } else if (confidence >= config.autoRejectAbove) {
    autoAction = 'reject';
    status = 'rejected';
  } else if (confidence >= config.flagThreshold) {
    autoAction = 'flag';
    status = 'pending_manual';
  } else {
    autoAction = 'approve';
    status = 'approved';
  }

  return { autoResult, autoAction, autoConfidence: confidence, status };
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return {
    status: 'ok',
    plugin: 'content-moderation',
    timestamp: new Date().toISOString(),
    version: '1.0.0',
  };
});

fastify.get('/ready', async () => {
  try {
    await modDb.getStats();
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
// Review Submission Endpoints
// ============================================================================

fastify.post<{ Body: SubmitReviewRequest }>('/api/review', async (request, reply) => {
  const db = scopedDb(request);
  const body = request.body;

  if (!body.contentType || !body.contentId) {
    reply.code(400);
    throw new Error('contentType and contentId are required');
  }

  const { autoResult, autoAction, autoConfidence, status } = autoModerate(
    body.contentText,
    body.contentUrl,
  );

  const review = await db.createReview(body, autoResult, autoAction, autoConfidence, status);

  return {
    reviewId: review.id,
    status: review.status,
    autoResult,
    action: autoAction,
  };
});

fastify.post<{ Body: BatchReviewRequest }>('/api/review/batch', async (request, reply) => {
  const db = scopedDb(request);
  const { items } = request.body;

  if (!items || !Array.isArray(items)) {
    reply.code(400);
    throw new Error('items array is required');
  }

  const results = [];
  for (const item of items) {
    const { autoResult, autoAction, autoConfidence, status } = autoModerate(
      item.contentText,
      item.contentUrl,
    );

    const review = await db.createReview(
      {
        contentType: item.contentType,
        contentId: item.contentId,
        contentText: item.contentText,
        contentUrl: item.contentUrl,
        authorId: item.authorId,
      },
      autoResult,
      autoAction,
      autoConfidence,
      status,
    );

    results.push({
      reviewId: review.id,
      status: review.status,
      autoResult,
      action: autoAction,
    });
  }

  return { results, total: results.length };
});

// ============================================================================
// Queue Management Endpoints
// ============================================================================

fastify.get<{ Querystring: QueueQuery }>('/api/queue', async (request) => {
  const db = scopedDb(request);
  const { status, contentType, limit, offset, sortBy } = request.query;

  const result = await db.getQueue(
    status || 'pending_manual',
    contentType,
    limit ? parseInt(limit) : 50,
    offset ? parseInt(offset) : 0,
    sortBy || 'oldest',
  );

  return result;
});

fastify.put<{ Params: { id: string }; Body: ManualReviewRequest }>('/api/reviews/:id', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getReviewById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('Review not found');
  }

  const { manualAction, reason, policyViolated, reviewerId } = request.body;

  if (!manualAction) {
    reply.code(400);
    throw new Error('manualAction is required');
  }

  const reviewer = reviewerId || (request.headers['x-reviewer-id'] as string) || 'system';
  const updated = await db.updateReviewDecision(
    request.params.id,
    manualAction,
    reviewer,
    reason,
    policyViolated,
  );

  return updated;
});

// ============================================================================
// Review History Endpoints
// ============================================================================

fastify.get<{ Querystring: ReviewListQuery }>('/api/reviews', async (request) => {
  const db = scopedDb(request);
  const { authorId, status, contentType, from, to, limit, offset } = request.query;

  const result = await db.getReviews({
    authorId,
    status,
    contentType,
    from,
    to,
    limit: limit ? parseInt(limit) : 50,
    offset: offset ? parseInt(offset) : 0,
  });

  return result;
});

fastify.get<{ Params: { id: string } }>('/api/reviews/:id', async (request, reply) => {
  const db = scopedDb(request);
  const review = await db.getReviewById(request.params.id);

  if (!review) {
    reply.code(404);
    throw new Error('Review not found');
  }

  return review;
});

// ============================================================================
// Appeal Endpoints
// ============================================================================

fastify.post<{ Body: SubmitAppealRequest }>('/api/appeals', async (request, reply) => {
  const db = scopedDb(request);
  const { reviewId, reason, appellantId } = request.body;

  if (!reviewId || !reason) {
    reply.code(400);
    throw new Error('reviewId and reason are required');
  }

  const review = await db.getReviewById(reviewId);
  if (!review) {
    reply.code(404);
    throw new Error('Review not found');
  }

  const appellant = appellantId || review.author_id || 'unknown';
  const appeal = await db.createAppeal(reviewId, appellant, reason);

  reply.code(201);
  return appeal;
});

fastify.get<{ Querystring: AppealListQuery }>('/api/appeals', async (request) => {
  const db = scopedDb(request);
  const { status, limit, offset } = request.query;

  const result = await db.getAppeals(
    status,
    limit ? parseInt(limit) : 50,
    offset ? parseInt(offset) : 0,
  );

  return result;
});

fastify.put<{ Params: { id: string }; Body: ResolveAppealRequest }>('/api/appeals/:id', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getAppealById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('Appeal not found');
  }

  const { status, resolution, resolvedBy } = request.body;

  if (!status || !resolution) {
    reply.code(400);
    throw new Error('status and resolution are required');
  }

  const resolver = resolvedBy || (request.headers['x-reviewer-id'] as string) || 'system';
  const resolved = await db.resolveAppeal(request.params.id, status, resolution, resolver);

  // If overturned, update the review status back to approved
  if (status === 'overturned' && existing.review_id) {
    await db.updateReviewDecision(
      existing.review_id,
      'approve',
      resolver,
      `Appeal overturned: ${resolution}`,
    );
  }

  return resolved;
});

// ============================================================================
// Policy Endpoints
// ============================================================================

fastify.post<{ Body: CreatePolicyRequest }>('/api/policies', async (request, reply) => {
  const db = scopedDb(request);
  const body = request.body;

  if (!body.name || !body.rules) {
    reply.code(400);
    throw new Error('name and rules are required');
  }

  const policy = await db.createPolicy(body);
  reply.code(201);
  return policy;
});

fastify.get('/api/policies', async (request) => {
  const db = scopedDb(request);
  const policies = await db.getPolicies();
  return { data: policies, total: policies.length };
});

fastify.get<{ Params: { id: string } }>('/api/policies/:id', async (request, reply) => {
  const db = scopedDb(request);
  const policy = await db.getPolicyById(request.params.id);

  if (!policy) {
    reply.code(404);
    throw new Error('Policy not found');
  }

  return policy;
});

fastify.put<{ Params: { id: string }; Body: UpdatePolicyRequest }>('/api/policies/:id', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getPolicyById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('Policy not found');
  }

  const updated = await db.updatePolicy(request.params.id, request.body);
  return updated;
});

fastify.delete<{ Params: { id: string } }>('/api/policies/:id', async (request, reply) => {
  const db = scopedDb(request);
  await db.deletePolicy(request.params.id);
  reply.code(204);
});

// ============================================================================
// User Moderation Endpoints
// ============================================================================

fastify.get<{ Params: { userId: string } }>('/api/users/:userId/strikes', async (request) => {
  const db = scopedDb(request);
  const strikes = await db.getUserStrikes(request.params.userId);
  return { data: strikes, total: strikes.length };
});

fastify.post<{ Params: { userId: string }; Body: AddStrikeRequest }>('/api/users/:userId/strikes', async (request, reply) => {
  const db = scopedDb(request);
  const body = request.body;

  if (!body.strikeType) {
    reply.code(400);
    throw new Error('strikeType is required');
  }

  const strike = await db.addStrike(request.params.userId, body);

  reply.code(201);
  return strike;
});

fastify.get<{ Params: { userId: string } }>('/api/users/:userId/status', async (request) => {
  const db = scopedDb(request);
  const userId = request.params.userId;

  const totalStrikes = await db.getTotalStrikeCount(userId);
  const activeStrikes = await db.getActiveStrikeCount(userId);
  const isBanned = activeStrikes >= config.strikeBanThreshold;
  const strikes = await db.getUserStrikes(userId);

  const restrictions: string[] = [];
  if (activeStrikes >= config.strikeWarnThreshold) {
    restrictions.push('content_flagged_for_review');
  }
  if (isBanned) {
    restrictions.push('account_suspended');
  }

  return {
    userId,
    totalStrikes,
    activeStrikes,
    isBanned,
    restrictions,
    strikes,
  };
});

// ============================================================================
// Statistics Endpoint
// ============================================================================

fastify.get<{ Querystring: StatsQuery }>('/api/stats', async (request) => {
  const db = scopedDb(request);
  const { from, to } = request.query;
  const stats = await db.getStats(from, to);
  return stats;
});

// ============================================================================
// Server Startup
// ============================================================================

async function start() {
  try {
    await fastify.register(cors, { origin: true });

    const db = createDatabase(config.database);
    await db.connect();
    modDb = new ModerationDatabase(db);

    logger.info('Content moderation database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Content moderation plugin server listening on ${config.host}:${config.port}`);
    logger.info(`Health check: http://${config.host}:${config.port}/health`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start content-moderation server', { error: message });
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
