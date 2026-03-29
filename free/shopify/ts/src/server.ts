/**
 * Shopify Plugin Server
 * Fastify server for webhook handling and API endpoints
 */

import Fastify, { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import cors from '@fastify/cors';
import crypto from 'crypto';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ShopifyConfig } from './config.js';
import { ShopifyClient } from './client.js';
import { ShopifyDatabase } from './database.js';
import { ShopifySyncService } from './sync.js';
import { ShopifyWebhookHandler, WebhookPayload } from './webhooks.js';

const logger = createLogger('shopify:server');

export interface ShopifyServer {
  app: FastifyInstance;
  start: () => Promise<void>;
  stop: () => Promise<void>;
}

export async function createServer(config: ShopifyConfig): Promise<ShopifyServer> {
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large payloads
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    methods: ['GET', 'POST', 'PUT', 'DELETE'],
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    config.security.rateLimitMax ?? 100,
    config.security.rateLimitWindowMs ?? 60000
  );

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (config.security.apiKey) {
    app.addHook('preHandler', createAuthHook(config.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Initialize services
  const client = new ShopifyClient(
    config.shopifyShopDomain,
    config.shopifyAccessToken,
    config.shopifyApiVersion
  );
  const db = new ShopifyDatabase();
  await db.connect();
  await db.initializeSchema();

  const syncService = new ShopifySyncService(client, db);
  const webhookHandler = new ShopifyWebhookHandler(client, db, syncService);

  // Add scoped database middleware
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(req: unknown): ShopifyDatabase {
    return (req as unknown as Record<string, unknown>).scopedDb as ShopifyDatabase;
  }

  // Raw body parser for webhook signature verification
  app.addContentTypeParser(
    'application/json',
    { parseAs: 'buffer' },
    (req, body, done) => {
      try {
        const rawBody = Buffer.isBuffer(body) ? body : Buffer.from(body);
        const json = JSON.parse(rawBody.toString());
        (req as unknown as { rawBody: Buffer }).rawBody = rawBody;
        done(null, json);
      } catch (err) {
        done(err as Error, undefined);
      }
    }
  );

  // Health check endpoint (basic liveness)
  app.get('/health', async () => {
    return { status: 'ok', plugin: 'shopify', timestamp: new Date().toISOString() };
  });

  // Readiness check (verifies database connectivity)
  app.get('/ready', async (request: FastifyRequest, reply: FastifyReply) => {
    try {
      await scopedDb(request).execute('SELECT 1');
      return { ready: true, plugin: 'shopify', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'shopify',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // Liveness check (application state with sync info)
  app.get('/live', async (request: FastifyRequest) => {
    const stats = await scopedDb(request).getStats();
    const shop = await scopedDb(request).getShop();
    return {
      alive: true,
      plugin: 'shopify',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      shop: shop ? { name: shop.name, domain: shop.domain } : null,
      stats: {
        products: stats.products,
        customers: stats.customers,
        orders: stats.orders,
        lastSync: stats.lastSyncedAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // Webhook endpoint
  app.post('/webhook', async (request: FastifyRequest, reply: FastifyReply) => {
    const topic = request.headers['x-shopify-topic'] as string;
    const shopDomain = request.headers['x-shopify-shop-domain'] as string;
    const hmacHeader = request.headers['x-shopify-hmac-sha256'] as string;
    const webhookId = request.headers['x-shopify-webhook-id'] as string || crypto.randomUUID();

    if (!topic) {
      logger.warn('Webhook missing topic header');
      return reply.status(400).send({ error: 'Missing X-Shopify-Topic header' });
    }

    if (!shopDomain) {
      logger.warn('Webhook missing shop domain header');
      return reply.status(400).send({ error: 'Missing X-Shopify-Shop-Domain header' });
    }

    // Verify webhook signature if secret is configured
    if (config.shopifyWebhookSecret && hmacHeader) {
      const rawBody = (request as unknown as { rawBody: Buffer }).rawBody;
      const computedHmac = crypto
        .createHmac('sha256', config.shopifyWebhookSecret)
        .update(rawBody)
        .digest('base64');

      if (computedHmac !== hmacHeader) {
        logger.warn('Webhook signature verification failed', { topic, shopDomain });
        return reply.status(401).send({ error: 'Invalid signature' });
      }
    }

    const payload = request.body as WebhookPayload;

    try {
      await webhookHandler.handle(webhookId, topic, shopDomain, payload);
      return reply.status(200).send({ received: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { topic, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // API Endpoints
  // =========================================================================

  // Trigger sync
  app.post('/api/sync', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = request.body as { resources?: string[] } | undefined;
    const resources = body?.resources as Array<'shop' | 'products' | 'collections' | 'customers' | 'orders' | 'inventory'> | undefined;

    try {
      const result = await syncService.sync({ resources });
      return reply.send(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Get sync status
  app.get('/api/status', async (request: FastifyRequest, reply: FastifyReply) => {
    try {
      const stats = await scopedDb(request).getStats();
      const shop = await scopedDb(request).getShop();
      return reply.send({
        shop: shop ? { name: shop.name, domain: shop.domain } : null,
        stats,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Shop endpoint
  app.get('/api/shop', async (request: FastifyRequest, reply: FastifyReply) => {
    try {
      const shop = await scopedDb(request).getShop();
      return reply.send({ shop });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Products endpoints
  app.get('/api/products', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; offset?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);

    try {
      const products = await scopedDb(request).listProducts(limit, offset);
      const total = await scopedDb(request).countProducts();
      return reply.send({ products, total, limit, offset });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/products/:id', async (request: FastifyRequest, reply: FastifyReply) => {
    const params = request.params as { id: string };
    const productId = parseInt(params.id, 10);

    try {
      const product = await scopedDb(request).getProduct(productId);
      if (!product) {
        return reply.status(404).send({ error: 'Product not found' });
      }
      const variants = await scopedDb(request).getProductVariants(productId);
      return reply.send({ product, variants });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Customers endpoints
  app.get('/api/customers', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; offset?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);

    try {
      const customers = await scopedDb(request).listCustomers(limit, offset);
      const total = await scopedDb(request).countCustomers();
      return reply.send({ customers, total, limit, offset });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/customers/:id', async (request: FastifyRequest, reply: FastifyReply) => {
    const params = request.params as { id: string };
    const customerId = parseInt(params.id, 10);

    try {
      const customer = await scopedDb(request).getCustomer(customerId);
      if (!customer) {
        return reply.status(404).send({ error: 'Customer not found' });
      }
      return reply.send({ customer });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Orders endpoints
  app.get('/api/orders', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; offset?: string; status?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);

    try {
      const orders = await scopedDb(request).listOrders(query.status, limit, offset);
      const total = await scopedDb(request).countOrders(query.status);
      return reply.send({ orders, total, limit, offset });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/orders/:id', async (request: FastifyRequest, reply: FastifyReply) => {
    const params = request.params as { id: string };
    const orderId = parseInt(params.id, 10);

    try {
      const order = await scopedDb(request).getOrder(orderId);
      if (!order) {
        return reply.status(404).send({ error: 'Order not found' });
      }
      const items = await scopedDb(request).getOrderItems(orderId);
      return reply.send({ order, items });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Collections endpoints
  app.get('/api/collections', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; offset?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);

    try {
      const collections = await db.listCollections(limit, offset);
      const total = await db.countCollections();
      return reply.send({ collections, total, limit, offset });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Inventory endpoints
  app.get('/api/inventory', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; offset?: string };
    const limit = parseInt(query.limit ?? '100', 10);
    const offset = parseInt(query.offset ?? '0', 10);

    try {
      const inventory = await db.listInventory(limit, offset);
      const total = await db.countInventory();
      return reply.send({ inventory, total, limit, offset });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Webhook events endpoint
  app.get('/api/webhook-events', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string; topic?: string };
    const limit = parseInt(query.limit ?? '50', 10);

    try {
      const events = await db.listWebhookEvents(query.topic, limit);
      return reply.send({ events });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Analytics endpoints
  app.get('/api/analytics/daily-sales', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { days?: string };
    const days = Math.min(Math.max(parseInt(query.days ?? '30', 10), 1), 365); // Clamp to 1-365 days

    try {
      const result = await db.execute(
        `SELECT * FROM shopify_sales_overview
         WHERE order_date >= CURRENT_DATE - INTERVAL '1 day' * $1
         ORDER BY order_date DESC`,
        [days]
      );
      return reply.send({ dailySales: result });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/analytics/top-products', async (request: FastifyRequest, reply: FastifyReply) => {
    const query = request.query as { limit?: string };
    const limit = parseInt(query.limit ?? '10', 10);

    try {
      const result = await db.query(
        `SELECT * FROM shopify_top_products LIMIT $1`,
        [limit]
      );
      return reply.send({ topProducts: result.rows });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/analytics/customer-value', async (_request: FastifyRequest, reply: FastifyReply) => {
    try {
      const result = await db.query('SELECT * FROM shopify_customer_value LIMIT 100');
      return reply.send({ customers: result.rows });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  const start = async (): Promise<void> => {
    try {
      await app.listen({ port: config.port, host: config.host });
      logger.success(`Shopify plugin server running on ${config.host}:${config.port}`);
      logger.info('Endpoints:');
      logger.info('  POST /webhook - Receive Shopify webhooks');
      logger.info('  POST /api/sync - Trigger data sync');
      logger.info('  GET  /api/status - Get sync status');
      logger.info('  GET  /api/shop - Get shop info');
      logger.info('  GET  /api/products - List products');
      logger.info('  GET  /api/customers - List customers');
      logger.info('  GET  /api/orders - List orders');
      logger.info('  GET  /api/collections - List collections');
      logger.info('  GET  /api/inventory - List inventory');
      logger.info('  GET  /api/analytics/* - Analytics endpoints');
    } catch (error) {
      logger.error('Failed to start server', { error });
      throw error;
    }
  };

  const stop = async (): Promise<void> => {
    await app.close();
    await db.disconnect();
    logger.info('Server stopped');
  };

  return { app, start, stop };
}
