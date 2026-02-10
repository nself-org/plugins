# Debugging Guide

Comprehensive guide to debugging nself plugins, including logging, tracing, network debugging, and performance profiling.

---

## Table of Contents

1. [Enabling Debug Logging](#enabling-debug-logging)
2. [Reading Log Files](#reading-log-files)
3. [Database Query Logging](#database-query-logging)
4. [Network Debugging](#network-debugging)
5. [Webhook Debugging](#webhook-debugging)
6. [Performance Profiling](#performance-profiling)
7. [Memory Debugging](#memory-debugging)
8. [Common Debugging Patterns](#common-debugging-patterns)
9. [Debugging Tools](#debugging-tools)
10. [Production Debugging](#production-debugging)

---

## Enabling Debug Logging

### Environment Variables

```bash
# Enable debug mode
DEBUG=true npm start server

# Set log level
LOG_LEVEL=debug npm start sync

# Combine both
DEBUG=true LOG_LEVEL=debug npm start server
```

### Log Levels

| Level | When to Use | What's Logged |
|-------|-------------|---------------|
| `error` | Production | Errors only |
| `warn` | Production | Errors + warnings |
| `info` | Default | Normal operations |
| `debug` | Development | Everything including debug info |

### .env Configuration

```bash
# .env file
DEBUG=true
LOG_LEVEL=debug
LOG_FILE=/tmp/nself-stripe.log
LOG_FORMAT=json  # or 'text' (default)
```

### Programmatic Configuration

```typescript
// src/config.ts
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('stripe:sync', {
  level: process.env.LOG_LEVEL || 'info',
  file: process.env.LOG_FILE,
  format: process.env.LOG_FORMAT === 'json' ? 'json' : 'text',
});

export default logger;
```

### Per-Module Logging

```typescript
// Different log levels for different modules
const clientLogger = createLogger('stripe:client', { level: 'debug' });
const syncLogger = createLogger('stripe:sync', { level: 'info' });
const dbLogger = createLogger('stripe:database', { level: 'warn' });
```

### Temporary Debug Logging

```typescript
// Add temporary debug statements
logger.debug('Customer data:', customer);
logger.debug('API response:', { status: response.status, data: response.data });

// Use console for quick debugging (remove before commit)
console.log('DEBUG:', customer);
console.trace('Stack trace');
```

---

## Reading Log Files

### Console Output

```bash
# Standard output with timestamps and colors
npm start server

# Example output:
# 2026-01-30T10:30:45.123Z [stripe:server] INFO Server listening on port 3001
# 2026-01-30T10:30:50.234Z [stripe:webhook] DEBUG Webhook received: customer.created
# 2026-01-30T10:30:50.345Z [stripe:database] INFO Customer cus_xxxxx upserted
```

### File Logging

```bash
# Enable file logging
LOG_FILE=~/.nself/logs/stripe.log npm start server

# Tail logs in real-time
tail -f ~/.nself/logs/stripe.log

# Follow with colors (macOS)
tail -f ~/.nself/logs/stripe.log | ccze -A

# Search logs
grep ERROR ~/.nself/logs/stripe.log
grep -A 5 -B 5 "rate limit" ~/.nself/logs/stripe.log  # Context

# View last 100 lines
tail -n 100 ~/.nself/logs/stripe.log

# View logs from last hour
find ~/.nself/logs -name "*.log" -mmin -60 -exec tail {} \;
```

### JSON Format Logs

```bash
# Enable JSON logging
LOG_FORMAT=json npm start server

# Example output:
# {"timestamp":"2026-01-30T10:30:45.123Z","level":"info","module":"stripe:server","message":"Server started","port":3001}

# Parse with jq
tail -f ~/.nself/logs/stripe.log | jq .

# Filter by level
cat stripe.log | jq 'select(.level == "error")'

# Extract errors with timestamps
cat stripe.log | jq -r 'select(.level == "error") | "\(.timestamp) \(.message)"'

# Count errors by type
cat stripe.log | jq -r 'select(.level == "error") | .message' | sort | uniq -c
```

### Log Rotation

```bash
# Manual rotation
mv ~/.nself/logs/stripe.log ~/.nself/logs/stripe-$(date +%Y%m%d).log

# Automated with logrotate
cat > /etc/logrotate.d/nself <<EOF
/home/user/.nself/logs/*.log {
    daily
    rotate 14
    compress
    delaycompress
    notifempty
    create 0644 user user
    sharedscripts
    postrotate
        pkill -HUP -f "npm start server"
    endscript
}
EOF
```

---

## Database Query Logging

### PostgreSQL Query Logging

#### Enable in postgresql.conf

```bash
# Find config file
psql -c "SHOW config_file"

# Edit config
sudo nano /path/to/postgresql.conf

# Add these lines:
log_statement = 'all'           # Log all queries
log_duration = on               # Log query duration
log_min_duration_statement = 0  # Log all (or set threshold in ms)
log_line_prefix = '%t [%p]: '   # Timestamp and PID

# Reload PostgreSQL
sudo systemctl reload postgresql
# or
pg_ctl reload
```

#### View PostgreSQL Logs

```bash
# Find log location
psql -c "SHOW log_directory"
psql -c "SHOW log_filename"

# Typical locations:
# macOS: /opt/homebrew/var/postgresql@14/log/
# Linux: /var/log/postgresql/
# Docker: docker logs <container>

# Tail logs
tail -f /opt/homebrew/var/postgresql@14/log/postgresql-$(date +%Y-%m-%d).log

# Filter for slow queries
grep "duration:" /var/log/postgresql/postgresql.log | awk '$4 > 1000'  # > 1 second
```

### Application-Level Query Logging

```typescript
// In database.ts
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('stripe:database', { level: 'debug' });

class Database {
  async execute(query: string, params: any[] = []): Promise<any> {
    const startTime = Date.now();

    logger.debug('Executing query:', { query, params });

    try {
      const result = await this.pool.query(query, params);
      const duration = Date.now() - startTime;

      logger.debug('Query completed:', {
        duration: `${duration}ms`,
        rows: result.rowCount,
      });

      // Warn on slow queries
      if (duration > 1000) {
        logger.warn('Slow query detected:', {
          query: query.substring(0, 100),
          duration: `${duration}ms`,
        });
      }

      return result;
    } catch (error) {
      logger.error('Query failed:', {
        query,
        params,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
      throw error;
    }
  }
}
```

### Analyze Query Performance

```sql
-- Enable timing
\timing

-- Explain query plan
EXPLAIN ANALYZE SELECT * FROM stripe_customers WHERE email = 'user@example.com';

-- Output shows:
-- Seq Scan on stripe_customers  (cost=0.00..25.88 rows=1 width=...)
--   Filter: (email = 'user@example.com')
--   Planning Time: 0.123 ms
--   Execution Time: 1.234 ms

-- Check for missing indexes
SELECT schemaname, tablename, indexname
FROM pg_indexes
WHERE schemaname = 'public' AND tablename LIKE 'stripe_%';

-- Analyze table statistics
ANALYZE stripe_customers;

-- View table statistics
SELECT * FROM pg_stat_user_tables WHERE relname = 'stripe_customers';
```

### Query the Query Log

```sql
-- Enable pg_stat_statements extension
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- View slow queries
SELECT
  substring(query, 1, 100) AS short_query,
  round(total_exec_time::numeric, 2) AS total_time,
  calls,
  round(mean_exec_time::numeric, 2) AS mean_time,
  round((100 * total_exec_time / sum(total_exec_time) OVER ())::numeric, 2) AS percentage
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 20;

-- Reset statistics
SELECT pg_stat_statements_reset();
```

---

## Network Debugging

### HTTP Request Logging

```typescript
// In client.ts
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('stripe:client');

class StripeClient {
  async request<T>(method: string, endpoint: string, data?: any): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;

    logger.debug('HTTP request:', {
      method,
      url,
      headers: this.sanitizeHeaders(this.headers),
      body: data ? JSON.stringify(data).substring(0, 200) : undefined,
    });

    const startTime = Date.now();

    try {
      const response = await fetch(url, {
        method,
        headers: this.headers,
        body: data ? JSON.stringify(data) : undefined,
      });

      const duration = Date.now() - startTime;

      logger.debug('HTTP response:', {
        status: response.status,
        duration: `${duration}ms`,
        headers: {
          'content-type': response.headers.get('content-type'),
          'x-request-id': response.headers.get('x-request-id'),
          'x-ratelimit-remaining': response.headers.get('x-ratelimit-remaining'),
        },
      });

      const json = await response.json();

      if (!response.ok) {
        logger.error('HTTP error:', {
          status: response.status,
          body: json,
        });
        throw new Error(`HTTP ${response.status}: ${JSON.stringify(json)}`);
      }

      return json;
    } catch (error) {
      const duration = Date.now() - startTime;
      logger.error('HTTP request failed:', {
        method,
        url,
        duration: `${duration}ms`,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
      throw error;
    }
  }

  private sanitizeHeaders(headers: Record<string, string>): Record<string, string> {
    const sanitized = { ...headers };
    if (sanitized.Authorization) {
      sanitized.Authorization = 'Bearer ***';
    }
    return sanitized;
  }
}
```

### Using curl to Debug API Calls

```bash
# Test Stripe API
curl https://api.stripe.com/v1/customers \
  -u "sk_test_xxxxx:" \
  -H "Stripe-Version: 2023-10-16" \
  -v  # Verbose output

# Verbose output shows:
# > GET /v1/customers HTTP/2
# > Host: api.stripe.com
# > Authorization: Bearer sk_test_xxxxx
# < HTTP/2 200
# < content-type: application/json
# < x-request-id: req_xxxxx

# Test GitHub API
curl https://api.github.com/user/repos \
  -H "Authorization: token ghp_xxxxx" \
  -H "Accept: application/vnd.github.v3+json" \
  -v

# Test Shopify API
curl https://yourshop.myshopify.com/admin/api/2024-01/products.json \
  -H "X-Shopify-Access-Token: shpat_xxxxx" \
  -v
```

### Using httpie (Better than curl)

```bash
# Install
brew install httpie

# Test Stripe API
http GET https://api.stripe.com/v1/customers \
  Authorization:"Bearer sk_test_xxxxx" \
  Stripe-Version:2023-10-16

# Output is colorized and formatted JSON
# Shows request and response headers clearly

# POST request
http POST https://api.stripe.com/v1/customers \
  Authorization:"Bearer sk_test_xxxxx" \
  email=test@example.com \
  name="Test Customer"
```

### Network Packet Capture

```bash
# Capture HTTP traffic with tcpdump
sudo tcpdump -i any -A 'tcp port 80 or tcp port 443' -w capture.pcap

# Analyze with Wireshark
wireshark capture.pcap

# Or use tshark (command-line Wireshark)
tshark -r capture.pcap -Y http -T fields -e http.request.uri -e http.response.code
```

### Proxy Debugging

```bash
# Use mitmproxy to intercept HTTPS
brew install mitmproxy

# Start proxy
mitmproxy -p 8080

# Configure app to use proxy
export HTTP_PROXY=http://localhost:8080
export HTTPS_PROXY=http://localhost:8080

npm start server

# mitmproxy will show all HTTP traffic
# Press 'i' to intercept requests
# Press 'e' to edit requests
```

---

## Webhook Debugging

### Local Webhook Testing

#### Using Stripe CLI

```bash
# Install
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks
stripe listen --forward-to http://localhost:3001/webhook

# Output:
# Ready! Your webhook signing secret is whsec_xxxxx
# (copy this to .env as STRIPE_WEBHOOK_SECRET)

# In another terminal, trigger events
stripe trigger customer.created
stripe trigger payment_intent.succeeded
stripe trigger customer.subscription.created

# Watch logs
DEBUG=true LOG_LEVEL=debug npm start server
```

#### Using ngrok

```bash
# Install
brew install ngrok

# Start tunnel
ngrok http 3001

# Output:
# Forwarding https://abc123.ngrok.io -> http://localhost:3001

# Configure webhook in service dashboard:
# https://abc123.ngrok.io/webhook

# View requests in ngrok web UI:
# http://127.0.0.1:4040
```

#### Using localhost.run (No installation)

```bash
# Start tunnel
ssh -R 80:localhost:3001 localhost.run

# Output will show public URL
# Use in webhook configuration
```

### Webhook Request Logging

```typescript
// In webhooks.ts or server.ts
fastify.post('/webhook', async (request, reply) => {
  const startTime = Date.now();

  // Log raw request
  logger.debug('Webhook received:', {
    headers: request.headers,
    bodyLength: request.body ? Buffer.byteLength(JSON.stringify(request.body)) : 0,
  });

  // Log raw body (for signature verification debugging)
  const rawBody = request.body instanceof Buffer
    ? request.body.toString('utf8')
    : JSON.stringify(request.body);

  logger.debug('Raw body:', {
    body: rawBody.substring(0, 500),  // First 500 chars
  });

  try {
    // Verify signature
    const signature = request.headers['stripe-signature'];
    if (!signature) {
      logger.warn('Missing signature header');
      return reply.status(400).send({ error: 'Missing signature' });
    }

    logger.debug('Verifying signature:', {
      signature: signature.substring(0, 50) + '...',
      secretPrefix: webhookSecret.substring(0, 10) + '...',
    });

    const event = stripe.webhooks.constructEvent(
      rawBody,
      signature as string,
      webhookSecret
    );

    logger.info('Webhook verified:', {
      type: event.type,
      id: event.id,
    });

    // Process event
    await handleEvent(event);

    const duration = Date.now() - startTime;
    logger.info('Webhook processed:', {
      type: event.type,
      duration: `${duration}ms`,
    });

    return reply.status(200).send({ received: true });

  } catch (error) {
    const duration = Date.now() - startTime;
    logger.error('Webhook error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      duration: `${duration}ms`,
      stack: error instanceof Error ? error.stack : undefined,
    });

    // Return 400 for signature errors, 500 for processing errors
    const statusCode = error instanceof Error && error.message.includes('signature')
      ? 400
      : 500;

    return reply.status(statusCode).send({
      error: error instanceof Error ? error.message : 'Unknown error',
    });
  }
});
```

### Webhook Event Storage

```sql
-- Create webhook events table for debugging
CREATE TABLE stripe_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  type VARCHAR(100) NOT NULL,
  data JSONB NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  processed_at TIMESTAMP,
  error TEXT,
  raw_body TEXT  -- Store raw body for debugging
);

CREATE INDEX idx_webhook_events_type ON stripe_webhook_events(type);
CREATE INDEX idx_webhook_events_created ON stripe_webhook_events(created_at DESC);
CREATE INDEX idx_webhook_events_error ON stripe_webhook_events(error) WHERE error IS NOT NULL;
```

```typescript
// Store all webhook events
async storeWebhookEvent(event: any, rawBody: string): Promise<void> {
  await this.db.execute(
    `INSERT INTO stripe_webhook_events (id, type, data, raw_body)
     VALUES ($1, $2, $3, $4)
     ON CONFLICT (id) DO NOTHING`,
    [event.id, event.type, JSON.stringify(event.data), rawBody]
  );
}

// Query failed webhooks
async getFailedWebhooks(): Promise<any[]> {
  const result = await this.db.query(
    `SELECT * FROM stripe_webhook_events
     WHERE error IS NOT NULL
     ORDER BY created_at DESC
     LIMIT 100`
  );
  return result.rows;
}
```

### Replaying Webhooks

```bash
# Query stored webhook events
psql $DATABASE_URL <<EOF
SELECT id, type, created_at, error
FROM stripe_webhook_events
WHERE error IS NOT NULL
ORDER BY created_at DESC;
EOF

# Get raw body for replay
psql $DATABASE_URL -t -c "
  SELECT raw_body
  FROM stripe_webhook_events
  WHERE id = 'evt_xxxxx'
" > webhook-payload.json

# Replay webhook
curl -X POST http://localhost:3001/webhook \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: $(stripe webhook sign webhook-payload.json)" \
  -d @webhook-payload.json
```

### Webhook Signature Debugging

```typescript
// Debug signature verification
function debugSignatureVerification(
  rawBody: string,
  signature: string,
  secret: string
): void {
  logger.debug('Signature verification debug:', {
    bodyLength: Buffer.byteLength(rawBody),
    bodyPreview: rawBody.substring(0, 100),
    signature: signature.substring(0, 100),
    secretPrefix: secret.substring(0, 10),
  });

  // Parse Stripe signature header
  // Format: t=timestamp,v1=signature
  const sigParts = signature.split(',').reduce((acc, part) => {
    const [key, value] = part.split('=');
    acc[key] = value;
    return acc;
  }, {} as Record<string, string>);

  logger.debug('Parsed signature:', sigParts);

  // Manually compute expected signature
  const timestamp = sigParts.t;
  const payload = `${timestamp}.${rawBody}`;

  const crypto = require('crypto');
  const expectedSignature = crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex');

  logger.debug('Signature comparison:', {
    received: sigParts.v1,
    expected: expectedSignature,
    match: sigParts.v1 === expectedSignature,
  });

  // Check timestamp tolerance
  const currentTime = Math.floor(Date.now() / 1000);
  const timestampAge = currentTime - parseInt(timestamp);

  logger.debug('Timestamp check:', {
    webhookTime: new Date(parseInt(timestamp) * 1000).toISOString(),
    currentTime: new Date(currentTime * 1000).toISOString(),
    ageSeconds: timestampAge,
    withinTolerance: timestampAge <= 300,  // 5 minutes
  });
}
```

---

## Performance Profiling

### Node.js Built-in Profiler

```bash
# Run with profiler
node --prof dist/index.js server

# Generates isolate-*-v8.log

# Process log file
node --prof-process isolate-*-v8.log > processed.txt

# View hotspots
cat processed.txt | less

# Look for:
# - [Summary] - Overall statistics
# - [JavaScript] - Time spent in JS code
# - [C++] - Time spent in native code
# - [Bottom up (heavy) profile] - Most expensive functions
```

### Using clinic.js

```bash
# Install
npm install -g clinic

# Profile CPU
clinic doctor -- node dist/index.js server

# Profile I/O
clinic bubbleprof -- node dist/index.js sync --full

# Profile memory
clinic heapprofiler -- node dist/index.js sync --full

# Opens HTML report in browser
# Shows flame graphs, recommendations
```

### Manual Performance Measurement

```typescript
// Measure function execution time
async function measurePerformance<T>(
  name: string,
  fn: () => Promise<T>
): Promise<T> {
  const startTime = performance.now();
  const startMemory = process.memoryUsage().heapUsed;

  try {
    const result = await fn();
    const endTime = performance.now();
    const endMemory = process.memoryUsage().heapUsed;

    logger.info('Performance metric:', {
      operation: name,
      duration: `${(endTime - startTime).toFixed(2)}ms`,
      memoryDelta: `${((endMemory - startMemory) / 1024 / 1024).toFixed(2)}MB`,
    });

    return result;
  } catch (error) {
    const endTime = performance.now();
    logger.error('Performance metric (failed):', {
      operation: name,
      duration: `${(endTime - startTime).toFixed(2)}ms`,
      error: error instanceof Error ? error.message : 'Unknown error',
    });
    throw error;
  }
}

// Usage
await measurePerformance('syncCustomers', async () => {
  await this.syncCustomers();
});
```

### Database Performance

```sql
-- Enable timing
\timing on

-- Run query
SELECT * FROM stripe_customers WHERE email LIKE '%@gmail.com';

-- Output: Time: 123.456 ms

-- Analyze query plan
EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT * FROM stripe_customers WHERE email LIKE '%@gmail.com';

-- Check index usage
SELECT
  schemaname,
  tablename,
  indexname,
  idx_scan as scans,
  idx_tup_read as tuples_read,
  idx_tup_fetch as tuples_fetched
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
ORDER BY idx_scan DESC;

-- Find unused indexes
SELECT
  schemaname,
  tablename,
  indexname
FROM pg_stat_user_indexes
WHERE idx_scan = 0
AND indexname NOT LIKE 'pg_toast%';
```

---

## Memory Debugging

### Monitor Memory Usage

```bash
# Watch memory in real-time
watch -n 1 'ps aux | grep node | grep -v grep'

# Output shows RSS (resident set size) and VSZ (virtual memory size)

# More detailed with Node.js
node -e "setInterval(() => console.log(process.memoryUsage()), 1000)" &
npm start sync --full

# Output:
# {
#   rss: 123456789,        # Resident set size
#   heapTotal: 12345678,   # Total heap allocated
#   heapUsed: 1234567,     # Heap actually used
#   external: 12345,       # C++ objects
#   arrayBuffers: 1234     # ArrayBuffer and SharedArrayBuffer
# }
```

### Detect Memory Leaks

```bash
# Install heap profiler
npm install -g heapdump

# Take heap snapshots
kill -USR2 <pid>  # Generates heapdump-*.heapsnapshot

# Or programmatically
const heapdump = require('heapdump');
heapdump.writeSnapshot((err, filename) => {
  console.log('Heap snapshot written to', filename);
});

# Analyze in Chrome DevTools
# 1. Open Chrome DevTools
# 2. Memory tab
# 3. Load snapshot
# 4. Compare snapshots to find leaks
```

### Memory Profiling with clinic

```bash
# Profile memory
clinic heapprofiler -- node dist/index.js sync --full

# Opens flamegraph showing memory allocations
# Look for unexpected peaks or steady growth
```

### Common Memory Issues

```typescript
// BAD: Memory leak - global variable accumulates
const allCustomers: Customer[] = [];

async function syncCustomers() {
  const customers = await client.listCustomers();
  allCustomers.push(...customers);  // Never cleared!
}

// GOOD: Process in chunks
async function syncCustomers() {
  let cursor: string | undefined;

  while (cursor !== undefined) {
    const { data, next_cursor } = await client.listCustomers({ cursor });

    // Process chunk
    await processCustomers(data);

    // Allow GC to collect processed data
    cursor = next_cursor;
  }
}

// BAD: Event listener leak
setInterval(() => {
  const emitter = new EventEmitter();
  emitter.on('event', handler);  // Never removed!
}, 1000);

// GOOD: Remove listeners
const emitter = new EventEmitter();
emitter.on('event', handler);
// Later:
emitter.removeListener('event', handler);

// BAD: Unclosed database connections
async function query() {
  const client = await pool.connect();
  return client.query('SELECT * FROM customers');
  // client.release() never called!
}

// GOOD: Always release
async function query() {
  const client = await pool.connect();
  try {
    return await client.query('SELECT * FROM customers');
  } finally {
    client.release();
  }
}
```

---

## Common Debugging Patterns

### Debug Sync Issues

```typescript
// Add progress logging
async syncCustomers(): Promise<void> {
  logger.info('Starting customer sync');

  let total = 0;
  let cursor: string | undefined;

  while (cursor !== undefined) {
    logger.debug('Fetching page', { cursor });

    const response = await this.client.listCustomers({ cursor, limit: 100 });

    logger.debug('Received page', {
      count: response.data.length,
      hasMore: response.has_more,
    });

    for (const customer of response.data) {
      try {
        await this.database.upsertCustomer(customer);
        total++;

        if (total % 100 === 0) {
          logger.info('Sync progress', { synced: total });
        }
      } catch (error) {
        logger.error('Failed to sync customer', {
          customerId: customer.id,
          error: error instanceof Error ? error.message : 'Unknown error',
        });
        // Continue with next customer
      }
    }

    cursor = response.next_cursor;
  }

  logger.info('Customer sync complete', { total });
}
```

### Debug Rate Limiting

```typescript
class RateLimiter {
  private tokens: number;
  private lastRefill: number;

  async acquire(): Promise<void> {
    logger.debug('RateLimiter state', {
      tokens: this.tokens,
      maxTokens: this.maxTokens,
      timeSinceLastRefill: Date.now() - this.lastRefill,
    });

    if (this.tokens < 1) {
      const waitTime = this.calculateWaitTime();
      logger.warn('Rate limit reached, waiting', { waitTime: `${waitTime}ms` });
      await this.wait(waitTime);
    }

    this.tokens--;
  }
}
```

### Debug API Responses

```typescript
async request<T>(endpoint: string): Promise<T> {
  const response = await fetch(endpoint);

  // Log full response for debugging
  logger.debug('API response', {
    url: endpoint,
    status: response.status,
    headers: Object.fromEntries(response.headers.entries()),
  });

  const text = await response.text();

  logger.debug('Response body', {
    body: text.substring(0, 1000),  // First 1000 chars
    length: text.length,
  });

  try {
    return JSON.parse(text);
  } catch (error) {
    logger.error('JSON parse error', {
      body: text.substring(0, 500),
      error: error instanceof Error ? error.message : 'Unknown error',
    });
    throw error;
  }
}
```

### Debug Database Operations

```typescript
async upsertCustomer(customer: CustomerRecord): Promise<void> {
  logger.debug('Upserting customer', {
    id: customer.id,
    email: customer.email,
  });

  const query = `
    INSERT INTO stripe_customers (id, email, name, created_at, synced_at)
    VALUES ($1, $2, $3, $4, NOW())
    ON CONFLICT (id) DO UPDATE SET
      email = EXCLUDED.email,
      name = EXCLUDED.name,
      synced_at = NOW()
  `;

  const params = [customer.id, customer.email, customer.name, customer.created_at];

  logger.debug('Executing query', {
    query: query.replace(/\s+/g, ' ').trim(),
    params,
  });

  try {
    const result = await this.db.execute(query, params);

    logger.debug('Query result', {
      rowCount: result.rowCount,
      command: result.command,
    });
  } catch (error) {
    logger.error('Query failed', {
      query,
      params,
      error: error instanceof Error ? error.message : 'Unknown error',
    });
    throw error;
  }
}
```

---

## Debugging Tools

### VS Code Debugging

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "type": "node",
      "request": "launch",
      "name": "Debug Server",
      "program": "${workspaceFolder}/dist/index.js",
      "args": ["server"],
      "env": {
        "DEBUG": "true",
        "LOG_LEVEL": "debug"
      },
      "cwd": "${workspaceFolder}",
      "console": "integratedTerminal",
      "sourceMaps": true,
      "outFiles": ["${workspaceFolder}/dist/**/*.js"]
    },
    {
      "type": "node",
      "request": "launch",
      "name": "Debug Sync",
      "program": "${workspaceFolder}/dist/index.js",
      "args": ["sync", "--full"],
      "env": {
        "DEBUG": "true",
        "LOG_LEVEL": "debug"
      },
      "cwd": "${workspaceFolder}",
      "console": "integratedTerminal",
      "sourceMaps": true
    }
  ]
}
```

### Chrome DevTools

```bash
# Start with inspector
node --inspect dist/index.js server

# Output: Debugger listening on ws://127.0.0.1:9229/...

# Open Chrome
chrome://inspect

# Click "inspect" on your process
# Set breakpoints, inspect variables, profile
```

### Using llnode for Core Dumps

```bash
# Install
npm install -g llnode

# Generate core dump when process crashes
ulimit -c unlimited

# Or manually
kill -ABRT <pid>

# Analyze core dump
llnode /path/to/node /path/to/core

# In llnode:
v8 bt  # Backtrace
v8 findjsobjects  # Find JS objects in heap
v8 findjsinstances SomeClass  # Find instances of class
```

---

## Production Debugging

### Structured Logging

```typescript
// Use structured logging in production
import winston from 'winston';

const logger = winston.createLogger({
  level: 'info',
  format: winston.format.json(),
  defaultMeta: { service: 'stripe-plugin' },
  transports: [
    new winston.transports.File({ filename: 'error.log', level: 'error' }),
    new winston.transports.File({ filename: 'combined.log' }),
  ],
});

// Add request ID to all logs
logger.defaultMeta = {
  ...logger.defaultMeta,
  requestId: request.headers['x-request-id'],
};

logger.info('Processing webhook', {
  type: event.type,
  customerId: event.data.object.id,
});
```

### Error Tracking with Sentry

```bash
npm install @sentry/node
```

```typescript
import * as Sentry from '@sentry/node';

Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: process.env.NODE_ENV,
  tracesSampleRate: 1.0,
});

// Wrap errors
try {
  await syncCustomers();
} catch (error) {
  Sentry.captureException(error, {
    tags: {
      operation: 'syncCustomers',
    },
    extra: {
      customerCount: count,
    },
  });
  throw error;
}
```

### Distributed Tracing

```bash
npm install @opentelemetry/sdk-node @opentelemetry/auto-instrumentations-node
```

```typescript
import { NodeSDK } from '@opentelemetry/sdk-node';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';

const sdk = new NodeSDK({
  traceExporter: new JaegerExporter({
    endpoint: 'http://localhost:14268/api/traces',
  }),
  instrumentations: [getNodeAutoInstrumentations()],
});

sdk.start();

// Traces HTTP requests, database queries automatically
```

### Health Checks

```typescript
// Add comprehensive health endpoint
fastify.get('/health', async (request, reply) => {
  const health = {
    status: 'ok',
    timestamp: new Date().toISOString(),
    uptime: process.uptime(),
    memory: process.memoryUsage(),
    checks: {
      database: 'unknown',
      api: 'unknown',
    },
  };

  // Check database
  try {
    await db.query('SELECT 1');
    health.checks.database = 'ok';
  } catch (error) {
    health.status = 'degraded';
    health.checks.database = 'error';
  }

  // Check API
  try {
    await client.ping();
    health.checks.api = 'ok';
  } catch (error) {
    health.status = 'degraded';
    health.checks.api = 'error';
  }

  const statusCode = health.status === 'ok' ? 200 : 503;
  return reply.status(statusCode).send(health);
});
```

### Metrics Collection

```typescript
// Simple in-memory metrics
class Metrics {
  private counters = new Map<string, number>();
  private gauges = new Map<string, number>();

  increment(name: string, value = 1): void {
    this.counters.set(name, (this.counters.get(name) || 0) + value);
  }

  gauge(name: string, value: number): void {
    this.gauges.set(name, value);
  }

  getAll(): Record<string, any> {
    return {
      counters: Object.fromEntries(this.counters),
      gauges: Object.fromEntries(this.gauges),
    };
  }
}

const metrics = new Metrics();

// Track operations
async syncCustomers() {
  const startTime = Date.now();
  try {
    // ... sync logic
    metrics.increment('sync.customers.success');
  } catch (error) {
    metrics.increment('sync.customers.error');
    throw error;
  } finally {
    const duration = Date.now() - startTime;
    metrics.gauge('sync.customers.duration', duration);
  }
}

// Expose via endpoint
fastify.get('/metrics', async (request, reply) => {
  return metrics.getAll();
});
```

---

## Quick Debug Checklist

When encountering an issue:

- [ ] Enable debug logging: `DEBUG=true LOG_LEVEL=debug`
- [ ] Check recent logs: `tail -f ~/.nself/logs/*.log`
- [ ] Verify environment variables: `env | grep -E 'DATABASE|API_KEY'`
- [ ] Test database connection: `psql $DATABASE_URL -c "SELECT 1"`
- [ ] Check PostgreSQL logs: `tail -f /var/log/postgresql/postgresql.log`
- [ ] Test API connectivity: `curl -v <api-endpoint>`
- [ ] Check port availability: `lsof -i :<port>`
- [ ] Monitor memory: `watch -n 1 'ps aux | grep node'`
- [ ] Review webhook events: `SELECT * FROM *_webhook_events ORDER BY created_at DESC LIMIT 10`
- [ ] Check error rate: `SELECT COUNT(*) FROM *_webhook_events WHERE error IS NOT NULL`

---

**Last Updated:** 2026-01-30
