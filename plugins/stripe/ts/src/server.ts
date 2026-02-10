/**
 * Stripe Plugin Server
 * HTTP server for webhooks and API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, verifyStripeSignature, ApiRateLimiter, createAuthHook, createRateLimitHook } from '@nself/plugin-utils';
import { StripeDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { createStripeAccountContexts, runStripeAccountSync, runStripeAccountReconcile } from './account-sync.js';

const logger = createLogger('stripe:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new StripeDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();
  const accountContexts = createStripeAccountContexts(fullConfig, db);
  const primaryContext = accountContexts[0];

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large webhook payloads
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

  // Raw body parser for webhook signature verification
  app.addContentTypeParser('application/json', { parseAs: 'string' }, (req, body, done) => {
    try {
      const json = JSON.parse(body as string);
      (req as unknown as { rawBody: string }).rawBody = body as string;
      done(null, json);
    } catch (err) {
      done(err as Error, undefined);
    }
  });

  // Health check endpoint (basic liveness)
  app.get('/health', async () => {
    return { status: 'ok', plugin: 'stripe', timestamp: new Date().toISOString() };
  });

  // Readiness check (verifies database connectivity)
  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'stripe', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'stripe',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // Liveness check (application state with sync info)
  app.get('/live', async () => {
    const stats = await db.getStats();
    return {
      alive: true,
      plugin: 'stripe',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        customers: stats.customers,
        subscriptions: stats.subscriptions,
        lastSync: stats.lastSyncedAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // Status endpoint
  app.get('/status', async () => {
    const stats = await db.getStats();
    return {
      plugin: 'stripe',
      version: '1.0.0',
      status: 'running',
      accounts: accountContexts.map(context => context.account.id),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // Webhook endpoint
  app.post('/webhooks/stripe', async (request, reply) => {
    const signature = request.headers['stripe-signature'] as string | undefined;
    const rawBody = (request as unknown as { rawBody: string }).rawBody;

    if (!signature) {
      logger.warn('Missing Stripe signature header');
      return reply.status(400).send({ error: 'Missing signature' });
    }

    const contextsWithSecrets = accountContexts.filter(context => Boolean(context.account.webhookSecret));
    const matchedContext = contextsWithSecrets.length > 0
      ? contextsWithSecrets.find(context => verifyStripeSignature(rawBody, signature, context.account.webhookSecret))
      : primaryContext;

    if (!matchedContext) {
      logger.warn('Invalid Stripe signature for all configured accounts');
      return reply.status(401).send({ error: 'Invalid signature' });
    }

    try {
      if (!matchedContext.account.webhookSecret) {
        logger.warn('Stripe webhook secret is not configured for matched account');
        return reply.status(400).send({ error: 'Webhook secret not configured' });
      }

      const event = matchedContext.client.constructEvent(rawBody, signature, matchedContext.account.webhookSecret);
      await matchedContext.webhookHandler.handle(event);
      return { received: true, account: matchedContext.account.id };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  // Sync endpoint
  app.post('/sync', async (request, reply) => {
    const { resources, incremental, accounts } = request.body as {
      resources?: string[];
      incremental?: boolean;
      accounts?: string[];
    };

    try {
      const selectedContexts = Array.isArray(accounts) && accounts.length > 0
        ? accountContexts.filter(context => accounts.includes(context.account.id))
        : accountContexts;

      if (selectedContexts.length === 0) {
        return reply.status(400).send({
          error: 'No matching accounts selected',
          availableAccounts: accountContexts.map(context => context.account.id),
        });
      }

      const result = await runStripeAccountSync(selectedContexts, {
        resources: resources as Array<'customers' | 'products' | 'prices' | 'subscriptions' | 'invoices' | 'payment_intents' | 'payment_methods'>,
        incremental,
      });
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Reconcile endpoint
  app.post('/reconcile', async (request, reply) => {
    const { lookbackDays = 7, accounts } = request.body as {
      lookbackDays?: number;
      accounts?: string[];
    };

    try {
      const selectedContexts = Array.isArray(accounts) && accounts.length > 0
        ? accountContexts.filter(context => accounts.includes(context.account.id))
        : accountContexts;

      if (selectedContexts.length === 0) {
        return reply.status(400).send({
          error: 'No matching accounts selected',
          availableAccounts: accountContexts.map(context => context.account.id),
        });
      }

      const result = await runStripeAccountReconcile(selectedContexts, lookbackDays);
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reconciliation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // API endpoints for querying synced data
  app.get('/api/customers', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const customers = await db.listCustomers(limit, offset);
    const total = await db.countCustomers();
    return { data: customers, total, limit, offset };
  });

  app.get('/api/customers/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const customer = await db.getCustomer(id);
    if (!customer) {
      return reply.status(404).send({ error: 'Customer not found' });
    }
    return customer;
  });

  app.get('/api/products', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const products = await db.listProducts(limit, offset);
    const total = await db.countProducts();
    return { data: products, total, limit, offset };
  });

  app.get('/api/products/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const product = await db.getProduct(id);
    if (!product) {
      return reply.status(404).send({ error: 'Product not found' });
    }
    return product;
  });

  app.get('/api/prices', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const prices = await db.listPrices(limit, offset);
    const total = await db.countPrices();
    return { data: prices, total, limit, offset };
  });

  app.get('/api/prices/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const price = await db.getPrice(id);
    if (!price) {
      return reply.status(404).send({ error: 'Price not found' });
    }
    return price;
  });

  app.get('/api/subscriptions', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number;
      offset?: number;
      status?: string;
    };
    const subscriptions = await db.listSubscriptions(limit, offset);
    const total = await db.countSubscriptions(status);
    return { data: subscriptions, total, limit, offset };
  });

  app.get('/api/subscriptions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const subscription = await db.getSubscription(id);
    if (!subscription) {
      return reply.status(404).send({ error: 'Subscription not found' });
    }
    return subscription;
  });

  app.get('/api/invoices', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number;
      offset?: number;
      status?: string;
    };
    const invoices = await db.listInvoices(limit, offset);
    const total = await db.countInvoices(status);
    return { data: invoices, total, limit, offset };
  });

  app.get('/api/invoices/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const invoice = await db.getInvoice(id);
    if (!invoice) {
      return reply.status(404).send({ error: 'Invoice not found' });
    }
    return invoice;
  });

  // Charges
  app.get('/api/charges', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const charges = await db.listCharges(limit, offset);
    const total = await db.countCharges();
    return { data: charges, total, limit, offset };
  });

  app.get('/api/charges/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const charge = await db.getCharge(id);
    if (!charge) {
      return reply.status(404).send({ error: 'Charge not found' });
    }
    return charge;
  });

  // Refunds
  app.get('/api/refunds', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const refunds = await db.listRefunds(limit, offset);
    const total = await db.countRefunds();
    return { data: refunds, total, limit, offset };
  });

  app.get('/api/refunds/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const refund = await db.getRefund(id);
    if (!refund) {
      return reply.status(404).send({ error: 'Refund not found' });
    }
    return refund;
  });

  // Disputes
  app.get('/api/disputes', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const disputes = await db.listDisputes(limit, offset);
    const total = await db.countDisputes();
    return { data: disputes, total, limit, offset };
  });

  app.get('/api/disputes/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const dispute = await db.getDispute(id);
    if (!dispute) {
      return reply.status(404).send({ error: 'Dispute not found' });
    }
    return dispute;
  });

  // Payment Intents
  app.get('/api/payment-intents', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const paymentIntents = await db.listPaymentIntents(limit, offset);
    const total = await db.countPaymentIntents();
    return { data: paymentIntents, total, limit, offset };
  });

  app.get('/api/payment-intents/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const paymentIntent = await db.getPaymentIntent(id);
    if (!paymentIntent) {
      return reply.status(404).send({ error: 'Payment intent not found' });
    }
    return paymentIntent;
  });

  // Payment Methods
  app.get('/api/payment-methods', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const paymentMethods = await db.listPaymentMethods(limit, offset);
    const total = await db.countPaymentMethods();
    return { data: paymentMethods, total, limit, offset };
  });

  app.get('/api/payment-methods/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const paymentMethod = await db.getPaymentMethod(id);
    if (!paymentMethod) {
      return reply.status(404).send({ error: 'Payment method not found' });
    }
    return paymentMethod;
  });

  // Coupons
  app.get('/api/coupons', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const coupons = await db.listCoupons(limit, offset);
    const total = await db.countCoupons();
    return { data: coupons, total, limit, offset };
  });

  app.get('/api/coupons/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const coupon = await db.getCoupon(id);
    if (!coupon) {
      return reply.status(404).send({ error: 'Coupon not found' });
    }
    return coupon;
  });

  // Promotion Codes
  app.get('/api/promotion-codes', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const promotionCodes = await db.listPromotionCodes(limit, offset);
    const total = await db.countPromotionCodes();
    return { data: promotionCodes, total, limit, offset };
  });

  app.get('/api/promotion-codes/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const promotionCode = await db.getPromotionCode(id);
    if (!promotionCode) {
      return reply.status(404).send({ error: 'Promotion code not found' });
    }
    return promotionCode;
  });

  // Balance Transactions
  app.get('/api/balance-transactions', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const transactions = await db.listBalanceTransactions(limit, offset);
    const total = await db.countBalanceTransactions();
    return { data: transactions, total, limit, offset };
  });

  app.get('/api/balance-transactions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const transaction = await db.getBalanceTransaction(id);
    if (!transaction) {
      return reply.status(404).send({ error: 'Balance transaction not found' });
    }
    return transaction;
  });

  // Tax Rates
  app.get('/api/tax-rates', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const taxRates = await db.listTaxRates(limit, offset);
    const total = await db.countTaxRates();
    return { data: taxRates, total, limit, offset };
  });

  app.get('/api/tax-rates/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const taxRate = await db.getTaxRate(id);
    if (!taxRate) {
      return reply.status(404).send({ error: 'Tax rate not found' });
    }
    return taxRate;
  });

  // Webhook Events
  app.get('/api/events', async (request) => {
    const { limit = 100, offset = 0, type } = request.query as {
      limit?: number;
      offset?: number;
      type?: string;
    };
    const events = await db.listWebhookEvents(type, limit, offset);
    return { data: events, limit, offset };
  });

  // Stats endpoint
  app.get('/api/stats', async () => {
    return await db.getStats();
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    accountContexts,
    client: primaryContext.client,
    syncService: primaryContext.syncService,
    webhookHandler: primaryContext.webhookHandler,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Stripe plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Webhook endpoint: http://${fullConfig.host}:${fullConfig.port}/webhooks/stripe`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
