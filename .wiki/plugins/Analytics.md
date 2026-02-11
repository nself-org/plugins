# Analytics Plugin

Event tracking, counters, funnels, and quota management analytics engine for nself. Track user behavior, measure conversion, enforce rate limits, and gain insights into your application usage patterns.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Event Tracking](#event-tracking)
- [Counters](#counters)
- [Funnels](#funnels)
- [Quotas](#quotas)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Analytics plugin provides a comprehensive analytics engine for tracking events, managing counters, analyzing conversion funnels, and enforcing quotas. It's designed for high-volume event ingestion with efficient rollup and aggregation capabilities.

### Key Features

- **Event Tracking** - Capture custom events with rich metadata and user context
- **Counter Management** - Increment/decrement counters with automatic time-based rollup
- **Funnel Analysis** - Track multi-step user flows and measure conversion rates
- **Quota Enforcement** - Define and enforce rate limits and usage quotas
- **Time-Series Data** - Automatic rollup of counters by hour, day, week, month
- **Real-time Analytics** - Query current metrics and trends in real-time
- **Batch Processing** - Efficient batch event ingestion for high throughput
- **Custom Dimensions** - Attach arbitrary metadata to events and counters
- **Webhook Integration** - Trigger webhooks on quota violations and funnel completions
- **Multi-Account Support** - Isolate analytics data per account

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Events | Tracked user and system events | `analytics_events` |
| Counters | Aggregated counters with rollup | `analytics_counters` |
| Funnels | Multi-step conversion funnels | `analytics_funnels` |
| Quotas | Usage quotas and limits | `analytics_quotas` |
| Quota Violations | Logged quota violations | `analytics_quota_violations` |
| Webhook Events | Outbound webhook event log | `analytics_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install analytics

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "ANALYTICS_PLUGIN_PORT=3304" >> .env

# Initialize database schema
nself plugin analytics init

# Start server
nself plugin analytics server --port 3304

# Track an event
curl -X POST http://localhost:3304/api/track \
  -H "Content-Type: application/json" \
  -d '{
    "eventName": "page_view",
    "userId": "user_123",
    "properties": {
      "page": "/products",
      "referrer": "https://google.com"
    }
  }'

# Increment a counter
curl -X POST http://localhost:3304/api/counters/increment \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api_requests",
    "value": 1,
    "dimensions": {
      "endpoint": "/api/users",
      "method": "GET"
    }
  }'

# Check analytics dashboard
nself plugin analytics dashboard
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `ANALYTICS_PLUGIN_PORT` | No | `3304` | HTTP server port |
| `ANALYTICS_BATCH_SIZE` | No | `100` | Batch size for event ingestion |
| `ANALYTICS_ROLLUP_INTERVAL_MS` | No | `3600000` | Counter rollup interval in ms (1 hour) |
| `ANALYTICS_EVENT_RETENTION_DAYS` | No | `90` | Days to retain raw events |
| `ANALYTICS_COUNTER_RETENTION_DAYS` | No | `365` | Days to retain counter data |
| `ANALYTICS_API_KEY` | No | - | API key for authentication (optional) |
| `ANALYTICS_RATE_LIMIT_MAX` | No | `1000` | Max requests per window |
| `ANALYTICS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://localhost:5432/nself
ANALYTICS_PLUGIN_PORT=3304
ANALYTICS_BATCH_SIZE=500
ANALYTICS_ROLLUP_INTERVAL_MS=1800000  # 30 minutes
ANALYTICS_EVENT_RETENTION_DAYS=30
ANALYTICS_COUNTER_RETENTION_DAYS=730   # 2 years
ANALYTICS_API_KEY=your_analytics_api_key_here
```

---

## CLI Commands

### init

Initialize the analytics database schema.

```bash
nself plugin analytics init
```

Creates all required tables, indexes, and time-series partitions.

### server

Start the analytics HTTP server.

```bash
# Start with default port
nself plugin analytics server

# Start with custom port
nself plugin analytics server --port 3500
```

**Options:**
- `-p, --port <port>` - Server port (default: 3304)
- `-h, --host <host>` - Server host (default: 0.0.0.0)

### track

Track a custom event from CLI.

```bash
# Track simple event
nself plugin analytics track --event "user_login" --user "user_123"

# Track event with properties
nself plugin analytics track \
  --event "purchase" \
  --user "user_123" \
  --properties '{"product":"PROD-001","amount":99.99}'
```

**Options:**
- `--event <name>` - Event name (required)
- `--user <id>` - User ID
- `--session <id>` - Session ID
- `--properties <json>` - Event properties as JSON

### counters

Manage and query counters.

```bash
# List all counters
nself plugin analytics counters list

# Get counter value
nself plugin analytics counters get api_requests

# Get counter with dimensions
nself plugin analytics counters get api_requests \
  --dimensions '{"endpoint":"/api/users"}'

# Increment counter
nself plugin analytics counters increment api_requests --value 1

# Decrement counter
nself plugin analytics counters decrement api_requests --value 1

# Get counter history
nself plugin analytics counters history api_requests \
  --from "2026-02-01" \
  --to "2026-02-11" \
  --granularity hour
```

**Granularity options:** `hour`, `day`, `week`, `month`

### funnels

Analyze conversion funnels.

```bash
# List all funnels
nself plugin analytics funnels list

# Create a funnel
nself plugin analytics funnels create \
  --name "signup_funnel" \
  --steps "visit_signup,submit_form,verify_email,complete_profile"

# Get funnel conversion data
nself plugin analytics funnels analyze signup_funnel \
  --from "2026-02-01" \
  --to "2026-02-11"

# Get user path through funnel
nself plugin analytics funnels path signup_funnel \
  --user "user_123"
```

### quotas

Manage usage quotas.

```bash
# List all quotas
nself plugin analytics quotas list

# Create a quota
nself plugin analytics quotas create \
  --name "api_calls_per_day" \
  --limit 10000 \
  --period day \
  --user "user_123"

# Check quota usage
nself plugin analytics quotas check api_calls_per_day \
  --user "user_123"

# List quota violations
nself plugin analytics quotas violations \
  --quota "api_calls_per_day" \
  --from "2026-02-01"
```

### rollup

Trigger manual counter rollup.

```bash
# Rollup all counters
nself plugin analytics rollup

# Rollup specific counter
nself plugin analytics rollup --counter api_requests

# Rollup with specific granularity
nself plugin analytics rollup --granularity day
```

### dashboard

View analytics dashboard.

```bash
nself plugin analytics dashboard
```

**Output:**
```
=== Analytics Dashboard ===

Events (Last 24h):
  Total: 12,450
  Unique Users: 3,201
  Top Events:
    - page_view: 5,234
    - button_click: 2,891
    - api_request: 1,654

Counters:
  api_requests: 45,678 (today)
  active_users: 3,201
  errors: 23

Quotas:
  api_calls_per_day: 8,234 / 10,000 (82.3%)
  storage_gb: 45.2 / 100 (45.2%)

Funnels:
  signup_funnel: 23.4% conversion
  checkout_funnel: 67.8% conversion
```

### stats

Show detailed analytics statistics.

```bash
nself plugin analytics stats
```

---

## REST API

### Health Check Endpoints

#### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "analytics",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready

Readiness check with database connectivity.

**Response:**
```json
{
  "ready": true,
  "plugin": "analytics",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Event Tracking

#### POST /api/track

Track a custom event.

**Request Body:**
```json
{
  "eventName": "page_view",
  "userId": "user_123",
  "sessionId": "session_456",
  "timestamp": "2026-02-11T10:00:00Z",
  "properties": {
    "page": "/products",
    "referrer": "https://google.com",
    "device": "mobile"
  }
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "eventName": "page_view",
  "userId": "user_123",
  "tracked": true,
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### POST /api/track/batch

Track multiple events in batch.

**Request Body:**
```json
{
  "events": [
    {
      "eventName": "page_view",
      "userId": "user_123",
      "properties": {"page": "/home"}
    },
    {
      "eventName": "button_click",
      "userId": "user_123",
      "properties": {"button": "signup"}
    }
  ]
}
```

**Response:** `201 Created`
```json
{
  "tracked": 2,
  "failed": 0,
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /api/events

Query tracked events.

**Query Parameters:**
- `userId` (string) - Filter by user ID
- `sessionId` (string) - Filter by session ID
- `eventName` (string) - Filter by event name
- `from` (ISO date) - Start date
- `to` (ISO date) - End date
- `limit` (number) - Results per page (default: 100)
- `offset` (number) - Pagination offset

**Response:** `200 OK`
```json
{
  "events": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "eventName": "page_view",
      "userId": "user_123",
      "sessionId": "session_456",
      "properties": {
        "page": "/products",
        "referrer": "https://google.com"
      },
      "timestamp": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 12450,
  "limit": 100,
  "offset": 0
}
```

### Counter Management

#### POST /api/counters/increment

Increment a counter.

**Request Body:**
```json
{
  "name": "api_requests",
  "value": 1,
  "dimensions": {
    "endpoint": "/api/users",
    "method": "GET",
    "status": "200"
  },
  "timestamp": "2026-02-11T10:00:00Z"
}
```

**Response:** `200 OK`
```json
{
  "name": "api_requests",
  "value": 45679,
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### POST /api/counters/decrement

Decrement a counter.

**Request Body:**
```json
{
  "name": "active_connections",
  "value": 1
}
```

**Response:** `200 OK`

#### GET /api/counters/:name

Get current counter value.

**Query Parameters:**
- `dimensions` (JSON) - Filter by dimensions

**Response:** `200 OK`
```json
{
  "name": "api_requests",
  "value": 45679,
  "dimensions": {},
  "lastUpdated": "2026-02-11T10:00:00.000Z"
}
```

#### GET /api/counters/:name/history

Get counter history with time-series data.

**Query Parameters:**
- `from` (ISO date) - Start date (required)
- `to` (ISO date) - End date (required)
- `granularity` (string) - `hour`, `day`, `week`, `month` (default: day)
- `dimensions` (JSON) - Filter by dimensions

**Response:** `200 OK`
```json
{
  "name": "api_requests",
  "granularity": "day",
  "data": [
    {
      "timestamp": "2026-02-10T00:00:00.000Z",
      "value": 42156
    },
    {
      "timestamp": "2026-02-11T00:00:00.000Z",
      "value": 45679
    }
  ]
}
```

#### GET /api/counters

List all counters.

**Response:** `200 OK`
```json
{
  "counters": [
    {
      "name": "api_requests",
      "value": 45679,
      "lastUpdated": "2026-02-11T10:00:00.000Z"
    },
    {
      "name": "active_users",
      "value": 3201,
      "lastUpdated": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 2
}
```

### Funnel Analysis

#### POST /api/funnels

Create a conversion funnel.

**Request Body:**
```json
{
  "name": "signup_funnel",
  "description": "User signup conversion funnel",
  "steps": [
    "visit_signup",
    "submit_form",
    "verify_email",
    "complete_profile"
  ],
  "conversionWindow": 86400
}
```

**Response:** `201 Created`

#### GET /api/funnels/:name/analyze

Analyze funnel conversion rates.

**Query Parameters:**
- `from` (ISO date) - Start date (required)
- `to` (ISO date) - End date (required)
- `groupBy` (string) - Group by dimension

**Response:** `200 OK`
```json
{
  "funnel": "signup_funnel",
  "period": {
    "from": "2026-02-01T00:00:00.000Z",
    "to": "2026-02-11T23:59:59.000Z"
  },
  "steps": [
    {
      "step": "visit_signup",
      "users": 10000,
      "conversion": 100.0,
      "dropoff": 0.0
    },
    {
      "step": "submit_form",
      "users": 7500,
      "conversion": 75.0,
      "dropoff": 25.0
    },
    {
      "step": "verify_email",
      "users": 5000,
      "conversion": 50.0,
      "dropoff": 33.3
    },
    {
      "step": "complete_profile",
      "users": 2340,
      "conversion": 23.4,
      "dropoff": 53.2
    }
  ],
  "overallConversion": 23.4
}
```

#### GET /api/funnels/:name/users/:userId

Get user's path through funnel.

**Response:** `200 OK`
```json
{
  "userId": "user_123",
  "funnel": "signup_funnel",
  "completedSteps": [
    {
      "step": "visit_signup",
      "timestamp": "2026-02-10T10:00:00.000Z"
    },
    {
      "step": "submit_form",
      "timestamp": "2026-02-10T10:05:00.000Z"
    },
    {
      "step": "verify_email",
      "timestamp": "2026-02-10T10:30:00.000Z"
    }
  ],
  "currentStep": "verify_email",
  "completed": false,
  "abandonedAt": null
}
```

#### GET /api/funnels

List all funnels.

**Response:** `200 OK`
```json
{
  "funnels": [
    {
      "name": "signup_funnel",
      "description": "User signup conversion funnel",
      "steps": ["visit_signup", "submit_form", "verify_email", "complete_profile"],
      "createdAt": "2026-02-01T00:00:00.000Z"
    }
  ],
  "total": 1
}
```

### Quota Management

#### POST /api/quotas

Create a usage quota.

**Request Body:**
```json
{
  "name": "api_calls_per_day",
  "description": "Daily API call limit",
  "limit": 10000,
  "period": "day",
  "scope": "user",
  "scopeId": "user_123",
  "resetOnViolation": false,
  "webhookOnViolation": true
}
```

**Response:** `201 Created`

#### GET /api/quotas/:name/check

Check quota usage.

**Query Parameters:**
- `scopeId` (string) - Scope identifier (e.g., user ID)

**Response:** `200 OK`
```json
{
  "name": "api_calls_per_day",
  "limit": 10000,
  "used": 8234,
  "remaining": 1766,
  "percentage": 82.34,
  "exceeded": false,
  "resetAt": "2026-02-12T00:00:00.000Z"
}
```

#### POST /api/quotas/:name/consume

Consume quota units.

**Request Body:**
```json
{
  "scopeId": "user_123",
  "units": 1
}
```

**Response:** `200 OK`
```json
{
  "allowed": true,
  "remaining": 1765,
  "resetAt": "2026-02-12T00:00:00.000Z"
}
```

Or if exceeded:
```json
{
  "allowed": false,
  "exceeded": true,
  "limit": 10000,
  "used": 10001,
  "resetAt": "2026-02-12T00:00:00.000Z"
}
```

#### GET /api/quotas/:name/violations

List quota violations.

**Query Parameters:**
- `from` (ISO date) - Start date
- `to` (ISO date) - End date
- `scopeId` (string) - Filter by scope ID

**Response:** `200 OK`
```json
{
  "violations": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440000",
      "quotaName": "api_calls_per_day",
      "scopeId": "user_123",
      "limit": 10000,
      "actualUsage": 10050,
      "excess": 50,
      "timestamp": "2026-02-11T14:30:00.000Z"
    }
  ],
  "total": 1
}
```

#### GET /api/quotas

List all quotas.

**Response:** `200 OK`
```json
{
  "quotas": [
    {
      "name": "api_calls_per_day",
      "description": "Daily API call limit",
      "limit": 10000,
      "period": "day",
      "scope": "user"
    }
  ],
  "total": 1
}
```

### Rollup

#### POST /api/rollup

Trigger counter rollup.

**Request Body:**
```json
{
  "counter": "api_requests",
  "granularity": "hour"
}
```

**Response:** `200 OK`
```json
{
  "rolledUp": true,
  "counter": "api_requests",
  "recordsProcessed": 1250,
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

---

## Webhook Events

The Analytics plugin can dispatch webhooks for key events:

| Event | Description |
|-------|-------------|
| `event.tracked` | New event tracked |
| `counter.incremented` | Counter incremented |
| `quota.exceeded` | Quota limit exceeded |
| `funnel.completed` | User completed funnel |
| `rollup.completed` | Counter rollup completed |

### Example Webhook Payload

**quota.exceeded:**
```json
{
  "eventType": "quota.exceeded",
  "timestamp": "2026-02-11T14:30:00.000Z",
  "payload": {
    "quotaName": "api_calls_per_day",
    "scopeId": "user_123",
    "limit": 10000,
    "actualUsage": 10050,
    "excess": 50
  }
}
```

**funnel.completed:**
```json
{
  "eventType": "funnel.completed",
  "timestamp": "2026-02-11T10:00:00.000Z",
  "payload": {
    "funnelName": "signup_funnel",
    "userId": "user_123",
    "duration": 1800,
    "steps": ["visit_signup", "submit_form", "verify_email", "complete_profile"]
  }
}
```

---

## Database Schema

### analytics_events

Stores raw tracked events.

```sql
CREATE TABLE analytics_events (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_name VARCHAR(255) NOT NULL,
  user_id VARCHAR(255),
  session_id VARCHAR(255),
  properties JSONB DEFAULT '{}',
  timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_analytics_events_source_account ON analytics_events(source_account_id);
CREATE INDEX idx_analytics_events_event_name ON analytics_events(event_name);
CREATE INDEX idx_analytics_events_user_id ON analytics_events(user_id);
CREATE INDEX idx_analytics_events_session_id ON analytics_events(session_id);
CREATE INDEX idx_analytics_events_timestamp ON analytics_events(timestamp DESC);
CREATE INDEX idx_analytics_events_properties ON analytics_events USING GIN(properties);
```

### analytics_counters

Stores counter values with dimensions.

```sql
CREATE TABLE analytics_counters (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  value BIGINT DEFAULT 0,
  dimensions JSONB DEFAULT '{}',
  granularity VARCHAR(32) DEFAULT 'raw',
  period_start TIMESTAMP WITH TIME ZONE,
  period_end TIMESTAMP WITH TIME ZONE,
  last_updated TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name, dimensions, granularity, period_start)
);

CREATE INDEX idx_analytics_counters_source_account ON analytics_counters(source_account_id);
CREATE INDEX idx_analytics_counters_name ON analytics_counters(name);
CREATE INDEX idx_analytics_counters_dimensions ON analytics_counters USING GIN(dimensions);
CREATE INDEX idx_analytics_counters_granularity ON analytics_counters(granularity);
CREATE INDEX idx_analytics_counters_period ON analytics_counters(period_start, period_end);
CREATE INDEX idx_analytics_counters_updated ON analytics_counters(last_updated DESC);
```

### analytics_funnels

Defines conversion funnels.

```sql
CREATE TABLE analytics_funnels (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  steps TEXT[] NOT NULL,
  conversion_window INTEGER DEFAULT 86400,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_analytics_funnels_source_account ON analytics_funnels(source_account_id);
CREATE INDEX idx_analytics_funnels_name ON analytics_funnels(name);
```

### analytics_quotas

Defines usage quotas and limits.

```sql
CREATE TABLE analytics_quotas (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  quota_limit BIGINT NOT NULL,
  period VARCHAR(32) NOT NULL,
  scope VARCHAR(64) NOT NULL,
  scope_id VARCHAR(255),
  reset_on_violation BOOLEAN DEFAULT FALSE,
  webhook_on_violation BOOLEAN DEFAULT TRUE,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name, scope, scope_id)
);

CREATE INDEX idx_analytics_quotas_source_account ON analytics_quotas(source_account_id);
CREATE INDEX idx_analytics_quotas_name ON analytics_quotas(name);
CREATE INDEX idx_analytics_quotas_scope ON analytics_quotas(scope, scope_id);
```

### analytics_quota_violations

Logs quota violations.

```sql
CREATE TABLE analytics_quota_violations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  quota_id UUID REFERENCES analytics_quotas(id) ON DELETE CASCADE,
  quota_name VARCHAR(255) NOT NULL,
  scope_id VARCHAR(255),
  quota_limit BIGINT NOT NULL,
  actual_usage BIGINT NOT NULL,
  excess BIGINT NOT NULL,
  timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_analytics_quota_violations_source_account ON analytics_quota_violations(source_account_id);
CREATE INDEX idx_analytics_quota_violations_quota ON analytics_quota_violations(quota_id);
CREATE INDEX idx_analytics_quota_violations_scope ON analytics_quota_violations(scope_id);
CREATE INDEX idx_analytics_quota_violations_timestamp ON analytics_quota_violations(timestamp DESC);
```

### analytics_webhook_events

Tracks outbound webhook events.

```sql
CREATE TABLE analytics_webhook_events (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  dispatched BOOLEAN DEFAULT FALSE,
  dispatched_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_analytics_webhook_events_source_account ON analytics_webhook_events(source_account_id);
CREATE INDEX idx_analytics_webhook_events_type ON analytics_webhook_events(event_type);
CREATE INDEX idx_analytics_webhook_events_dispatched ON analytics_webhook_events(dispatched);
CREATE INDEX idx_analytics_webhook_events_created ON analytics_webhook_events(created_at DESC);
```

---

## Event Tracking

### When to Track Events

Track events for:
- User actions (clicks, form submissions, navigation)
- System events (API calls, errors, background jobs)
- Business metrics (purchases, signups, conversions)
- Performance metrics (page load times, query duration)

### Event Naming Conventions

Use consistent naming:
- **Object-Action pattern**: `user_login`, `order_created`, `video_played`
- **Namespacing**: `api.request`, `payment.succeeded`, `email.sent`
- **Lowercase with underscores**: `page_view` not `PageView` or `page-view`

### Event Properties

Include rich context:

```javascript
{
  "eventName": "product_purchased",
  "userId": "user_123",
  "sessionId": "session_456",
  "properties": {
    "productId": "PROD-001",
    "productName": "Premium Plan",
    "price": 99.99,
    "currency": "USD",
    "paymentMethod": "credit_card",
    "referrer": "google_ads",
    "platform": "web",
    "device": "mobile"
  }
}
```

### High-Volume Event Tracking

Use batch API for high throughput:

```javascript
const events = users.map(user => ({
  eventName: 'email_sent',
  userId: user.id,
  properties: {
    emailType: 'welcome',
    subject: 'Welcome to the platform'
  }
}));

await fetch('http://localhost:3304/api/track/batch', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({ events })
});
```

---

## Counters

### Counter Types

**Raw Counters** - Current values, updated in real-time
**Rolled-Up Counters** - Aggregated by time period (hour, day, week, month)

### Counter Dimensions

Use dimensions to segment counters:

```javascript
// Increment API request counter with dimensions
await fetch('http://localhost:3304/api/counters/increment', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    name: 'api_requests',
    value: 1,
    dimensions: {
      endpoint: '/api/users',
      method: 'GET',
      status: '200',
      region: 'us-east-1'
    }
  })
});
```

### Automatic Rollup

Counters are automatically rolled up at configured intervals:

```sql
-- Raw counter (real-time)
SELECT value FROM analytics_counters
WHERE name = 'api_requests'
  AND granularity = 'raw'
  AND dimensions = '{}';

-- Hourly rollup
SELECT value FROM analytics_counters
WHERE name = 'api_requests'
  AND granularity = 'hour'
  AND period_start = '2026-02-11 10:00:00';

-- Daily rollup
SELECT value FROM analytics_counters
WHERE name = 'api_requests'
  AND granularity = 'day'
  AND period_start = '2026-02-11 00:00:00';
```

---

## Funnels

### Creating Effective Funnels

1. **Define clear steps** - Each step should be a distinct event
2. **Order matters** - Steps must occur in sequence
3. **Set conversion window** - Time limit for completing funnel (seconds)
4. **Track granularly** - More steps = more insights but lower conversion

### Example: E-commerce Checkout Funnel

```javascript
// Create funnel
await fetch('http://localhost:3304/api/funnels', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    name: 'checkout_funnel',
    description: 'E-commerce checkout flow',
    steps: [
      'view_cart',
      'enter_shipping',
      'enter_payment',
      'review_order',
      'complete_purchase'
    ],
    conversionWindow: 3600  // 1 hour
  })
});

// Track funnel events
await trackEvent('view_cart', userId);
await trackEvent('enter_shipping', userId);
await trackEvent('enter_payment', userId);
await trackEvent('review_order', userId);
await trackEvent('complete_purchase', userId);

// Analyze conversion
const analysis = await fetch(
  'http://localhost:3304/api/funnels/checkout_funnel/analyze?from=2026-02-01&to=2026-02-11'
).then(r => r.json());

console.log(`Overall conversion: ${analysis.overallConversion}%`);
```

---

## Quotas

### Quota Periods

- `minute` - Resets every minute
- `hour` - Resets every hour
- `day` - Resets at midnight UTC
- `week` - Resets on Sunday
- `month` - Resets on 1st of month
- `year` - Resets on January 1st

### Quota Scopes

- `global` - Single quota for entire system
- `user` - Per-user quota (requires scopeId)
- `account` - Per-account quota
- `ip` - Per-IP address quota
- `custom` - Custom scope identifier

### Enforcing Quotas

```javascript
// Check and consume quota
const response = await fetch(
  'http://localhost:3304/api/quotas/api_calls_per_day/consume',
  {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      scopeId: 'user_123',
      units: 1
    })
  }
).then(r => r.json());

if (!response.allowed) {
  throw new Error(`Quota exceeded. Resets at ${response.resetAt}`);
}

// Proceed with API call
await handleApiRequest();
```

---

## Examples

### Example 1: Page View Tracking

```javascript
// Track page views with referrer
async function trackPageView(userId, page, referrer) {
  await fetch('http://localhost:3304/api/track', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      eventName: 'page_view',
      userId,
      properties: {
        page,
        referrer,
        timestamp: new Date().toISOString()
      }
    })
  });

  // Increment page view counter
  await fetch('http://localhost:3304/api/counters/increment', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      name: 'page_views',
      value: 1,
      dimensions: { page }
    })
  });
}

// Usage
await trackPageView('user_123', '/products', 'https://google.com');
```

### Example 2: API Rate Limiting with Quotas

```javascript
// Middleware for Express.js
async function apiRateLimitMiddleware(req, res, next) {
  const userId = req.user.id;

  const response = await fetch(
    'http://localhost:3304/api/quotas/api_calls_per_hour/consume',
    {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        scopeId: userId,
        units: 1
      })
    }
  ).then(r => r.json());

  if (!response.allowed) {
    return res.status(429).json({
      error: 'Rate limit exceeded',
      resetAt: response.resetAt
    });
  }

  next();
}

app.use('/api', apiRateLimitMiddleware);
```

### Example 3: Signup Funnel Analysis

```bash
# Create signup funnel
curl -X POST http://localhost:3304/api/funnels \
  -H "Content-Type: application/json" \
  -d '{
    "name": "signup_funnel",
    "steps": ["visit_landing", "click_signup", "submit_form", "verify_email", "complete_onboarding"],
    "conversionWindow": 604800
  }'

# Analyze last 30 days
curl "http://localhost:3304/api/funnels/signup_funnel/analyze?from=2026-01-12&to=2026-02-11"

# Result:
{
  "steps": [
    {"step": "visit_landing", "users": 50000, "conversion": 100.0},
    {"step": "click_signup", "users": 15000, "conversion": 30.0, "dropoff": 70.0},
    {"step": "submit_form", "users": 10000, "conversion": 20.0, "dropoff": 33.3},
    {"step": "verify_email", "users": 7500, "conversion": 15.0, "dropoff": 25.0},
    {"step": "complete_onboarding", "users": 5000, "conversion": 10.0, "dropoff": 33.3}
  ],
  "overallConversion": 10.0
}
```

### Example 4: Real-time Dashboard Metrics

```javascript
// Fetch real-time metrics for dashboard
async function getDashboardMetrics() {
  const [events, counters, quotas] = await Promise.all([
    fetch('http://localhost:3304/api/events?from=2026-02-11T00:00:00Z').then(r => r.json()),
    fetch('http://localhost:3304/api/counters').then(r => r.json()),
    fetch('http://localhost:3304/api/quotas').then(r => r.json())
  ]);

  return {
    eventsToday: events.total,
    activeUsers: counters.counters.find(c => c.name === 'active_users')?.value || 0,
    apiCalls: counters.counters.find(c => c.name === 'api_requests')?.value || 0,
    quotaUsage: quotas.quotas.map(q => ({
      name: q.name,
      percentage: (q.used / q.limit) * 100
    }))
  };
}
```

### Example 5: Counter Time-Series Visualization

```javascript
// Fetch hourly API request data for chart
async function getApiRequestsChart() {
  const response = await fetch(
    'http://localhost:3304/api/counters/api_requests/history?from=2026-02-10T00:00:00Z&to=2026-02-11T23:59:59Z&granularity=hour'
  ).then(r => r.json());

  // Format for Chart.js
  return {
    labels: response.data.map(d => new Date(d.timestamp).toLocaleTimeString()),
    datasets: [{
      label: 'API Requests',
      data: response.data.map(d => d.value),
      borderColor: 'rgb(75, 192, 192)',
      tension: 0.1
    }]
  };
}
```

---

## Troubleshooting

### Events Not Being Tracked

**Symptom:** Events submitted but not appearing in database.

**Solutions:**
```bash
# Check server logs
docker logs analytics-plugin

# Verify database connection
nself plugin analytics init

# Test event tracking
curl -X POST http://localhost:3304/api/track \
  -H "Content-Type: application/json" \
  -d '{"eventName":"test","userId":"test_user"}'

# Query events
curl "http://localhost:3304/api/events?eventName=test"
```

### Counters Not Incrementing

**Symptom:** Counter values not updating.

**Solutions:**
```bash
# Verify counter exists
curl http://localhost:3304/api/counters/your_counter_name

# Test increment
curl -X POST http://localhost:3304/api/counters/increment \
  -H "Content-Type: application/json" \
  -d '{"name":"your_counter_name","value":1}'

# Check for dimension mismatches
# Ensure dimensions match exactly between increment calls
```

### Quota Always Exceeds

**Symptom:** Quota reports exceeded even with low usage.

**Solutions:**
```bash
# Check quota configuration
curl http://localhost:3304/api/quotas/your_quota_name

# Verify scope ID matches
curl "http://localhost:3304/api/quotas/your_quota_name/check?scopeId=user_123"

# Reset quota (if needed)
# Delete and recreate quota with correct limits
```

### Funnel Conversion Shows 0%

**Symptom:** Funnel analysis shows no conversions.

**Solutions:**
- Verify all step events are being tracked
- Check conversion window is large enough
- Ensure user IDs are consistent across steps
- Verify step order matches actual user flow

```bash
# Check if steps are being tracked
curl "http://localhost:3304/api/events?eventName=step1_name"
curl "http://localhost:3304/api/events?eventName=step2_name"

# Check user path
curl http://localhost:3304/api/funnels/your_funnel/users/user_123
```

### High Memory Usage

**Symptom:** Analytics plugin consuming excessive memory.

**Solutions:**
```bash
# Reduce event retention
export ANALYTICS_EVENT_RETENTION_DAYS=30

# Reduce batch size
export ANALYTICS_BATCH_SIZE=50

# Enable rollup to aggregate old data
nself plugin analytics rollup

# Purge old events
DELETE FROM analytics_events
WHERE created_at < NOW() - INTERVAL '30 days';
```

### Slow Query Performance

**Symptom:** API queries taking too long.

**Solutions:**
```sql
-- Add indexes for common queries
CREATE INDEX idx_custom_query ON analytics_events(user_id, event_name, timestamp DESC);

-- Use counter aggregates instead of event queries
-- Query counters, not raw events

-- Partition large tables by time
ALTER TABLE analytics_events PARTITION BY RANGE (timestamp);
```

---

**Need Help?**

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Source: https://github.com/acamarata/nself-plugins/tree/main/plugins/analytics
