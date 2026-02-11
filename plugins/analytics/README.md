# Analytics Plugin for nself

Production-ready analytics engine with event tracking, counters, funnels, and quota management.

## Features

- **Event Tracking**: Track custom events with properties and context
- **Counters**: Increment counters with automatic rollup (hourly → daily → monthly → all_time)
- **Funnels**: Define and analyze multi-step conversion funnels
- **Quotas**: Set usage limits with configurable actions (warn/block/throttle)
- **Dashboard**: Real-time analytics dashboard with top events and quota status
- **Multi-Account Support**: Isolated data per source_account_id

## Quick Start

### Install Dependencies

```bash
cd plugins/analytics/ts
npm install
npm run build
```

### Configuration

Copy `.env.example` to `.env` and configure:

```bash
# Required
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional
ANALYTICS_PLUGIN_PORT=3304
ANALYTICS_BATCH_SIZE=100
ANALYTICS_ROLLUP_INTERVAL_MS=3600000
ANALYTICS_EVENT_RETENTION_DAYS=90
ANALYTICS_COUNTER_RETENTION_DAYS=365
ANALYTICS_API_KEY=your_secret_key
```

### Initialize

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
node dist/cli.js server
# or in development
npm run dev
```

## CLI Commands

### Server Management

```bash
# Initialize database schema
nself-analytics init

# Start server
nself-analytics server --port 3304

# Check status
nself-analytics status

# View dashboard
nself-analytics dashboard
```

### Event Tracking

```bash
# Track an event
nself-analytics track --name "user_signup" --user "user123" \
  --properties '{"plan": "pro", "referral": "google"}'
```

### Counter Management

```bash
# List counters
nself-analytics counters list

# Get counter value
nself-analytics counters get --name "api_calls" --period "daily"

# Increment counter
nself-analytics counters increment --name "api_calls" --increment 5

# Trigger rollup
nself-analytics rollup
```

### Funnel Management

```bash
# Create funnel
nself-analytics funnels create \
  --name "Signup Funnel" \
  --steps '[
    {"name": "Landing", "event_name": "page_view"},
    {"name": "Signup Form", "event_name": "signup_started"},
    {"name": "Complete", "event_name": "signup_completed"}
  ]' \
  --window 24

# List funnels
nself-analytics funnels list

# Analyze funnel
nself-analytics funnels analyze <funnel-id>
```

### Quota Management

```bash
# Create quota
nself-analytics quotas create \
  --name "API Rate Limit" \
  --counter "api_calls" \
  --max 1000 \
  --period "hourly" \
  --scope "user"

# List quotas
nself-analytics quotas list

# Check quota
nself-analytics quotas check --counter "api_calls" --scope-id "user123"
```

## REST API

### Event Tracking

```bash
# Track single event
POST /v1/events
{
  "event_name": "button_click",
  "event_category": "engagement",
  "user_id": "user123",
  "session_id": "session456",
  "properties": {
    "button_id": "cta_primary",
    "page": "/pricing"
  },
  "context": {
    "user_agent": "Mozilla/5.0...",
    "ip": "1.2.3.4"
  }
}

# Track batch
POST /v1/events/batch
{
  "events": [
    { "event_name": "page_view", "user_id": "user1" },
    { "event_name": "button_click", "user_id": "user2" }
  ]
}

# Query events
GET /v1/events?event_name=page_view&limit=100&offset=0
```

### Counters

```bash
# Increment counter
POST /v1/counters/increment
{
  "counter_name": "api_calls",
  "dimension": "user123",
  "increment": 1,
  "metadata": {}
}

# Get counter value
GET /v1/counters?counter_name=api_calls&dimension=user123&period=daily

# Get timeseries
GET /v1/counters/api_calls/timeseries?period=daily&start_date=2024-01-01

# Trigger rollup
POST /v1/counters/rollup
```

### Funnels

```bash
# Create funnel
POST /v1/funnels
{
  "name": "Checkout Funnel",
  "description": "Product to purchase flow",
  "steps": [
    { "name": "Product View", "event_name": "product_viewed" },
    { "name": "Add to Cart", "event_name": "added_to_cart" },
    { "name": "Checkout", "event_name": "checkout_started" },
    { "name": "Purchase", "event_name": "purchase_completed" }
  ],
  "window_hours": 24
}

# List funnels
GET /v1/funnels?limit=100&offset=0

# Get funnel
GET /v1/funnels/:id

# Analyze funnel
GET /v1/funnels/:id/analyze

# Update funnel
PUT /v1/funnels/:id
{
  "enabled": false
}

# Delete funnel
DELETE /v1/funnels/:id
```

### Quotas

```bash
# Create quota
POST /v1/quotas
{
  "name": "API Rate Limit",
  "scope": "user",
  "scope_id": null,
  "counter_name": "api_calls",
  "max_value": 1000,
  "period": "hourly",
  "action_on_exceed": "block",
  "enabled": true
}

# List quotas
GET /v1/quotas?limit=100&offset=0

# Update quota
PUT /v1/quotas/:id
{
  "max_value": 2000
}

# Delete quota
DELETE /v1/quotas/:id

# Check quota
POST /v1/quotas/check
{
  "counter_name": "api_calls",
  "scope_id": "user123",
  "increment": 1
}

# List violations
GET /v1/violations?limit=100&offset=0
```

### Dashboard

```bash
# Get dashboard stats
GET /v1/dashboard

# Get status
GET /v1/status

# Health checks
GET /health
GET /ready
GET /live
```

## Database Schema

### Tables

- **analytics_events**: All tracked events with properties and context
- **analytics_counters**: Counter values with automatic rollup
- **analytics_funnels**: Funnel definitions
- **analytics_quotas**: Quota rules
- **analytics_quota_violations**: Quota violation records
- **analytics_webhook_events**: Webhook event log

### Multi-Account Support

All tables include `source_account_id` column for data isolation. Use headers:
- `X-Source-Account-Id: your-account-id`
- `X-App-Id: your-app-id`

## Counter Rollup

Counters automatically roll up:
- Hourly → Daily (every hour)
- Daily → Monthly (every day)
- All counters maintain `all_time` totals

Trigger manual rollup:
```bash
POST /v1/counters/rollup
```

Or via CLI:
```bash
nself-analytics rollup
```

## Funnel Analysis

Funnels calculate:
- Users at each step
- Conversion rate between steps
- Drop-off rate at each step
- Overall funnel conversion rate

Example analysis output:
```json
{
  "funnel_id": "uuid",
  "funnel_name": "Signup Funnel",
  "steps": [
    {
      "step_number": 1,
      "step_name": "Landing",
      "event_name": "page_view",
      "users": 1000,
      "conversion_rate": 100,
      "drop_off_rate": 0
    },
    {
      "step_number": 2,
      "step_name": "Signup Form",
      "event_name": "signup_started",
      "users": 500,
      "conversion_rate": 50,
      "drop_off_rate": 50
    },
    {
      "step_number": 3,
      "step_name": "Complete",
      "event_name": "signup_completed",
      "users": 400,
      "conversion_rate": 80,
      "drop_off_rate": 20
    }
  ],
  "total_entered": 1000,
  "total_completed": 400,
  "overall_conversion_rate": 40
}
```

## Quota Management

### Quota Scopes
- **app**: Application-wide limit
- **user**: Per-user limit
- **device**: Per-device limit

### Actions on Exceed
- **warn**: Log warning, allow operation
- **block**: Reject operation
- **throttle**: Rate limit future operations

### Example: API Rate Limiting

```javascript
// Before processing API request
const quotaCheck = await fetch('http://localhost:3304/v1/quotas/check', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    counter_name: 'api_calls',
    scope_id: userId,
    increment: 1
  })
});

const result = await quotaCheck.json();

if (!result.allowed && result.action === 'block') {
  return res.status(429).json({ error: 'Rate limit exceeded' });
}

// Process request and increment counter
await fetch('http://localhost:3304/v1/counters/increment', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    counter_name: 'api_calls',
    dimension: userId,
    increment: 1
  })
});
```

## Development

```bash
# Install dependencies
npm install

# Type check
npm run typecheck

# Build
npm run build

# Watch mode
npm run watch

# Development server with hot reload
npm run dev
```

## Production Deployment

1. Build the plugin:
```bash
npm run build
```

2. Set production environment variables

3. Initialize schema:
```bash
node dist/cli.js init
```

4. Start server with process manager:
```bash
pm2 start dist/cli.js --name analytics -- server
```

## License

Source-Available
