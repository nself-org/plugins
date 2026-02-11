/**
 * Entitlements Plugin Server
 * HTTP server for subscription plans, feature gating, and quota tracking
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { EntitlementsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreatePlanRequest,
  UpdatePlanRequest,
  CreateSubscriptionRequest,
  UpdateSubscriptionRequest,
  CreateFeatureRequest,
  TrackUsageRequest,
  AddAddonRequest,
  CreateGrantRequest,
  PlanType,
  BillingInterval,
  SubscriptionStatus,
} from './types.js';

const logger = createLogger('entitlements:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new EntitlementsDatabase();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({ logger: false, bodyLimit: 10 * 1024 * 1024 });
  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): EntitlementsDatabase {
    return (request as Record<string, unknown>).scopedDb as EntitlementsDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => ({ status: 'ok', plugin: 'entitlements', timestamp: new Date().toISOString() }));

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'entitlements', timestamp: new Date().toISOString() };
    } catch {
      return reply.status(503).send({ ready: false, plugin: 'entitlements', error: 'Database unavailable' });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { alive: true, plugin: 'entitlements', version: '1.0.0', uptime: process.uptime(), stats, timestamp: new Date().toISOString() };
  });

  // =========================================================================
  // Plans
  // =========================================================================

  app.get('/api/entitlements/plans', async (request) => {
    const { plan_type, billing_interval, is_public, is_archived } = request.query as Record<string, string | undefined>;
    const plans = await scopedDb(request).listPlans({
      plan_type: plan_type as PlanType,
      billing_interval: billing_interval as BillingInterval,
      is_public: is_public !== undefined ? is_public === 'true' : undefined,
      is_archived: is_archived !== undefined ? is_archived === 'true' : undefined,
    });
    return { data: plans };
  });

  app.post('/api/entitlements/plans', async (request, reply) => {
    const body = request.body as CreatePlanRequest;
    if (!body.name || !body.slug || !body.billing_interval || !body.plan_type) {
      return reply.status(400).send({ error: 'name, slug, billing_interval, and plan_type are required' });
    }
    const id = await scopedDb(request).createPlan(body);
    return { success: true, id };
  });

  app.get('/api/entitlements/plans/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const plan = await scopedDb(request).getPlan(id);
    if (!plan) return reply.status(404).send({ error: 'Plan not found' });
    return plan;
  });

  app.get('/api/entitlements/plans/slug/:slug', async (request, reply) => {
    const { slug } = request.params as { slug: string };
    const plan = await scopedDb(request).getPlanBySlug(slug);
    if (!plan) return reply.status(404).send({ error: 'Plan not found' });
    return plan;
  });

  app.put('/api/entitlements/plans/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updatePlan(id, request.body as UpdatePlanRequest);
    if (!updated) return reply.status(404).send({ error: 'Plan not found' });
    return { success: true };
  });

  app.delete('/api/entitlements/plans/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const archived = await scopedDb(request).archivePlan(id);
    if (!archived) return reply.status(404).send({ error: 'Plan not found' });
    return { success: true };
  });

  // =========================================================================
  // Subscriptions
  // =========================================================================

  app.get('/api/entitlements/subscriptions', async (request) => {
    const { workspace_id, user_id, status, plan_id } = request.query as Record<string, string | undefined>;
    const subs = await scopedDb(request).listSubscriptions({
      workspace_id, user_id, status: status as SubscriptionStatus, plan_id,
    });
    return { data: subs };
  });

  app.post('/api/entitlements/subscriptions', async (request, reply) => {
    const body = request.body as CreateSubscriptionRequest;
    if (!body.plan_id) return reply.status(400).send({ error: 'plan_id is required' });
    if (!body.workspace_id && !body.user_id) return reply.status(400).send({ error: 'workspace_id or user_id is required' });
    try {
      const id = await scopedDb(request).createSubscription(body, fullConfig.defaultTrialDays);
      return { success: true, id };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/api/entitlements/subscriptions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const sub = await scopedDb(request).getSubscription(id);
    if (!sub) return reply.status(404).send({ error: 'Subscription not found' });
    return sub;
  });

  app.put('/api/entitlements/subscriptions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateSubscription(id, request.body as UpdateSubscriptionRequest);
    if (!updated) return reply.status(404).send({ error: 'Subscription not found' });
    return { success: true };
  });

  app.post('/api/entitlements/subscriptions/:id/cancel', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { reason, immediate } = request.body as { reason?: string; immediate?: boolean };
    const canceled = await scopedDb(request).cancelSubscription(id, reason, immediate);
    if (!canceled) return reply.status(404).send({ error: 'Subscription not found' });
    return { success: true };
  });

  app.post('/api/entitlements/subscriptions/:id/pause', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { resume_at } = request.body as { resume_at?: string };
    const paused = await scopedDb(request).pauseSubscription(id, resume_at ? new Date(resume_at) : undefined);
    if (!paused) return reply.status(404).send({ error: 'Subscription not found' });
    return { success: true };
  });

  app.post('/api/entitlements/subscriptions/:id/resume', async (request, reply) => {
    const { id } = request.params as { id: string };
    const resumed = await scopedDb(request).resumeSubscription(id);
    if (!resumed) return reply.status(404).send({ error: 'Subscription not found' });
    return { success: true };
  });

  // =========================================================================
  // Feature Access
  // =========================================================================

  app.get('/api/entitlements/features', async (request) => {
    const { category, is_active } = request.query as { category?: string; is_active?: string };
    const features = await scopedDb(request).listFeatures(category, is_active !== undefined ? is_active === 'true' : undefined);
    return { data: features };
  });

  app.post('/api/entitlements/features', async (request, reply) => {
    const body = request.body as CreateFeatureRequest;
    if (!body.key || !body.name || !body.feature_type) {
      return reply.status(400).send({ error: 'key, name, and feature_type are required' });
    }
    const id = await scopedDb(request).createFeature(body);
    return { success: true, id };
  });

  app.get('/api/entitlements/features/:key/check', async (request) => {
    const { key } = request.params as { key: string };
    const { workspaceId, userId } = request.query as { workspaceId?: string; userId?: string };
    const result = await scopedDb(request).checkFeatureAccess(key, workspaceId, userId);
    return result;
  });

  // =========================================================================
  // Quotas
  // =========================================================================

  app.get('/api/entitlements/quotas', async (request) => {
    const { workspace_id, user_id, subscription_id, quota_key } = request.query as Record<string, string | undefined>;
    const quotas = await scopedDb(request).listQuotas({ workspace_id, user_id, subscription_id, quota_key });
    return { data: quotas };
  });

  app.get('/api/entitlements/quotas/:key/check', async (request) => {
    const { key } = request.params as { key: string };
    const { workspaceId, userId, amount } = request.query as { workspaceId?: string; userId?: string; amount?: string };
    const result = await scopedDb(request).checkQuotaAvailability(key, amount ? Number(amount) : 1, workspaceId, userId);
    return result;
  });

  app.post('/api/entitlements/quotas/:id/reset', async (request, reply) => {
    const { id } = request.params as { id: string };
    const reset = await scopedDb(request).resetQuota(id);
    if (!reset) return reply.status(404).send({ error: 'Quota not found' });
    return { success: true };
  });

  // =========================================================================
  // Usage Tracking
  // =========================================================================

  app.post('/api/entitlements/usage/track', async (request, reply) => {
    const body = request.body as TrackUsageRequest;
    if (!body.quota_key) return reply.status(400).send({ error: 'quota_key is required' });
    const result = await scopedDb(request).trackUsage(body);
    return result;
  });

  // =========================================================================
  // Addons
  // =========================================================================

  app.get('/api/entitlements/subscriptions/:id/addons', async (request) => {
    const { id } = request.params as { id: string };
    const addons = await scopedDb(request).listAddons(id);
    return { data: addons };
  });

  app.post('/api/entitlements/subscriptions/:id/addons', async (request, reply) => {
    const { id: subscription_id } = request.params as { id: string };
    const body = request.body as Omit<AddAddonRequest, 'subscription_id'>;
    if (!body.addon_plan_id) return reply.status(400).send({ error: 'addon_plan_id is required' });
    try {
      const addonId = await scopedDb(request).addAddon({ ...body, subscription_id });
      return { success: true, id: addonId };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(400).send({ error: message });
    }
  });

  app.delete('/api/entitlements/addons/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const removed = await scopedDb(request).removeAddon(id);
    if (!removed) return reply.status(404).send({ error: 'Addon not found' });
    return { success: true };
  });

  // =========================================================================
  // Grants
  // =========================================================================

  app.get('/api/entitlements/grants', async (request) => {
    const { workspace_id, user_id, feature_key, is_active } = request.query as Record<string, string | undefined>;
    const grants = await scopedDb(request).listGrants({
      workspace_id, user_id, feature_key,
      is_active: is_active !== undefined ? is_active === 'true' : undefined,
    });
    return { data: grants };
  });

  app.post('/api/entitlements/grants', async (request, reply) => {
    const body = request.body as CreateGrantRequest;
    if (!body.feature_key || body.feature_value === undefined) {
      return reply.status(400).send({ error: 'feature_key and feature_value are required' });
    }
    const id = await scopedDb(request).createGrant(body);
    return { success: true, id };
  });

  app.delete('/api/entitlements/grants/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const revoked = await scopedDb(request).revokeGrant(id);
    if (!revoked) return reply.status(404).send({ error: 'Grant not found' });
    return { success: true };
  });

  // =========================================================================
  // Events
  // =========================================================================

  app.get('/api/entitlements/events', async (request) => {
    const { workspace_id, user_id, event_type, subscription_id, limit = '100', offset = '0' } = request.query as Record<string, string | undefined>;
    const events = await scopedDb(request).listEvents(Number(limit), Number(offset), { workspace_id, user_id, event_type, subscription_id });
    return { data: events };
  });

  // =========================================================================
  // Stats / Status
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { plugin: 'entitlements', version: '1.0.0', status: 'running', stats, timestamp: new Date().toISOString() };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return { app, db, config: fullConfig, shutdown };
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const { app, config: fullConfig } = await createServer(config);
  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.success(`Entitlements plugin listening on ${fullConfig.host}:${fullConfig.port}`);
}
