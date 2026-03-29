# Webhooks Plugin

Production-ready outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue.

## Features

- **Reliable Delivery**: Automatic retry with exponential backoff
- **HMAC Signing**: Secure webhook signatures using SHA-256
- **Dead Letter Queue**: Capture failed deliveries for investigation
- **Multi-Endpoint**: Dispatch events to multiple endpoints simultaneously
- **Auto-Disable**: Automatically disable endpoints after consecutive failures
- **Event Types**: Register and track custom event types
- **Statistics**: Comprehensive delivery statistics and success rates
- **Multi-App Support**: Isolate webhooks by source_account_id

## Installation

```bash
cd plugins/webhooks/ts
npm install
npm run build
```

## Quick Start

### 1. Initialize Database

```bash
npm run init
```

### 2. Start Server

```bash
npm start
# or in development:
npm run dev
```

### 3. Register an Endpoint

```bash
curl -X POST http://localhost:3403/v1/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/webhook",
    "events": ["user.created", "order.completed"],
    "description": "Production webhook endpoint"
  }'
```

### 4. Dispatch an Event

```bash
curl -X POST http://localhost:3403/v1/dispatch \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "user.created",
    "payload": {
      "user_id": "123",
      "email": "user@example.com"
    }
  }'
```

## Configuration

See `.env.example` for all configuration options.

### Required

- `DATABASE_URL` or `POSTGRES_*` settings

### Optional

- `WEBHOOKS_PLUGIN_PORT` - Server port (default: 3403)
- `WEBHOOKS_MAX_ATTEMPTS` - Max retry attempts (default: 5)
- `WEBHOOKS_REQUEST_TIMEOUT_MS` - Request timeout (default: 30000)
- `WEBHOOKS_CONCURRENT_DELIVERIES` - Concurrent delivery limit (default: 10)
- `WEBHOOKS_RETRY_DELAYS` - Retry delays in milliseconds (default: 10s,30s,2m,15m,1h)
- `WEBHOOKS_AUTO_DISABLE_THRESHOLD` - Auto-disable after N failures (default: 10)

## API Endpoints

### Webhook Management

- `POST /v1/endpoints` - Create endpoint
- `GET /v1/endpoints` - List endpoints
- `GET /v1/endpoints/:id` - Get endpoint details
- `PUT /v1/endpoints/:id` - Update endpoint
- `DELETE /v1/endpoints/:id` - Delete endpoint
- `POST /v1/endpoints/:id/test` - Send test webhook
- `POST /v1/endpoints/:id/rotate-secret` - Rotate signing secret
- `POST /v1/endpoints/:id/enable` - Re-enable endpoint

### Event Dispatch

- `POST /v1/dispatch` - Dispatch event to matching endpoints

### Deliveries

- `GET /v1/deliveries` - List deliveries (with filters)
- `GET /v1/deliveries/:id` - Get delivery details
- `POST /v1/deliveries/:id/retry` - Retry failed delivery

### Event Types

- `GET /v1/event-types` - List registered event types
- `POST /v1/event-types` - Register event type

### Dead Letter Queue

- `GET /v1/dead-letter` - List dead letter items
- `POST /v1/dead-letter/:id/retry` - Retry dead letter
- `POST /v1/dead-letter/:id/resolve` - Mark as resolved

### Statistics

- `GET /v1/stats` - Delivery statistics

### Health

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (database connectivity)
- `GET /live` - Liveness check with stats

## CLI Commands

### Server

```bash
# Start server
npm run server

# With custom port
npm run server -- --port 4000
```

### Status

```bash
# View overall status
npm run status

# Detailed statistics
npm run stats
```

### Endpoints

```bash
# List all endpoints
npm run endpoints list

# Create endpoint
npm run endpoints create "https://example.com/webhook" \
  --events "user.created,order.completed" \
  --description "Production endpoint"

# Delete endpoint
npm run endpoints delete <endpoint-id>

# Enable endpoint
npm run endpoints enable <endpoint-id>

# Test endpoint
npm run endpoints test <endpoint-id>
```

### Deliveries

```bash
# List deliveries
npm run deliveries list

# Filter by status
npm run deliveries list --status failed

# Retry delivery
npm run deliveries retry <delivery-id>
```

### Dead Letter Queue

```bash
# List unresolved dead letters
npm run dead-letter list --unresolved

# Resolve dead letter
npm run dead-letter resolve <id>
```

### Event Types

```bash
# List event types
npm run event-types list

# Register event type
npm run event-types register "user.created" \
  --description "User account created" \
  --plugin "auth"
```

### Dispatch Events

```bash
# Dispatch event
npm run dispatch "user.created" '{"user_id":"123"}'

# Dispatch to specific endpoints
npm run dispatch "order.completed" '{"order_id":"456"}' \
  --endpoints "endpoint-1,endpoint-2"
```

## Webhook Signature Verification

All webhook deliveries include an `X-Webhook-Signature` header with the format:

```
t=<timestamp>,v1=<signature>
```

### Verification (Node.js)

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, secret) {
  const parts = signature.split(',');
  const timestamp = parts.find(p => p.startsWith('t=')).split('=')[1];
  const hash = parts.find(p => p.startsWith('v1=')).split('=')[1];

  const signedPayload = `${timestamp}.${payload}`;
  const expectedHash = crypto
    .createHmac('sha256', secret)
    .update(signedPayload)
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(hash),
    Buffer.from(expectedHash)
  );
}

// Usage in Express
app.post('/webhook', express.raw({ type: 'application/json' }), (req, res) => {
  const signature = req.headers['x-webhook-signature'];
  const isValid = verifyWebhook(
    req.body.toString(),
    signature,
    process.env.WEBHOOK_SECRET
  );

  if (!isValid) {
    return res.status(401).send('Invalid signature');
  }

  const event = JSON.parse(req.body);
  // Handle event...
  res.send({ received: true });
});
```

## Database Schema

### webhook_endpoints

Stores registered webhook endpoints.

- `id` (UUID) - Unique endpoint ID
- `source_account_id` (VARCHAR) - Multi-app isolation
- `url` (TEXT) - Webhook URL
- `secret` (VARCHAR) - HMAC signing secret
- `events` (TEXT[]) - Subscribed event types
- `enabled` (BOOLEAN) - Endpoint status
- `failure_count` (INTEGER) - Consecutive failures

### webhook_deliveries

Tracks all delivery attempts.

- `id` (UUID) - Unique delivery ID
- `endpoint_id` (UUID) - Target endpoint
- `event_type` (VARCHAR) - Event type
- `payload` (JSONB) - Event payload
- `status` (VARCHAR) - pending/delivering/delivered/failed/dead_letter
- `attempt_count` (INTEGER) - Number of attempts
- `response_status` (INTEGER) - HTTP response code
- `response_time_ms` (INTEGER) - Response time

### webhook_event_types

Registry of available event types.

- `id` (UUID) - Unique ID
- `name` (VARCHAR) - Event type name
- `description` (TEXT) - Description
- `source_plugin` (VARCHAR) - Originating plugin
- `schema` (JSONB) - JSON schema
- `sample_payload` (JSONB) - Example payload

### webhook_dead_letters

Failed deliveries exceeding max attempts.

- `id` (UUID) - Unique ID
- `delivery_id` (UUID) - Original delivery
- `endpoint_id` (UUID) - Target endpoint
- `event_type` (VARCHAR) - Event type
- `payload` (JSONB) - Event payload
- `last_error` (TEXT) - Error message
- `resolved` (BOOLEAN) - Resolution status

## Retry Logic

Deliveries use exponential backoff:

1. **10 seconds** - First retry
2. **30 seconds** - Second retry
3. **2 minutes** - Third retry
4. **15 minutes** - Fourth retry
5. **1 hour** - Fifth retry

After max attempts, deliveries move to the dead letter queue.

## Auto-Disable

Endpoints are automatically disabled after 10 consecutive failures (configurable). This prevents indefinite retry loops and alerts you to broken endpoints.

To re-enable:

```bash
npm run endpoints enable <endpoint-id>
```

Or via API:

```bash
curl -X POST http://localhost:3403/v1/endpoints/<id>/enable
```

## Multi-App Support

The webhooks plugin supports multi-app isolation using `source_account_id`:

```bash
# Set via header
curl -X POST http://localhost:3403/v1/endpoints \
  -H "X-Source-Account-Id: production" \
  -H "Content-Type: application/json" \
  -d '{"url": "...", "events": [...]}'

# Or via query parameter
curl "http://localhost:3403/v1/endpoints?source_account_id=production"
```

## Production Deployment

### Systemd Service

```ini
[Unit]
Description=nself Webhooks Plugin
After=network.target postgresql.service

[Service]
Type=simple
User=nself
WorkingDirectory=/opt/nself/plugins/webhooks/ts
Environment=NODE_ENV=production
ExecStart=/usr/bin/node dist/server.js
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Docker

```dockerfile
FROM node:18-alpine

WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY dist ./dist

ENV PORT=3403
EXPOSE 3403

CMD ["node", "dist/server.js"]
```

### Environment Variables

```bash
# Production .env
DATABASE_URL=postgresql://user:pass@db.internal:5432/nself
WEBHOOKS_PLUGIN_PORT=3403
WEBHOOKS_API_KEY=your-secure-api-key
WEBHOOKS_CONCURRENT_DELIVERIES=50
WEBHOOKS_MAX_ATTEMPTS=5
LOG_LEVEL=info
```

## Monitoring

### Health Checks

```bash
# Kubernetes liveness probe
GET /health

# Kubernetes readiness probe
GET /ready
```

### Metrics

View delivery statistics:

```bash
curl http://localhost:3403/v1/stats
```

Returns:
- Endpoint counts (total/enabled/disabled)
- Delivery counts by status
- Success rates
- Average response times
- Dead letter counts

## Troubleshooting

### Deliveries Stuck in Pending

Check server logs and ensure the background processor is running:

```bash
# Server should log:
# "Starting webhook delivery processor"
```

### Endpoint Auto-Disabled

Check failure count:

```bash
curl http://localhost:3403/v1/endpoints/<id>
```

Review recent deliveries:

```bash
curl "http://localhost:3403/v1/deliveries?endpoint_id=<id>&status=failed"
```

### High Failure Rate

1. Check endpoint URLs are reachable
2. Verify signature verification on receiving end
3. Check endpoint timeout settings
4. Review dead letter queue for patterns

## License

Source-Available License

## Support

For issues and questions, see the [nself-plugins repository](https://github.com/nself-org/plugins).
