/**
 * PayPal Plugin Server
 * Fastify HTTP server with REST API and webhook handling
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { loadConfig, type Config } from './config.js';
import { createPayPalDatabase, PayPalDatabase } from './database.js';
import {
  createPayPalAccountContexts,
  runPayPalAccountSync,
  runPayPalAccountReconcile,
  type PayPalAccountContext,
} from './account-sync.js';
import type { PayPalWebhookEvent } from './types.js';

const logger = createLogger('paypal:server');

export async function createServer(configOverrides?: Partial<Config>) {
  const config = loadConfig(configOverrides);
  const db = createPayPalDatabase({
    host: config.databaseHost,
    port: config.databasePort,
    database: config.databaseName,
    user: config.databaseUser,
    password: config.databasePassword,
    ssl: config.databaseSsl,
  });

  await db.connect();
  await db.initializeSchema();

  const contexts = createPayPalAccountContexts(config, db);

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  await app.register(cors, { origin: true });

  // Disconnect DB when server closes (ensures event loop drains cleanly in tests)
  app.addHook('onClose', async () => {
    await db.disconnect();
  });

  // Raw body for webhook signature verification
  app.addContentTypeParser('application/json', { parseAs: 'string' }, (_req, body, done) => {
    done(null, body);
  });

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped PayPalDatabase from request */
  function scopedDb(request: unknown): PayPalDatabase {
    return (request as Record<string, unknown>).scopedDb as PayPalDatabase;
  }

  // ─── Health Endpoints ──────────────────────────────────────────────────

  app.get('/health', async () => ({ status: 'ok', plugin: 'paypal', version: '1.0.0' }));
  app.get('/ready', async () => ({ ready: true }));
  app.get('/live', async () => ({ live: true }));

  // ─── Status ────────────────────────────────────────────────────────────

  app.get('/status', async (request) => {
    const sdb = scopedDb(request);
    const stats = await sdb.getStats();
    return {
      plugin: 'paypal',
      version: '1.0.0',
      environment: config.environment,
      accounts: contexts.map(c => ({
        id: c.account.id,
        hasWebhookId: !!c.account.webhookId,
      })),
      stats,
    };
  });

  // ─── Sync Endpoints ───────────────────────────────────────────────────

  app.post('/sync', async (request) => {
    const body = typeof request.body === 'string' ? JSON.parse(request.body) : request.body;
    const options = (body as Record<string, unknown>) ?? {};
    const selectedContexts = filterContexts(contexts, options.accounts as string[] | undefined);
    const result = await runPayPalAccountSync(selectedContexts, config, {
      incremental: options.incremental as boolean | undefined,
    });
    return result;
  });

  app.post('/reconcile', async (request) => {
    const body = typeof request.body === 'string' ? JSON.parse(request.body) : request.body;
    const options = (body as Record<string, unknown>) ?? {};
    const lookbackDays = (options.lookbackDays as number) ?? 7;
    const selectedContexts = filterContexts(contexts, options.accounts as string[] | undefined);
    const result = await runPayPalAccountReconcile(selectedContexts, config, lookbackDays);
    return result;
  });

  // ─── Webhook Endpoint ─────────────────────────────────────────────────

  app.post('/webhooks/paypal', async (request, reply) => {
    const rawBody = request.body as string;
    const headers = request.headers as Record<string, string>;

    // Try to find the matching account by verifying signature
    let matchedContext: PayPalAccountContext | null = null;

    for (const context of contexts) {
      if (!context.account.webhookId) continue;

      const isValid = await context.client.verifyWebhookSignature(
        context.account.webhookId,
        headers,
        rawBody
      );

      if (isValid) {
        matchedContext = context;
        break;
      }
    }

    // If no verification succeeded but we have contexts without webhook IDs, use first context
    if (!matchedContext) {
      const hasAnyWebhookId = contexts.some(c => c.account.webhookId);
      if (hasAnyWebhookId) {
        logger.warn('Webhook signature verification failed for all accounts');
        return reply.status(401).send({ error: 'Invalid webhook signature' });
      }
      matchedContext = contexts[0];
      logger.warn('No webhook IDs configured; skipping verification');
    }

    try {
      const event = JSON.parse(rawBody) as PayPalWebhookEvent;
      await matchedContext.webhookHandler.handleEvent(event);
      return reply.status(200).send({ received: true, processed: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  // ─── API Endpoints ────────────────────────────────────────────────────

  app.get('/api/transactions', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.queryTransactions({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
      status: query.status,
    });
  });

  app.get('/api/orders', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.queryOrders({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
    });
  });

  app.get('/api/subscriptions', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.querySubscriptions({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
      status: query.status,
    });
  });

  app.get('/api/disputes', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.queryDisputes({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
    });
  });

  app.get('/api/refunds', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.queryRefunds({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
    });
  });

  app.get('/api/stats', async (request) => scopedDb(request).getStats());

  app.get('/api/events', async (request) => {
    const sdb = scopedDb(request);
    const query = request.query as Record<string, string>;
    return sdb.queryWebhookEvents({ limit: query.limit ? parseInt(query.limit, 10) : 50 });
  });

  // ─── Start Server ─────────────────────────────────────────────────────

  const address = await app.listen({ port: config.port, host: config.host });
  logger.success(`PayPal plugin server listening on ${address}`);
  logger.info(`Accounts: ${contexts.map(c => c.account.id).join(', ')}`);
  logger.info(`Environment: ${config.environment}`);

  return app;
}

function filterContexts(
  contexts: PayPalAccountContext[],
  accounts?: string[]
): PayPalAccountContext[] {
  if (!accounts || accounts.length === 0) return contexts;
  return contexts.filter(c => accounts.includes(c.account.id));
}

// Auto-start when run directly
const isMain = process.argv[1]?.endsWith('server.js') || process.argv[1]?.endsWith('server.ts');
if (isMain) {
  createServer().catch(err => {
    logger.error('Failed to start server', { error: err instanceof Error ? err.message : String(err) });
    process.exit(1);
  });
}
