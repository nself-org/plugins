/**
 * Donorbox Plugin Server
 * Fastify HTTP server with REST API and webhook handling
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { loadConfig, type Config } from './config.js';
import { createDonorboxDatabase, DonorboxDatabase } from './database.js';
import {
  createDonorboxAccountContexts,
  runDonorboxAccountSync,
  runDonorboxAccountReconcile,
  type DonorboxAccountContext,
} from './account-sync.js';
import { DonorboxWebhookHandler } from './webhooks.js';

const logger = createLogger('donorbox:server');

export async function createServer(configOverrides?: Partial<Config>) {
  const config = loadConfig(configOverrides);
  const db = createDonorboxDatabase({
    host: config.databaseHost,
    port: config.databasePort,
    database: config.databaseName,
    user: config.databaseUser,
    password: config.databasePassword,
    ssl: config.databaseSsl,
  });

  await db.connect();
  await db.initializeSchema();

  const contexts = createDonorboxAccountContexts(config, db);

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  await app.register(cors, { origin: true });

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

  /** Extract scoped DonorboxDatabase from request */
  function scopedDb(request: unknown): DonorboxDatabase {
    return (request as Record<string, unknown>).scopedDb as DonorboxDatabase;
  }

  // ─── Health Endpoints ──────────────────────────────────────────────────

  app.get('/health', async () => ({ status: 'ok', plugin: 'donorbox', version: '1.0.0' }));
  app.get('/ready', async () => ({ ready: true }));
  app.get('/live', async () => ({ live: true }));

  // ─── Status ────────────────────────────────────────────────────────────

  app.get('/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'donorbox',
      version: '1.0.0',
      accounts: contexts.map(c => ({ id: c.account.id })),
      stats,
    };
  });

  // ─── Sync Endpoints ───────────────────────────────────────────────────

  app.post('/sync', async (request) => {
    const body = typeof request.body === 'string' ? JSON.parse(request.body) : request.body;
    const options = (body as Record<string, unknown>) ?? {};
    const selectedContexts = filterContexts(contexts, options.accounts as string[] | undefined);
    const result = await runDonorboxAccountSync(selectedContexts, {
      incremental: options.incremental as boolean | undefined,
    });
    return result;
  });

  app.post('/reconcile', async (request) => {
    const body = typeof request.body === 'string' ? JSON.parse(request.body) : request.body;
    const options = (body as Record<string, unknown>) ?? {};
    const lookbackDays = (options.lookbackDays as number) ?? 7;
    const selectedContexts = filterContexts(contexts, options.accounts as string[] | undefined);
    const result = await runDonorboxAccountReconcile(selectedContexts, lookbackDays);
    return result;
  });

  // ─── Webhook Endpoint ─────────────────────────────────────────────────

  app.post('/webhooks/donorbox', async (request, reply) => {
    const rawBody = request.body as string;
    const signatureHeader = request.headers['donorbox-signature'] as string | undefined;

    // Find matching account by signature verification
    let matchedContext: DonorboxAccountContext | null = null;

    for (const context of contexts) {
      if (!context.account.webhookSecret) continue;

      const isValid = DonorboxWebhookHandler.verifySignature(
        rawBody,
        signatureHeader ?? '',
        context.account.webhookSecret
      );

      if (isValid) {
        matchedContext = context;
        break;
      }
    }

    if (!matchedContext) {
      const hasAnySecret = contexts.some(c => c.account.webhookSecret);
      if (hasAnySecret) {
        logger.warn('Webhook signature verification failed for all accounts');
        return reply.status(401).send({ error: 'Invalid webhook signature' });
      }
      matchedContext = contexts[0];
      logger.warn('No webhook secrets configured; skipping verification');
    }

    try {
      const payload = JSON.parse(rawBody) as Record<string, unknown>;
      const eventType = (payload.type ?? payload.event ?? 'donation.created') as string;
      await matchedContext.webhookHandler.handleEvent(eventType, payload);
      return reply.status(200).send({ received: true, processed: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  // ─── API Endpoints ────────────────────────────────────────────────────

  app.get('/api/campaigns', async (request) => {
    const query = request.query as Record<string, string>;
    return scopedDb(request).queryCampaigns({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
    });
  });

  app.get('/api/donors', async (request) => {
    const query = request.query as Record<string, string>;
    return scopedDb(request).queryDonors({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
    });
  });

  app.get('/api/donations', async (request) => {
    const query = request.query as Record<string, string>;
    return scopedDb(request).queryDonations({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
      status: query.status,
    });
  });

  app.get('/api/plans', async (request) => {
    const query = request.query as Record<string, string>;
    return scopedDb(request).queryPlans({
      limit: query.limit ? parseInt(query.limit, 10) : 100,
      offset: query.offset ? parseInt(query.offset, 10) : 0,
      status: query.status,
    });
  });

  app.get('/api/stats', async (request) => scopedDb(request).getStats());

  app.get('/api/events', async (request) => {
    const query = request.query as Record<string, string>;
    return scopedDb(request).queryWebhookEvents({ limit: query.limit ? parseInt(query.limit, 10) : 50 });
  });

  // ─── Start Server ─────────────────────────────────────────────────────

  const address = await app.listen({ port: config.port, host: config.host });
  logger.success(`Donorbox plugin server listening on ${address}`);
  logger.info(`Accounts: ${contexts.map(c => c.account.id).join(', ')}`);

  return app;
}

function filterContexts(
  contexts: DonorboxAccountContext[],
  accounts?: string[]
): DonorboxAccountContext[] {
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
