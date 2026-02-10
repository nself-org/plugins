# Best Practices Guide

**Version**: 1.0.0
**Last Updated**: January 30, 2026

---

## Table of Contents

1. [Code Organization](#1-code-organization)
2. [Error Handling](#2-error-handling)
3. [Database Practices](#3-database-practices)
4. [API Integration](#4-api-integration)
5. [Webhook Handling](#5-webhook-handling)
6. [Security Practices](#6-security-practices)
7. [Performance Optimization](#7-performance-optimization)
8. [Testing Strategies](#8-testing-strategies)
9. [Monitoring & Observability](#9-monitoring--observability)
10. [Documentation](#10-documentation)
11. [Version Control](#11-version-control)
12. [Dependency Management](#12-dependency-management)

---

## 1. Code Organization

### 1.1 Project Structure

Follow the standardized plugin structure for consistency:

```
plugins/<name>/
├── plugin.json           # Plugin manifest (metadata, config, dependencies)
└── ts/
    ├── src/
    │   ├── types.ts      # All TypeScript interfaces (API + DB)
    │   ├── config.ts     # Environment variable loading
    │   ├── client.ts     # API client with rate limiting
    │   ├── database.ts   # Schema and CRUD operations
    │   ├── sync.ts       # Data synchronization orchestration
    │   ├── webhooks.ts   # Webhook event handlers
    │   ├── server.ts     # Fastify HTTP server
    │   ├── cli.ts        # Commander.js CLI
    │   └── index.ts      # Module exports
    ├── package.json
    ├── tsconfig.json
    └── .env.example
```

### 1.2 File Responsibilities

**Maintain strict separation of concerns:**

| File | Purpose | What to Include |
|------|---------|-----------------|
| `types.ts` | Type definitions | API response types, DB record types, config interfaces |
| `config.ts` | Configuration | Environment variable validation, default values |
| `client.ts` | API integration | API methods, rate limiting, response mapping |
| `database.ts` | Data persistence | Schema DDL, CRUD operations, queries |
| `sync.ts` | Data orchestration | Full/incremental sync, progress tracking |
| `webhooks.ts` | Event processing | Webhook handlers, signature verification |
| `server.ts` | HTTP interface | REST endpoints, webhook routes, health checks |
| `cli.ts` | Command-line | User commands, argument parsing, output formatting |
| `index.ts` | Public API | Module exports only |

### 1.3 Naming Conventions

**Database Objects:**
```typescript
// Tables: <service>_<resource> (plural)
stripe_customers
github_repositories
shopify_products

// Indexes: idx_<table>_<column(s)>
idx_stripe_customers_email
idx_github_repos_owner_name

// Views: <service>_<description>
stripe_mrr
github_active_repos
shopify_inventory_summary
```

**TypeScript Types:**
```typescript
// API response types: match service naming
interface StripeCustomer { ... }
interface GitHubRepository { ... }

// Database record types: add "Record" suffix
interface StripeCustomerRecord { ... }
interface GitHubRepositoryRecord { ... }

// Configuration types: end with "Config"
interface StripeConfig { ... }
interface SyncOptions { ... }
```

**Functions and Methods:**
```typescript
// Use clear, action-oriented names
async syncCustomers(): Promise<void>
async upsertCustomer(customer: CustomerRecord): Promise<void>
async getCustomerById(id: string): Promise<CustomerRecord | null>

// Handler functions: handle<Resource><Action>
async handleCustomerCreated(event: Event): Promise<void>
async handleSubscriptionUpdated(event: Event): Promise<void>
```

### 1.4 Import Patterns

**Use consistent import ordering:**

```typescript
// 1. External dependencies
import Stripe from 'stripe';
import Fastify from 'fastify';
import { Command } from 'commander';

// 2. Shared utilities
import { createLogger, Database } from '@nself/plugin-utils';

// 3. Local modules (with .js extension for NodeNext)
import type { StripeCustomerRecord } from './types.js';
import { StripeClient } from './client.js';
import { StripeDatabase } from './database.js';
import { loadConfig } from './config.js';
```

**Always use `.js` extensions** for local imports when using NodeNext module resolution:

```typescript
// CORRECT
import { SomeType } from './types.js';

// WRONG - will cause TypeScript errors
import { SomeType } from './types';
```

### 1.5 Module Exports

Keep `index.ts` clean and focused on exports only:

```typescript
// index.ts
export * from './types.js';
export { StripeClient } from './client.js';
export { StripeDatabase } from './database.js';
export { StripeSyncService } from './sync.js';
export { createServer } from './server.js';
export { loadConfig } from './config.js';
```

---

## 2. Error Handling

### 2.1 Try-Catch Patterns

**Always catch and handle errors appropriately:**

```typescript
// GOOD: Specific error handling with logging
async syncCustomers(): Promise<number> {
  try {
    const customers = await this.client.listAllCustomers();
    await this.db.upsertCustomers(customers);
    logger.info('Customers synced', { count: customers.length });
    return customers.length;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to sync customers', { error: message });
    throw error; // Re-throw for caller to handle
  }
}

// BAD: Silent failure
async syncCustomers(): Promise<number> {
  try {
    const customers = await this.client.listAllCustomers();
    await this.db.upsertCustomers(customers);
    return customers.length;
  } catch (error) {
    return 0; // Don't hide errors!
  }
}
```

### 2.2 Error Logging

**Extract error messages safely:**

```typescript
// GOOD: Safe error message extraction
catch (error) {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Operation failed', { error: message });
  throw error;
}

// ACCEPTABLE: Include stack trace for debugging
catch (error) {
  logger.error('Operation failed', {
    error: error instanceof Error ? error.message : String(error),
    stack: error instanceof Error ? error.stack : undefined,
  });
  throw error;
}

// BAD: Unsafe error access
catch (error) {
  logger.error('Operation failed', { error: error.message }); // May be undefined
}
```

### 2.3 HTTP Error Handling

**Return appropriate status codes and messages:**

```typescript
// In server.ts
app.get('/api/customers/:id', async (request, reply) => {
  const { id } = request.params as { id: string };

  try {
    const customer = await db.getCustomer(id);

    if (!customer) {
      return reply.status(404).send({
        error: 'Customer not found',
        id,
      });
    }

    return customer;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to fetch customer', { id, error: message });
    return reply.status(500).send({
      error: 'Internal server error',
      message: 'Failed to retrieve customer',
    });
  }
});
```

### 2.4 Database Error Recovery

**Handle database errors gracefully:**

```typescript
async upsertCustomer(customer: CustomerRecord): Promise<void> {
  try {
    await this.db.execute(
      `INSERT INTO stripe_customers (...) VALUES (...)
       ON CONFLICT (id) DO UPDATE SET ...`,
      [customer.id, /* ... */]
    );
  } catch (error) {
    if (error instanceof Error && error.message.includes('unique constraint')) {
      // Handle specific constraint violations
      logger.warn('Duplicate customer', { id: customer.id });
      // Retry or skip
    } else {
      logger.error('Failed to upsert customer', {
        id: customer.id,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
      throw error;
    }
  }
}
```

### 2.5 Webhook Error Handling

**Store and track webhook processing errors:**

```typescript
async handle(event: WebhookEvent): Promise<void> {
  // 1. Store raw event immediately
  await this.db.insertWebhookEvent(event);

  try {
    // 2. Process event
    const handler = this.handlers.get(event.type);
    if (handler) {
      await handler(event);
    }

    // 3. Mark as processed
    await this.db.markEventProcessed(event.id);
    logger.success('Webhook processed', { type: event.type, id: event.id });
  } catch (error) {
    // 4. Store error for retry/debugging
    const message = error instanceof Error ? error.message : 'Unknown error';
    await this.db.markEventProcessed(event.id, message);
    logger.error('Webhook processing failed', {
      type: event.type,
      id: event.id,
      error: message,
    });
    // Don't throw - return 200 to prevent retries for unrecoverable errors
  }
}
```

---

## 3. Database Practices

### 3.1 Schema Design

**Follow PostgreSQL best practices:**

```sql
-- GOOD: Well-structured table
CREATE TABLE IF NOT EXISTS stripe_customers (
    -- Primary key
    id VARCHAR(255) PRIMARY KEY,

    -- Required fields (NOT NULL)
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Optional fields (nullable)
    name VARCHAR(255),
    phone VARCHAR(255),
    description TEXT,

    -- Structured data
    metadata JSONB DEFAULT '{}',
    address JSONB,

    -- Audit fields
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Add indexes for common queries
CREATE INDEX IF NOT EXISTS idx_stripe_customers_email
    ON stripe_customers(email);

CREATE INDEX IF NOT EXISTS idx_stripe_customers_created
    ON stripe_customers(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_stripe_customers_deleted
    ON stripe_customers(deleted_at)
    WHERE deleted_at IS NOT NULL;
```

### 3.2 Indexing Strategy

**Index columns used in WHERE, JOIN, and ORDER BY:**

```sql
-- Index for lookup queries
CREATE INDEX idx_subscriptions_customer
    ON stripe_subscriptions(customer_id);

-- Index for status filtering
CREATE INDEX idx_subscriptions_status
    ON stripe_subscriptions(status);

-- Composite index for common query patterns
CREATE INDEX idx_subscriptions_customer_status
    ON stripe_subscriptions(customer_id, status);

-- Partial index for active records only
CREATE INDEX idx_subscriptions_active
    ON stripe_subscriptions(customer_id, current_period_end)
    WHERE status = 'active';

-- Index for JSONB queries
CREATE INDEX idx_customers_metadata
    ON stripe_customers USING GIN (metadata);
```

### 3.3 Upsert Patterns

**Use ON CONFLICT for idempotent operations:**

```typescript
// GOOD: Upsert with conflict resolution
async upsertCustomer(customer: CustomerRecord): Promise<void> {
  await this.db.execute(
    `INSERT INTO stripe_customers (
       id, email, name, metadata, created_at, synced_at
     )
     VALUES ($1, $2, $3, $4, $5, NOW())
     ON CONFLICT (id) DO UPDATE SET
       email = EXCLUDED.email,
       name = EXCLUDED.name,
       metadata = EXCLUDED.metadata,
       synced_at = NOW()`,
    [customer.id, customer.email, customer.name, customer.metadata, customer.created_at]
  );
}

// BETTER: Use shared Database.upsert helper
async upsertCustomer(customer: CustomerRecord): Promise<void> {
  await this.db.upsert(
    'stripe_customers',
    customer,
    ['id'], // conflict columns
    ['email', 'name', 'metadata'] // columns to update
  );
}
```

### 3.4 Bulk Operations

**Batch inserts for performance:**

```typescript
// GOOD: Bulk upsert for large datasets
async upsertCustomers(customers: CustomerRecord[]): Promise<number> {
  if (customers.length === 0) return 0;

  // Use shared bulkUpsert helper
  return await this.db.bulkUpsert(
    'stripe_customers',
    customers,
    ['id'],
    ['email', 'name', 'metadata']
  );
}

// For very large datasets, batch in chunks
async upsertManyCustomers(customers: CustomerRecord[]): Promise<number> {
  const BATCH_SIZE = 1000;
  let total = 0;

  for (let i = 0; i < customers.length; i += BATCH_SIZE) {
    const batch = customers.slice(i, i + BATCH_SIZE);
    total += await this.upsertCustomers(batch);
    logger.debug('Batch upserted', {
      progress: `${i + batch.length}/${customers.length}`,
    });
  }

  return total;
}
```

### 3.5 Transactions

**Use transactions for related operations:**

```typescript
// GOOD: Transaction for multi-table updates
async createSubscription(subscription: SubscriptionRecord): Promise<void> {
  await this.db.transaction(async (client) => {
    // Insert subscription
    await client.query(
      `INSERT INTO stripe_subscriptions (...)
       VALUES (...) ON CONFLICT (id) DO UPDATE SET ...`,
      [/* ... */]
    );

    // Insert subscription items
    for (const item of subscription.items) {
      await client.query(
        `INSERT INTO stripe_subscription_items (...)
         VALUES (...) ON CONFLICT (id) DO UPDATE SET ...`,
        [/* ... */]
      );
    }

    // Update customer
    await client.query(
      `UPDATE stripe_customers
       SET has_active_subscription = true
       WHERE id = $1`,
      [subscription.customer_id]
    );
  });
}
```

### 3.6 Connection Pooling

**Configure pool settings appropriately:**

```typescript
// In shared/src/database.ts
this.pool = new Pool({
  host: config.host,
  port: config.port,
  database: config.database,
  user: config.user,
  password: config.password,
  ssl: config.ssl ? { rejectUnauthorized: false } : undefined,

  // Pool configuration
  max: config.maxConnections ?? 10,      // Max connections
  min: 2,                                 // Min connections
  idleTimeoutMillis: 30000,              // 30s idle timeout
  connectionTimeoutMillis: 5000,         // 5s connection timeout
});

// Handle pool errors
this.pool.on('error', (err) => {
  logger.error('Unexpected database pool error', { error: err.message });
});
```

### 3.7 Query Performance

**Optimize queries for performance:**

```typescript
// GOOD: Use specific columns
async listCustomers(limit: number, offset: number): Promise<CustomerRecord[]> {
  const result = await this.db.query<CustomerRecord>(
    `SELECT id, email, name, created_at, synced_at
     FROM stripe_customers
     WHERE deleted_at IS NULL
     ORDER BY created_at DESC
     LIMIT $1 OFFSET $2`,
    [limit, offset]
  );
  return result.rows;
}

// BAD: SELECT *
async listCustomers(limit: number, offset: number): Promise<CustomerRecord[]> {
  const result = await this.db.query(
    `SELECT * FROM stripe_customers LIMIT $1 OFFSET $2`,
    [limit, offset]
  );
  return result.rows;
}

// GOOD: Count with index
async countCustomers(): Promise<number> {
  const result = await this.db.queryOne<{ count: string }>(
    `SELECT COUNT(*) as count
     FROM stripe_customers
     WHERE deleted_at IS NULL`
  );
  return parseInt(result?.count ?? '0', 10);
}
```

---

## 4. API Integration

### 4.1 Rate Limiting

**Always implement rate limiting:**

```typescript
import { RateLimiter } from '@nself/plugin-utils';

export class StripeClient {
  private rateLimiter: RateLimiter;

  constructor(apiKey: string) {
    // Stripe allows 100 req/sec in live mode, 25 in test mode
    this.rateLimiter = new RateLimiter(100);
  }

  private async request<T>(endpoint: string): Promise<T> {
    // Wait for rate limit token
    await this.rateLimiter.acquire();

    // Make request
    return await this.http.get<T>(endpoint);
  }
}
```

### 4.2 Retry Logic

**Implement exponential backoff for retries:**

```typescript
import { withRetry } from '@nself/plugin-utils';

async fetchCustomer(id: string): Promise<Customer> {
  return await withRetry(
    async () => {
      const response = await fetch(`${this.baseUrl}/customers/${id}`, {
        headers: { Authorization: `Bearer ${this.apiKey}` },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      return await response.json();
    },
    {
      maxRetries: 3,
      baseDelay: 1000,        // Start with 1s
      maxDelay: 30000,        // Max 30s
      backoffMultiplier: 2,   // Double each time
    }
  );
}
```

### 4.3 Pagination

**Handle pagination correctly:**

```typescript
// GOOD: Standard pagination pattern
async listAllCustomers(): Promise<CustomerRecord[]> {
  const customers: CustomerRecord[] = [];
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

    logger.debug('Fetched batch', {
      count: response.data.length,
      total: customers.length,
    });
  }

  return customers;
}

// BETTER: Use async generator for memory efficiency
async *listCustomers(): AsyncGenerator<CustomerRecord[]> {
  for await (const customer of this.stripe.customers.list({ limit: 100 })) {
    yield [this.mapCustomer(customer)];
  }
}
```

### 4.4 Response Mapping

**Always map API responses to typed records:**

```typescript
// GOOD: Explicit mapping with null handling
private mapCustomer(customer: Stripe.Customer): CustomerRecord {
  return {
    id: customer.id,
    email: customer.email,
    name: customer.name ?? null,                          // Handle optional
    phone: customer.phone ?? null,
    currency: customer.currency ?? null,
    balance: customer.balance,
    delinquent: customer.delinquent ?? false,
    metadata: customer.metadata ?? {},                    // Default to empty object
    address: customer.address ?? null,
    created_at: new Date(customer.created * 1000),        // Convert Unix timestamp
    deleted_at: null,
  };
}

// BAD: Direct passthrough
private mapCustomer(customer: Stripe.Customer): CustomerRecord {
  return customer as unknown as CustomerRecord; // Type assertion without validation
}
```

### 4.5 Incremental Sync

**Support incremental syncing for efficiency:**

```typescript
async syncCustomers(options?: { since?: Date }): Promise<number> {
  const since = options?.since ?? await this.db.getLastSyncTime('stripe_customers');

  const params: Stripe.CustomerListParams = {};
  if (since) {
    // Only fetch customers created/updated since last sync
    params.created = { gte: Math.floor(since.getTime() / 1000) };
    logger.info('Incremental sync', { since });
  } else {
    logger.info('Full sync');
  }

  const customers = await this.client.listAllCustomers(params);
  await this.db.upsertCustomers(customers);

  return customers.length;
}
```

### 4.6 API Client Configuration

**Make clients configurable and testable:**

```typescript
export interface ClientConfig {
  apiKey: string;
  apiVersion?: string;
  baseUrl?: string;
  timeout?: number;
  rateLimitPerSecond?: number;
}

export class StripeClient {
  private config: ClientConfig;
  private rateLimiter: RateLimiter;

  constructor(config: ClientConfig) {
    this.config = {
      baseUrl: 'https://api.stripe.com',
      timeout: 30000,
      rateLimitPerSecond: 100,
      ...config, // Allow overrides
    };

    this.rateLimiter = new RateLimiter(this.config.rateLimitPerSecond);
  }
}
```

---

## 5. Webhook Handling

### 5.1 Signature Verification

**Always verify webhook signatures:**

```typescript
import { verifyStripeSignature } from '@nself/plugin-utils';

app.post('/webhooks/stripe', async (request, reply) => {
  const signature = request.headers['stripe-signature'] as string | undefined;
  const rawBody = (request as unknown as { rawBody: string }).rawBody;

  if (!signature) {
    logger.warn('Missing Stripe signature header');
    return reply.status(400).send({ error: 'Missing signature' });
  }

  // Verify signature
  if (!verifyStripeSignature(rawBody, signature, config.webhookSecret)) {
    logger.warn('Invalid Stripe signature');
    return reply.status(401).send({ error: 'Invalid signature' });
  }

  // Process webhook
  try {
    const event = JSON.parse(rawBody);
    await webhookHandler.handle(event);
    return { received: true };
  } catch (error) {
    logger.error('Webhook processing failed', { error });
    return reply.status(500).send({ error: 'Processing failed' });
  }
});
```

### 5.2 Raw Body Preservation

**Preserve raw body for signature verification:**

```typescript
// In server.ts
app.addContentTypeParser('application/json', { parseAs: 'string' }, (req, body, done) => {
  try {
    const json = JSON.parse(body as string);
    // Store raw body for signature verification
    (req as unknown as { rawBody: string }).rawBody = body as string;
    done(null, json);
  } catch (err) {
    done(err as Error, undefined);
  }
});
```

### 5.3 Idempotency

**Handle duplicate webhooks gracefully:**

```typescript
// Store event ID to prevent duplicate processing
async insertWebhookEvent(event: WebhookEventRecord): Promise<void> {
  await this.db.execute(
    `INSERT INTO stripe_webhook_events (
       id, type, data, created_at, received_at
     )
     VALUES ($1, $2, $3, $4, NOW())
     ON CONFLICT (id) DO NOTHING`, // Ignore duplicates
    [event.id, event.type, event.data, event.created_at]
  );
}

// Or check before processing
async handle(event: WebhookEvent): Promise<void> {
  // Check if already processed
  const existing = await this.db.queryOne(
    'SELECT id FROM stripe_webhook_events WHERE id = $1',
    [event.id]
  );

  if (existing) {
    logger.debug('Event already processed', { id: event.id });
    return;
  }

  // Process event...
}
```

### 5.4 Event Storage

**Store all webhook events for debugging:**

```sql
CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(255) NOT NULL,
    api_version VARCHAR(50),
    data JSONB NOT NULL,
    object_type VARCHAR(100),
    object_id VARCHAR(255),

    -- Processing status
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,

    -- Audit
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for finding unprocessed events
CREATE INDEX idx_webhook_events_processed
    ON stripe_webhook_events(processed, received_at)
    WHERE processed = false;
```

### 5.5 Handler Registration

**Use a registry pattern for webhook handlers:**

```typescript
export class WebhookHandler {
  private handlers: Map<string, WebhookHandlerFn>;

  constructor() {
    this.handlers = new Map();
    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    // Customer events
    this.register('customer.created', this.handleCustomerCreated.bind(this));
    this.register('customer.updated', this.handleCustomerUpdated.bind(this));
    this.register('customer.deleted', this.handleCustomerDeleted.bind(this));

    // Subscription events
    this.register('customer.subscription.created', this.handleSubscriptionCreated.bind(this));
    this.register('customer.subscription.updated', this.handleSubscriptionUpdated.bind(this));

    // ... more handlers
  }

  register(eventType: string, handler: WebhookHandlerFn): void {
    this.handlers.set(eventType, handler);
  }

  async handle(event: WebhookEvent): Promise<void> {
    const handler = this.handlers.get(event.type);

    if (handler) {
      await handler(event);
    } else {
      logger.debug('No handler for event type', { type: event.type });
    }
  }
}
```

### 5.6 Retry Strategy

**Implement retry logic for failed webhooks:**

```typescript
// Retry failed events
async retryFailedEvents(): Promise<number> {
  const failedEvents = await this.db.query<WebhookEventRecord>(
    `SELECT * FROM stripe_webhook_events
     WHERE processed = false
       AND retry_count < 5
       AND received_at > NOW() - INTERVAL '24 hours'
     ORDER BY received_at ASC
     LIMIT 100`
  );

  let retried = 0;
  for (const event of failedEvents.rows) {
    try {
      await this.handle(event);
      retried++;
    } catch (error) {
      await this.db.execute(
        `UPDATE stripe_webhook_events
         SET retry_count = retry_count + 1,
             error = $2
         WHERE id = $1`,
        [event.id, error instanceof Error ? error.message : 'Unknown error']
      );
    }
  }

  return retried;
}
```

---

## 6. Security Practices

### 6.1 API Key Authentication

**Implement API key authentication for endpoints:**

```typescript
import { createAuthHook } from '@nself/plugin-utils';

// In server.ts
if (config.security.apiKey) {
  app.addHook('preHandler', createAuthHook(config.security.apiKey) as never);
  logger.info('API key authentication enabled');
}

// Shared implementation (shared/src/security.ts)
export function createAuthHook(apiKey: string) {
  return async (request: FastifyRequest, reply: FastifyReply) => {
    // Skip health check endpoints
    if (request.url.startsWith('/health') ||
        request.url.startsWith('/ready') ||
        request.url.startsWith('/live')) {
      return;
    }

    const authHeader = request.headers.authorization;
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return reply.status(401).send({ error: 'Missing or invalid authorization' });
    }

    const token = authHeader.substring(7);
    if (token !== apiKey) {
      return reply.status(403).send({ error: 'Invalid API key' });
    }
  };
}
```

### 6.2 Rate Limiting

**Implement rate limiting on all endpoints:**

```typescript
import { ApiRateLimiter, createRateLimitHook } from '@nself/plugin-utils';

// Configure rate limiter
const rateLimiter = new ApiRateLimiter(
  config.security.rateLimitMax ?? 100,        // Max requests
  config.security.rateLimitWindowMs ?? 60000  // Per window (60s)
);

// Apply to all requests
app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

// Shared implementation
export class ApiRateLimiter {
  private requests: Map<string, number[]>;

  constructor(
    private maxRequests: number,
    private windowMs: number
  ) {
    this.requests = new Map();
  }

  check(identifier: string): boolean {
    const now = Date.now();
    const requests = this.requests.get(identifier) ?? [];

    // Remove old requests
    const validRequests = requests.filter(time => now - time < this.windowMs);

    if (validRequests.length >= this.maxRequests) {
      return false; // Rate limit exceeded
    }

    validRequests.push(now);
    this.requests.set(identifier, validRequests);
    return true;
  }
}
```

### 6.3 Input Validation

**Always validate and sanitize inputs:**

```typescript
// Validate customer ID
app.get('/api/customers/:id', async (request, reply) => {
  const { id } = request.params as { id: string };

  // Validate ID format (Stripe IDs start with cus_)
  if (!id || !id.startsWith('cus_')) {
    return reply.status(400).send({
      error: 'Invalid customer ID format',
    });
  }

  // Validate length
  if (id.length > 255) {
    return reply.status(400).send({
      error: 'Customer ID too long',
    });
  }

  const customer = await db.getCustomer(id);
  if (!customer) {
    return reply.status(404).send({ error: 'Customer not found' });
  }

  return customer;
});

// Validate query parameters
app.get('/api/customers', async (request) => {
  const { limit, offset } = request.query as {
    limit?: string;
    offset?: string;
  };

  // Parse and validate limit
  const parsedLimit = Math.min(
    Math.max(parseInt(limit ?? '100', 10), 1),
    1000 // Max 1000 per request
  );

  // Parse and validate offset
  const parsedOffset = Math.max(parseInt(offset ?? '0', 10), 0);

  const customers = await db.listCustomers(parsedLimit, parsedOffset);
  return { data: customers, limit: parsedLimit, offset: parsedOffset };
});
```

### 6.4 SQL Injection Prevention

**Always use parameterized queries:**

```typescript
// GOOD: Parameterized query
async getCustomer(id: string): Promise<CustomerRecord | null> {
  return await this.db.queryOne<CustomerRecord>(
    'SELECT * FROM stripe_customers WHERE id = $1',
    [id]
  );
}

// BAD: String concatenation (SQL injection vulnerability!)
async getCustomer(id: string): Promise<CustomerRecord | null> {
  return await this.db.queryOne<CustomerRecord>(
    `SELECT * FROM stripe_customers WHERE id = '${id}'`
  );
}

// GOOD: Dynamic WHERE clause with parameterized values
async listCustomers(filters: { email?: string; status?: string }): Promise<CustomerRecord[]> {
  const conditions: string[] = [];
  const params: unknown[] = [];
  let paramIndex = 1;

  if (filters.email) {
    conditions.push(`email = $${paramIndex++}`);
    params.push(filters.email);
  }

  if (filters.status) {
    conditions.push(`status = $${paramIndex++}`);
    params.push(filters.status);
  }

  const whereClause = conditions.length > 0
    ? `WHERE ${conditions.join(' AND ')}`
    : '';

  const result = await this.db.query<CustomerRecord>(
    `SELECT * FROM stripe_customers ${whereClause}`,
    params
  );

  return result.rows;
}
```

### 6.5 Environment Variables

**Never commit secrets, use environment variables:**

```typescript
// config.ts
import { config as dotenvConfig } from 'dotenv';

dotenvConfig();

export interface Config {
  stripeApiKey: string;
  stripeWebhookSecret: string;
  databaseUrl: string;
  apiKey?: string;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const config: Config = {
    stripeApiKey: process.env.STRIPE_API_KEY ?? '',
    stripeWebhookSecret: process.env.STRIPE_WEBHOOK_SECRET ?? '',
    databaseUrl: process.env.DATABASE_URL ?? '',
    apiKey: process.env.API_KEY,
    ...overrides,
  };

  // Validate required config
  if (!config.stripeApiKey) {
    throw new Error('STRIPE_API_KEY is required');
  }

  if (!config.databaseUrl) {
    throw new Error('DATABASE_URL is required');
  }

  return config;
}
```

```bash
# .env.example
STRIPE_API_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
DATABASE_URL=postgresql://user:pass@localhost:5432/db
API_KEY=your_secret_key_here

# Security settings
RATE_LIMIT_MAX=100
RATE_LIMIT_WINDOW_MS=60000
```

### 6.6 CORS Configuration

**Configure CORS appropriately:**

```typescript
import cors from '@fastify/cors';

// Development: Allow all origins
await app.register(cors, {
  origin: true,
  credentials: true,
});

// Production: Restrict origins
await app.register(cors, {
  origin: [
    'https://yourdomain.com',
    'https://app.yourdomain.com',
  ],
  credentials: true,
  methods: ['GET', 'POST'],
});
```

---

## 7. Performance Optimization

### 7.1 Database Connection Pooling

**Configure connection pools appropriately:**

```typescript
// shared/src/database.ts
this.pool = new Pool({
  // Connection settings
  host: config.host,
  port: config.port,
  database: config.database,
  user: config.user,
  password: config.password,

  // Pool configuration
  max: config.maxConnections ?? 10,      // Max concurrent connections
  min: 2,                                 // Keep 2 connections warm
  idleTimeoutMillis: 30000,              // Close idle after 30s
  connectionTimeoutMillis: 5000,         // 5s timeout for new connections

  // Enable keep-alive
  keepAlive: true,
  keepAliveInitialDelayMillis: 10000,
});
```

### 7.2 Caching Strategies

**Implement caching for frequently accessed data:**

```typescript
export class CachedClient {
  private cache: Map<string, { data: unknown; expiry: number }>;

  constructor(private ttlMs: number = 60000) {
    this.cache = new Map();
  }

  async get<T>(key: string, fetcher: () => Promise<T>): Promise<T> {
    const cached = this.cache.get(key);

    if (cached && cached.expiry > Date.now()) {
      return cached.data as T;
    }

    const data = await fetcher();
    this.cache.set(key, {
      data,
      expiry: Date.now() + this.ttlMs,
    });

    return data;
  }

  clear(): void {
    this.cache.clear();
  }
}

// Usage
const cache = new CachedClient(60000); // 1 minute TTL

async getProduct(id: string): Promise<Product> {
  return await cache.get(`product:${id}`, async () => {
    return await this.client.getProduct(id);
  });
}
```

### 7.3 Batch Processing

**Process records in batches:**

```typescript
async syncAllCustomers(): Promise<number> {
  const BATCH_SIZE = 100;
  let total = 0;
  let startingAfter: string | undefined;

  while (true) {
    // Fetch batch
    const response = await this.stripe.customers.list({
      limit: BATCH_SIZE,
      starting_after: startingAfter,
    });

    if (response.data.length === 0) break;

    // Map to records
    const records = response.data.map(c => this.mapCustomer(c));

    // Bulk upsert
    await this.db.bulkUpsert('stripe_customers', records, ['id']);

    total += records.length;
    startingAfter = response.data[response.data.length - 1].id;

    logger.debug('Batch synced', { count: records.length, total });

    if (!response.has_more) break;
  }

  return total;
}
```

### 7.4 Query Optimization

**Use EXPLAIN to optimize queries:**

```sql
-- Analyze query performance
EXPLAIN ANALYZE
SELECT c.*, COUNT(s.id) as subscription_count
FROM stripe_customers c
LEFT JOIN stripe_subscriptions s ON s.customer_id = c.id
WHERE c.created_at > NOW() - INTERVAL '30 days'
GROUP BY c.id
ORDER BY c.created_at DESC
LIMIT 100;

-- Add indexes based on results
CREATE INDEX idx_customers_created ON stripe_customers(created_at DESC);
CREATE INDEX idx_subscriptions_customer ON stripe_subscriptions(customer_id);
```

### 7.5 Memory Management

**Avoid loading large datasets into memory:**

```typescript
// BAD: Loads everything into memory
async exportAllCustomers(): Promise<Customer[]> {
  return await this.client.listAllCustomers(); // Could be millions of records!
}

// GOOD: Stream results
async *exportCustomers(): AsyncGenerator<Customer[]> {
  for await (const batch of this.client.listCustomers()) {
    yield batch;
  }
}

// Usage
for await (const batch of exportCustomers()) {
  await writeToFile(batch);
}
```

### 7.6 Parallel Processing

**Use Promise.all for independent operations:**

```typescript
// GOOD: Parallel execution
async syncAll(): Promise<void> {
  await Promise.all([
    this.syncCustomers(),
    this.syncProducts(),
    this.syncPrices(),
  ]);
}

// BAD: Sequential execution
async syncAll(): Promise<void> {
  await this.syncCustomers();
  await this.syncProducts();
  await this.syncPrices();
}

// GOOD: Controlled concurrency
async syncWithConcurrency(ids: string[], concurrency: number): Promise<void> {
  const chunks: string[][] = [];
  for (let i = 0; i < ids.length; i += concurrency) {
    chunks.push(ids.slice(i, i + concurrency));
  }

  for (const chunk of chunks) {
    await Promise.all(chunk.map(id => this.syncResource(id)));
  }
}
```

---

## 8. Testing Strategies

### 8.1 Unit Tests

**Test individual functions in isolation:**

```typescript
// tests/client.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { StripeClient } from '../src/client';

describe('StripeClient', () => {
  let client: StripeClient;

  beforeEach(() => {
    client = new StripeClient('sk_test_123');
  });

  it('should map customer correctly', () => {
    const apiCustomer = {
      id: 'cus_123',
      email: 'test@example.com',
      name: 'Test User',
      created: 1234567890,
    };

    const record = client['mapCustomer'](apiCustomer);

    expect(record.id).toBe('cus_123');
    expect(record.email).toBe('test@example.com');
    expect(record.name).toBe('Test User');
    expect(record.created_at).toBeInstanceOf(Date);
  });

  it('should handle pagination', async () => {
    // Mock Stripe SDK
    vi.spyOn(client['stripe'].customers, 'list')
      .mockResolvedValueOnce({
        data: [{ id: 'cus_1', email: 'user1@example.com' }],
        has_more: true,
      })
      .mockResolvedValueOnce({
        data: [{ id: 'cus_2', email: 'user2@example.com' }],
        has_more: false,
      });

    const customers = await client.listAllCustomers();

    expect(customers).toHaveLength(2);
    expect(customers[0].id).toBe('cus_1');
    expect(customers[1].id).toBe('cus_2');
  });
});
```

### 8.2 Integration Tests

**Test components working together:**

```typescript
// tests/integration/sync.test.ts
import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { StripeClient } from '../src/client';
import { StripeDatabase } from '../src/database';
import { StripeSyncService } from '../src/sync';

describe('Stripe Sync Integration', () => {
  let db: StripeDatabase;
  let client: StripeClient;
  let syncService: StripeSyncService;

  beforeAll(async () => {
    // Use test database
    db = new StripeDatabase({
      database: 'nself_test',
    });
    await db.connect();
    await db.initializeSchema();

    // Use test API key
    client = new StripeClient(process.env.STRIPE_TEST_KEY!);
    syncService = new StripeSyncService(client, db);
  });

  afterAll(async () => {
    await db.execute('TRUNCATE stripe_customers CASCADE');
    await db.disconnect();
  });

  it('should sync customers to database', async () => {
    const count = await syncService.syncCustomers();

    expect(count).toBeGreaterThan(0);

    const dbCount = await db.countCustomers();
    expect(dbCount).toBe(count);
  });
});
```

### 8.3 Webhook Testing

**Test webhook handlers with fixtures:**

```typescript
// tests/webhooks.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { StripeWebhookHandler } from '../src/webhooks';
import customerCreatedFixture from './fixtures/customer.created.json';

describe('Webhook Handlers', () => {
  let handler: StripeWebhookHandler;

  beforeEach(() => {
    handler = new StripeWebhookHandler(mockClient, mockDb, mockSync);
  });

  it('should handle customer.created event', async () => {
    await handler.handle(customerCreatedFixture);

    expect(mockDb.insertWebhookEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'customer.created',
        id: customerCreatedFixture.id,
      })
    );

    expect(mockSync.syncSingleResource).toHaveBeenCalledWith(
      'customer',
      'cus_123'
    );
  });
});
```

### 8.4 Test Fixtures

**Create reusable test data:**

```typescript
// tests/fixtures/customer.ts
export const mockCustomer = {
  id: 'cus_test123',
  email: 'test@example.com',
  name: 'Test Customer',
  created: 1609459200,
  metadata: {},
};

export const mockCustomerRecord = {
  id: 'cus_test123',
  email: 'test@example.com',
  name: 'Test Customer',
  created_at: new Date('2021-01-01T00:00:00Z'),
  synced_at: new Date(),
  metadata: {},
};
```

### 8.5 Mocking

**Mock external dependencies:**

```typescript
import { vi } from 'vitest';

// Mock database
const mockDb = {
  connect: vi.fn(),
  disconnect: vi.fn(),
  execute: vi.fn(),
  query: vi.fn(),
  upsert: vi.fn(),
};

// Mock API client
const mockClient = {
  listAllCustomers: vi.fn().mockResolvedValue([mockCustomer]),
  getCustomer: vi.fn().mockResolvedValue(mockCustomer),
};

// Mock logger
vi.mock('@nself/plugin-utils', () => ({
  createLogger: () => ({
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  }),
}));
```

### 8.6 Test Coverage

**Aim for comprehensive coverage:**

```json
{
  "scripts": {
    "test": "vitest",
    "test:coverage": "vitest --coverage",
    "test:ui": "vitest --ui"
  },
  "devDependencies": {
    "@vitest/coverage-v8": "^1.0.0",
    "@vitest/ui": "^1.0.0",
    "vitest": "^1.0.0"
  }
}
```

```typescript
// vitest.config.ts
export default {
  test: {
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: [
        'node_modules/',
        'dist/',
        '**/*.test.ts',
        '**/*.spec.ts',
      ],
      threshold: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
};
```

---

## 9. Monitoring & Observability

### 9.1 Structured Logging

**Use structured logging throughout:**

```typescript
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('stripe:sync');

// GOOD: Structured with context
logger.info('Syncing customers', {
  count: customers.length,
  incremental: false,
  startTime: Date.now(),
});

logger.error('Sync failed', {
  resource: 'customers',
  error: error.message,
  duration: Date.now() - startTime,
});

// BAD: Unstructured string
logger.info(`Syncing ${customers.length} customers`);
```

### 9.2 Log Levels

**Use appropriate log levels:**

```typescript
// DEBUG: Detailed diagnostic information
logger.debug('Fetching customer batch', {
  limit: 100,
  startingAfter,
});

// INFO: General informational messages
logger.info('Sync completed', {
  resource: 'customers',
  count: 1234,
  duration: 5000,
});

// WARN: Warning messages (potential issues)
logger.warn('Rate limit approaching', {
  remaining: 10,
  resetAt: new Date(),
});

// ERROR: Error messages (failures)
logger.error('Sync failed', {
  resource: 'customers',
  error: error.message,
  stack: error.stack,
});

// SUCCESS: Completion messages (custom level)
logger.success('All resources synced', {
  totalRecords: 5678,
  duration: 30000,
});
```

### 9.3 Performance Metrics

**Track performance metrics:**

```typescript
export class MetricsTracker {
  private metrics: Map<string, number[]>;

  constructor() {
    this.metrics = new Map();
  }

  track(name: string, value: number): void {
    const values = this.metrics.get(name) ?? [];
    values.push(value);
    this.metrics.set(name, values);
  }

  getStats(name: string): { avg: number; min: number; max: number } | null {
    const values = this.metrics.get(name);
    if (!values || values.length === 0) return null;

    return {
      avg: values.reduce((a, b) => a + b, 0) / values.length,
      min: Math.min(...values),
      max: Math.max(...values),
    };
  }
}

// Usage
const metrics = new MetricsTracker();

async syncCustomers(): Promise<void> {
  const start = Date.now();

  try {
    const customers = await this.client.listAllCustomers();
    await this.db.upsertCustomers(customers);

    const duration = Date.now() - start;
    metrics.track('sync.customers.duration', duration);
    metrics.track('sync.customers.count', customers.length);

    logger.info('Customers synced', {
      count: customers.length,
      duration,
      avgDuration: metrics.getStats('sync.customers.duration')?.avg,
    });
  } catch (error) {
    metrics.track('sync.customers.errors', 1);
    throw error;
  }
}
```

### 9.4 Health Checks

**Implement comprehensive health checks:**

```typescript
// Basic liveness (is the app running?)
app.get('/health', async () => {
  return {
    status: 'ok',
    plugin: 'stripe',
    timestamp: new Date().toISOString(),
  };
});

// Readiness (can the app serve traffic?)
app.get('/ready', async (_request, reply) => {
  try {
    // Check database connectivity
    await db.query('SELECT 1');

    // Check external dependencies if needed
    // await client.ping();

    return {
      ready: true,
      plugin: 'stripe',
      timestamp: new Date().toISOString(),
    };
  } catch (error) {
    logger.error('Readiness check failed', { error });
    return reply.status(503).send({
      ready: false,
      error: 'Service unavailable',
      timestamp: new Date().toISOString(),
    });
  }
});

// Liveness (detailed application state)
app.get('/live', async () => {
  const stats = await db.getStats();

  return {
    alive: true,
    plugin: 'stripe',
    version: '1.0.0',
    uptime: process.uptime(),
    memory: {
      used: Math.round(process.memoryUsage().heapUsed / 1024 / 1024),
      total: Math.round(process.memoryUsage().heapTotal / 1024 / 1024),
    },
    stats: {
      customers: stats.customers,
      subscriptions: stats.subscriptions,
      lastSync: stats.lastSyncedAt,
    },
    timestamp: new Date().toISOString(),
  };
});
```

### 9.5 Error Tracking

**Track and categorize errors:**

```typescript
interface ErrorEvent {
  type: 'api' | 'database' | 'webhook' | 'validation';
  message: string;
  stack?: string;
  context?: Record<string, unknown>;
  timestamp: Date;
}

export class ErrorTracker {
  private errors: ErrorEvent[] = [];

  track(event: ErrorEvent): void {
    this.errors.push(event);

    // Log error
    logger.error(`${event.type} error`, {
      message: event.message,
      context: event.context,
    });

    // Send to external monitoring service
    // this.sendToSentry(event);
  }

  getRecent(limit: number = 10): ErrorEvent[] {
    return this.errors.slice(-limit);
  }
}

// Usage
const errorTracker = new ErrorTracker();

try {
  await this.syncCustomers();
} catch (error) {
  errorTracker.track({
    type: 'api',
    message: error instanceof Error ? error.message : 'Unknown error',
    stack: error instanceof Error ? error.stack : undefined,
    context: { resource: 'customers' },
    timestamp: new Date(),
  });
  throw error;
}
```

### 9.6 Statistics Endpoint

**Expose statistics for monitoring:**

```typescript
app.get('/api/stats', async () => {
  const stats = await db.getStats();

  return {
    plugin: 'stripe',
    version: '1.0.0',

    // Resource counts
    resources: {
      customers: stats.customers,
      products: stats.products,
      subscriptions: stats.subscriptions,
      invoices: stats.invoices,
    },

    // Sync status
    sync: {
      lastSyncedAt: stats.lastSyncedAt,
      lastSyncDuration: stats.lastSyncDuration,
      totalSyncs: stats.totalSyncs,
    },

    // Webhook stats
    webhooks: {
      processed: stats.webhooksProcessed,
      failed: stats.webhooksFailed,
      pending: stats.webhooksPending,
    },

    // System stats
    system: {
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      cpu: process.cpuUsage(),
    },

    timestamp: new Date().toISOString(),
  };
});
```

---

## 10. Documentation

### 10.1 Code Comments

**Write clear, helpful comments:**

```typescript
/**
 * Syncs all customers from Stripe to the database.
 * Supports both full and incremental syncing based on the 'since' parameter.
 *
 * @param options - Sync options
 * @param options.since - Only sync customers created/updated after this date
 * @param options.limit - Maximum number of customers to sync
 * @returns Total number of customers synced
 *
 * @example
 * // Full sync
 * const count = await syncService.syncCustomers();
 *
 * @example
 * // Incremental sync
 * const lastSync = await db.getLastSyncTime('stripe_customers');
 * const count = await syncService.syncCustomers({ since: lastSync });
 */
async syncCustomers(options?: {
  since?: Date;
  limit?: number;
}): Promise<number> {
  // Implementation...
}
```

### 10.2 README Files

**Include comprehensive README for each plugin:**

```markdown
# Stripe Plugin

Complete Stripe integration for nself.

## Features

- 100% data sync for all Stripe objects
- Real-time webhook processing
- REST API for querying synced data
- CLI for management and operations

## Installation

\`\`\`bash
nself plugin install stripe
\`\`\`

## Configuration

Required environment variables:

\`\`\`bash
STRIPE_API_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
DATABASE_URL=postgresql://...
\`\`\`

## Usage

### CLI Commands

\`\`\`bash
# Initialize database schema
nself stripe init

# Run full sync
nself stripe sync

# Start webhook server
nself stripe server
\`\`\`

### API Endpoints

- `GET /api/customers` - List customers
- `GET /api/customers/:id` - Get customer by ID
- `POST /sync` - Trigger sync

## Development

\`\`\`bash
cd plugins/stripe/ts
npm install
npm run dev
\`\`\`

## License

MIT
```

### 10.3 API Documentation

**Document all REST endpoints:**

```typescript
/**
 * GET /api/customers
 *
 * Lists all customers with pagination.
 *
 * Query Parameters:
 * - limit: Number of customers to return (default: 100, max: 1000)
 * - offset: Number of customers to skip (default: 0)
 *
 * Response:
 * {
 *   "data": [
 *     {
 *       "id": "cus_...",
 *       "email": "customer@example.com",
 *       "name": "John Doe",
 *       "created_at": "2024-01-01T00:00:00Z"
 *     }
 *   ],
 *   "total": 1234,
 *   "limit": 100,
 *   "offset": 0
 * }
 *
 * Status Codes:
 * - 200: Success
 * - 400: Invalid parameters
 * - 500: Server error
 */
app.get('/api/customers', async (request) => {
  // Implementation...
});
```

### 10.4 Type Documentation

**Document complex types:**

```typescript
/**
 * Represents a Stripe customer in the database.
 *
 * @property id - Unique Stripe customer ID (starts with "cus_")
 * @property email - Customer's email address
 * @property name - Customer's full name (optional)
 * @property metadata - Custom metadata as key-value pairs
 * @property created_at - When the customer was created in Stripe
 * @property synced_at - When the record was last synced from Stripe
 * @property deleted_at - When the customer was deleted (soft delete)
 */
export interface StripeCustomerRecord {
  id: string;
  email: string | null;
  name: string | null;
  phone: string | null;
  currency: string | null;
  balance: number;
  metadata: Record<string, string>;
  created_at: Date;
  synced_at: Date;
  deleted_at: Date | null;
}
```

### 10.5 Changelog

**Maintain a CHANGELOG.md:**

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.1] - 2026-01-30

### Added
- Support for subscription schedules
- Incremental sync for better performance
- Rate limiting on API endpoints

### Fixed
- Webhook signature verification edge case
- Memory leak in pagination loop
- Race condition in bulk upsert

### Changed
- Improved error messages
- Updated Stripe SDK to v14.10.0

## [1.0.0] - 2026-01-15

### Added
- Initial release
- Support for 21 Stripe object types
- Webhook processing for 70+ events
- REST API with 50+ endpoints
```

---

## 11. Version Control

### 11.1 Commit Messages

**Write clear, descriptive commit messages:**

```bash
# GOOD: Clear, specific commit messages
git commit -m "feat: Add subscription schedule sync support"
git commit -m "fix: Handle null email in customer mapping"
git commit -m "perf: Optimize bulk upsert for large datasets"
git commit -m "docs: Update API documentation for /sync endpoint"

# Format: <type>: <description>
# Types: feat, fix, docs, style, refactor, perf, test, chore

# BAD: Vague commit messages
git commit -m "updates"
git commit -m "fix bug"
git commit -m "WIP"
```

### 11.2 Branch Strategy

**Use feature branches:**

```bash
# Create feature branch
git checkout -b feat/subscription-schedules

# Make changes and commit
git add .
git commit -m "feat: Add subscription schedule support"

# Push to remote
git push origin feat/subscription-schedules

# Create pull request on GitHub
gh pr create --title "Add subscription schedule support"
```

### 11.3 Git Ignore

**Properly configure .gitignore:**

```gitignore
# Dependencies
node_modules/
package-lock.json

# Build output
dist/
build/
*.tsbuildinfo

# Environment variables
.env
.env.local
.env.*.local

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
*.log
logs/

# Database
*.db
*.sqlite

# Test coverage
coverage/
.nyc_output/
```

### 11.4 Pre-commit Validation

**Use pre-commit hooks:**

```json
{
  "scripts": {
    "prepare": "husky install",
    "lint": "eslint . --ext .ts",
    "format": "prettier --write \"src/**/*.ts\"",
    "typecheck": "tsc --noEmit"
  },
  "devDependencies": {
    "husky": "^8.0.0",
    "lint-staged": "^15.0.0"
  },
  "lint-staged": {
    "*.ts": [
      "eslint --fix",
      "prettier --write",
      "tsc --noEmit"
    ]
  }
}
```

```bash
# .husky/pre-commit
#!/bin/sh
. "$(dirname "$0")/_/husky.sh"

npm run typecheck
npm run lint
npx lint-staged
```

### 11.5 Release Process

**Follow semantic versioning:**

```bash
# 1. Update version in package.json and plugin.json
npm version patch  # 1.0.0 -> 1.0.1
npm version minor  # 1.0.0 -> 1.1.0
npm version major  # 1.0.0 -> 2.0.0

# 2. Update CHANGELOG.md

# 3. Commit changes
git add .
git commit -m "chore: Release v1.0.1"

# 4. Create git tag
git tag -a v1.0.1 -m "Release v1.0.1

- Fixed webhook signature verification
- Added subscription schedule support"

# 5. Push to remote
git push origin main
git push origin v1.0.1

# 6. GitHub Actions will:
#    - Update registry.json
#    - Create GitHub Release
#    - Deploy Cloudflare Worker
```

---

## 12. Dependency Management

### 12.1 Package.json Configuration

**Structure package.json properly:**

```json
{
  "name": "@nself/stripe-plugin",
  "version": "1.0.0",
  "description": "Stripe integration for nself",
  "type": "module",
  "engines": {
    "node": ">=18.0.0"
  },
  "scripts": {
    "build": "tsc",
    "dev": "tsx watch src/server.ts",
    "start": "node dist/server.js",
    "typecheck": "tsc --noEmit",
    "test": "vitest",
    "test:coverage": "vitest --coverage"
  },
  "dependencies": {
    "@nself/plugin-utils": "workspace:*",
    "stripe": "^14.10.0",
    "fastify": "^4.25.0",
    "commander": "^11.1.0",
    "pg": "^8.11.0"
  },
  "devDependencies": {
    "@types/node": "^20.10.0",
    "@types/pg": "^8.10.0",
    "typescript": "^5.3.0",
    "tsx": "^4.7.0",
    "vitest": "^1.0.0"
  }
}
```

### 12.2 Version Pinning

**Pin critical dependencies:**

```json
{
  "dependencies": {
    // Pin major version for SDK
    "stripe": "^14.10.0",

    // Pin exact version for critical dependencies
    "@nself/plugin-utils": "1.0.0",

    // Allow minor/patch updates for stable packages
    "fastify": "^4.25.0",
    "pg": "^8.11.0"
  }
}
```

### 12.3 Security Audits

**Regularly audit dependencies:**

```bash
# Check for vulnerabilities
npm audit

# Fix vulnerabilities automatically
npm audit fix

# Update dependencies
npm update

# Check for outdated packages
npm outdated

# Use npm-check-updates for major updates
npx npm-check-updates
npx npm-check-updates -u
```

### 12.4 Workspace Management

**Use npm workspaces for monorepo:**

```json
{
  "name": "nself-plugins",
  "version": "1.0.0",
  "private": true,
  "workspaces": [
    "shared",
    "plugins/*/ts"
  ],
  "scripts": {
    "build": "npm run build --workspaces",
    "test": "npm run test --workspaces",
    "typecheck": "npm run typecheck --workspaces"
  }
}
```

```bash
# Build all workspaces
npm run build --workspaces

# Build specific workspace
npm run build --workspace=@nself/stripe-plugin

# Install dependency in workspace
npm install stripe --workspace=@nself/stripe-plugin
```

### 12.5 Dependency Documentation

**Document dependencies and their purpose:**

```typescript
/**
 * Dependencies:
 *
 * Production:
 * - stripe: Official Stripe SDK for API access
 * - fastify: Fast HTTP server for webhooks and REST API
 * - commander: CLI framework for command-line interface
 * - pg: PostgreSQL client for database operations
 * - @nself/plugin-utils: Shared utilities (logging, database, etc.)
 *
 * Development:
 * - typescript: TypeScript compiler
 * - tsx: TypeScript execution for development
 * - vitest: Testing framework
 * - @types/*: Type definitions
 */
```

---

## Summary

This Best Practices Guide covers the essential practices for developing nself plugins:

1. **Code Organization**: Follow standardized structure and naming conventions
2. **Error Handling**: Implement comprehensive error handling with proper logging
3. **Database Practices**: Use proper schema design, indexing, and transactions
4. **API Integration**: Implement rate limiting, retry logic, and pagination
5. **Webhook Handling**: Verify signatures, handle idempotency, and store events
6. **Security Practices**: Authenticate requests, validate inputs, and protect secrets
7. **Performance Optimization**: Use caching, batching, and connection pooling
8. **Testing Strategies**: Write unit, integration, and end-to-end tests
9. **Monitoring & Observability**: Implement structured logging and health checks
10. **Documentation**: Write clear comments, README files, and API docs
11. **Version Control**: Use semantic versioning and meaningful commit messages
12. **Dependency Management**: Pin versions, audit regularly, and use workspaces

Following these practices ensures your plugins are maintainable, reliable, performant, and secure.

---

**Questions or suggestions?** Open an issue on GitHub or join the discussion in the nself community.
