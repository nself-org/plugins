# TypeScript Plugin Development Guide

A comprehensive guide for building nself plugins in TypeScript with 100% data sync capabilities.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Getting Started](#getting-started)
3. [Shared Utilities Reference](#shared-utilities-reference)
4. [Plugin Structure](#plugin-structure)
5. [Building Each Component](#building-each-component)
6. [Database Schema Patterns](#database-schema-patterns)
7. [Webhook Implementation](#webhook-implementation)
8. [REST API Design](#rest-api-design)
9. [CLI Commands](#cli-commands)
10. [Testing](#testing)
11. [Deployment](#deployment)
12. [Best Practices](#best-practices)

---

## Architecture Overview

### How nself Plugins Work

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  External API   │────▶│  Plugin Server  │────▶│   PostgreSQL    │
│  (Stripe, etc.) │     │  (TypeScript)   │     │   Database      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │
        │                       │
        ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│    Webhooks     │     │    REST API     │
│  (Real-time)    │     │  (Query Data)   │
└─────────────────┘     └─────────────────┘
```

### Data Flow

1. **Historical Sync**: Plugin fetches all data via service API → stores in PostgreSQL
2. **Real-time Updates**: Service sends webhooks → Plugin processes and updates PostgreSQL
3. **Query Access**: Applications query synced data via Plugin's REST API

### Plugin Components

| Component | File | Purpose |
|-----------|------|---------|
| Types | `types.ts` | TypeScript interfaces for all resources |
| Client | `client.ts` | API wrapper with rate limiting |
| Database | `database.ts` | Schema and CRUD operations |
| Sync | `sync.ts` | Orchestrates full data sync |
| Webhooks | `webhooks.ts` | Event handlers |
| Server | `server.ts` | HTTP server (Fastify) |
| CLI | `cli.ts` | Command-line interface |
| Config | `config.ts` | Environment configuration |

---

## Getting Started

### Prerequisites

- Node.js 20+
- npm or pnpm
- PostgreSQL 14+
- Service API credentials

### Setup Development Environment

```bash
# Clone repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins

# Install and build shared utilities
cd shared
npm install
npm run build

# Install a plugin
cd ../plugins/stripe/ts
npm install
cp .env.example .env
# Edit .env with your credentials

# Run in development
npm run dev
```

### Project Structure

```
nself-plugins/
├── shared/
│   ├── src/
│   │   ├── types.ts      # Core type definitions
│   │   ├── logger.ts     # Logging utilities
│   │   ├── database.ts   # Database connection
│   │   ├── http.ts       # HTTP client
│   │   └── webhook.ts    # Webhook helpers
│   └── package.json
│
├── plugins/
│   ├── stripe/ts/
│   ├── github/ts/
│   └── shopify/ts/
│
├── .wiki/
│   ├── DEVELOPMENT.md
│   ├── TYPESCRIPT_PLUGIN_GUIDE.md  # This file
│   └── plugins/
│
└── registry.json         # Plugin metadata
```

---

## Shared Utilities Reference

### Logger

```typescript
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('plugin:module');

logger.debug('Debug message', { context: 'data' });
logger.info('Info message');
logger.warn('Warning message');
logger.error('Error message', error);
logger.success('Success message');
```

### Database

```typescript
import { createDatabase, type Database } from '@nself/plugin-utils';

const db = createDatabase();
await db.connect();

// Query with parameters
const result = await db.query<User>('SELECT * FROM users WHERE id = $1', [id]);

// Single row
const user = await db.queryOne<User>('SELECT * FROM users WHERE id = $1', [id]);

// Execute (INSERT/UPDATE/DELETE)
await db.execute('INSERT INTO users (name) VALUES ($1)', [name]);

// Count
const count = await db.count('users', 'active = $1', [true]);

// Execute SQL file
await db.executeSqlFile('/path/to/schema.sql');
```

### HTTP Client

```typescript
import { HttpClient, RateLimiter } from '@nself/plugin-utils';

const rateLimiter = new RateLimiter(10); // 10 requests/second
const http = new HttpClient({
  baseUrl: 'https://api.service.com',
  headers: { 'Authorization': `Bearer ${token}` },
  timeout: 30000,
});

// Make requests
const data = await http.get<Response>('/endpoint');
const created = await http.post<Response>('/endpoint', body);
const updated = await http.put<Response>('/endpoint', body);
await http.delete('/endpoint');
```

### Webhook Verification

```typescript
import { verifyWebhookSignature } from '@nself/plugin-utils';

// HMAC SHA-256 verification
const isValid = verifyWebhookSignature(
  payload,
  signature,
  secret,
  'sha256'
);
```

---

## Plugin Structure

### Required Files

```
plugins/<name>/ts/
├── src/
│   ├── types.ts        # Type definitions (REQUIRED)
│   ├── client.ts       # API client (REQUIRED)
│   ├── database.ts     # Database operations (REQUIRED)
│   ├── sync.ts         # Sync service (REQUIRED)
│   ├── webhooks.ts     # Webhook handlers (REQUIRED)
│   ├── config.ts       # Configuration (REQUIRED)
│   ├── server.ts       # HTTP server (REQUIRED)
│   ├── cli.ts          # CLI interface (REQUIRED)
│   └── index.ts        # Exports (REQUIRED)
├── package.json
├── tsconfig.json
├── .env.example
└── README.md
```

### package.json Template

```json
{
  "name": "@nself/plugin-<name>",
  "version": "1.0.0",
  "type": "module",
  "main": "dist/index.js",
  "types": "dist/index.d.ts",
  "bin": {
    "nself-<name>": "dist/cli.js"
  },
  "scripts": {
    "build": "tsc",
    "watch": "tsc --watch",
    "dev": "tsx watch src/server.ts",
    "start": "node dist/server.js",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "@nself/plugin-utils": "file:../../../shared",
    "commander": "^12.0.0",
    "dotenv": "^16.0.0",
    "fastify": "^4.0.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "tsx": "^4.0.0",
    "typescript": "^5.0.0"
  }
}
```

### tsconfig.json Template

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "outDir": "dist",
    "rootDir": "src",
    "declaration": true
  },
  "include": ["src/**/*"]
}
```

---

## Building Each Component

### 1. types.ts - Type Definitions

Define interfaces for every syncable resource:

```typescript
/**
 * Service-specific type definitions
 */

// Base record interface
export interface BaseRecord {
  synced_at?: Date;
}

// Resource record
export interface CustomerRecord extends BaseRecord {
  id: string;
  email: string | null;
  name: string | null;
  created_at: Date;
  updated_at: Date;
  // ... all fields from API
}

// Webhook event record
export interface WebhookEventRecord {
  id: string;
  type: string;
  data: unknown;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  received_at: Date;
}

// Sync stats
export interface SyncStats {
  customers: number;
  orders: number;
  // ... counts for all resources
}

// All syncable resources
export const ALL_RESOURCES = [
  'customers',
  'orders',
  // ...
] as const;

export type Resource = (typeof ALL_RESOURCES)[number];
```

### 2. client.ts - API Client

Wrap the service API with rate limiting:

```typescript
import { createLogger, HttpClient, RateLimiter } from '@nself/plugin-utils';
import type { CustomerRecord } from './types.js';

const logger = createLogger('service:client');

export class ServiceClient {
  private http: HttpClient;
  private rateLimiter: RateLimiter;

  constructor(apiKey: string) {
    this.http = new HttpClient({
      baseUrl: 'https://api.service.com',
      headers: { 'Authorization': `Bearer ${apiKey}` },
    });
    this.rateLimiter = new RateLimiter(10); // 10/sec
  }

  private async request<T>(method: string, endpoint: string, data?: unknown): Promise<T> {
    await this.rateLimiter.acquire();
    // ... make request
  }

  // List all with pagination
  async listAllCustomers(): Promise<CustomerRecord[]> {
    logger.info('Listing all customers');
    const customers: CustomerRecord[] = [];
    let cursor: string | undefined;

    do {
      const response = await this.request<{ data: any[]; has_more: boolean; next_cursor?: string }>(
        'GET',
        `/customers${cursor ? `?starting_after=${cursor}` : ''}`
      );

      customers.push(...response.data.map(c => this.mapCustomer(c)));
      cursor = response.has_more ? response.next_cursor : undefined;
    } while (cursor);

    return customers;
  }

  // Map API response to typed record
  private mapCustomer(c: any): CustomerRecord {
    return {
      id: c.id,
      email: c.email,
      name: c.name,
      created_at: new Date(c.created * 1000),
      updated_at: new Date(c.updated * 1000),
    };
  }
}
```

### 3. database.ts - Database Operations

Handle schema and CRUD:

```typescript
import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type { CustomerRecord, SyncStats } from './types.js';

const logger = createLogger('service:db');

export class ServiceDatabase {
  private db: Database;

  constructor(db?: Database) {
    this.db = db ?? createDatabase();
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  // Initialize schema
  async initializeSchema(): Promise<void> {
    logger.info('Initializing schema...');

    await this.db.executeSqlFile(`
      CREATE TABLE IF NOT EXISTS service_customers (
        id VARCHAR(255) PRIMARY KEY,
        email VARCHAR(255),
        name VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_service_customers_email
        ON service_customers(email);
    `);

    logger.success('Schema initialized');
  }

  // Upsert (insert or update)
  async upsertCustomer(customer: CustomerRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO service_customers (id, email, name, created_at, updated_at, synced_at)
       VALUES ($1, $2, $3, $4, $5, NOW())
       ON CONFLICT (id) DO UPDATE SET
         email = EXCLUDED.email,
         name = EXCLUDED.name,
         updated_at = EXCLUDED.updated_at,
         synced_at = NOW()`,
      [customer.id, customer.email, customer.name, customer.created_at, customer.updated_at]
    );
  }

  // Bulk upsert
  async upsertCustomers(customers: CustomerRecord[]): Promise<number> {
    for (const customer of customers) {
      await this.upsertCustomer(customer);
    }
    return customers.length;
  }

  // Read operations
  async listCustomers(limit = 100, offset = 0): Promise<CustomerRecord[]> {
    const result = await this.db.query<CustomerRecord>(
      'SELECT * FROM service_customers ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset]
    );
    return result.rows;
  }

  // Count
  async countCustomers(): Promise<number> {
    return this.db.count('service_customers');
  }

  // Delete
  async deleteCustomer(id: string): Promise<void> {
    await this.db.execute('DELETE FROM service_customers WHERE id = $1', [id]);
  }

  // Get stats
  async getStats(): Promise<SyncStats> {
    const [customers, orders] = await Promise.all([
      this.countCustomers(),
      this.countOrders(),
    ]);
    return { customers, orders };
  }
}
```

### 4. sync.ts - Sync Service

Orchestrate full data synchronization:

```typescript
import { createLogger } from '@nself/plugin-utils';
import { ServiceClient } from './client.js';
import { ServiceDatabase } from './database.js';
import type { Resource, ALL_RESOURCES } from './types.js';

const logger = createLogger('service:sync');

export interface SyncOptions {
  resources?: Resource[];
  incremental?: boolean;
  since?: Date;
}

export interface SyncResult {
  resource: string;
  synced: number;
  duration: number;
}

export class SyncService {
  constructor(
    private client: ServiceClient,
    private database: ServiceDatabase
  ) {}

  async syncAll(options: SyncOptions = {}): Promise<SyncResult[]> {
    const resources = options.resources ?? ALL_RESOURCES;
    const results: SyncResult[] = [];

    logger.info('Starting full sync', { resources });

    for (const resource of resources) {
      const start = Date.now();
      let synced = 0;

      try {
        switch (resource) {
          case 'customers':
            synced = await this.syncCustomers();
            break;
          case 'orders':
            synced = await this.syncOrders();
            break;
          // ... more resources
        }

        results.push({
          resource,
          synced,
          duration: Date.now() - start,
        });

        logger.success(`Synced ${resource}`, { count: synced });
      } catch (error) {
        logger.error(`Failed to sync ${resource}`, error);
        results.push({ resource, synced: 0, duration: Date.now() - start });
      }
    }

    return results;
  }

  private async syncCustomers(): Promise<number> {
    const customers = await this.client.listAllCustomers();
    return this.database.upsertCustomers(customers);
  }

  private async syncOrders(): Promise<number> {
    const orders = await this.client.listAllOrders();
    return this.database.upsertOrders(orders);
  }
}
```

### 5. webhooks.ts - Webhook Handlers

Process incoming webhook events:

```typescript
import { createLogger, verifyWebhookSignature } from '@nself/plugin-utils';
import { ServiceDatabase } from './database.js';
import { ServiceClient } from './client.js';
import type { WebhookEventRecord } from './types.js';

const logger = createLogger('service:webhooks');

export class WebhookHandler {
  constructor(
    private database: ServiceDatabase,
    private client: ServiceClient,
    private secret?: string
  ) {}

  // Verify signature
  verifySignature(payload: string, signature: string): boolean {
    if (!this.secret) return true;
    return verifyWebhookSignature(payload, signature, this.secret, 'sha256');
  }

  // Process event
  async processEvent(event: WebhookEventRecord): Promise<void> {
    logger.info('Processing webhook', { type: event.type, id: event.id });

    try {
      // Store event
      await this.database.insertWebhookEvent(event);

      // Route to handler
      await this.routeEvent(event);

      // Mark processed
      await this.database.markEventProcessed(event.id);
    } catch (error) {
      logger.error('Webhook processing failed', error);
      await this.database.markEventProcessed(event.id, String(error));
      throw error;
    }
  }

  private async routeEvent(event: WebhookEventRecord): Promise<void> {
    const data = event.data as any;

    switch (event.type) {
      case 'customer.created':
      case 'customer.updated':
        await this.handleCustomerEvent(data);
        break;

      case 'customer.deleted':
        await this.database.deleteCustomer(data.id);
        break;

      case 'order.created':
        await this.handleOrderEvent(data);
        break;

      // ... more event types

      default:
        logger.warn('Unhandled webhook type', { type: event.type });
    }
  }

  private async handleCustomerEvent(data: any): Promise<void> {
    // Re-fetch to get complete data
    const customer = await this.client.getCustomer(data.id);
    if (customer) {
      await this.database.upsertCustomer(customer);
    }
  }

  private async handleOrderEvent(data: any): Promise<void> {
    const order = await this.client.getOrder(data.id);
    if (order) {
      await this.database.upsertOrder(order.order);
      await this.database.upsertOrderItems(order.items);
    }
  }
}
```

### 6. server.ts - HTTP Server

Expose REST API and webhook endpoint:

```typescript
import Fastify from 'fastify';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ServiceDatabase } from './database.js';
import { ServiceClient } from './client.js';
import { SyncService } from './sync.js';
import { WebhookHandler } from './webhooks.js';

const logger = createLogger('service:server');

export async function createServer() {
  const config = loadConfig();
  const fastify = Fastify({ logger: false });

  // Initialize components
  const database = new ServiceDatabase();
  await database.connect();
  await database.initializeSchema();

  const client = new ServiceClient(config.apiKey);
  const sync = new SyncService(client, database);
  const webhooks = new WebhookHandler(database, client, config.webhookSecret);

  // Health check
  fastify.get('/health', async () => ({ status: 'ok' }));

  // Webhook endpoint
  fastify.post('/webhook', async (request, reply) => {
    const signature = request.headers['x-signature'] as string;
    const payload = JSON.stringify(request.body);

    if (!webhooks.verifySignature(payload, signature)) {
      return reply.status(401).send({ error: 'Invalid signature' });
    }

    const body = request.body as any;
    await webhooks.processEvent({
      id: body.id,
      type: body.type,
      data: body.data,
      processed: false,
      processed_at: null,
      error: null,
      received_at: new Date(),
    });

    return { received: true };
  });

  // Sync endpoint
  fastify.post('/api/sync', async (request) => {
    const { resources } = request.body as { resources?: string[] };
    const results = await sync.syncAll({ resources: resources as any });
    return { results };
  });

  // Status endpoint
  fastify.get('/api/status', async () => {
    const stats = await database.getStats();
    return { stats };
  });

  // Resource endpoints
  fastify.get('/api/customers', async (request) => {
    const { limit = 100, offset = 0 } = request.query as any;
    const customers = await database.listCustomers(limit, offset);
    return { customers };
  });

  fastify.get('/api/customers/:id', async (request) => {
    const { id } = request.params as { id: string };
    const customer = await database.getCustomer(id);
    if (!customer) {
      return { error: 'Not found' };
    }
    return { customer };
  });

  // ... more endpoints

  // Start server
  await fastify.listen({ port: config.port, host: config.host });
  logger.success(`Server running on ${config.host}:${config.port}`);

  return fastify;
}

// Run if executed directly
createServer().catch(console.error);
```

### 7. cli.ts - Command Line Interface

```typescript
#!/usr/bin/env node

import { Command } from 'commander';
import { loadConfig } from './config.js';
import { ServiceDatabase } from './database.js';
import { ServiceClient } from './client.js';
import { SyncService } from './sync.js';

const program = new Command();

program
  .name('nself-service')
  .description('Service plugin for nself')
  .version('1.0.0');

program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    const db = new ServiceDatabase();
    await db.connect();
    await db.initializeSchema();
    await db.disconnect();
    console.log('Schema initialized');
  });

program
  .command('sync')
  .description('Sync all data')
  .option('-r, --resources <resources>', 'Comma-separated resources to sync')
  .action(async (options) => {
    const config = loadConfig();
    const db = new ServiceDatabase();
    const client = new ServiceClient(config.apiKey);
    const sync = new SyncService(client, db);

    await db.connect();
    const results = await sync.syncAll({
      resources: options.resources?.split(','),
    });

    console.table(results);
    await db.disconnect();
  });

program
  .command('status')
  .description('Show sync status')
  .action(async () => {
    const db = new ServiceDatabase();
    await db.connect();
    const stats = await db.getStats();
    console.table(stats);
    await db.disconnect();
  });

program
  .command('server')
  .description('Start HTTP server')
  .option('-p, --port <port>', 'Server port', '3000')
  .action(async (options) => {
    process.env.PORT = options.port;
    await import('./server.js');
  });

program.parse();
```

---

## Database Schema Patterns

### Naming Conventions

- Tables: `<service>_<resource>` (e.g., `stripe_customers`)
- Indexes: `idx_<table>_<column>` (e.g., `idx_stripe_customers_email`)
- Views: `<service>_<description>` (e.g., `stripe_revenue_summary`)

### Standard Columns

```sql
-- Every table should have:
id VARCHAR(255) PRIMARY KEY,        -- Service's ID
created_at TIMESTAMP WITH TIME ZONE, -- When created in service
updated_at TIMESTAMP WITH TIME ZONE, -- When last updated in service
synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() -- When last synced
```

### Foreign Keys

```sql
-- Use ON DELETE CASCADE for child records
CREATE TABLE service_order_items (
  id VARCHAR(255) PRIMARY KEY,
  order_id VARCHAR(255) REFERENCES service_orders(id) ON DELETE CASCADE,
  ...
);

-- Use ON DELETE SET NULL for optional references
CREATE TABLE service_customers (
  id VARCHAR(255) PRIMARY KEY,
  default_address_id VARCHAR(255) REFERENCES service_addresses(id) ON DELETE SET NULL,
  ...
);
```

### JSONB for Flexible Data

```sql
-- Store nested/variable data as JSONB
metadata JSONB DEFAULT '{}',
custom_fields JSONB DEFAULT '[]',
addresses JSONB DEFAULT '[]',

-- Query JSONB
SELECT * FROM table WHERE metadata->>'key' = 'value';
SELECT * FROM table WHERE custom_fields @> '[{"name": "foo"}]';
```

### Analytics Views

```sql
-- Create views for common queries
CREATE OR REPLACE VIEW service_daily_revenue AS
SELECT
  DATE(created_at) AS date,
  COUNT(*) AS order_count,
  SUM(total) AS revenue
FROM service_orders
WHERE status = 'completed'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

---

## Webhook Implementation

### Signature Verification

Each service uses different signature schemes:

```typescript
// Stripe: HMAC SHA-256 with timestamp
const signature = request.headers['stripe-signature'];
const timestamp = extractTimestamp(signature);
const payload = `${timestamp}.${rawBody}`;
const expected = hmac('sha256', secret, payload);

// GitHub: HMAC SHA-256
const signature = request.headers['x-hub-signature-256'];
const expected = 'sha256=' + hmac('sha256', secret, rawBody);

// Shopify: HMAC SHA-256 base64
const signature = request.headers['x-shopify-hmac-sha256'];
const expected = base64(hmac('sha256', secret, rawBody));
```

### Event Processing Pattern

```typescript
async processEvent(event: WebhookEvent): Promise<void> {
  // 1. Store raw event (for debugging/replay)
  await this.database.insertWebhookEvent(event);

  try {
    // 2. Route to appropriate handler
    await this.routeEvent(event);

    // 3. Mark as processed
    await this.database.markEventProcessed(event.id);
  } catch (error) {
    // 4. Store error for retry
    await this.database.markEventProcessed(event.id, error.message);
    throw error;
  }
}
```

### Idempotency

Always use upsert to handle duplicate events:

```typescript
// Event may be delivered multiple times
async handleCustomerCreated(data: any): Promise<void> {
  // Fetch fresh data
  const customer = await this.client.getCustomer(data.id);

  // Upsert handles duplicates
  await this.database.upsertCustomer(customer);
}
```

---

## REST API Design

### Standard Endpoints

```
GET    /health              - Health check
POST   /webhook             - Webhook receiver
POST   /api/sync            - Trigger sync
GET    /api/status          - Sync status

GET    /api/<resources>     - List resources
GET    /api/<resources>/:id - Get single resource
GET    /api/<resources>/:id/<subresource> - Get related data
```

### Query Parameters

```
?limit=100       - Number of results (default: 100)
?offset=0        - Pagination offset
?since=<date>    - Filter by date
?status=<status> - Filter by status
```

### Response Format

```json
{
  "data": [...],
  "meta": {
    "total": 1000,
    "limit": 100,
    "offset": 0,
    "has_more": true
  }
}
```

---

## Best Practices

### 1. Error Handling

```typescript
try {
  await operation();
} catch (error) {
  logger.error('Operation failed', { error, context });
  // Don't throw in webhooks - mark as failed for retry
}
```

### 2. Rate Limiting

```typescript
// Always use rate limiter for API calls
const rateLimiter = new RateLimiter(requestsPerSecond);
await rateLimiter.acquire();
await apiCall();
```

### 3. Pagination

```typescript
// Always handle pagination
async function* listAll<T>(endpoint: string): AsyncGenerator<T[]> {
  let cursor: string | undefined;
  do {
    const response = await fetch(endpoint + (cursor ? `?after=${cursor}` : ''));
    yield response.data;
    cursor = response.has_more ? response.next_cursor : undefined;
  } while (cursor);
}
```

### 4. Incremental Sync

```typescript
// Support syncing only changes
async syncIncremental(since: Date): Promise<number> {
  const changes = await this.client.listChanges(since);
  return this.database.upsertMany(changes);
}
```

### 5. Transaction Safety

```typescript
// Use transactions for multi-table operations
await this.database.transaction(async (tx) => {
  await tx.execute('INSERT INTO orders ...');
  await tx.execute('INSERT INTO order_items ...');
});
```

---

## Checklist for New Plugins

- [ ] All API resources have type definitions
- [ ] All API resources have database tables
- [ ] All API resources have sync methods
- [ ] All webhook events have handlers
- [ ] REST API exposes all synced data
- [ ] CLI has init, sync, status, server commands
- [ ] .env.example documents all variables
- [ ] README.md has complete documentation
- [ ] Rate limiting implemented
- [ ] Error handling implemented
- [ ] Logging implemented
- [ ] registry.json updated
