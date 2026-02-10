# Plugin System Architecture

**Version**: 1.0.0
**Last Updated**: January 30, 2026
**Target**: nself v0.4.8+

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture Diagram](#architecture-diagram)
3. [Component Breakdown](#component-breakdown)
4. [File Structure](#file-structure)
5. [Plugin Lifecycle](#plugin-lifecycle)
6. [Database Schema Management](#database-schema-management)
7. [Webhook Handling Architecture](#webhook-handling-architecture)
8. [REST API Architecture](#rest-api-architecture)
9. [CLI Command System](#cli-command-system)
10. [Shared Utilities](#shared-utilities)
11. [Registry System](#registry-system)
12. [Security Model](#security-model)
13. [Data Flow Diagrams](#data-flow-diagrams)
14. [Implementation Patterns](#implementation-patterns)
15. [Performance Considerations](#performance-considerations)

---

## Overview

The nself plugin system is a modular, extensible architecture designed to sync external service data to PostgreSQL with real-time webhook support. Each plugin is a self-contained TypeScript/Node.js application that implements a standardized interface.

### Core Principles

1. **Zero Data Loss**: Every resource type from a service has corresponding database tables
2. **Real-time Sync**: Webhooks update data within seconds of changes
3. **Idempotent Operations**: All database operations use upsert patterns
4. **Type Safety**: Full TypeScript coverage with strict mode enabled
5. **Observability**: Comprehensive logging and statistics tracking
6. **Modularity**: Shared utilities minimize duplication across plugins

### Key Features

- **100% API Coverage**: All resource types from services are synced
- **Webhook Processing**: Event-driven updates with signature verification
- **REST API**: Query synced data via HTTP endpoints
- **CLI Interface**: Command-line tools for management and monitoring
- **Database Views**: Pre-built analytics queries (MRR, churn, etc.)
- **Error Recovery**: Automatic retry logic with exponential backoff

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         nself Plugin Ecosystem                           │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                ┌───────────────────┼───────────────────┐
                │                   │                   │
                ▼                   ▼                   ▼
        ┌───────────────┐   ┌───────────────┐   ┌───────────────┐
        │ Stripe Plugin │   │ GitHub Plugin │   │Shopify Plugin │
        │   Port 3001   │   │   Port 3002   │   │   Port 3003   │
        └───────────────┘   └───────────────┘   └───────────────┘
                │                   │                   │
                └───────────────────┼───────────────────┘
                                    │
        ┌───────────────────────────────────────────────────────┐
        │              Plugin Architecture Layers                │
        │                                                        │
        │  ┌──────────────────────────────────────────────────┐ │
        │  │            Layer 1: Interface Layer               │ │
        │  │  ┌─────────┐  ┌─────────┐  ┌──────────────────┐  │ │
        │  │  │   CLI   │  │  Server │  │  Webhook Handler │  │ │
        │  │  │ (cli.ts)│  │(server  │  │  (webhooks.ts)   │  │ │
        │  │  │         │  │  .ts)   │  │                  │  │ │
        │  │  └────┬────┘  └────┬────┘  └────────┬─────────┘  │ │
        │  └───────┼────────────┼─────────────────┼───────────┘ │
        │          │            │                 │             │
        │  ┌───────┼────────────┼─────────────────┼───────────┐ │
        │  │       │   Layer 2: Business Logic Layer       │   │ │
        │  │       ▼            ▼                 ▼           │ │
        │  │  ┌─────────────────────────────────────────┐    │ │
        │  │  │      Sync Service (sync.ts)             │    │ │
        │  │  │  - Full sync orchestration              │    │ │
        │  │  │  - Incremental sync                     │    │ │
        │  │  │  - Progress tracking                    │    │ │
        │  │  └──────────────────┬──────────────────────┘    │ │
        │  └────────────────────┼──────────────────────────┘ │
        │                       │                             │
        │  ┌────────────────────┼──────────────────────────┐ │
        │  │       Layer 3: Data Access Layer              │ │
        │  │                    ▼                           │ │
        │  │  ┌─────────────────────────┐ ┌──────────────┐ │ │
        │  │  │  Client (client.ts)     │ │Database      │ │ │
        │  │  │  - API calls            │ │(database.ts) │ │ │
        │  │  │  - Rate limiting        │ │- Schema init │ │ │
        │  │  │  - Pagination           │ │- CRUD ops    │ │ │
        │  │  │  - Type mapping         │ │- Upserts     │ │ │
        │  │  └─────────┬───────────────┘ └──────┬───────┘ │ │
        │  └────────────┼─────────────────────────┼────────┘ │
        │               │                         │          │
        │  ┌────────────┼─────────────────────────┼────────┐ │
        │  │      Layer 4: External Integration Layer      │ │
        │  │            ▼                         ▼          │ │
        │  │  ┌──────────────────┐    ┌──────────────────┐ │ │
        │  │  │ External API     │    │   PostgreSQL     │ │ │
        │  │  │ (Stripe, GitHub, │    │   Database       │ │ │
        │  │  │  Shopify, etc.)  │    │                  │ │ │
        │  │  └──────────────────┘    └──────────────────┘ │ │
        │  └──────────────────────────────────────────────┘ │
        └───────────────────────────────────────────────────┘
                                    │
        ┌───────────────────────────────────────────────────────┐
        │               Shared Utilities Layer                   │
        │  ┌────────┐ ┌─────────┐ ┌────────┐ ┌──────────────┐  │
        │  │ Logger │ │Database │ │  HTTP  │ │   Webhook    │  │
        │  │        │ │  Pool   │ │ Client │ │   Helpers    │  │
        │  └────────┘ └─────────┘ └────────┘ └──────────────┘  │
        └───────────────────────────────────────────────────────┘
                                    │
        ┌───────────────────────────────────────────────────────┐
        │                  Registry System                       │
        │                                                        │
        │  ┌──────────────────┐        ┌────────────────────┐  │
        │  │  registry.json   │───────▶│ Cloudflare Worker  │  │
        │  │  (GitHub)        │        │ plugins.nself.org  │  │
        │  └──────────────────┘        └────────────────────┘  │
        │                                       │               │
        │                                       ▼               │
        │                              ┌────────────────────┐  │
        │                              │   nself CLI        │  │
        │                              │   (installer)      │  │
        │                              └────────────────────┘  │
        └───────────────────────────────────────────────────────┘
```

---

## Component Breakdown

### 1. Types Module (`types.ts`)

**Purpose**: Central type definitions for the entire plugin

**Responsibilities**:
- API response interfaces (from external service)
- Database record interfaces (PostgreSQL types)
- Configuration interfaces
- Internal service types

**Example**:

```typescript
// API Response Type (from Stripe API)
export interface StripeCustomer {
  id: string;
  email: string | null;
  name: string | null;
  created: number;
  metadata: Record<string, string>;
}

// Database Record Type (PostgreSQL)
export interface StripeCustomerRecord {
  id: string;
  email: string | null;
  name: string | null;
  metadata: Record<string, string>;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
  deleted_at: Date | null;
}

// Configuration Type
export interface StripePluginConfig {
  apiKey: string;
  webhookSecret?: string;
  port: number;
  database: DatabaseConfig;
}
```

**Key Pattern**: Separate API types from database types to handle:
- Unix timestamps → PostgreSQL timestamps
- Optional fields → nullable database columns
- Nested objects → JSONB columns

---

### 2. Client Module (`client.ts`)

**Purpose**: Type-safe wrapper around external service APIs

**Responsibilities**:
- HTTP request management
- Rate limiting enforcement
- Pagination handling
- Response mapping to database types
- Error handling and retries

**Architecture**:

```typescript
export class StripeClient {
  private stripe: Stripe;  // Official SDK
  private rateLimiter: RateLimiter;

  constructor(apiKey: string) {
    this.stripe = new Stripe(apiKey);
  }

  // Generator for memory-efficient pagination
  async *listCustomers(): AsyncGenerator<StripeCustomerRecord[]> {
    for await (const customer of this.stripe.customers.list({ limit: 100 })) {
      yield [this.mapCustomer(customer)];
    }
  }

  // Traditional pagination with auto-fetch
  async listAllCustomers(): Promise<StripeCustomerRecord[]> {
    const customers: StripeCustomerRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.customers.list({
        limit: 100,
        starting_after: startingAfter,
      });

      customers.push(...response.data.map(c => this.mapCustomer(c)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }
    }

    return customers;
  }

  // Type mapping: API → Database
  private mapCustomer(api: Stripe.Customer): StripeCustomerRecord {
    return {
      id: api.id,
      email: api.email ?? null,
      name: api.name ?? null,
      metadata: api.metadata,
      created_at: new Date(api.created * 1000),  // Unix → Date
      updated_at: new Date(),
      synced_at: new Date(),
      deleted_at: null,
    };
  }
}
```

**Rate Limiting Strategy**:

```typescript
// From shared utilities
export class RateLimiter {
  private tokens: number;
  private lastRefill: number;
  private maxTokens: number;
  private refillRate: number; // tokens per second

  constructor(requestsPerSecond: number) {
    this.maxTokens = requestsPerSecond;
    this.tokens = requestsPerSecond;
    this.refillRate = requestsPerSecond;
    this.lastRefill = Date.now();
  }

  async acquire(): Promise<void> {
    this.refill();

    if (this.tokens < 1) {
      const waitTime = (1 - this.tokens) / this.refillRate * 1000;
      await new Promise(resolve => setTimeout(resolve, waitTime));
      this.refill();
    }

    this.tokens -= 1;
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    this.tokens = Math.min(
      this.maxTokens,
      this.tokens + elapsed * this.refillRate
    );
    this.lastRefill = now;
  }
}
```

---

### 3. Database Module (`database.ts`)

**Purpose**: PostgreSQL schema and CRUD operations

**Responsibilities**:
- Schema initialization (CREATE TABLE statements)
- Index creation for performance
- View creation for analytics
- Upsert operations (INSERT ... ON CONFLICT)
- Query helpers
- Statistics collection

**Schema Pattern**:

```sql
CREATE TABLE IF NOT EXISTS stripe_customers (
  -- Primary key (from API)
  id VARCHAR(255) PRIMARY KEY,

  -- Business fields
  email VARCHAR(255),
  name VARCHAR(255),
  phone VARCHAR(50),
  description TEXT,

  -- Numeric fields
  balance BIGINT DEFAULT 0,

  -- Boolean fields
  delinquent BOOLEAN DEFAULT FALSE,

  -- JSONB for complex objects
  metadata JSONB DEFAULT '{}',
  address JSONB,
  shipping JSONB,

  -- Timestamps
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_stripe_customers_email
  ON stripe_customers(email);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_created
  ON stripe_customers(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_deleted
  ON stripe_customers(deleted_at)
  WHERE deleted_at IS NOT NULL;
```

**Upsert Pattern** (critical for idempotency):

```typescript
async upsertCustomer(record: StripeCustomerRecord): Promise<void> {
  await this.db.execute(
    `INSERT INTO stripe_customers (
      id, email, name, phone, description, balance,
      delinquent, metadata, address, shipping,
      created_at, synced_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
    ON CONFLICT (id) DO UPDATE SET
      email = EXCLUDED.email,
      name = EXCLUDED.name,
      phone = EXCLUDED.phone,
      description = EXCLUDED.description,
      balance = EXCLUDED.balance,
      delinquent = EXCLUDED.delinquent,
      metadata = EXCLUDED.metadata,
      address = EXCLUDED.address,
      shipping = EXCLUDED.shipping,
      updated_at = NOW(),
      synced_at = NOW()`,
    [
      record.id,
      record.email,
      record.name,
      record.phone,
      record.description,
      record.balance,
      record.delinquent,
      JSON.stringify(record.metadata),
      JSON.stringify(record.address),
      JSON.stringify(record.shipping),
      record.created_at,
    ]
  );
}
```

**Soft Delete Pattern**:

```typescript
async deleteCustomer(id: string): Promise<void> {
  await this.db.execute(
    `UPDATE stripe_customers
     SET deleted_at = NOW(), updated_at = NOW()
     WHERE id = $1`,
    [id]
  );
}
```

**Analytics Views**:

```sql
-- Monthly Recurring Revenue (MRR)
CREATE OR REPLACE VIEW stripe_mrr AS
SELECT
  DATE_TRUNC('month', current_period_start) AS month,
  COUNT(*) AS subscription_count,
  SUM(
    CASE
      WHEN billing_interval = 'month' THEN amount
      WHEN billing_interval = 'year' THEN amount / 12
      ELSE 0
    END
  ) / 100.0 AS mrr
FROM stripe_subscriptions
WHERE status IN ('active', 'trialing')
  AND deleted_at IS NULL
GROUP BY DATE_TRUNC('month', current_period_start)
ORDER BY month DESC;

-- Active Subscriptions
CREATE OR REPLACE VIEW stripe_active_subscriptions AS
SELECT
  s.*,
  c.email AS customer_email,
  c.name AS customer_name,
  p.name AS product_name
FROM stripe_subscriptions s
LEFT JOIN stripe_customers c ON s.customer_id = c.id
LEFT JOIN stripe_products p ON s.product_id = p.id
WHERE s.status IN ('active', 'trialing')
  AND s.deleted_at IS NULL;
```

---

### 4. Sync Service (`sync.ts`)

**Purpose**: Orchestrate data synchronization from external API to database

**Responsibilities**:
- Full historical sync (all data)
- Incremental sync (recent changes only)
- Progress tracking and logging
- Error handling and recovery
- Resource ordering (dependencies first)

**Architecture**:

```typescript
export class StripeSyncService {
  private client: StripeClient;
  private db: StripeDatabase;
  private syncing = false;

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    if (this.syncing) {
      throw new Error('Sync already in progress');
    }

    this.syncing = true;
    const startTime = Date.now();
    const stats = this.initializeStats();

    try {
      // Sync in dependency order
      const resources = options.resources ?? [
        'customers',      // Must sync before subscriptions
        'products',       // Must sync before prices
        'prices',         // Must sync before subscriptions
        'subscriptions',  // Depends on customers, prices
        'invoices',       // Depends on customers, subscriptions
        'payment_intents',
        'charges',
        'refunds',
      ];

      for (const resource of resources) {
        await this.syncResource(resource, stats);
      }

      return {
        success: true,
        stats,
        errors: [],
        duration: Date.now() - startTime,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });

      return {
        success: false,
        stats,
        errors: [message],
        duration: Date.now() - startTime,
      };
    } finally {
      this.syncing = false;
    }
  }

  private async syncResource(
    resource: string,
    stats: SyncStats
  ): Promise<void> {
    logger.info(`Syncing ${resource}...`);
    const startTime = Date.now();
    let count = 0;

    switch (resource) {
      case 'customers':
        for await (const batch of this.client.listCustomers()) {
          for (const customer of batch) {
            await this.db.upsertCustomer(customer);
            count++;
          }
          logger.debug(`Synced ${count} customers`);
        }
        stats.customers = count;
        break;

      case 'products':
        const products = await this.client.listAllProducts();
        for (const product of products) {
          await this.db.upsertProduct(product);
        }
        stats.products = products.length;
        count = products.length;
        break;

      // ... more resources
    }

    const duration = Date.now() - startTime;
    logger.info(`Synced ${count} ${resource} in ${duration}ms`);
  }
}
```

**Incremental Sync Pattern**:

```typescript
async incrementalSync(since: Date): Promise<SyncResult> {
  // Fetch only records updated since timestamp
  const customers = await this.client.listAllCustomers({
    created: { gte: Math.floor(since.getTime() / 1000) }
  });

  for (const customer of customers) {
    await this.db.upsertCustomer(customer);
  }

  return {
    success: true,
    synced: customers.length,
    duration: Date.now() - startTime,
  };
}
```

---

### 5. Webhook Handler (`webhooks.ts`)

**Purpose**: Process real-time events from external services

**Responsibilities**:
- Event signature verification
- Event routing to handlers
- Database updates
- Event storage (audit log)
- Error recovery and retries

**Architecture**:

```typescript
export class StripeWebhookHandler {
  private handlers: Map<string, WebhookHandlerFn>;

  constructor(
    private client: StripeClient,
    private db: StripeDatabase,
    private syncService: StripeSyncService
  ) {
    this.handlers = new Map();
    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    // Customer events
    this.register('customer.created', this.handleCustomerCreated.bind(this));
    this.register('customer.updated', this.handleCustomerUpdated.bind(this));
    this.register('customer.deleted', this.handleCustomerDeleted.bind(this));

    // Subscription events
    this.register('customer.subscription.created',
      this.handleSubscriptionCreated.bind(this));
    this.register('customer.subscription.updated',
      this.handleSubscriptionUpdated.bind(this));
    this.register('customer.subscription.deleted',
      this.handleSubscriptionDeleted.bind(this));

    // Invoice events
    this.register('invoice.paid', this.handleInvoicePaid.bind(this));
    this.register('invoice.payment_failed',
      this.handleInvoicePaymentFailed.bind(this));

    // ... 70+ total events
  }

  async processEvent(event: Stripe.Event): Promise<void> {
    // 1. Store raw event for audit trail
    await this.db.insertWebhookEvent({
      id: event.id,
      type: event.type,
      data: event.data,
      created_at: new Date(event.created * 1000),
      processed: false,
    });

    try {
      // 2. Route to handler
      const handler = this.handlers.get(event.type);

      if (handler) {
        await handler(event);
      } else {
        logger.warn(`No handler for event type: ${event.type}`);
      }

      // 3. Mark as processed
      await this.db.markEventProcessed(event.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`Failed to process event ${event.id}`, { error: message });

      // Store error for debugging
      await this.db.markEventProcessed(event.id, message);
      throw error;
    }
  }

  private async handleCustomerCreated(event: Stripe.Event): Promise<void> {
    const customer = event.data.object as Stripe.Customer;
    const record = this.client.mapCustomer(customer);
    await this.db.upsertCustomer(record);
    logger.info(`Customer created: ${customer.id}`);
  }

  private async handleSubscriptionUpdated(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;

    // Re-fetch to get complete data with expansions
    const fullSubscription = await this.client.getSubscription(subscription.id);

    if (fullSubscription) {
      await this.db.upsertSubscription(fullSubscription);
      logger.info(`Subscription updated: ${subscription.id}`);
    }
  }
}
```

**Signature Verification** (Stripe example):

```typescript
// From shared utilities
export function verifyStripeSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const [timestampPart, signaturePart] = signature.split(',');
  const timestamp = timestampPart.split('=')[1];
  const expectedSignature = signaturePart.split('=')[1];

  // Prevent replay attacks (5 minute window)
  const currentTime = Math.floor(Date.now() / 1000);
  if (Math.abs(currentTime - parseInt(timestamp)) > 300) {
    return false;
  }

  // Verify HMAC-SHA256 signature
  const signedPayload = `${timestamp}.${payload}`;
  const expectedSig = crypto
    .createHmac('sha256', secret)
    .update(signedPayload)
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(expectedSignature),
    Buffer.from(expectedSig)
  );
}
```

---

### 6. Server Module (`server.ts`)

**Purpose**: HTTP server for webhooks and REST API

**Responsibilities**:
- Webhook endpoint (POST /webhook)
- REST API endpoints (GET /api/*)
- Manual sync endpoint (POST /api/sync)
- Health checks
- CORS handling
- Rate limiting
- Authentication

**Architecture**:

```typescript
export async function createServer(config: Config) {
  const app = Fastify({ logger: false });

  // Initialize components
  const client = new StripeClient(config.apiKey);
  const db = new StripeDatabase();
  const syncService = new StripeSyncService(client, db);
  const webhookHandler = new StripeWebhookHandler(client, db, syncService);

  await db.connect();
  await db.initializeSchema();

  // Middleware
  await app.register(cors);

  // Rate limiting
  const rateLimiter = new ApiRateLimiter(100, 60000); // 100 req/min
  app.addHook('preHandler', createRateLimitHook(rateLimiter));

  // API key authentication (optional)
  if (config.security.apiKey) {
    app.addHook('preHandler', createAuthHook(config.security.apiKey));
  }

  // Raw body parser for webhook verification
  app.addContentTypeParser('application/json', { parseAs: 'string' },
    (req, body, done) => {
      try {
        const json = JSON.parse(body as string);
        req.rawBody = body as string;
        done(null, json);
      } catch (err) {
        done(err, undefined);
      }
    }
  );

  // Health checks
  app.get('/health', async () => ({
    status: 'ok',
    plugin: 'stripe',
    timestamp: new Date().toISOString(),
  }));

  app.get('/ready', async (_, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, timestamp: new Date().toISOString() };
    } catch (error) {
      return reply.status(503).send({
        ready: false,
        error: 'Database unavailable'
      });
    }
  });

  // Webhook endpoint
  app.post('/webhook', async (request, reply) => {
    const signature = request.headers['stripe-signature'] as string;

    if (!signature || !config.webhookSecret) {
      return reply.status(400).send({ error: 'Missing signature' });
    }

    // Verify signature
    const isValid = verifyStripeSignature(
      request.rawBody,
      signature,
      config.webhookSecret
    );

    if (!isValid) {
      logger.warn('Invalid webhook signature');
      return reply.status(401).send({ error: 'Invalid signature' });
    }

    // Process event
    try {
      const event = request.body as Stripe.Event;
      await webhookHandler.processEvent(event);
      return { received: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  // Sync endpoint
  app.post('/api/sync', async () => {
    const result = await syncService.sync();
    return result;
  });

  // Status endpoint
  app.get('/api/status', async () => {
    const stats = await db.getStats();
    return {
      plugin: 'stripe',
      version: '1.0.0',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // Data query endpoints
  app.get('/api/customers', async (request) => {
    const { limit = 100, offset = 0 } = request.query as Record<string, string>;
    const customers = await db.listCustomers(
      parseInt(limit),
      parseInt(offset)
    );
    return { data: customers };
  });

  app.get('/api/subscriptions', async (request) => {
    const { status } = request.query as Record<string, string>;
    const subscriptions = await db.listSubscriptions({ status });
    return { data: subscriptions };
  });

  app.get('/api/mrr', async () => {
    const mrr = await db.getMRR();
    return { data: mrr };
  });

  return app;
}
```

---

### 7. CLI Module (`cli.ts`)

**Purpose**: Command-line interface for plugin management

**Responsibilities**:
- Initialize database schema
- Trigger data sync
- Start webhook server
- Query data
- View statistics

**Architecture**:

```typescript
import { Command } from 'commander';

const program = new Command();

program
  .name('nself-stripe')
  .description('Stripe plugin for nself')
  .version('1.0.0');

// Initialize database
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    const db = new StripeDatabase();
    await db.connect();
    await db.initializeSchema();
    console.log('✓ Database schema initialized');
    await db.disconnect();
  });

// Sync data
program
  .command('sync')
  .description('Sync all Stripe data to database')
  .option('-r, --resources <items>', 'Resources to sync (comma-separated)')
  .option('-i, --incremental', 'Incremental sync only')
  .action(async (options) => {
    const client = new StripeClient(config.apiKey);
    const db = new StripeDatabase();
    const syncService = new StripeSyncService(client, db);

    await db.connect();

    const syncOptions: SyncOptions = {
      incremental: options.incremental,
      resources: options.resources?.split(','),
    };

    console.log('Starting sync...');
    const result = await syncService.sync(syncOptions);

    console.log('✓ Sync complete');
    console.log(`  Customers: ${result.stats.customers}`);
    console.log(`  Subscriptions: ${result.stats.subscriptions}`);
    console.log(`  Duration: ${result.duration}ms`);

    await db.disconnect();
  });

// Start server
program
  .command('server')
  .description('Start webhook server')
  .option('-p, --port <number>', 'Port number', '3001')
  .action(async (options) => {
    const config = loadConfig({ port: parseInt(options.port) });
    const server = await createServer(config);

    await server.listen({
      port: config.port,
      host: config.host
    });

    console.log(`✓ Server running on http://${config.host}:${config.port}`);
    console.log(`  Webhook: http://${config.host}:${config.port}/webhook`);
    console.log(`  API: http://${config.host}:${config.port}/api`);
  });

// Status command
program
  .command('status')
  .description('Show sync status and statistics')
  .action(async () => {
    const db = new StripeDatabase();
    await db.connect();

    const stats = await db.getStats();

    console.log('Plugin Status:');
    console.log(`  Customers: ${stats.customers}`);
    console.log(`  Subscriptions: ${stats.subscriptions}`);
    console.log(`  Invoices: ${stats.invoices}`);
    console.log(`  Last Sync: ${stats.lastSyncedAt?.toISOString() ?? 'Never'}`);

    await db.disconnect();
  });

// Query commands
program
  .command('customers')
  .description('List customers')
  .option('-l, --limit <number>', 'Number of results', '10')
  .action(async (options) => {
    const db = new StripeDatabase();
    await db.connect();

    const customers = await db.listCustomers(parseInt(options.limit));
    console.table(customers.map(c => ({
      id: c.id,
      email: c.email,
      name: c.name,
      created: c.created_at,
    })));

    await db.disconnect();
  });

program.parse();
```

---

## File Structure

### Plugin Directory Layout

```
plugins/stripe/
├── plugin.json                 # Plugin manifest
├── README.md                   # Plugin documentation
├── .env.example                # Environment variable template
└── ts/                         # TypeScript implementation
    ├── package.json            # npm dependencies
    ├── tsconfig.json           # TypeScript configuration
    ├── src/
    │   ├── types.ts            # Type definitions
    │   ├── config.ts           # Configuration loader
    │   ├── client.ts           # API client
    │   ├── database.ts         # Database operations
    │   ├── sync.ts             # Sync service
    │   ├── webhooks.ts         # Webhook handlers
    │   ├── server.ts           # HTTP server
    │   ├── cli.ts              # CLI commands
    │   └── index.ts            # Public exports
    ├── dist/                   # Compiled JavaScript (gitignored)
    │   ├── index.js
    │   ├── cli.js
    │   └── ...
    └── node_modules/           # Dependencies (gitignored)
```

### Shared Utilities Structure

```
shared/
├── package.json
├── tsconfig.json
├── src/
│   ├── types.ts                # Common interfaces
│   ├── logger.ts               # Logging utilities
│   ├── database.ts             # PostgreSQL connection pool
│   ├── http.ts                 # HTTP client
│   ├── webhook.ts              # Webhook helpers
│   ├── security.ts             # Security utilities (NEW)
│   ├── validation.ts           # Input validation (NEW)
│   └── index.ts                # Public exports
└── dist/                       # Compiled output
    └── ...
```

### Import Resolution

All plugins use NodeNext module resolution:

```json
{
  "compilerOptions": {
    "module": "NodeNext",
    "moduleResolution": "NodeNext"
  }
}
```

**Critical**: Always use `.js` extensions in imports (TypeScript resolves to `.ts`):

```typescript
// ✓ Correct
import { StripeClient } from './client.js';
import { createLogger } from '@nself/plugin-utils';

// ✗ Wrong
import { StripeClient } from './client';
import { StripeClient } from './client.ts';
```

---

## Plugin Lifecycle

### 1. Installation Phase

```
┌─────────────────────────────────────────────────────────────┐
│ User: nself plugin install stripe                           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. nself CLI fetches registry.json                          │
│    from https://plugins.nself.org/registry.json             │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Validate plugin metadata                                 │
│    - Check minNselfVersion compatibility                    │
│    - Verify checksums                                       │
│    - Check dependencies                                     │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Clone plugin from GitHub                                 │
│    git clone --depth 1 --branch v1.0.0                      │
│    https://github.com/acamarata/nself-plugins.git           │
│    Extract: plugins/stripe/                                 │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Install dependencies                                     │
│    cd plugins/stripe/ts                                     │
│    npm install                                              │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Build plugin                                             │
│    npm run build                                            │
│    (TypeScript → JavaScript in dist/)                       │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Run post-install hook (if defined)                       │
│    bash plugins/stripe/install.sh                           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Add to nself config                                      │
│    ~/.nself/config.json                                     │
│    {                                                        │
│      "plugins": {                                           │
│        "stripe": {                                          │
│          "installed": true,                                 │
│          "version": "1.0.0",                                │
│          "path": "~/.nself/plugins/stripe"                  │
│        }                                                    │
│      }                                                      │
│    }                                                        │
└─────────────────────────────────────────────────────────────┘
```

### 2. Configuration Phase

```
┌─────────────────────────────────────────────────────────────┐
│ User: nself plugin config stripe                            │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Load plugin.json to determine required env vars          │
│    Required: STRIPE_API_KEY                                 │
│    Optional: STRIPE_WEBHOOK_SECRET, PORT, etc.              │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Interactive prompts for missing values                   │
│    ? Stripe API Key: sk_test_***                            │
│    ? Webhook Secret (optional): whsec_***                   │
│    ? Database URL: postgresql://localhost/nself             │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Validate configuration                                   │
│    - Test API key with test request                         │
│    - Verify database connectivity                           │
│    - Check port availability                                │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Save to .env file                                        │
│    plugins/stripe/ts/.env                                   │
│    STRIPE_API_KEY=sk_test_***                               │
│    DATABASE_URL=postgresql://...                            │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Initialize database schema                               │
│    nself-stripe init                                        │
│    Creates all tables, indexes, views                       │
└─────────────────────────────────────────────────────────────┘
```

### 3. Sync Phase

```
┌─────────────────────────────────────────────────────────────┐
│ User: nself plugin run stripe sync                          │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Load configuration                                       │
│    - Read .env file                                         │
│    - Validate required vars                                 │
│    - Initialize logger                                      │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Initialize components                                    │
│    client = new StripeClient(apiKey)                        │
│    db = new StripeDatabase()                                │
│    sync = new StripeSyncService(client, db)                 │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Connect to database                                      │
│    await db.connect()                                       │
│    await db.initializeSchema() // Idempotent                │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Sync resources in dependency order                       │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ a. Customers (independent)                          │ │
│    │    - Fetch from API (paginated)                     │ │
│    │    - Upsert to database                             │ │
│    │    - Log progress                                   │ │
│    └─────────────────────────────────────────────────────┘ │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ b. Products (independent)                           │ │
│    └─────────────────────────────────────────────────────┘ │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ c. Prices (depends on products)                     │ │
│    └─────────────────────────────────────────────────────┘ │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ d. Subscriptions (depends on customers, prices)     │ │
│    └─────────────────────────────────────────────────────┘ │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ e. Invoices (depends on customers, subscriptions)   │ │
│    └─────────────────────────────────────────────────────┘ │
│    ┌─────────────────────────────────────────────────────┐ │
│    │ f. Payment Intents, Charges, Refunds                │ │
│    └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Return sync results                                      │
│    {                                                        │
│      success: true,                                         │
│      stats: {                                               │
│        customers: 1250,                                     │
│        subscriptions: 450,                                  │
│        invoices: 3200                                       │
│      },                                                     │
│      duration: 45000 // 45 seconds                          │
│    }                                                        │
└─────────────────────────────────────────────────────────────┘
```

### 4. Webhook Server Phase

```
┌─────────────────────────────────────────────────────────────┐
│ User: nself plugin run stripe server                        │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Initialize HTTP server (Fastify)                         │
│    - Load configuration                                     │
│    - Register middleware (CORS, rate limiting, auth)        │
│    - Register routes                                        │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Initialize components                                    │
│    client = new StripeClient(apiKey)                        │
│    db = new StripeDatabase()                                │
│    sync = new StripeSyncService(client, db)                 │
│    webhooks = new StripeWebhookHandler(client, db, sync)    │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Connect to database                                      │
│    await db.connect()                                       │
│    await db.initializeSchema()                              │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Start HTTP server                                        │
│    await server.listen({ port: 3001, host: '0.0.0.0' })    │
│    Logger: Server running on http://0.0.0.0:3001           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Ready to receive webhooks                                │
│    Endpoints:                                               │
│    - POST /webhook (Stripe events)                          │
│    - POST /api/sync (manual sync trigger)                   │
│    - GET /api/status (statistics)                           │
│    - GET /api/* (data queries)                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Database Schema Management

### Schema Initialization Flow

```typescript
async initializeSchema(): Promise<void> {
  // 1. Create extension for UUID generation
  await this.db.execute('CREATE EXTENSION IF NOT EXISTS "uuid-ossp"');

  // 2. Create tables in dependency order
  await this.createCoreObjectTables();
  await this.createBillingTables();
  await this.createPaymentTables();
  await this.createWebhookTables();

  // 3. Create indexes for performance
  await this.createIndexes();

  // 4. Create analytics views
  await this.createViews();

  // 5. Create functions and triggers (if needed)
  await this.createFunctions();
}
```

### Table Design Patterns

**1. Core Objects** (customers, products, prices):

```sql
CREATE TABLE IF NOT EXISTS stripe_customers (
  id VARCHAR(255) PRIMARY KEY,          -- Stripe ID
  email VARCHAR(255),                   -- Nullable fields
  name VARCHAR(255),
  metadata JSONB DEFAULT '{}',          -- Flexible data
  created_at TIMESTAMP WITH TIME ZONE,  -- From API
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),  -- Auto-update
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),   -- Sync tracking
  deleted_at TIMESTAMP WITH TIME ZONE   -- Soft delete
);
```

**2. Relationship Tables** (subscriptions, invoices):

```sql
CREATE TABLE IF NOT EXISTS stripe_subscriptions (
  id VARCHAR(255) PRIMARY KEY,
  customer_id VARCHAR(255) NOT NULL,    -- Foreign key
  status VARCHAR(50) NOT NULL,
  current_period_start TIMESTAMP WITH TIME ZONE,
  current_period_end TIMESTAMP WITH TIME ZONE,

  -- Foreign key constraints (optional)
  CONSTRAINT fk_customer
    FOREIGN KEY (customer_id)
    REFERENCES stripe_customers(id)
    ON DELETE CASCADE
);
```

**3. Junction Tables** (subscription_items):

```sql
CREATE TABLE IF NOT EXISTS stripe_subscription_items (
  id VARCHAR(255) PRIMARY KEY,
  subscription_id VARCHAR(255) NOT NULL,
  price_id VARCHAR(255) NOT NULL,
  quantity INTEGER DEFAULT 1,

  CONSTRAINT fk_subscription
    FOREIGN KEY (subscription_id)
    REFERENCES stripe_subscriptions(id)
    ON DELETE CASCADE,

  CONSTRAINT fk_price
    FOREIGN KEY (price_id)
    REFERENCES stripe_prices(id)
    ON DELETE RESTRICT
);
```

**4. Event Log Tables**:

```sql
CREATE TABLE IF NOT EXISTS stripe_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  type VARCHAR(255) NOT NULL,
  data JSONB NOT NULL,                  -- Full event payload
  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
  processed BOOLEAN DEFAULT FALSE,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,                           -- Processing errors
  retry_count INTEGER DEFAULT 0
);

-- Index for unprocessed events
CREATE INDEX IF NOT EXISTS idx_webhook_events_unprocessed
  ON stripe_webhook_events(created_at)
  WHERE processed = FALSE;
```

### Index Strategy

**1. Primary Lookups** (id, email):

```sql
CREATE INDEX IF NOT EXISTS idx_stripe_customers_email
  ON stripe_customers(email);
```

**2. Time-Based Queries**:

```sql
CREATE INDEX IF NOT EXISTS idx_stripe_customers_created
  ON stripe_customers(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_stripe_invoices_period
  ON stripe_invoices(period_start DESC, period_end DESC);
```

**3. Status Filters**:

```sql
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_status
  ON stripe_subscriptions(status)
  WHERE deleted_at IS NULL;
```

**4. Foreign Keys**:

```sql
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_customer
  ON stripe_subscriptions(customer_id);

CREATE INDEX IF NOT EXISTS idx_stripe_invoices_customer
  ON stripe_invoices(customer_id);
```

**5. Partial Indexes** (for common filters):

```sql
-- Active subscriptions only
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_active
  ON stripe_subscriptions(customer_id, current_period_end)
  WHERE status IN ('active', 'trialing') AND deleted_at IS NULL;

-- Failed payments only
CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_failed
  ON stripe_payment_intents(created_at DESC)
  WHERE status = 'requires_payment_method';
```

### Analytics Views

**Monthly Recurring Revenue (MRR)**:

```sql
CREATE OR REPLACE VIEW stripe_mrr AS
SELECT
  DATE_TRUNC('month', current_period_start) AS month,
  COUNT(DISTINCT customer_id) AS unique_customers,
  COUNT(*) AS subscription_count,
  SUM(
    CASE
      WHEN items.recurring_interval = 'month'
        THEN items.amount
      WHEN items.recurring_interval = 'year'
        THEN items.amount / 12.0
      WHEN items.recurring_interval = 'week'
        THEN items.amount * 52.0 / 12.0
      WHEN items.recurring_interval = 'day'
        THEN items.amount * 365.0 / 12.0
      ELSE 0
    END
  ) / 100.0 AS mrr_cents
FROM stripe_subscriptions s
JOIN LATERAL (
  SELECT
    i.price_id,
    i.quantity,
    p.unit_amount * i.quantity AS amount,
    p.recurring->>'interval' AS recurring_interval
  FROM stripe_subscription_items i
  JOIN stripe_prices p ON i.price_id = p.id
  WHERE i.subscription_id = s.id
) items ON true
WHERE s.status IN ('active', 'trialing')
  AND s.deleted_at IS NULL
GROUP BY DATE_TRUNC('month', current_period_start)
ORDER BY month DESC;
```

**Customer Lifetime Value (LTV)**:

```sql
CREATE OR REPLACE VIEW stripe_customer_ltv AS
SELECT
  c.id AS customer_id,
  c.email,
  c.name,
  c.created_at AS customer_since,
  COALESCE(SUM(i.amount_paid), 0) / 100.0 AS total_revenue,
  COUNT(DISTINCT i.id) AS invoice_count,
  COUNT(DISTINCT s.id) AS subscription_count,
  MAX(i.created_at) AS last_payment_date,
  DATE_PART('day', NOW() - c.created_at) AS days_as_customer,
  CASE
    WHEN DATE_PART('day', NOW() - c.created_at) > 0
    THEN (COALESCE(SUM(i.amount_paid), 0) / 100.0) /
         (DATE_PART('day', NOW() - c.created_at) / 30.0)
    ELSE 0
  END AS avg_monthly_revenue
FROM stripe_customers c
LEFT JOIN stripe_subscriptions s ON c.id = s.customer_id
  AND s.deleted_at IS NULL
LEFT JOIN stripe_invoices i ON c.id = i.customer_id
  AND i.status = 'paid'
  AND i.deleted_at IS NULL
WHERE c.deleted_at IS NULL
GROUP BY c.id, c.email, c.name, c.created_at
ORDER BY total_revenue DESC;
```

**Churn Analysis**:

```sql
CREATE OR REPLACE VIEW stripe_churn_analysis AS
SELECT
  DATE_TRUNC('month', canceled_at) AS month,
  COUNT(*) AS churned_subscriptions,
  COUNT(DISTINCT customer_id) AS churned_customers,
  SUM(
    CASE
      WHEN items.recurring_interval = 'month'
        THEN items.amount
      WHEN items.recurring_interval = 'year'
        THEN items.amount / 12.0
      ELSE 0
    END
  ) / 100.0 AS churned_mrr
FROM stripe_subscriptions s
JOIN LATERAL (
  SELECT
    SUM(p.unit_amount * i.quantity) AS amount,
    p.recurring->>'interval' AS recurring_interval
  FROM stripe_subscription_items i
  JOIN stripe_prices p ON i.price_id = p.id
  WHERE i.subscription_id = s.id
  GROUP BY p.recurring
) items ON true
WHERE s.status = 'canceled'
  AND s.canceled_at IS NOT NULL
  AND s.deleted_at IS NULL
GROUP BY DATE_TRUNC('month', canceled_at)
ORDER BY month DESC;
```

---

## Webhook Handling Architecture

### Event Processing Flow

```
┌─────────────────────────────────────────────────────────────┐
│ External Service (Stripe) sends webhook                     │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ POST /webhook                                               │
│ Headers:                                                    │
│   - stripe-signature: t=1234567890,v1=abcdef...            │
│ Body (JSON):                                                │
│   {                                                         │
│     "id": "evt_1234",                                       │
│     "type": "customer.subscription.updated",                │
│     "data": { "object": {...} }                             │
│   }                                                         │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Raw Body Capture                                         │
│    - Fastify parser saves raw body for signature check      │
│    - req.rawBody = raw string                               │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Signature Verification                                   │
│    const isValid = verifyStripeSignature(                   │
│      req.rawBody,                                           │
│      req.headers['stripe-signature'],                       │
│      webhookSecret                                          │
│    );                                                       │
│                                                             │
│    Process:                                                 │
│    a. Extract timestamp from signature                      │
│    b. Check timestamp freshness (< 5 min)                   │
│    c. Compute HMAC-SHA256 of timestamp.payload              │
│    d. Compare with provided signature (timing-safe)         │
└─────────────────────────────────────────────────────────────┘
                         │
                    ┌────┴────┐
                    │ Valid?  │
                    └────┬────┘
                  NO     │     YES
                    ▼    │     ▼
            ┌───────────┐│┌─────────────────────────────────┐
            │ Return    │││ 3. Store Event (Audit Trail)     │
            │ 401       │││    await db.insertWebhookEvent({ │
            │ Unauthorized││    id: event.id,                │
            └───────────┘││    type: event.type,            │
                         ││    data: event.data,            │
                         ││    created_at: ...,             │
                         ││    processed: false             │
                         ││  });                            │
                         │└─────────────────────────────────┘
                         │              │
                         │              ▼
                         │┌─────────────────────────────────┐
                         ││ 4. Route to Handler              │
                         ││    const handler =               │
                         ││      handlers.get(event.type);   │
                         ││                                  │
                         ││    if (handler) {                │
                         ││      await handler(event);       │
                         ││    }                             │
                         │└─────────────────────────────────┘
                         │              │
                         │              ▼
                         │┌─────────────────────────────────┐
                         ││ 5. Execute Handler               │
                         ││    - Parse event data            │
                         ││    - Fetch additional data if    │
                         ││      needed (expansions)         │
                         ││    - Upsert to database          │
                         ││    - Log operation               │
                         │└─────────────────────────────────┘
                         │              │
                         │         ┌────┴─────┐
                         │         │ Success? │
                         │         └────┬─────┘
                         │      YES     │     NO
                         │         ▼    │     ▼
                         │    ┌────────┐│┌──────────────────┐
                         │    │ 6a. Mark││ 6b. Log Error    │
                         │    │ Processed││   Store error msg│
                         │    │ await db ││   in DB          │
                         │    │ .markEvent││ Consider retry  │
                         │    │ Processed()││                 │
                         │    └────────┘│└──────────────────┘
                         │              │
                         │              ▼
                         │┌─────────────────────────────────┐
                         ││ 7. Return 200 OK                 │
                         ││    { "received": true }          │
                         │└─────────────────────────────────┘
                         └──────────────────────────────────────
```

### Handler Registration Pattern

```typescript
class StripeWebhookHandler {
  private handlers: Map<string, WebhookHandlerFn> = new Map();

  private registerDefaultHandlers(): void {
    // Customer lifecycle
    this.register('customer.created', async (event) => {
      const customer = event.data.object as Stripe.Customer;
      await this.db.upsertCustomer(this.client.mapCustomer(customer));
    });

    this.register('customer.updated', async (event) => {
      const customer = event.data.object as Stripe.Customer;
      await this.db.upsertCustomer(this.client.mapCustomer(customer));
    });

    this.register('customer.deleted', async (event) => {
      const customer = event.data.object as Stripe.Customer;
      await this.db.deleteCustomer(customer.id);
    });

    // Subscription lifecycle
    this.register('customer.subscription.created', async (event) => {
      const sub = event.data.object as Stripe.Subscription;
      // Re-fetch with expansions for complete data
      const fullSub = await this.client.getSubscription(sub.id);
      if (fullSub) {
        await this.db.upsertSubscription(fullSub);
        // Also sync subscription items
        for (const item of fullSub.items) {
          await this.db.upsertSubscriptionItem(item);
        }
      }
    });

    // Invoice events
    this.register('invoice.paid', async (event) => {
      const invoice = event.data.object as Stripe.Invoice;
      await this.db.upsertInvoice(this.client.mapInvoice(invoice));
      // Update subscription status if applicable
      if (invoice.subscription) {
        const sub = await this.client.getSubscription(
          invoice.subscription as string
        );
        if (sub) {
          await this.db.upsertSubscription(sub);
        }
      }
    });

    this.register('invoice.payment_failed', async (event) => {
      const invoice = event.data.object as Stripe.Invoice;
      await this.db.upsertInvoice(this.client.mapInvoice(invoice));
      // Potentially trigger alerting logic here
      logger.warn(`Payment failed for invoice ${invoice.id}`, {
        customerId: invoice.customer,
        amount: invoice.amount_due,
      });
    });

    // ... 70+ more handlers
  }

  register(eventType: string, handler: WebhookHandlerFn): void {
    this.handlers.set(eventType, handler);
  }

  async processEvent(event: Stripe.Event): Promise<void> {
    const handler = this.handlers.get(event.type);

    if (!handler) {
      logger.warn(`No handler registered for ${event.type}`);
      return;
    }

    await handler(event);
  }
}
```

### Retry Logic for Failed Events

```typescript
// Background job to retry failed webhook events
async retryFailedEvents(): Promise<void> {
  const failedEvents = await this.db.query<WebhookEventRecord>(
    `SELECT * FROM stripe_webhook_events
     WHERE processed = FALSE
       AND retry_count < 3
       AND created_at > NOW() - INTERVAL '24 hours'
     ORDER BY created_at ASC
     LIMIT 100`
  );

  for (const eventRecord of failedEvents.rows) {
    try {
      // Reconstruct Stripe event
      const event: Stripe.Event = {
        id: eventRecord.id,
        type: eventRecord.type,
        data: eventRecord.data,
        created: Math.floor(eventRecord.created_at.getTime() / 1000),
        // ... other fields
      };

      await this.processEvent(event);

      await this.db.execute(
        `UPDATE stripe_webhook_events
         SET processed = TRUE, processed_at = NOW()
         WHERE id = $1`,
        [event.id]
      );

      logger.info(`Retry successful for event ${event.id}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';

      await this.db.execute(
        `UPDATE stripe_webhook_events
         SET retry_count = retry_count + 1, error = $2
         WHERE id = $1`,
        [eventRecord.id, message]
      );

      logger.error(`Retry failed for event ${eventRecord.id}`, { error: message });
    }
  }
}
```

---

## REST API Architecture

### Endpoint Categories

**1. Health Checks**:

```typescript
// Basic liveness
app.get('/health', async () => ({
  status: 'ok',
  plugin: 'stripe',
  timestamp: new Date().toISOString(),
}));

// Readiness (database check)
app.get('/ready', async (_, reply) => {
  try {
    await db.query('SELECT 1');
    return { ready: true };
  } catch (error) {
    return reply.status(503).send({ ready: false, error: 'Database unavailable' });
  }
});

// Liveness with metrics
app.get('/live', async () => {
  const stats = await db.getStats();
  return {
    alive: true,
    uptime: process.uptime(),
    memory: process.memoryUsage(),
    stats: {
      customers: stats.customers,
      subscriptions: stats.subscriptions,
    },
  };
});
```

**2. Webhook Receiver**:

```typescript
app.post('/webhook', async (request, reply) => {
  // Already covered in webhook section
});
```

**3. Sync Operations**:

```typescript
// Full sync
app.post('/api/sync', async (request) => {
  const options = request.body as SyncOptions;
  const result = await syncService.sync(options);
  return result;
});

// Incremental sync
app.post('/api/sync/incremental', async (request) => {
  const { since } = request.body as { since: string };
  const result = await syncService.incrementalSync(new Date(since));
  return result;
});

// Sync specific resource
app.post('/api/sync/:resource', async (request) => {
  const { resource } = request.params;
  const result = await syncService.sync({ resources: [resource] });
  return result;
});
```

**4. Data Queries**:

```typescript
// List customers
app.get('/api/customers', async (request) => {
  const { limit = '100', offset = '0', email } = request.query as Record<string, string>;

  let query = 'SELECT * FROM stripe_customers WHERE deleted_at IS NULL';
  const params: unknown[] = [];

  if (email) {
    params.push(`%${email}%`);
    query += ` AND email ILIKE $${params.length}`;
  }

  query += ` ORDER BY created_at DESC LIMIT $${params.length + 1} OFFSET $${params.length + 2}`;
  params.push(parseInt(limit), parseInt(offset));

  const result = await db.query(query, params);

  return {
    data: result.rows,
    total: result.rowCount,
    limit: parseInt(limit),
    offset: parseInt(offset),
  };
});

// Get customer by ID
app.get('/api/customers/:id', async (request, reply) => {
  const { id } = request.params;
  const customer = await db.getCustomer(id);

  if (!customer) {
    return reply.status(404).send({ error: 'Customer not found' });
  }

  return { data: customer };
});

// List subscriptions
app.get('/api/subscriptions', async (request) => {
  const { status, customer_id } = request.query as Record<string, string>;
  const subscriptions = await db.listSubscriptions({ status, customer_id });
  return { data: subscriptions };
});

// Get MRR
app.get('/api/mrr', async () => {
  const mrr = await db.query('SELECT * FROM stripe_mrr ORDER BY month DESC LIMIT 12');
  return { data: mrr.rows };
});

// Get customer LTV
app.get('/api/ltv', async (request) => {
  const { limit = '100' } = request.query as Record<string, string>;
  const ltv = await db.query(
    'SELECT * FROM stripe_customer_ltv ORDER BY total_revenue DESC LIMIT $1',
    [parseInt(limit)]
  );
  return { data: ltv.rows };
});
```

**5. Statistics**:

```typescript
app.get('/api/status', async () => {
  const stats = await db.getStats();
  return {
    plugin: 'stripe',
    version: '1.0.0',
    stats,
    timestamp: new Date().toISOString(),
  };
});

app.get('/api/stats/overview', async () => {
  const [customers, subscriptions, invoices, mrr] = await Promise.all([
    db.query('SELECT COUNT(*) FROM stripe_customers WHERE deleted_at IS NULL'),
    db.query('SELECT COUNT(*) FROM stripe_subscriptions WHERE status IN (\'active\', \'trialing\') AND deleted_at IS NULL'),
    db.query('SELECT COUNT(*), SUM(amount_paid) FROM stripe_invoices WHERE status = \'paid\' AND deleted_at IS NULL'),
    db.query('SELECT SUM(mrr_cents) AS total_mrr FROM stripe_mrr WHERE month = DATE_TRUNC(\'month\', NOW())'),
  ]);

  return {
    customers: parseInt(customers.rows[0].count),
    activeSubscriptions: parseInt(subscriptions.rows[0].count),
    paidInvoices: parseInt(invoices.rows[0].count),
    totalRevenue: parseFloat(invoices.rows[0].sum ?? 0) / 100,
    currentMRR: parseFloat(mrr.rows[0]?.total_mrr ?? 0),
  };
});
```

---

## CLI Command System

### Command Structure

```typescript
program
  .name('nself-stripe')
  .description('Stripe plugin for nself')
  .version('1.0.0');

// Global options
program
  .option('-v, --verbose', 'Enable verbose logging')
  .option('--config <path>', 'Path to config file')
  .hook('preAction', (thisCommand) => {
    const options = thisCommand.opts();
    if (options.verbose) {
      process.env.LOG_LEVEL = 'debug';
    }
  });

// Commands grouped by category
```

### Initialization Commands

```typescript
program
  .command('init')
  .description('Initialize database schema')
  .option('--drop', 'Drop existing tables before creating')
  .action(async (options) => {
    const db = new StripeDatabase();
    await db.connect();

    if (options.drop) {
      console.log('⚠️  Dropping existing tables...');
      await db.dropSchema();
    }

    await db.initializeSchema();
    console.log('✓ Database schema initialized');

    await db.disconnect();
  });

program
  .command('setup')
  .description('Interactive setup wizard')
  .action(async () => {
    const inquirer = (await import('inquirer')).default;

    const answers = await inquirer.prompt([
      {
        type: 'password',
        name: 'apiKey',
        message: 'Stripe API Key:',
        validate: (input) => input.startsWith('sk_') || 'Invalid API key format',
      },
      {
        type: 'password',
        name: 'webhookSecret',
        message: 'Webhook Secret (optional):',
      },
      {
        type: 'input',
        name: 'databaseUrl',
        message: 'Database URL:',
        default: 'postgresql://localhost:5432/nself',
      },
    ]);

    // Save to .env file
    const envContent = `
STRIPE_API_KEY=${answers.apiKey}
STRIPE_WEBHOOK_SECRET=${answers.webhookSecret}
DATABASE_URL=${answers.databaseUrl}
PORT=3001
HOST=0.0.0.0
    `.trim();

    await fs.writeFile('.env', envContent);
    console.log('✓ Configuration saved to .env');
  });
```

### Sync Commands

```typescript
program
  .command('sync')
  .description('Sync all Stripe data to database')
  .option('-r, --resources <items>', 'Resources to sync (comma-separated)')
  .option('-i, --incremental', 'Incremental sync only')
  .option('--since <date>', 'Sync data since date (ISO format)')
  .action(async (options) => {
    const config = loadConfig();
    const client = new StripeClient(config.stripeApiKey);
    const db = new StripeDatabase();
    const syncService = new StripeSyncService(client, db);

    await db.connect();

    const syncOptions: SyncOptions = {
      incremental: options.incremental,
      since: options.since ? new Date(options.since) : undefined,
      resources: options.resources?.split(','),
    };

    console.log('🔄 Starting sync...');
    const result = await syncService.sync(syncOptions);

    console.log('✓ Sync complete');
    console.log(`  Duration: ${result.duration}ms`);
    console.log(`  Customers: ${result.stats.customers}`);
    console.log(`  Subscriptions: ${result.stats.subscriptions}`);
    console.log(`  Invoices: ${result.stats.invoices}`);

    await db.disconnect();
  });
```

### Server Commands

```typescript
program
  .command('server')
  .description('Start webhook server')
  .option('-p, --port <number>', 'Port number', '3001')
  .option('-h, --host <string>', 'Host address', '0.0.0.0')
  .option('--no-auth', 'Disable API key authentication')
  .action(async (options) => {
    const config = loadConfig({
      port: parseInt(options.port),
      host: options.host,
      security: {
        ...loadConfig().security,
        apiKey: options.auth ? loadConfig().security.apiKey : undefined,
      },
    });

    const server = await createServer(config);

    await server.listen({ port: config.port, host: config.host });

    console.log('✓ Server running');
    console.log(`  URL: http://${config.host}:${config.port}`);
    console.log(`  Webhook: http://${config.host}:${config.port}/webhook`);
    console.log(`  API: http://${config.host}:${config.port}/api`);
    console.log(`  Auth: ${config.security.apiKey ? 'Enabled' : 'Disabled'}`);
  });
```

### Query Commands

```typescript
program
  .command('customers')
  .description('List customers')
  .option('-l, --limit <number>', 'Number of results', '10')
  .option('-e, --email <email>', 'Filter by email')
  .option('--json', 'Output as JSON')
  .action(async (options) => {
    const db = new StripeDatabase();
    await db.connect();

    const customers = await db.listCustomers({
      limit: parseInt(options.limit),
      email: options.email,
    });

    if (options.json) {
      console.log(JSON.stringify(customers, null, 2));
    } else {
      console.table(customers.map(c => ({
        ID: c.id.substring(0, 20),
        Email: c.email ?? 'N/A',
        Name: c.name ?? 'N/A',
        Created: c.created_at.toLocaleDateString(),
      })));
    }

    await db.disconnect();
  });

program
  .command('subscriptions')
  .description('List subscriptions')
  .option('-s, --status <status>', 'Filter by status')
  .option('-c, --customer <id>', 'Filter by customer ID')
  .action(async (options) => {
    const db = new StripeDatabase();
    await db.connect();

    const subscriptions = await db.listSubscriptions({
      status: options.status,
      customer_id: options.customer,
    });

    console.table(subscriptions.map(s => ({
      ID: s.id.substring(0, 20),
      Customer: s.customer_id.substring(0, 20),
      Status: s.status,
      'Current Period': `${s.current_period_start.toLocaleDateString()} - ${s.current_period_end.toLocaleDateString()}`,
    })));

    await db.disconnect();
  });

program
  .command('mrr')
  .description('Show monthly recurring revenue')
  .option('-m, --months <number>', 'Number of months', '12')
  .action(async (options) => {
    const db = new StripeDatabase();
    await db.connect();

    const mrr = await db.query(
      'SELECT * FROM stripe_mrr ORDER BY month DESC LIMIT $1',
      [parseInt(options.months)]
    );

    console.table(mrr.rows.map(row => ({
      Month: new Date(row.month).toLocaleDateString('en-US', { year: 'numeric', month: 'long' }),
      Subscriptions: row.subscription_count,
      Customers: row.unique_customers,
      'MRR ($)': row.mrr_cents.toFixed(2),
    })));

    await db.disconnect();
  });
```

---

## Shared Utilities

### Logger

```typescript
export class Logger {
  private name: string;
  private level: LogLevel;

  constructor(name: string, level: LogLevel = 'info') {
    this.name = name;
    this.level = level;
  }

  debug(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('debug')) {
      this.log('debug', message, meta);
    }
  }

  info(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('info')) {
      this.log('info', message, meta);
    }
  }

  warn(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('warn')) {
      this.log('warn', message, meta);
    }
  }

  error(message: string, error?: Error | unknown): void {
    const meta = error instanceof Error
      ? { error: error.message, stack: error.stack }
      : { error };
    this.log('error', message, meta);
  }

  success(message: string, meta?: Record<string, unknown>): void {
    this.log('info', `✓ ${message}`, meta);
  }

  private log(level: LogLevel, message: string, meta?: Record<string, unknown>): void {
    const timestamp = new Date().toISOString();
    const levelStr = level.toUpperCase().padEnd(5);
    const metaStr = meta ? ` ${JSON.stringify(meta)}` : '';

    console.log(`${timestamp} ${levelStr} [${this.name}] ${message}${metaStr}`);
  }
}

export function createLogger(name: string): Logger {
  const level = (process.env.LOG_LEVEL ?? 'info') as LogLevel;
  return new Logger(name, level);
}
```

### Database Connection Pool

```typescript
export class Database {
  private pool: pg.Pool;
  private connected = false;

  constructor(config: DatabaseConfig) {
    this.pool = new Pool({
      host: config.host,
      port: config.port,
      database: config.database,
      user: config.user,
      password: config.password,
      ssl: config.ssl ? { rejectUnauthorized: false } : undefined,
      max: config.maxConnections ?? 10,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 5000,
    });

    this.pool.on('error', (err) => {
      logger.error('Unexpected database pool error', err);
    });
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    const client = await this.pool.connect();
    client.release();
    this.connected = true;
  }

  async query<T>(text: string, params?: unknown[]): Promise<pg.QueryResult<T>> {
    return this.pool.query<T>(text, params);
  }

  async execute(text: string, params?: unknown[]): Promise<number> {
    const result = await this.pool.query(text, params);
    return result.rowCount ?? 0;
  }

  async transaction<T>(callback: (client: pg.PoolClient) => Promise<T>): Promise<T> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      const result = await callback(client);
      await client.query('COMMIT');
      return result;
    } catch (error) {
      await client.query('ROLLBACK');
      throw error;
    } finally {
      client.release();
    }
  }
}

export function createDatabase(config?: DatabaseConfig): Database {
  const dbConfig = config ?? {
    host: process.env.DB_HOST ?? 'localhost',
    port: parseInt(process.env.DB_PORT ?? '5432'),
    database: process.env.DB_NAME ?? 'nself',
    user: process.env.DB_USER ?? 'postgres',
    password: process.env.DB_PASSWORD ?? '',
    ssl: process.env.DB_SSL === 'true',
  };

  return new Database(dbConfig);
}
```

### HTTP Client

```typescript
export class HttpClient {
  private config: HttpClientConfig;

  async get<T>(endpoint: string, params?: Record<string, string>): Promise<T> {
    return this.request<T>('GET', endpoint, { params });
  }

  async post<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>('POST', endpoint, { body: data });
  }

  private async request<T>(
    method: string,
    endpoint: string,
    options?: { body?: unknown; params?: Record<string, string> }
  ): Promise<T> {
    let url = `${this.config.baseUrl}${endpoint}`;

    if (options?.params) {
      const searchParams = new URLSearchParams(options.params);
      url += `?${searchParams.toString()}`;
    }

    const response = await fetch(url, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...this.config.headers,
      },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return response.json();
  }
}
```

### Webhook Utilities

```typescript
export function verifyStripeSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const [timestampPart, signaturePart] = signature.split(',');
  const timestamp = timestampPart.split('=')[1];
  const expectedSignature = signaturePart.split('=')[1];

  // Prevent replay attacks
  const currentTime = Math.floor(Date.now() / 1000);
  if (Math.abs(currentTime - parseInt(timestamp)) > 300) {
    return false;
  }

  // Verify signature
  const signedPayload = `${timestamp}.${payload}`;
  const expectedSig = crypto
    .createHmac('sha256', secret)
    .update(signedPayload)
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(expectedSignature),
    Buffer.from(expectedSig)
  );
}

export function verifyGitHubSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const expectedSig = `sha256=${crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex')}`;

  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expectedSig)
  );
}

export function verifyShopifySignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const expectedSig = crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('base64');

  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expectedSig)
  );
}
```

---

## Registry System

### Registry Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Registry Distribution                     │
└─────────────────────────────────────────────────────────────┘

┌────────────────────┐
│  registry.json     │  Source of truth (GitHub)
│  (main branch)     │
└─────────┬──────────┘
          │
          │ On version tag (v*):
          │ 1. GitHub Actions validates
          │ 2. Updates timestamp/checksums
          │ 3. Commits back to main
          │ 4. Notifies Worker
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│        Cloudflare Worker (plugins.nself.org)                 │
│                                                              │
│  ┌────────────────┐         ┌─────────────────────────┐     │
│  │  KV Cache      │◄────────│  Worker Code            │     │
│  │  - registry    │         │  - Fetches from GitHub  │     │
│  │  - 1 hour TTL  │         │  - Caches in KV         │     │
│  └────────────────┘         │  - Serves requests      │     │
│                             └─────────────────────────┘     │
│                                       │                      │
└───────────────────────────────────────┼──────────────────────┘
                                        │
                              ┌─────────┴─────────┐
                              │                   │
                              ▼                   ▼
                    ┌──────────────────┐ ┌──────────────────┐
                    │  nself CLI       │ │  nself CLI       │
                    │  (User A)        │ │  (User B)        │
                    │                  │ │                  │
                    │  $ nself plugin  │ │  $ nself plugin  │
                    │    list          │ │    install       │
                    └──────────────────┘ └──────────────────┘
```

### Registry JSON Structure

```json
{
  "$schema": "./registry-schema.json",
  "version": "1.0.0",
  "lastUpdated": "2026-01-30T12:00:00Z",
  "plugins": {
    "stripe": {
      "name": "stripe",
      "version": "1.0.0",
      "description": "Stripe billing data sync with webhook handling",
      "author": "nself",
      "license": "Source-Available",
      "homepage": "https://github.com/acamarata/nself-plugins/tree/main/plugins/stripe",
      "repository": "https://github.com/acamarata/nself-plugins",
      "path": "plugins/stripe",
      "minNselfVersion": "0.4.8",
      "category": "billing",
      "tags": ["payments", "billing", "subscriptions"],
      "implementation": {
        "language": "typescript",
        "runtime": "node",
        "minNodeVersion": "18.0.0",
        "entryPoint": "ts/dist/index.js",
        "cli": "ts/dist/cli.js",
        "cliName": "nself-stripe",
        "defaultPort": 3001,
        "packageManager": "npm",
        "framework": "fastify"
      },
      "tables": [
        "stripe_customers",
        "stripe_products",
        "stripe_subscriptions"
      ],
      "views": [
        "stripe_mrr",
        "stripe_active_subscriptions"
      ],
      "webhooks": [
        "customer.created",
        "customer.updated",
        "subscription.created"
      ],
      "cliCommands": [
        {"name": "sync", "description": "Sync all Stripe data"},
        {"name": "server", "description": "Start webhook server"}
      ],
      "apiEndpoints": [
        {"method": "POST", "path": "/webhook", "description": "Stripe webhook receiver"},
        {"method": "GET", "path": "/api/customers", "description": "List customers"}
      ],
      "envVars": [
        {"name": "STRIPE_API_KEY", "required": true, "description": "Stripe API secret key"},
        {"name": "DATABASE_URL", "required": true, "description": "PostgreSQL connection"}
      ],
      "dependencies": ["pg", "stripe", "fastify", "commander"],
      "checksums": {"sha256": "abc123..."}
    }
  }
}
```

### Cloudflare Worker Implementation

```javascript
// .workers/plugins-registry/src/index.js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // GET /registry.json - Full registry
    if (url.pathname === '/registry.json') {
      return await this.getRegistry(env);
    }

    // GET /plugins/:name - Individual plugin
    if (url.pathname.startsWith('/plugins/')) {
      const name = url.pathname.split('/')[2];
      return await this.getPlugin(name, env);
    }

    // POST /api/sync - Cache invalidation
    if (url.pathname === '/api/sync' && request.method === 'POST') {
      return await this.invalidateCache(request, env);
    }

    return new Response('Not Found', { status: 404 });
  },

  async getRegistry(env) {
    // Try cache first
    const cached = await env.REGISTRY_CACHE.get('registry', { type: 'json' });
    if (cached) {
      return new Response(JSON.stringify(cached), {
        headers: {
          'Content-Type': 'application/json',
          'Cache-Control': 'public, max-age=3600',
          'X-Cache': 'HIT'
        }
      });
    }

    // Fetch from GitHub
    const response = await fetch(
      'https://raw.githubusercontent.com/acamarata/nself-plugins/main/registry.json'
    );

    if (!response.ok) {
      return new Response('Registry unavailable', { status: 503 });
    }

    const registry = await response.json();

    // Cache for 1 hour
    await env.REGISTRY_CACHE.put('registry', JSON.stringify(registry), {
      expirationTtl: 3600
    });

    return new Response(JSON.stringify(registry), {
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'public, max-age=3600',
        'X-Cache': 'MISS'
      }
    });
  },

  async getPlugin(name, env) {
    const registry = await this.getRegistry(env);
    const data = await registry.json();

    const plugin = data.plugins[name];
    if (!plugin) {
      return new Response('Plugin not found', { status: 404 });
    }

    return new Response(JSON.stringify(plugin), {
      headers: { 'Content-Type': 'application/json' }
    });
  },

  async invalidateCache(request, env) {
    // Verify auth token
    const authHeader = request.headers.get('Authorization');
    if (!authHeader || authHeader !== `Bearer ${env.GITHUB_SYNC_TOKEN}`) {
      return new Response('Unauthorized', { status: 401 });
    }

    // Delete cached registry
    await env.REGISTRY_CACHE.delete('registry');

    return new Response(JSON.stringify({ invalidated: true }), {
      headers: { 'Content-Type': 'application/json' }
    });
  }
};
```

---

## Security Model

### 1. API Key Authentication

```typescript
// Middleware for protected endpoints
export function createAuthHook(apiKey: string) {
  return async (request: FastifyRequest, reply: FastifyReply) => {
    // Skip health checks
    if (request.url.startsWith('/health') ||
        request.url.startsWith('/ready') ||
        request.url.startsWith('/live')) {
      return;
    }

    const authHeader = request.headers.authorization;

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return reply.status(401).send({ error: 'Missing authorization header' });
    }

    const token = authHeader.substring(7);

    if (token !== apiKey) {
      return reply.status(401).send({ error: 'Invalid API key' });
    }
  };
}
```

### 2. Rate Limiting

```typescript
export class ApiRateLimiter {
  private requests: Map<string, number[]> = new Map();
  private maxRequests: number;
  private windowMs: number;

  constructor(maxRequests: number, windowMs: number) {
    this.maxRequests = maxRequests;
    this.windowMs = windowMs;
  }

  isAllowed(identifier: string): boolean {
    const now = Date.now();
    const windowStart = now - this.windowMs;

    // Get recent requests
    const requests = this.requests.get(identifier) ?? [];

    // Remove old requests
    const recentRequests = requests.filter(time => time > windowStart);

    // Check limit
    if (recentRequests.length >= this.maxRequests) {
      return false;
    }

    // Add new request
    recentRequests.push(now);
    this.requests.set(identifier, recentRequests);

    return true;
  }
}

export function createRateLimitHook(limiter: ApiRateLimiter) {
  return async (request: FastifyRequest, reply: FastifyReply) => {
    const identifier = request.ip;

    if (!limiter.isAllowed(identifier)) {
      return reply.status(429).send({
        error: 'Rate limit exceeded',
        retryAfter: 60
      });
    }
  };
}
```

### 3. Webhook Signature Enforcement

```typescript
// Reject webhooks without valid signatures
if (!config.webhookSecret) {
  logger.warn('Webhook secret not configured - signatures will not be verified!');
}

app.post('/webhook', async (request, reply) => {
  const signature = request.headers['stripe-signature'];

  if (!signature) {
    logger.warn('Webhook received without signature');
    return reply.status(400).send({ error: 'Missing signature' });
  }

  if (!config.webhookSecret) {
    logger.error('Cannot verify webhook - secret not configured');
    return reply.status(500).send({ error: 'Server configuration error' });
  }

  const isValid = verifyStripeSignature(
    request.rawBody,
    signature,
    config.webhookSecret
  );

  if (!isValid) {
    logger.warn('Invalid webhook signature', {
      ip: request.ip,
      eventType: (request.body as any)?.type
    });
    return reply.status(401).send({ error: 'Invalid signature' });
  }

  // Process webhook
});
```

### 4. Input Validation

```typescript
// Validate query parameters
app.get('/api/customers', async (request, reply) => {
  const { limit, offset } = request.query as Record<string, string>;

  // Validate limit
  const parsedLimit = parseInt(limit ?? '100');
  if (isNaN(parsedLimit) || parsedLimit < 1 || parsedLimit > 1000) {
    return reply.status(400).send({
      error: 'Invalid limit parameter (must be 1-1000)'
    });
  }

  // Validate offset
  const parsedOffset = parseInt(offset ?? '0');
  if (isNaN(parsedOffset) || parsedOffset < 0) {
    return reply.status(400).send({
      error: 'Invalid offset parameter (must be >= 0)'
    });
  }

  // Continue with validated params
});
```

### 5. SQL Injection Prevention

```typescript
// ALWAYS use parameterized queries
// ✓ Good
await db.query(
  'SELECT * FROM stripe_customers WHERE email = $1',
  [userInput]
);

// ✗ Bad (SQL injection vulnerability)
await db.query(
  `SELECT * FROM stripe_customers WHERE email = '${userInput}'`
);
```

### 6. Environment Variable Security

```bash
# .env file (gitignored)
STRIPE_API_KEY=sk_live_***  # Never commit to git
DATABASE_URL=postgresql://user:pass@localhost/db  # Sensitive
WEBHOOK_SECRET=whsec_***  # Critical for security
API_KEY=secret_***  # For REST API auth
```

```typescript
// Load and validate
export function loadConfig(): Config {
  dotenv.config();

  const apiKey = process.env.STRIPE_API_KEY;
  if (!apiKey) {
    throw new Error('STRIPE_API_KEY environment variable is required');
  }

  // Warn if using test key in production
  if (process.env.NODE_ENV === 'production' && apiKey.startsWith('sk_test_')) {
    logger.warn('⚠️  Using test API key in production environment!');
  }

  return {
    stripeApiKey: apiKey,
    webhookSecret: process.env.STRIPE_WEBHOOK_SECRET,
    // ... more config
  };
}
```

---

## Data Flow Diagrams

### Full Sync Data Flow

```
[External API]
    │
    │ 1. Plugin requests data (paginated)
    ▼
┌────────────────┐
│  API Client    │
│  - Fetch pages │
│  - Map types   │
└───────┬────────┘
        │
        │ 2. Mapped records
        ▼
┌────────────────┐
│  Sync Service  │
│  - Batch       │
│  - Order       │
│  - Progress    │
└───────┬────────┘
        │
        │ 3. Upsert records
        ▼
┌────────────────┐
│   Database     │
│  - Upsert      │
│  - Index       │
│  - Update stats│
└────────────────┘
```

### Webhook Data Flow

```
[External Service]
    │
    │ 1. Event occurs (e.g., customer updated)
    │
    │ 2. POST to /webhook with signature
    ▼
┌─────────────────┐
│  HTTP Server    │
│  - Parse body   │
│  - Verify sig   │
└────────┬────────┘
         │
         │ 3. Store raw event
         ▼
┌─────────────────┐
│    Database     │
│  webhook_events │
└────────┬────────┘
         │
         │ 4. Route to handler
         ▼
┌─────────────────┐
│ Webhook Handler │
│  - Parse event  │
│  - Fetch details│
└────────┬────────┘
         │
         │ 5. Upsert affected records
         ▼
┌─────────────────┐
│    Database     │
│  - customers    │
│  - subscriptions│
│  - etc.         │
└────────┬────────┘
         │
         │ 6. Mark event processed
         ▼
┌─────────────────┐
│    Database     │
│  webhook_events │
│  processed=true │
└─────────────────┘
```

### Query Data Flow

```
[Client]
    │
    │ 1. GET /api/customers?limit=100
    ▼
┌─────────────────┐
│  HTTP Server    │
│  - Auth check   │
│  - Rate limit   │
│  - Validate     │
└────────┬────────┘
         │
         │ 2. Execute query
         ▼
┌─────────────────┐
│    Database     │
│  - SELECT       │
│  - Apply limits │
│  - Sort         │
└────────┬────────┘
         │
         │ 3. Format response
         ▼
┌─────────────────┐
│  HTTP Server    │
│  - JSON         │
│  - Pagination   │
└────────┬────────┘
         │
         │ 4. Return data
         ▼
    [Client]
```

---

## Implementation Patterns

### 1. Upsert Pattern (Idempotent Writes)

```typescript
// Always use ON CONFLICT for idempotency
async upsertRecord(record: Record): Promise<void> {
  await this.db.execute(
    `INSERT INTO table_name (id, field1, field2, synced_at)
     VALUES ($1, $2, $3, NOW())
     ON CONFLICT (id) DO UPDATE SET
       field1 = EXCLUDED.field1,
       field2 = EXCLUDED.field2,
       updated_at = NOW(),
       synced_at = NOW()`,
    [record.id, record.field1, record.field2]
  );
}
```

### 2. Pagination Pattern (Memory Efficient)

```typescript
// Generator for streaming large datasets
async *listAll(): AsyncGenerator<Record[]> {
  let hasMore = true;
  let cursor: string | undefined;

  while (hasMore) {
    const response = await this.api.list({
      limit: 100,
      starting_after: cursor
    });

    yield response.data.map(item => this.map(item));

    hasMore = response.has_more;
    cursor = response.data.length > 0
      ? response.data[response.data.length - 1].id
      : undefined;
  }
}

// Usage
for await (const batch of client.listAll()) {
  for (const record of batch) {
    await db.upsert(record);
  }
}
```

### 3. Error Handling Pattern

```typescript
async processEvent(event: Event): Promise<void> {
  try {
    // 1. Store raw event first
    await this.db.insertEvent(event);

    try {
      // 2. Process event
      await this.handleEvent(event);

      // 3. Mark success
      await this.db.markProcessed(event.id);
    } catch (processingError) {
      // 4. Store error for debugging
      await this.db.markProcessed(
        event.id,
        processingError instanceof Error
          ? processingError.message
          : 'Unknown error'
      );

      // 5. Re-throw to trigger retry logic
      throw processingError;
    }
  } catch (storageError) {
    // Critical failure - couldn't even store event
    logger.error('Failed to store event', storageError);
    throw storageError;
  }
}
```

### 4. Configuration Loading Pattern

```typescript
export function loadConfig(overrides?: Partial<Config>): Config {
  // 1. Load from .env
  dotenv.config();

  // 2. Parse with defaults
  const config: Config = {
    apiKey: process.env.API_KEY ?? '',
    port: parseInt(process.env.PORT ?? '3001'),
    host: process.env.HOST ?? '0.0.0.0',
    database: {
      host: process.env.DB_HOST ?? 'localhost',
      port: parseInt(process.env.DB_PORT ?? '5432'),
      database: process.env.DB_NAME ?? 'nself',
      user: process.env.DB_USER ?? 'postgres',
      password: process.env.DB_PASSWORD ?? '',
      ssl: process.env.DB_SSL === 'true',
    },
    ...overrides,
  };

  // 3. Validate required fields
  if (!config.apiKey) {
    throw new Error('API_KEY is required');
  }

  return config;
}
```

---

## Performance Considerations

### 1. Database Connection Pooling

```typescript
// Use connection pools, not individual connections
const pool = new Pool({
  max: 10,  // Maximum connections
  idleTimeoutMillis: 30000,  // Close idle connections
  connectionTimeoutMillis: 5000,  // Timeout for new connections
});
```

### 2. Batch Operations

```typescript
// Don't insert one at a time
// ✗ Bad
for (const record of records) {
  await db.execute('INSERT INTO ...');  // 1000 queries
}

// ✓ Good - Batch in chunks
const BATCH_SIZE = 100;
for (let i = 0; i < records.length; i += BATCH_SIZE) {
  const batch = records.slice(i, i + BATCH_SIZE);
  await db.transaction(async (client) => {
    for (const record of batch) {
      await client.query('INSERT INTO ...', [record]);
    }
  });
}
```

### 3. Index Strategy

```sql
-- Index frequently queried columns
CREATE INDEX idx_customers_email ON customers(email);

-- Partial indexes for common filters
CREATE INDEX idx_active_subs ON subscriptions(customer_id)
  WHERE status = 'active' AND deleted_at IS NULL;

-- Covering indexes for read-heavy queries
CREATE INDEX idx_customer_lookup ON customers(email, name, created_at);
```

### 4. Pagination Limits

```typescript
// Enforce reasonable limits
app.get('/api/customers', async (request, reply) => {
  const limit = Math.min(
    parseInt(request.query.limit ?? '100'),
    1000  // Maximum 1000 records per request
  );

  // Use cursor-based pagination for large datasets
  const cursor = request.query.cursor;
  // ...
});
```

### 5. Caching Strategy

```typescript
// Cache expensive queries
const cache = new Map<string, { data: unknown; expires: number }>();

async function getCachedMRR(): Promise<MRRData[]> {
  const key = 'mrr';
  const cached = cache.get(key);

  if (cached && cached.expires > Date.now()) {
    return cached.data as MRRData[];
  }

  const data = await db.query('SELECT * FROM stripe_mrr');

  cache.set(key, {
    data: data.rows,
    expires: Date.now() + 300000  // 5 minutes
  });

  return data.rows;
}
```

---

## Conclusion

The nself plugin system is a production-ready, scalable architecture for syncing external service data to PostgreSQL. Key strengths:

1. **Type Safety**: Full TypeScript coverage prevents runtime errors
2. **Idempotency**: Upsert patterns allow safe re-runs
3. **Real-time**: Webhooks keep data current within seconds
4. **Observability**: Comprehensive logging and statistics
5. **Security**: Signature verification, rate limiting, authentication
6. **Modularity**: Shared utilities reduce duplication
7. **Registry**: Centralized discovery and versioning

This architecture has been battle-tested with Stripe, GitHub, and Shopify plugins, each syncing 15-21 tables with 20-70+ webhook events.

---

**For More Information**:
- Plugin Development: [DEVELOPMENT.md](../DEVELOPMENT.md)
- TypeScript Guide: [TYPESCRIPT_PLUGIN_GUIDE.md](../TYPESCRIPT_PLUGIN_GUIDE.md)
- Contributing: [CONTRIBUTING.md](../CONTRIBUTING.md)
- Individual Plugin Docs: [plugins/](../plugins/)
