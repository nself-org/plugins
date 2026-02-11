# Webhooks Plugin

Outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue for nself. Reliably deliver events from your application to external endpoints with automatic retries and comprehensive delivery tracking.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Delivery System](#delivery-system)
- [Dead Letter Queue](#dead-letter-queue)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Webhooks plugin provides a robust outbound webhook delivery system for sending events from your nself application to external HTTP endpoints. It handles the complexity of webhook delivery including retries, HMAC signature verification, concurrent delivery management, and dead-letter queuing for failed deliveries.

### Key Features

- **Reliable Delivery** - Automatic retries with exponential backoff for failed deliveries
- **HMAC Signing** - Cryptographically sign webhook payloads for recipient verification
- **Concurrent Delivery** - Process multiple webhook deliveries in parallel
- **Dead Letter Queue** - Capture permanently failed deliveries for manual review
- **Event Type Registry** - Document and validate event types across your system
- **Auto-Disable** - Automatically disable endpoints after repeated failures
- **Delivery Analytics** - Track delivery success rates, response times, and failure patterns
- **Custom Headers** - Add custom HTTP headers per endpoint
- **Payload Size Limits** - Configure maximum payload sizes to prevent abuse
- **Multi-Account Support** - Isolate webhook data per account

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Webhook Endpoints | Configured webhook destinations | `webhook_endpoints` |
| Webhook Deliveries | Delivery attempts and results | `webhook_deliveries` |
| Event Types | Registered event type definitions | `webhook_event_types` |
| Dead Letters | Failed deliveries for manual review | `webhook_dead_letters` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install webhooks

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "WEBHOOKS_PLUGIN_PORT=3403" >> .env

# Initialize database schema
nself plugin webhooks init

# Start server
nself plugin webhooks server --port 3403

# Register a webhook endpoint
curl -X POST http://localhost:3403/api/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/webhooks",
    "description": "Production webhook endpoint",
    "events": ["user.created", "order.completed"]
  }'

# Dispatch an event
curl -X POST http://localhost:3403/api/dispatch \
  -H "Content-Type: application/json" \
  -d '{
    "eventType": "user.created",
    "payload": {
      "userId": "user_123",
      "email": "user@example.com",
      "createdAt": "2026-02-11T10:00:00Z"
    }
  }'

# Check delivery status
nself plugin webhooks status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `WEBHOOKS_PLUGIN_PORT` | No | `3403` | HTTP server port |
| `WEBHOOKS_MAX_ATTEMPTS` | No | `5` | Maximum delivery attempts before dead-letter |
| `WEBHOOKS_REQUEST_TIMEOUT_MS` | No | `30000` | HTTP request timeout in milliseconds (30s) |
| `WEBHOOKS_MAX_PAYLOAD_SIZE` | No | `1048576` | Maximum payload size in bytes (1MB) |
| `WEBHOOKS_CONCURRENT_DELIVERIES` | No | `10` | Number of concurrent deliveries to process |
| `WEBHOOKS_RETRY_DELAYS` | No | `10000,30000,120000,900000,3600000` | Comma-separated retry delays in ms |
| `WEBHOOKS_AUTO_DISABLE_THRESHOLD` | No | `10` | Consecutive failures before auto-disabling endpoint |
| `WEBHOOKS_API_KEY` | No | - | API key for authentication (optional) |
| `WEBHOOKS_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `WEBHOOKS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Retry Delays Explained

The default retry delays create an exponential backoff pattern:
- **Attempt 1**: Immediate delivery
- **Attempt 2**: 10 seconds after failure
- **Attempt 3**: 30 seconds after failure
- **Attempt 4**: 2 minutes after failure
- **Attempt 5**: 15 minutes after failure
- **Attempt 6**: 1 hour after failure

After max attempts, the delivery moves to the dead-letter queue.

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://localhost:5432/nself
WEBHOOKS_PLUGIN_PORT=3403
WEBHOOKS_MAX_ATTEMPTS=5
WEBHOOKS_REQUEST_TIMEOUT_MS=30000
WEBHOOKS_MAX_PAYLOAD_SIZE=2097152
WEBHOOKS_CONCURRENT_DELIVERIES=20
WEBHOOKS_AUTO_DISABLE_THRESHOLD=15
WEBHOOKS_API_KEY=your_secret_api_key_here
```

---

## CLI Commands

### init

Initialize the webhook database schema.

```bash
nself plugin webhooks init
```

Creates all required tables and indexes in PostgreSQL.

### server

Start the webhook delivery HTTP server.

```bash
# Start with default port
nself plugin webhooks server

# Start with custom port and host
nself plugin webhooks server --port 3500 --host 0.0.0.0
```

**Options:**
- `-p, --port <port>` - Server port (default: 3403)
- `-h, --host <host>` - Server host (default: 0.0.0.0)

### status

Show webhook delivery statistics and health.

```bash
nself plugin webhooks status
```

**Output:**
```
=== Webhook Statistics ===

Endpoints:
  Total: 5
  Enabled: 4
  Disabled: 1

Deliveries:
  Total: 1,247
  Pending: 12
  Delivered: 1,198
  Failed: 25
  Dead Letter: 12

Event Types:
  Registered: 15

Recent Failures:
  user.created → https://api.example.com/webhooks (503 Service Unavailable)
  order.completed → https://webhook.service.com/events (timeout)
```

### endpoints

Manage webhook endpoints.

```bash
# List all endpoints
nself plugin webhooks endpoints list

# List only enabled endpoints
nself plugin webhooks endpoints list --enabled

# Get endpoint details
nself plugin webhooks endpoints get <endpoint-id>

# Create new endpoint
nself plugin webhooks endpoints create \
  --url https://example.com/webhook \
  --events "user.created,user.updated" \
  --description "Production webhook"

# Update endpoint
nself plugin webhooks endpoints update <endpoint-id> \
  --enabled false

# Delete endpoint
nself plugin webhooks endpoints delete <endpoint-id>

# Rotate secret
nself plugin webhooks endpoints rotate-secret <endpoint-id>
```

### deliveries

View and manage webhook deliveries.

```bash
# List deliveries for an endpoint
nself plugin webhooks deliveries list --endpoint <endpoint-id>

# List failed deliveries
nself plugin webhooks deliveries list --status failed

# Retry a specific delivery
nself plugin webhooks deliveries retry <delivery-id>

# Retry all failed deliveries
nself plugin webhooks deliveries retry-failed

# Purge old deliveries
nself plugin webhooks deliveries purge --older-than 90
```

### dead-letters

Manage the dead-letter queue.

```bash
# List dead letters
nself plugin webhooks dead-letters list

# View dead letter details
nself plugin webhooks dead-letters get <id>

# Retry a dead letter
nself plugin webhooks dead-letters retry <id>

# Mark as resolved
nself plugin webhooks dead-letters resolve <id>

# Purge resolved dead letters
nself plugin webhooks dead-letters purge --resolved
```

### dispatch

Manually dispatch a webhook event.

```bash
nself plugin webhooks dispatch \
  --event-type "user.created" \
  --payload '{"userId":"123","email":"user@example.com"}'
```

### event-types

Manage registered event types.

```bash
# List event types
nself plugin webhooks event-types list

# Register new event type
nself plugin webhooks event-types register \
  --name "user.created" \
  --description "User account created" \
  --source-plugin "auth"

# Update event type
nself plugin webhooks event-types update user.created \
  --description "Updated description"

# Delete event type
nself plugin webhooks event-types delete user.created
```

---

## REST API

### Health Check Endpoints

#### GET /health

Health check endpoint (no auth required).

**Response:**
```json
{
  "status": "ok",
  "plugin": "webhooks",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready

Readiness check with database connectivity (no auth required).

**Response:**
```json
{
  "ready": true,
  "plugin": "webhooks",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live

Liveness check with delivery service status.

**Response:**
```json
{
  "live": true,
  "plugin": "webhooks",
  "sourceAccountId": "primary",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Webhook Endpoint Management

#### POST /api/endpoints

Create a new webhook endpoint.

**Request Body:**
```json
{
  "url": "https://example.com/webhooks",
  "description": "Production webhook endpoint",
  "events": ["user.created", "user.updated", "order.completed"],
  "headers": {
    "X-Custom-Header": "value"
  },
  "metadata": {
    "environment": "production",
    "team": "platform"
  }
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "url": "https://example.com/webhooks",
  "description": "Production webhook endpoint",
  "secret": "whsec_a1b2c3d4e5f6...",
  "events": ["user.created", "user.updated", "order.completed"],
  "headers": {
    "X-Custom-Header": "value"
  },
  "enabled": true,
  "failure_count": 0,
  "last_success_at": null,
  "last_failure_at": null,
  "disabled_at": null,
  "disabled_reason": null,
  "metadata": {
    "environment": "production",
    "team": "platform"
  },
  "created_at": "2026-02-11T10:00:00.000Z",
  "updated_at": "2026-02-11T10:00:00.000Z"
}
```

#### GET /api/endpoints

List all webhook endpoints.

**Query Parameters:**
- `enabled` (boolean) - Filter by enabled status

**Response:** `200 OK`
```json
{
  "endpoints": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "url": "https://example.com/webhooks",
      "description": "Production webhook endpoint",
      "events": ["user.created", "user.updated"],
      "enabled": true,
      "failure_count": 0,
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 1
}
```

#### GET /api/endpoints/:id

Get a specific webhook endpoint.

**Response:** `200 OK`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "url": "https://example.com/webhooks",
  "description": "Production webhook endpoint",
  "secret": "whsec_a1b2c3d4e5f6...",
  "events": ["user.created", "user.updated"],
  "enabled": true,
  "failure_count": 0,
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

#### PATCH /api/endpoints/:id

Update a webhook endpoint.

**Request Body:**
```json
{
  "description": "Updated description",
  "events": ["user.created", "user.updated", "user.deleted"],
  "enabled": false
}
```

**Response:** `200 OK`

#### DELETE /api/endpoints/:id

Delete a webhook endpoint.

**Response:** `204 No Content`

#### POST /api/endpoints/:id/rotate-secret

Rotate the webhook signing secret.

**Response:** `200 OK`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "secret": "whsec_new_secret_here...",
  "updated_at": "2026-02-11T10:00:00.000Z"
}
```

### Event Dispatch

#### POST /api/dispatch

Dispatch an event to all matching webhook endpoints.

**Request Body:**
```json
{
  "eventType": "user.created",
  "payload": {
    "userId": "user_123",
    "email": "user@example.com",
    "name": "John Doe",
    "createdAt": "2026-02-11T10:00:00Z"
  },
  "idempotencyKey": "optional-unique-key"
}
```

**Response:** `202 Accepted`
```json
{
  "eventType": "user.created",
  "endpointsMatched": 3,
  "deliveries": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440000",
      "endpoint_id": "550e8400-e29b-41d4-a716-446655440000",
      "status": "pending",
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

### Delivery Management

#### GET /api/deliveries

List webhook deliveries.

**Query Parameters:**
- `endpoint_id` (UUID) - Filter by endpoint
- `event_type` (string) - Filter by event type
- `status` (string) - Filter by status: `pending`, `delivered`, `failed`, `dead_letter`
- `limit` (number) - Results per page (default: 50)
- `offset` (number) - Pagination offset

**Response:** `200 OK`
```json
{
  "deliveries": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440000",
      "endpoint_id": "550e8400-e29b-41d4-a716-446655440000",
      "event_type": "user.created",
      "status": "delivered",
      "response_status": 200,
      "response_time_ms": 145,
      "attempt_count": 1,
      "delivered_at": "2026-02-11T10:00:00.000Z",
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 1247,
  "limit": 50,
  "offset": 0
}
```

#### GET /api/deliveries/:id

Get delivery details.

**Response:** `200 OK`
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440000",
  "endpoint_id": "550e8400-e29b-41d4-a716-446655440000",
  "event_type": "user.created",
  "payload": {
    "userId": "user_123",
    "email": "user@example.com"
  },
  "status": "delivered",
  "response_status": 200,
  "response_body": "{\"received\":true}",
  "response_time_ms": 145,
  "attempt_count": 1,
  "signature": "sha256=abc123...",
  "delivered_at": "2026-02-11T10:00:00.000Z",
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

#### POST /api/deliveries/:id/retry

Retry a failed delivery.

**Response:** `200 OK`
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "next_retry_at": "2026-02-11T10:00:10.000Z"
}
```

### Dead Letter Queue

#### GET /api/dead-letters

List dead letter deliveries.

**Query Parameters:**
- `resolved` (boolean) - Filter by resolved status
- `limit` (number) - Results per page
- `offset` (number) - Pagination offset

**Response:** `200 OK`
```json
{
  "deadLetters": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440000",
      "delivery_id": "660e8400-e29b-41d4-a716-446655440000",
      "endpoint_id": "550e8400-e29b-41d4-a716-446655440000",
      "event_type": "user.created",
      "last_error": "Connection timeout after 30s",
      "attempt_count": 5,
      "resolved": false,
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 12
}
```

#### POST /api/dead-letters/:id/retry

Retry a dead letter delivery.

**Response:** `200 OK`

#### POST /api/dead-letters/:id/resolve

Mark a dead letter as resolved.

**Response:** `200 OK`

### Event Types

#### GET /api/event-types

List registered event types.

**Response:** `200 OK`
```json
{
  "eventTypes": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440000",
      "name": "user.created",
      "description": "User account created",
      "source_plugin": "auth",
      "schema": {
        "type": "object",
        "properties": {
          "userId": {"type": "string"},
          "email": {"type": "string"}
        }
      },
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 15
}
```

#### POST /api/event-types

Register a new event type.

**Request Body:**
```json
{
  "name": "user.created",
  "description": "User account created",
  "sourcePlugin": "auth",
  "schema": {
    "type": "object",
    "properties": {
      "userId": {"type": "string"},
      "email": {"type": "string"}
    }
  },
  "samplePayload": {
    "userId": "user_123",
    "email": "user@example.com"
  }
}
```

**Response:** `201 Created`

### Statistics

#### GET /api/stats

Get webhook delivery statistics.

**Response:** `200 OK`
```json
{
  "endpoints": {
    "total": 5,
    "enabled": 4,
    "disabled": 1
  },
  "deliveries": {
    "total": 1247,
    "pending": 12,
    "delivered": 1198,
    "failed": 25,
    "dead_letter": 12
  },
  "eventTypes": {
    "total": 15
  },
  "byEndpoint": [
    {
      "endpoint_id": "550e8400-e29b-41d4-a716-446655440000",
      "url": "https://example.com/webhooks",
      "total": 500,
      "delivered": 485,
      "failed": 15,
      "success_rate": 97.0,
      "avg_response_time_ms": 142
    }
  ],
  "byEventType": [
    {
      "event_type": "user.created",
      "total": 350,
      "delivered": 345,
      "failed": 5,
      "success_rate": 98.6
    }
  ]
}
```

---

## Webhook Events

The Webhooks plugin is an **outbound** webhook delivery system. It does not receive webhooks itself, but rather sends webhooks to external endpoints when you dispatch events.

### Sending Webhooks

To send a webhook, dispatch an event via the API:

```bash
curl -X POST http://localhost:3403/api/dispatch \
  -H "Content-Type: application/json" \
  -d '{
    "eventType": "user.created",
    "payload": {
      "userId": "user_123",
      "email": "user@example.com"
    }
  }'
```

### Webhook Payload Format

Webhooks are delivered as HTTP POST requests with:

**Headers:**
```
Content-Type: application/json
X-Webhook-Signature: sha256=<hmac-signature>
X-Webhook-Event-Type: user.created
X-Webhook-Delivery-Id: 660e8400-e29b-41d4-a716-446655440000
X-Webhook-Timestamp: 1707645600
```

**Body:**
```json
{
  "eventType": "user.created",
  "payload": {
    "userId": "user_123",
    "email": "user@example.com",
    "name": "John Doe",
    "createdAt": "2026-02-11T10:00:00Z"
  },
  "timestamp": "2026-02-11T10:00:00.000Z",
  "deliveryId": "660e8400-e29b-41d4-a716-446655440000"
}
```

### Signature Verification

Recipients should verify the webhook signature to ensure authenticity:

```javascript
const crypto = require('crypto');

function verifyWebhookSignature(payload, signature, secret) {
  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(payload);
  const expected = 'sha256=' + hmac.digest('hex');
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expected)
  );
}

// Express.js example
app.post('/webhooks', (req, res) => {
  const signature = req.headers['x-webhook-signature'];
  const secret = 'whsec_your_secret_here';

  const rawBody = JSON.stringify(req.body);

  if (!verifyWebhookSignature(rawBody, signature, secret)) {
    return res.status(401).send('Invalid signature');
  }

  // Process webhook
  const { eventType, payload } = req.body;
  console.log(`Received ${eventType}:`, payload);

  res.status(200).send({ received: true });
});
```

---

## Database Schema

### webhook_endpoints

Stores webhook endpoint configurations.

```sql
CREATE TABLE webhook_endpoints (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  url TEXT NOT NULL,
  description TEXT,
  secret VARCHAR(255) NOT NULL,
  events TEXT[] NOT NULL,
  headers JSONB DEFAULT '{}',
  enabled BOOLEAN DEFAULT TRUE,
  failure_count INTEGER DEFAULT 0,
  last_success_at TIMESTAMP WITH TIME ZONE,
  last_failure_at TIMESTAMP WITH TIME ZONE,
  disabled_at TIMESTAMP WITH TIME ZONE,
  disabled_reason TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_webhook_endpoints_source_account ON webhook_endpoints(source_account_id);
CREATE INDEX idx_webhook_endpoints_enabled ON webhook_endpoints(enabled);
CREATE INDEX idx_webhook_endpoints_events ON webhook_endpoints USING GIN(events);
CREATE INDEX idx_webhook_endpoints_created ON webhook_endpoints(created_at DESC);
```

### webhook_deliveries

Tracks webhook delivery attempts and results.

```sql
CREATE TABLE webhook_deliveries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  status VARCHAR(32) DEFAULT 'pending',
  response_status INTEGER,
  response_body TEXT,
  response_time_ms INTEGER,
  attempt_count INTEGER DEFAULT 0,
  max_attempts INTEGER DEFAULT 5,
  next_retry_at TIMESTAMP WITH TIME ZONE,
  error_message TEXT,
  signature VARCHAR(255),
  delivered_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_webhook_deliveries_source_account ON webhook_deliveries(source_account_id);
CREATE INDEX idx_webhook_deliveries_endpoint ON webhook_deliveries(endpoint_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status);
CREATE INDEX idx_webhook_deliveries_event_type ON webhook_deliveries(event_type);
CREATE INDEX idx_webhook_deliveries_next_retry ON webhook_deliveries(next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_webhook_deliveries_created ON webhook_deliveries(created_at DESC);
```

### webhook_event_types

Registry of supported event types.

```sql
CREATE TABLE webhook_event_types (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  source_plugin VARCHAR(128),
  schema JSONB,
  sample_payload JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_webhook_event_types_source_account ON webhook_event_types(source_account_id);
CREATE INDEX idx_webhook_event_types_name ON webhook_event_types(name);
CREATE INDEX idx_webhook_event_types_source_plugin ON webhook_event_types(source_plugin);
```

### webhook_dead_letters

Failed deliveries that exceeded retry limits.

```sql
CREATE TABLE webhook_dead_letters (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  delivery_id UUID REFERENCES webhook_deliveries(id),
  endpoint_id UUID REFERENCES webhook_endpoints(id),
  event_type VARCHAR(128),
  payload JSONB,
  last_error TEXT,
  attempt_count INTEGER,
  resolved BOOLEAN DEFAULT FALSE,
  resolved_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_webhook_dead_letters_source_account ON webhook_dead_letters(source_account_id);
CREATE INDEX idx_webhook_dead_letters_delivery ON webhook_dead_letters(delivery_id);
CREATE INDEX idx_webhook_dead_letters_endpoint ON webhook_dead_letters(endpoint_id);
CREATE INDEX idx_webhook_dead_letters_resolved ON webhook_dead_letters(resolved);
CREATE INDEX idx_webhook_dead_letters_created ON webhook_dead_letters(created_at DESC);
```

---

## Delivery System

### How Delivery Works

1. **Event Dispatch** - You call `/api/dispatch` with an event type and payload
2. **Endpoint Matching** - System finds all enabled endpoints subscribed to that event type
3. **Delivery Creation** - Creates delivery records for each matching endpoint
4. **Background Processing** - Delivery worker processes pending deliveries concurrently
5. **HTTP Request** - Makes POST request to endpoint URL with signed payload
6. **Response Handling** - Records response status, body, and timing
7. **Retry Logic** - On failure, schedules retry with exponential backoff
8. **Dead Letter** - After max attempts, moves to dead-letter queue

### Delivery States

- **pending** - Waiting for delivery attempt
- **delivered** - Successfully delivered (2xx response)
- **failed** - Failed but will retry
- **dead_letter** - Exceeded max attempts, moved to DLQ

### Concurrency Control

The delivery service processes multiple webhooks concurrently based on `WEBHOOKS_CONCURRENT_DELIVERIES`. This allows high-throughput webhook delivery while respecting endpoint rate limits.

```typescript
// Default: 10 concurrent deliveries
WEBHOOKS_CONCURRENT_DELIVERIES=10

// High-volume setup: 50 concurrent deliveries
WEBHOOKS_CONCURRENT_DELIVERIES=50
```

### Auto-Disable

Endpoints are automatically disabled after consecutive failures exceed `WEBHOOKS_AUTO_DISABLE_THRESHOLD` (default: 10). This prevents wasting resources on permanently broken endpoints.

```sql
-- Check auto-disabled endpoints
SELECT id, url, failure_count, disabled_at, disabled_reason
FROM webhook_endpoints
WHERE enabled = false AND disabled_at IS NOT NULL
ORDER BY disabled_at DESC;
```

---

## Dead Letter Queue

### What is a Dead Letter?

A dead letter is a webhook delivery that permanently failed after exhausting all retry attempts. These are moved to the dead-letter queue for manual review and resolution.

### Viewing Dead Letters

```bash
# List all unresolved dead letters
nself plugin webhooks dead-letters list

# List resolved dead letters
nself plugin webhooks dead-letters list --resolved
```

### Retrying Dead Letters

```bash
# Retry a specific dead letter
nself plugin webhooks dead-letters retry <id>

# This creates a new delivery with fresh retry attempts
```

### Resolving Dead Letters

```bash
# Mark as resolved (no retry)
nself plugin webhooks dead-letters resolve <id>

# Use this for deliveries that should be ignored
```

### Dead Letter Analytics

```sql
-- Dead letters by event type
SELECT event_type, COUNT(*) as count
FROM webhook_dead_letters
WHERE resolved = false
GROUP BY event_type
ORDER BY count DESC;

-- Dead letters by endpoint
SELECT e.url, COUNT(d.id) as count
FROM webhook_dead_letters d
JOIN webhook_endpoints e ON d.endpoint_id = e.id
WHERE d.resolved = false
GROUP BY e.url
ORDER BY count DESC;
```

---

## Examples

### Example 1: User Registration Webhooks

```bash
# Register webhook for user events
curl -X POST http://localhost:3403/api/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://crm.example.com/webhooks/users",
    "description": "CRM user sync",
    "events": ["user.created", "user.updated", "user.deleted"]
  }'

# Dispatch user.created event
curl -X POST http://localhost:3403/api/dispatch \
  -H "Content-Type: application/json" \
  -d '{
    "eventType": "user.created",
    "payload": {
      "userId": "user_123",
      "email": "john@example.com",
      "name": "John Doe",
      "role": "customer",
      "createdAt": "2026-02-11T10:00:00Z"
    }
  }'
```

### Example 2: Order Processing Pipeline

```bash
# Register webhook for order events
curl -X POST http://localhost:3403/api/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://fulfillment.example.com/webhooks/orders",
    "description": "Fulfillment service",
    "events": ["order.created", "order.completed", "order.cancelled"],
    "headers": {
      "X-Fulfillment-Token": "secret-token-here"
    }
  }'

# Dispatch order.created event
curl -X POST http://localhost:3403/api/dispatch \
  -H "Content-Type: application/json" \
  -d '{
    "eventType": "order.created",
    "payload": {
      "orderId": "order_456",
      "customerId": "user_123",
      "items": [
        {"sku": "PROD-001", "quantity": 2},
        {"sku": "PROD-002", "quantity": 1}
      ],
      "total": 99.99,
      "createdAt": "2026-02-11T10:00:00Z"
    }
  }'
```

### Example 3: Monitoring with Dead Letter Alerts

```bash
# Query unresolved dead letters
curl http://localhost:3403/api/dead-letters?resolved=false

# Set up monitoring alert when dead letters exceed threshold
#!/bin/bash
DEAD_LETTER_COUNT=$(curl -s http://localhost:3403/api/dead-letters?resolved=false | jq '.total')

if [ "$DEAD_LETTER_COUNT" -gt 10 ]; then
  echo "ALERT: $DEAD_LETTER_COUNT unresolved dead letters!"
  # Send alert via Slack, email, etc.
fi
```

### Example 4: Webhook Endpoint with Custom Headers

```bash
# Create endpoint with authentication headers
curl -X POST http://localhost:3403/api/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://api.partner.com/webhooks",
    "description": "Partner API integration",
    "events": ["*"],
    "headers": {
      "Authorization": "Bearer partner-api-token",
      "X-Partner-ID": "partner-123",
      "X-Environment": "production"
    }
  }'
```

### Example 5: Bulk Event Dispatch

```typescript
// Node.js script to dispatch multiple events
import axios from 'axios';

const WEBHOOK_API = 'http://localhost:3403';

async function dispatchEvent(eventType: string, payload: object) {
  const response = await axios.post(`${WEBHOOK_API}/api/dispatch`, {
    eventType,
    payload
  });
  return response.data;
}

async function syncUsers(users: any[]) {
  for (const user of users) {
    await dispatchEvent('user.created', {
      userId: user.id,
      email: user.email,
      name: user.name,
      createdAt: user.created_at
    });
  }
}

// Run
const users = await fetchUsersFromDatabase();
await syncUsers(users);
console.log(`Dispatched ${users.length} user.created events`);
```

---

## Troubleshooting

### Deliveries Stuck in Pending

**Symptom:** Deliveries remain in `pending` status and never complete.

**Causes:**
- Delivery service not running
- Endpoint URL unreachable
- Network connectivity issues

**Solutions:**
```bash
# Check if server is running
curl http://localhost:3403/health

# Restart server
nself plugin webhooks server

# Check delivery logs
docker logs webhooks-plugin

# Retry stuck deliveries
nself plugin webhooks deliveries retry-failed
```

### High Failure Rate

**Symptom:** Most deliveries fail with 5xx errors or timeouts.

**Causes:**
- Endpoint service is down
- Endpoint is rate-limiting requests
- Payload too large
- Request timeout too short

**Solutions:**
```bash
# Check endpoint health
curl -X POST https://your-endpoint.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"test": true}'

# Increase timeout
export WEBHOOKS_REQUEST_TIMEOUT_MS=60000

# Reduce concurrent deliveries
export WEBHOOKS_CONCURRENT_DELIVERIES=5

# Check failed deliveries
curl http://localhost:3403/api/deliveries?status=failed
```

### Signature Verification Fails

**Symptom:** Recipient rejects webhooks with "Invalid signature" errors.

**Causes:**
- Wrong secret used for verification
- Body modified before verification
- Signature header not passed correctly

**Solutions:**
```javascript
// Correct verification (use raw body)
app.use(express.raw({type: 'application/json'}));

app.post('/webhook', (req, res) => {
  const signature = req.headers['x-webhook-signature'];
  const rawBody = req.body.toString('utf8');

  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(rawBody);
  const expected = 'sha256=' + hmac.digest('hex');

  if (signature !== expected) {
    return res.status(401).send('Invalid signature');
  }

  const payload = JSON.parse(rawBody);
  // Process payload
});
```

### Auto-Disabled Endpoints

**Symptom:** Endpoint automatically disabled after failures.

**Causes:**
- Consecutive failures exceeded threshold
- Endpoint service experiencing issues

**Solutions:**
```bash
# List disabled endpoints
curl http://localhost:3403/api/endpoints?enabled=false

# Check failure reason
curl http://localhost:3403/api/endpoints/<id>

# Fix endpoint issue, then re-enable
curl -X PATCH http://localhost:3403/api/endpoints/<id> \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Dead Letter Queue Growing

**Symptom:** Dead letter count increasing rapidly.

**Causes:**
- Endpoint permanently broken
- Invalid event payloads
- Configuration issues

**Solutions:**
```bash
# Analyze dead letters
curl http://localhost:3403/api/dead-letters?resolved=false

# Review common error patterns
curl http://localhost:3403/api/stats | jq '.byEndpoint'

# Fix underlying issue, then retry
nself plugin webhooks dead-letters retry <id>

# Or mark as resolved if not needed
nself plugin webhooks dead-letters resolve <id>
```

### Memory Issues with Large Payloads

**Symptom:** Server crashes or slows down with large webhooks.

**Causes:**
- Payload exceeds configured size limit
- Too many concurrent large deliveries

**Solutions:**
```bash
# Increase payload size limit
export WEBHOOKS_MAX_PAYLOAD_SIZE=5242880  # 5MB

# Reduce concurrent deliveries
export WEBHOOKS_CONCURRENT_DELIVERIES=5

# Monitor memory usage
docker stats webhooks-plugin
```

### Rate Limit Errors

**Symptom:** API returns 429 Too Many Requests.

**Causes:**
- Exceeding configured rate limit
- Burst of dispatch requests

**Solutions:**
```bash
# Increase rate limit
export WEBHOOKS_RATE_LIMIT_MAX=500
export WEBHOOKS_RATE_LIMIT_WINDOW_MS=60000

# Add delays between dispatches
sleep 0.1

# Use batch processing with controlled rate
```

---

**Need Help?**

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Source: https://github.com/acamarata/nself-plugins/tree/main/plugins/webhooks
