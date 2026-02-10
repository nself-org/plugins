# Notifications Plugin

Production-ready multi-channel notification system with email, push, and SMS support for nself.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Performance Considerations](#performance-considerations)
- [Security Notes](#security-notes)
- [Advanced Code Examples](#advanced-code-examples)
- [Monitoring & Alerting](#monitoring--alerting)
- [Use Cases](#use-cases)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Notifications plugin provides a complete multi-channel notification system supporting email, push notifications, and SMS. It includes a template engine, user preferences, delivery tracking, retry logic, rate limiting, batch/digest support, and provider fallback.

- **6 Database Tables** - Templates, preferences, notifications, queue, providers, batches
- **5 Analytics Views** - Delivery rates, engagement, provider health, user summary, queue backlog
- **3 Channels** - Email, push notifications, SMS
- **11 Providers** - Resend, SendGrid, Mailgun, AWS SES, SMTP, FCM, OneSignal, Web Push, Twilio, Plivo, AWS SNS
- **Template Engine** - Handlebars-based templates with variable substitution
- **GraphQL Integration** - `sendNotification()` Hasura Action

### Supported Providers

| Channel | Providers |
|---------|-----------|
| Email | Resend, SendGrid, Mailgun, AWS SES, SMTP (Gmail, Office 365, etc.) |
| Push | FCM (Firebase), OneSignal, Web Push (VAPID) |
| SMS | Twilio, Plivo, AWS SNS |

---

## Quick Start

```bash
# Install the plugin
cd ~/Sites/nself-plugins/plugins/notifications
bash install.sh

# Install TypeScript dependencies
cd ts
npm install
npm run build

# Configure environment
cp .env.example .env
# Edit .env with provider credentials

# Initialize database schema
nself plugin notifications init

# Start API server (Terminal 1)
nself plugin notifications server

# Start queue worker (Terminal 2)
nself plugin notifications worker

# Send a test notification
nself plugin notifications test email user@example.com
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `NOTIFICATIONS_EMAIL_PROVIDER` | No | `resend` | Email provider (resend, sendgrid, mailgun, ses, smtp) |
| `NOTIFICATIONS_EMAIL_API_KEY` | No | - | Email provider API key |
| `NOTIFICATIONS_EMAIL_FROM` | No | `noreply@example.com` | Default sender email address |
| `NOTIFICATIONS_EMAIL_ENABLED` | No | `false` | Enable email channel |
| `NOTIFICATIONS_PUSH_PROVIDER` | No | - | Push provider (fcm, onesignal, webpush) |
| `NOTIFICATIONS_PUSH_API_KEY` | No | - | Push provider API key |
| `NOTIFICATIONS_SMS_PROVIDER` | No | - | SMS provider (twilio, plivo, sns) |
| `NOTIFICATIONS_SMS_ACCOUNT_SID` | No | - | Twilio account SID |
| `NOTIFICATIONS_SMS_AUTH_TOKEN` | No | - | Twilio/Plivo auth token |
| `NOTIFICATIONS_SMS_FROM` | No | - | SMS sender phone number |
| `NOTIFICATIONS_QUEUE_BACKEND` | No | `redis` | Queue backend (redis or postgres) |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `WORKER_CONCURRENCY` | No | `5` | Number of concurrent workers |
| `NOTIFICATIONS_RETRY_ATTEMPTS` | No | `3` | Maximum retry attempts |
| `NOTIFICATIONS_RETRY_DELAY` | No | `1000` | Initial retry delay (ms) |
| `NOTIFICATIONS_MAX_RETRY_DELAY` | No | `300000` | Maximum retry delay (ms) |
| `NOTIFICATIONS_BATCH_INTERVAL` | No | `86400` | Batch/digest interval in seconds |
| `PORT` | No | `3102` | HTTP server port |
| `NOTIFICATIONS_DRY_RUN` | No | `false` | Test mode (no actual sending) |
| `NOTIFICATIONS_ENCRYPT_CONFIG` | No | `false` | Encrypt provider configs at rest |
| `NOTIFICATIONS_ENCRYPTION_KEY` | No | - | 32-character encryption key |
| `NOTIFICATIONS_WEBHOOK_SECRET` | No | - | Secret for webhook signature verification |
| `NOTIFICATIONS_WEBHOOK_VERIFY` | No | `false` | Enable webhook signature verification |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Email (Resend)
NOTIFICATIONS_EMAIL_ENABLED=true
NOTIFICATIONS_EMAIL_PROVIDER=resend
NOTIFICATIONS_EMAIL_API_KEY=re_xxxxxxxxxxxx
NOTIFICATIONS_EMAIL_FROM=notifications@example.com

# Queue
NOTIFICATIONS_QUEUE_BACKEND=redis
REDIS_URL=redis://localhost:6379
WORKER_CONCURRENCY=5

# Server
PORT=3102
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize and verify installation
nself plugin notifications init
```

### Template Management

```bash
# List all templates
nself plugin notifications template list

# Show template details
nself plugin notifications template show welcome_email

# Create new template
nself plugin notifications template create

# Update template
nself plugin notifications template update welcome_email

# Delete template
nself plugin notifications template delete old_template
```

### Testing

```bash
# Send test email
nself plugin notifications test email user@example.com

# Test a specific template
nself plugin notifications test template welcome_email user@example.com

# Check provider status
nself plugin notifications test providers

# Dry run (no actual sending)
NOTIFICATIONS_DRY_RUN=true nself plugin notifications test email user@example.com
```

### Statistics

```bash
# Overview
nself plugin notifications stats overview

# Delivery rates (last 30 days)
nself plugin notifications stats delivery 30

# Email engagement metrics
nself plugin notifications stats engagement 7

# Provider health
nself plugin notifications stats providers

# Top templates by usage
nself plugin notifications stats templates 20

# Recent failures
nself plugin notifications stats failures 50

# Hourly volume
nself plugin notifications stats hourly 24

# Export to JSON
nself plugin notifications stats export json stats.json

# Export to CSV
nself plugin notifications stats export csv notifications.csv
```

### Server & Worker

```bash
# Start HTTP server
nself plugin notifications server --port 3102 --host 0.0.0.0

# Start queue worker
nself plugin notifications worker --concurrency 10 --poll-interval 500
```

---

## REST API

The plugin exposes a REST API when the server is running.

### Base URL

```
http://localhost:3102
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/notifications/send` | Send a notification |
| `GET` | `/api/notifications/:id` | Get notification status |
| `GET` | `/api/templates` | List all templates |
| `GET` | `/api/templates/:name` | Get template by name |
| `POST` | `/api/preferences` | Update user preferences |
| `GET` | `/api/preferences/:user_id` | Get user preferences |
| `GET` | `/api/stats/delivery` | Delivery statistics |
| `GET` | `/api/stats/engagement` | Engagement metrics |
| `POST` | `/webhooks/notifications` | Webhook receiver for provider events |

### Send Notification

```http
POST /api/notifications/send
Content-Type: application/json

{
  "user_id": "123e4567-e89b-12d3-a456-426614174000",
  "channel": "email",
  "template": "welcome_email",
  "to": {
    "email": "user@example.com"
  },
  "variables": {
    "user_name": "John Doe",
    "app_name": "MyApp"
  }
}
```

Returns `{ success, notification_id, message }`.

### Get Notification Status

```http
GET /api/notifications/:id
```

Returns notification details including status (queued, sent, delivered, failed, bounced), timestamps (sent_at, delivered_at, opened_at), and channel information.

---

## Webhook Events

The plugin receives webhooks from notification providers to track delivery status.

### Inbound Provider Events

| Event | Description |
|-------|-------------|
| `delivery.succeeded` | Notification delivered successfully |
| `delivery.failed` | Delivery failed |
| `bounce` | Email bounced |
| `complaint` | Marked as spam |
| `open` | Email opened |
| `click` | Link clicked |
| `unsubscribe` | User unsubscribed |

Configure the webhook endpoint in your provider's dashboard:
```
POST https://your-domain.com/webhooks/notifications
```

Webhook signatures are verified when `NOTIFICATIONS_WEBHOOK_VERIFY=true` and `NOTIFICATIONS_WEBHOOK_SECRET` is set.

---

## Database Schema

### notification_templates

Reusable notification templates with Handlebars syntax.

```sql
CREATE TABLE notification_templates (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(100),                 -- transactional, marketing, system
    channels JSONB DEFAULT '["email"]',    -- supported channels
    subject VARCHAR(500),                  -- email subject (Handlebars)
    body_text TEXT,                        -- plain text body
    body_html TEXT,                        -- HTML body (Handlebars)
    variables JSONB DEFAULT '[]',          -- expected variables
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_notification_templates_name ON notification_templates(name);
CREATE INDEX idx_notification_templates_category ON notification_templates(category);
```

### notification_preferences

User opt-in/out settings per channel and category.

```sql
CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    channel VARCHAR(50) NOT NULL,          -- email, push, sms
    category VARCHAR(100),                 -- transactional, marketing, etc.
    enabled BOOLEAN DEFAULT TRUE,
    frequency VARCHAR(50),                 -- immediate, daily, weekly, disabled
    quiet_hours JSONB,                     -- {start, end, timezone}
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_notification_preferences_user_channel
    ON notification_preferences(user_id, channel, category);
```

### notifications

Sent notification log with delivery tracking.

```sql
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    channel VARCHAR(50) NOT NULL,          -- email, push, sms
    template_name VARCHAR(255),
    to_address VARCHAR(500),               -- email, phone, device token
    subject VARCHAR(500),
    body TEXT,
    variables JSONB,
    status VARCHAR(50) NOT NULL,           -- queued, sent, delivered, failed, bounced
    provider VARCHAR(50),                  -- resend, sendgrid, twilio, etc.
    provider_id VARCHAR(255),              -- provider message ID
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    sent_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    opened_at TIMESTAMP WITH TIME ZONE,
    clicked_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_notifications_user ON notifications(user_id);
CREATE INDEX idx_notifications_status ON notifications(status);
CREATE INDEX idx_notifications_channel ON notifications(channel);
CREATE INDEX idx_notifications_created ON notifications(created_at DESC);
```

### notification_queue

Async processing queue for pending notifications.

```sql
CREATE TABLE notification_queue (
    id UUID PRIMARY KEY,
    notification_id UUID REFERENCES notifications(id),
    priority INTEGER DEFAULT 0,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    locked_at TIMESTAMP WITH TIME ZONE,
    locked_by VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_notification_queue_priority ON notification_queue(priority DESC, created_at ASC);
CREATE INDEX idx_notification_queue_retry ON notification_queue(next_retry_at);
```

### notification_providers

Provider configurations and status.

```sql
CREATE TABLE notification_providers (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,            -- resend, sendgrid, twilio, etc.
    channel VARCHAR(50) NOT NULL,          -- email, push, sms
    priority INTEGER DEFAULT 0,            -- higher = preferred
    config JSONB,                          -- provider-specific config (encrypted)
    enabled BOOLEAN DEFAULT TRUE,
    last_success_at TIMESTAMP WITH TIME ZONE,
    last_failure_at TIMESTAMP WITH TIME ZONE,
    failure_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### notification_batches

Batch/digest tracking for grouped notifications.

```sql
CREATE TABLE notification_batches (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(100),
    interval_seconds INTEGER NOT NULL,
    config JSONB,                          -- {group_by, max_items}
    last_sent_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Analytics Views

### notification_delivery_rates

Delivery metrics by channel.

```sql
CREATE VIEW notification_delivery_rates AS
SELECT
    channel,
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    COUNT(*) FILTER (WHERE status = 'bounced') AS bounced,
    ROUND(COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL / NULLIF(COUNT(*), 0) * 100, 2)
        AS delivery_rate_pct
FROM notifications
GROUP BY channel;
```

### notification_engagement

Email open and click rates.

```sql
CREATE VIEW notification_engagement AS
SELECT
    template_name,
    COUNT(*) AS total_sent,
    COUNT(opened_at) AS opened,
    COUNT(clicked_at) AS clicked,
    ROUND(COUNT(opened_at)::DECIMAL / NULLIF(COUNT(*), 0) * 100, 2) AS open_rate_pct,
    ROUND(COUNT(clicked_at)::DECIMAL / NULLIF(COUNT(opened_at), 0) * 100, 2) AS click_rate_pct
FROM notifications
WHERE channel = 'email' AND status = 'delivered'
GROUP BY template_name
ORDER BY total_sent DESC;
```

### notification_provider_health

Provider status and reliability.

```sql
CREATE VIEW notification_provider_health AS
SELECT
    name,
    channel,
    enabled,
    priority,
    failure_count,
    last_success_at,
    last_failure_at
FROM notification_providers
ORDER BY channel, priority DESC;
```

### notification_user_summary

Per-user notification statistics.

```sql
CREATE VIEW notification_user_summary AS
SELECT
    user_id,
    COUNT(*) AS total_notifications,
    COUNT(*) FILTER (WHERE channel = 'email') AS email_count,
    COUNT(*) FILTER (WHERE channel = 'push') AS push_count,
    COUNT(*) FILTER (WHERE channel = 'sms') AS sms_count,
    MAX(created_at) AS last_notification_at
FROM notifications
GROUP BY user_id
ORDER BY total_notifications DESC;
```

### notification_queue_backlog

Current queue status.

```sql
CREATE VIEW notification_queue_backlog AS
SELECT
    COUNT(*) AS total_queued,
    COUNT(*) FILTER (WHERE locked_at IS NOT NULL) AS processing,
    COUNT(*) FILTER (WHERE locked_at IS NULL) AS pending,
    MIN(created_at) AS oldest_queued_at
FROM notification_queue
WHERE notification_id IN (
    SELECT id FROM notifications WHERE status = 'queued'
);
```

---

## Performance Considerations

### Batch Sending

Send notifications in batches to improve throughput and reduce database roundtrips.

#### Configuration

```bash
# Batch processing settings
NOTIFICATIONS_BATCH_SIZE=100              # Max notifications per batch
NOTIFICATIONS_BATCH_FLUSH_INTERVAL=5000   # Flush interval (ms)
WORKER_CONCURRENCY=10                     # Parallel workers
```

#### Database-Level Batching

```sql
-- Insert multiple notifications in one query
INSERT INTO notifications (id, user_id, channel, template_name, to_address, status)
SELECT
    gen_random_uuid(),
    user_id,
    'email',
    'weekly_digest',
    email,
    'queued'
FROM users
WHERE subscription_active = true
ON CONFLICT DO NOTHING;
```

#### Application-Level Batching

```typescript
// Batch send to multiple recipients
const recipients = [
  { email: 'user1@example.com', name: 'User 1' },
  { email: 'user2@example.com', name: 'User 2' },
  // ... up to 100 recipients
];

await fetch('http://localhost:3102/api/notifications/send-batch', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    template: 'marketing_campaign',
    recipients: recipients.map(r => ({
      to: { email: r.email },
      variables: { user_name: r.name }
    }))
  })
});
```

### Provider Failover

Automatically failover to backup providers when primary fails.

#### Configuration

```sql
-- Configure provider priorities (higher = preferred)
INSERT INTO notification_providers (name, channel, priority, config, enabled) VALUES
  ('resend', 'email', 100, '{"api_key": "re_xxx"}', true),
  ('sendgrid', 'email', 90, '{"api_key": "SG.xxx"}', true),
  ('mailgun', 'email', 80, '{"api_key": "xxx", "domain": "mg.example.com"}', true);
```

#### Failover Logic

The plugin automatically fails over when:
- Provider returns 5xx error (server error)
- Request times out after 30 seconds
- Provider reaches rate limit (429 response)
- Provider disabled due to high failure rate

```typescript
// Automatic failover sequence:
// 1. Try Resend (priority 100)
// 2. If fails, try SendGrid (priority 90)
// 3. If fails, try Mailgun (priority 80)
// 4. If all fail, notification marked as failed and queued for retry
```

#### Circuit Breaker

Providers are automatically disabled after consecutive failures:

```bash
# Circuit breaker settings
NOTIFICATIONS_CIRCUIT_BREAKER_THRESHOLD=10  # Failures before disabling
NOTIFICATIONS_CIRCUIT_BREAKER_TIMEOUT=300   # Seconds before retry
```

```sql
-- Check circuit breaker status
SELECT
    name,
    channel,
    failure_count,
    enabled,
    last_failure_at,
    EXTRACT(EPOCH FROM (NOW() - last_failure_at)) AS seconds_since_failure
FROM notification_providers
WHERE failure_count >= 10;
```

### Rate Limits Per Provider

#### Email Providers

| Provider | Free Tier | Paid Tier | Rate Limit |
|----------|-----------|-----------|------------|
| **Resend** | 100/day | 50,000/month | 10 req/sec |
| **SendGrid** | 100/day | 40,000/month | 600 req/min |
| **Mailgun** | 5,000/month | 50,000/month | 1,000 req/hour |
| **AWS SES** | 200/day (sandbox) | 50,000/day | 14 emails/sec |
| **SMTP** | Varies | Varies | 100-300/hour |

#### Push Providers

| Provider | Free Tier | Rate Limit |
|----------|-----------|------------|
| **FCM** | Unlimited | 600,000 req/min |
| **OneSignal** | Unlimited | 30 req/sec |
| **Web Push** | Unlimited | Browser-dependent |

#### SMS Providers

| Provider | Free Tier | Rate Limit |
|----------|-----------|------------|
| **Twilio** | Trial credit | 100 req/sec |
| **Plivo** | Trial credit | 200 req/sec |
| **AWS SNS** | 100 SMS/month | 20 req/sec |

#### Rate Limit Configuration

```bash
# Provider-specific rate limits
NOTIFICATIONS_RESEND_RATE_LIMIT=10          # req/sec
NOTIFICATIONS_SENDGRID_RATE_LIMIT=600       # req/min
NOTIFICATIONS_MAILGUN_RATE_LIMIT=1000       # req/hour
NOTIFICATIONS_TWILIO_RATE_LIMIT=100         # req/sec
```

#### Rate Limit Handling

```typescript
// Automatic rate limit handling:
// 1. Track requests per provider in Redis
// 2. Delay requests if approaching limit
// 3. Retry with exponential backoff if 429 received
// 4. Failover to different provider if rate limited
```

### Queue Optimization

#### Redis vs PostgreSQL Queue

**Redis Queue (Recommended for High Volume)**
- 10,000+ notifications/hour
- Sub-millisecond latency
- Requires Redis server
- Not persistent (use backup queue)

**PostgreSQL Queue (Recommended for Low Volume)**
- <1,000 notifications/hour
- Higher latency (10-50ms)
- No external dependencies
- Fully persistent

```bash
# Redis queue (high performance)
NOTIFICATIONS_QUEUE_BACKEND=redis
REDIS_URL=redis://localhost:6379

# PostgreSQL queue (simple setup)
NOTIFICATIONS_QUEUE_BACKEND=postgres
```

#### Queue Performance Tuning

```bash
# Worker optimization
WORKER_CONCURRENCY=20                      # Parallel workers
WORKER_POLL_INTERVAL=100                   # Poll interval (ms)
WORKER_BATCH_SIZE=50                       # Process N notifications per poll

# Queue cleanup
NOTIFICATIONS_QUEUE_RETENTION=604800       # Keep completed jobs 7 days
NOTIFICATIONS_AUTO_CLEANUP=true            # Auto-delete old jobs
```

#### Priority Queue

```sql
-- High priority notifications process first
UPDATE notification_queue
SET priority = 100
WHERE notification_id IN (
    SELECT id FROM notifications
    WHERE template_name IN ('password_reset', 'security_alert', 'payment_failed')
);
```

### Connection Pooling

```bash
# Database connection pool
DATABASE_POOL_MIN=5                        # Min connections
DATABASE_POOL_MAX=20                       # Max connections
DATABASE_POOL_IDLE_TIMEOUT=30000           # Idle timeout (ms)

# Redis connection pool
REDIS_POOL_MIN=2
REDIS_POOL_MAX=10
```

### Delivery Optimization

#### Smart Delivery Timing

```sql
-- Respect user quiet hours
CREATE OR REPLACE FUNCTION should_send_now(user_id UUID) RETURNS BOOLEAN AS $$
DECLARE
    prefs RECORD;
    current_hour INTEGER;
BEGIN
    SELECT quiet_hours INTO prefs
    FROM notification_preferences
    WHERE notification_preferences.user_id = should_send_now.user_id
    LIMIT 1;

    IF prefs.quiet_hours IS NULL THEN
        RETURN TRUE;
    END IF;

    current_hour := EXTRACT(HOUR FROM NOW() AT TIME ZONE (prefs.quiet_hours->>'timezone'));

    RETURN NOT (
        current_hour >= (prefs.quiet_hours->>'start')::INTEGER AND
        current_hour < (prefs.quiet_hours->>'end')::INTEGER
    );
END;
$$ LANGUAGE plpgsql;
```

#### Deduplication

```sql
-- Prevent duplicate notifications within time window
CREATE UNIQUE INDEX idx_notifications_dedup
ON notifications (user_id, template_name, DATE_TRUNC('hour', created_at))
WHERE status != 'failed';
```

#### Template Precompilation

Templates are compiled once and cached in memory:

```typescript
// Templates compiled on first use and cached
// Cache invalidated when template updated
NOTIFICATIONS_TEMPLATE_CACHE_TTL=3600  // Cache for 1 hour
```

### Monitoring Performance

```sql
-- Queue processing speed
SELECT
    COUNT(*) as processed,
    COUNT(*) / EXTRACT(EPOCH FROM (MAX(sent_at) - MIN(created_at))) as per_second
FROM notifications
WHERE sent_at > NOW() - INTERVAL '1 hour';

-- Average time in queue
SELECT
    AVG(EXTRACT(EPOCH FROM (sent_at - created_at))) as avg_queue_time_seconds
FROM notifications
WHERE sent_at > NOW() - INTERVAL '1 hour';

-- Provider response times (requires logging)
SELECT
    provider,
    AVG(response_time_ms) as avg_response_ms,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time_ms) as p95_response_ms
FROM notification_logs
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY provider;
```

---

## Security Notes

### API Key Management

**Best Practices:**

1. **Use Environment Variables** - Never hardcode API keys
2. **Encrypt at Rest** - Enable config encryption for database storage
3. **Rotate Regularly** - Change API keys every 90 days
4. **Use Secret Managers** - AWS Secrets Manager, HashiCorp Vault, etc.

```bash
# Encrypt provider configs in database
NOTIFICATIONS_ENCRYPT_CONFIG=true
NOTIFICATIONS_ENCRYPTION_KEY=your-32-character-secret-key12

# Use AWS Secrets Manager
NOTIFICATIONS_SECRETS_MANAGER=aws
AWS_REGION=us-east-1

# Use HashiCorp Vault
NOTIFICATIONS_SECRETS_MANAGER=vault
VAULT_ADDR=https://vault.example.com
VAULT_TOKEN=s.xxxxx
```

#### Reading from Secret Managers

```typescript
// Automatic secret resolution
// If NOTIFICATIONS_EMAIL_API_KEY starts with "aws:secretsmanager:"
// the plugin automatically fetches from AWS Secrets Manager

NOTIFICATIONS_EMAIL_API_KEY=aws:secretsmanager:prod/notifications/resend
NOTIFICATIONS_SMS_AUTH_TOKEN=vault:secret/data/notifications/twilio
```

### Template Injection Prevention

**Risk:** User-provided variables could inject malicious code into templates.

**Mitigation:**

```typescript
// 1. Handlebars auto-escapes HTML by default
// {{user_name}} → escaped (safe)
// {{{user_name}}} → unescaped (dangerous!)

// 2. Validate variable types
const ALLOWED_VARIABLE_TYPES = ['string', 'number', 'boolean'];

function sanitizeVariables(variables: Record<string, any>): Record<string, any> {
  const sanitized: Record<string, any> = {};

  for (const [key, value] of Object.entries(variables)) {
    const type = typeof value;

    if (!ALLOWED_VARIABLE_TYPES.includes(type)) {
      throw new Error(`Invalid variable type: ${type}`);
    }

    if (type === 'string') {
      // Strip HTML tags from user input
      sanitized[key] = value.replace(/<[^>]*>/g, '');
    } else {
      sanitized[key] = value;
    }
  }

  return sanitized;
}
```

#### Safe Template Patterns

```handlebars
<!-- SAFE: Auto-escaped -->
<p>Hello {{user_name}}!</p>

<!-- SAFE: Whitelisted HTML helper -->
<div>{{{sanitizedHtml body}}}</div>

<!-- DANGEROUS: Never use unescaped user input -->
<div>{{{user_bio}}}</div>

<!-- SAFE: Use built-in helpers -->
<a href="{{urlEncode redirect_url}}">Click here</a>
```

#### Template Validation

```sql
-- Restrict template editing to admins only
CREATE POLICY template_edit_policy ON notification_templates
FOR UPDATE
USING (
    EXISTS (
        SELECT 1 FROM users
        WHERE users.id = current_user_id()
        AND users.role = 'admin'
    )
);
```

### Webhook Security

#### Signature Verification

**Always verify webhook signatures to prevent spoofing:**

```bash
# Enable signature verification
NOTIFICATIONS_WEBHOOK_VERIFY=true
NOTIFICATIONS_WEBHOOK_SECRET=your-webhook-secret-key
```

#### Provider-Specific Verification

**Resend:**
```typescript
// Resend uses HMAC-SHA256
const signature = request.headers['resend-signature'];
const timestamp = request.headers['resend-timestamp'];
const payload = `${timestamp}.${rawBody}`;
const expectedSignature = crypto
  .createHmac('sha256', WEBHOOK_SECRET)
  .update(payload)
  .digest('hex');

if (signature !== expectedSignature) {
  throw new Error('Invalid signature');
}
```

**SendGrid:**
```typescript
// SendGrid uses ECDSA signature
const signature = request.headers['x-twilio-email-event-webhook-signature'];
const publicKey = SENDGRID_WEBHOOK_PUBLIC_KEY;
const verify = crypto.createVerify('RSA-SHA256');
verify.update(rawBody);

if (!verify.verify(publicKey, signature, 'base64')) {
  throw new Error('Invalid signature');
}
```

**Mailgun:**
```typescript
// Mailgun uses HMAC-SHA256
const signature = request.body.signature;
const token = signature.token;
const timestamp = signature.timestamp;
const sig = signature.signature;

const encoded = crypto
  .createHmac('sha256', MAILGUN_WEBHOOK_KEY)
  .update(`${timestamp}${token}`)
  .digest('hex');

if (sig !== encoded) {
  throw new Error('Invalid signature');
}
```

#### Webhook IP Whitelisting

```bash
# Restrict webhook endpoint to provider IPs
NOTIFICATIONS_WEBHOOK_IP_WHITELIST=192.0.2.0/24,198.51.100.0/24

# Example provider IP ranges:
# SendGrid: 168.245.0.0/16, 167.89.0.0/17
# Mailgun: 69.72.43.0/24, 50.56.21.0/24
```

### Rate Limiting (Security)

Prevent abuse with rate limiting:

```bash
# Per-IP rate limits
NOTIFICATIONS_API_RATE_LIMIT=100          # req/min per IP
NOTIFICATIONS_API_BURST_LIMIT=20          # burst allowance

# Per-user rate limits
NOTIFICATIONS_USER_EMAIL_LIMIT=100        # per hour
NOTIFICATIONS_USER_SMS_LIMIT=20           # per hour
NOTIFICATIONS_USER_PUSH_LIMIT=200         # per hour
```

### Data Privacy

#### PII Handling

```sql
-- Encrypt email addresses at rest
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE notifications_encrypted (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    to_address_encrypted BYTEA,  -- Encrypted email/phone
    encryption_key_id VARCHAR(50),
    -- ... other fields
);

-- Encrypt on insert
INSERT INTO notifications_encrypted (id, to_address_encrypted)
VALUES (
    gen_random_uuid(),
    pgp_sym_encrypt('user@example.com', current_setting('app.encryption_key'))
);

-- Decrypt on read
SELECT
    id,
    pgp_sym_decrypt(to_address_encrypted, current_setting('app.encryption_key'))::TEXT as to_address
FROM notifications_encrypted;
```

#### GDPR Compliance

```sql
-- User data export (GDPR Article 15)
SELECT
    n.id,
    n.channel,
    n.template_name,
    n.subject,
    n.status,
    n.created_at,
    n.delivered_at,
    n.opened_at
FROM notifications n
WHERE user_id = $1
ORDER BY created_at DESC;

-- Right to be forgotten (GDPR Article 17)
DELETE FROM notifications WHERE user_id = $1;
DELETE FROM notification_preferences WHERE user_id = $1;
```

#### Audit Logging

```sql
CREATE TABLE notification_audit_log (
    id UUID PRIMARY KEY,
    user_id UUID,
    action VARCHAR(50),  -- send, view, delete, export
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_audit_log_user ON notification_audit_log(user_id);
CREATE INDEX idx_audit_log_created ON notification_audit_log(created_at DESC);
```

### Secure SMTP Configuration

```bash
# Always use TLS
NOTIFICATIONS_SMTP_SECURE=true
NOTIFICATIONS_SMTP_PORT=465  # or 587 for STARTTLS

# Validate certificates
NOTIFICATIONS_SMTP_REJECT_UNAUTHORIZED=true

# Use app-specific passwords
NOTIFICATIONS_SMTP_PASSWORD=app-specific-password-not-main-password
```

---

## Advanced Code Examples

### Template Engine Usage

#### Creating Dynamic Templates

```sql
-- Insert a new template
INSERT INTO notification_templates (
    id,
    name,
    category,
    channels,
    subject,
    body_html,
    variables
) VALUES (
    gen_random_uuid(),
    'order_confirmation',
    'transactional',
    '["email"]',
    'Order #{{order_number}} confirmed',
    '<html>
        <body>
            <h1>Thanks for your order, {{customer_name}}!</h1>
            <p>Order #{{order_number}} has been confirmed.</p>

            <h2>Items</h2>
            <ul>
            {{#each items}}
                <li>{{this.name}} - ${{this.price}}</li>
            {{/each}}
            </ul>

            <p>Total: ${{total}}</p>

            {{#if tracking_number}}
            <p>Tracking: <a href="{{tracking_url}}">{{tracking_number}}</a></p>
            {{/if}}

            <p>Questions? Reply to this email or visit our <a href="{{help_url}}">Help Center</a>.</p>
        </body>
    </html>',
    '["customer_name", "order_number", "items", "total", "tracking_number", "tracking_url", "help_url"]'
);
```

#### Using Template with Variables

```typescript
// Send order confirmation
await fetch('http://localhost:3102/api/notifications/send', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    user_id: '123e4567-e89b-12d3-a456-426614174000',
    channel: 'email',
    template: 'order_confirmation',
    to: { email: 'customer@example.com' },
    variables: {
      customer_name: 'John Doe',
      order_number: 'ORD-2026-001',
      items: [
        { name: 'Product A', price: 29.99 },
        { name: 'Product B', price: 49.99 }
      ],
      total: 79.98,
      tracking_number: '1Z999AA10123456784',
      tracking_url: 'https://example.com/track/1Z999AA10123456784',
      help_url: 'https://example.com/help'
    }
  })
});
```

#### Custom Handlebars Helpers

```typescript
// Register custom helpers for advanced formatting
import Handlebars from 'handlebars';

// Currency formatting
Handlebars.registerHelper('currency', function(value: number, currency = 'USD') {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency
  }).format(value);
});

// Date formatting
Handlebars.registerHelper('formatDate', function(date: Date, format = 'long') {
  return new Intl.DateTimeFormat('en-US', {
    dateStyle: format as 'short' | 'medium' | 'long'
  }).format(new Date(date));
});

// Pluralization
Handlebars.registerHelper('pluralize', function(count: number, singular: string, plural: string) {
  return count === 1 ? singular : plural;
});

// Usage in templates:
// {{currency total 'EUR'}}
// {{formatDate order_date 'medium'}}
// You have {{item_count}} {{pluralize item_count 'item' 'items'}}
```

### Multi-Provider Failover

#### Configuring Multiple Providers

```typescript
// Initialize all email providers
const providers = [
  {
    name: 'resend',
    channel: 'email',
    priority: 100,
    config: {
      apiKey: process.env.RESEND_API_KEY,
      from: 'noreply@example.com'
    }
  },
  {
    name: 'sendgrid',
    channel: 'email',
    priority: 90,
    config: {
      apiKey: process.env.SENDGRID_API_KEY,
      from: 'noreply@example.com'
    }
  },
  {
    name: 'mailgun',
    channel: 'email',
    priority: 80,
    config: {
      apiKey: process.env.MAILGUN_API_KEY,
      domain: 'mg.example.com',
      from: 'noreply@example.com'
    }
  },
  {
    name: 'ses',
    channel: 'email',
    priority: 70,
    config: {
      region: 'us-east-1',
      accessKeyId: process.env.AWS_ACCESS_KEY_ID,
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY,
      from: 'noreply@example.com'
    }
  },
  {
    name: 'smtp',
    channel: 'email',
    priority: 60,
    config: {
      host: 'smtp.gmail.com',
      port: 587,
      secure: false,
      auth: {
        user: process.env.SMTP_USER,
        pass: process.env.SMTP_PASSWORD
      },
      from: 'noreply@example.com'
    }
  }
];

// Insert providers
for (const provider of providers) {
  await db.execute(
    `INSERT INTO notification_providers (id, name, channel, priority, config, enabled)
     VALUES (gen_random_uuid(), $1, $2, $3, $4, true)
     ON CONFLICT (name, channel) DO UPDATE SET
       priority = EXCLUDED.priority,
       config = EXCLUDED.config`,
    [provider.name, provider.channel, provider.priority, JSON.stringify(provider.config)]
  );
}
```

#### Failover Implementation

```typescript
async function sendWithFailover(notification: Notification): Promise<void> {
  // Get enabled providers sorted by priority
  const providers = await db.query(
    `SELECT name, config FROM notification_providers
     WHERE channel = $1 AND enabled = true
     ORDER BY priority DESC`,
    [notification.channel]
  );

  let lastError: Error | null = null;

  for (const provider of providers.rows) {
    try {
      await sendViaProvider(provider.name, provider.config, notification);

      // Success! Update provider stats
      await db.execute(
        `UPDATE notification_providers
         SET last_success_at = NOW(), failure_count = 0
         WHERE name = $1`,
        [provider.name]
      );

      return; // Exit on success

    } catch (error) {
      lastError = error as Error;

      // Track failure
      await db.execute(
        `UPDATE notification_providers
         SET last_failure_at = NOW(), failure_count = failure_count + 1
         WHERE name = $1`,
        [provider.name]
      );

      // Disable provider if too many failures
      const result = await db.query(
        `SELECT failure_count FROM notification_providers WHERE name = $1`,
        [provider.name]
      );

      if (result.rows[0].failure_count >= 10) {
        await db.execute(
          `UPDATE notification_providers SET enabled = false WHERE name = $1`,
          [provider.name]
        );
      }

      // Continue to next provider
      continue;
    }
  }

  // All providers failed
  throw new Error(`All providers failed. Last error: ${lastError?.message}`);
}
```

### Delivery Optimization

#### Intelligent Scheduling

```typescript
// Schedule notification for optimal delivery time
async function scheduleOptimal(
  userId: string,
  notification: Notification
): Promise<void> {
  // Get user timezone and preferences
  const prefs = await db.query(
    `SELECT quiet_hours FROM notification_preferences
     WHERE user_id = $1 AND channel = $2`,
    [userId, notification.channel]
  );

  const quietHours = prefs.rows[0]?.quiet_hours;
  const timezone = quietHours?.timezone || 'UTC';

  // Calculate next available send time
  const now = new Date();
  const localTime = new Date(now.toLocaleString('en-US', { timeZone: timezone }));
  const currentHour = localTime.getHours();

  let sendAt = now;

  if (quietHours &&
      currentHour >= quietHours.start &&
      currentHour < quietHours.end) {
    // Schedule for end of quiet hours
    const endHour = quietHours.end;
    sendAt = new Date(localTime);
    sendAt.setHours(endHour, 0, 0, 0);
  }

  // Insert into queue with scheduled time
  await db.execute(
    `INSERT INTO notification_queue (id, notification_id, next_retry_at)
     VALUES (gen_random_uuid(), $1, $2)`,
    [notification.id, sendAt]
  );
}
```

#### Batch Processing

```typescript
// Process notifications in batches
async function processBatch(batchSize = 100): Promise<void> {
  while (true) {
    // Fetch next batch
    const batch = await db.query(
      `SELECT nq.id as queue_id, n.*
       FROM notification_queue nq
       JOIN notifications n ON n.id = nq.notification_id
       WHERE nq.locked_at IS NULL
         AND (nq.next_retry_at IS NULL OR nq.next_retry_at <= NOW())
       ORDER BY nq.priority DESC, nq.created_at ASC
       LIMIT $1
       FOR UPDATE SKIP LOCKED`,
      [batchSize]
    );

    if (batch.rows.length === 0) {
      break; // No more work
    }

    // Lock batch
    const queueIds = batch.rows.map(r => r.queue_id);
    await db.execute(
      `UPDATE notification_queue
       SET locked_at = NOW(), locked_by = $1
       WHERE id = ANY($2)`,
      [process.pid, queueIds]
    );

    // Process in parallel
    await Promise.all(
      batch.rows.map(row => processNotification(row))
    );

    // Remove from queue
    await db.execute(
      `DELETE FROM notification_queue WHERE id = ANY($1)`,
      [queueIds]
    );
  }
}
```

### Batch Patterns

#### Daily Digest

```typescript
// Send daily digest of notifications
async function sendDailyDigest(userId: string): Promise<void> {
  // Collect notifications from last 24 hours
  const notifications = await db.query(
    `SELECT template_name, subject, created_at, variables
     FROM notifications
     WHERE user_id = $1
       AND created_at > NOW() - INTERVAL '24 hours'
       AND template_name NOT IN ('password_reset', 'security_alert')
     ORDER BY created_at DESC`,
    [userId]
  );

  if (notifications.rows.length === 0) {
    return; // No notifications to digest
  }

  // Group by category
  const grouped = notifications.rows.reduce((acc, n) => {
    const category = n.template_name.split('_')[0];
    if (!acc[category]) acc[category] = [];
    acc[category].push(n);
    return acc;
  }, {} as Record<string, any[]>);

  // Send digest email
  await fetch('http://localhost:3102/api/notifications/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: userId,
      channel: 'email',
      template: 'daily_digest',
      to: { email: await getUserEmail(userId) },
      variables: {
        count: notifications.rows.length,
        categories: grouped,
        date: new Date().toLocaleDateString()
      }
    })
  });

  // Mark as digested
  await db.execute(
    `UPDATE notifications
     SET metadata = jsonb_set(metadata, '{digested}', 'true')
     WHERE user_id = $1 AND created_at > NOW() - INTERVAL '24 hours'`,
    [userId]
  );
}
```

#### Bulk Campaign

```typescript
// Send bulk campaign to all subscribers
async function sendBulkCampaign(
  templateName: string,
  segmentQuery: string
): Promise<void> {
  const BATCH_SIZE = 1000;
  let offset = 0;

  while (true) {
    // Fetch subscriber batch
    const subscribers = await db.query(
      `${segmentQuery} LIMIT $1 OFFSET $2`,
      [BATCH_SIZE, offset]
    );

    if (subscribers.rows.length === 0) {
      break;
    }

    // Queue batch
    const values = subscribers.rows.map((s, i) =>
      `(gen_random_uuid(), '${s.id}', 'email', '${templateName}', '${s.email}', 'queued')`
    ).join(',');

    await db.execute(
      `INSERT INTO notifications (id, user_id, channel, template_name, to_address, status)
       VALUES ${values}`
    );

    offset += BATCH_SIZE;
  }

  console.log(`Queued ${offset} notifications for campaign`);
}

// Usage:
await sendBulkCampaign(
  'monthly_newsletter',
  `SELECT id, email FROM users WHERE subscription_active = true AND email_verified = true`
);
```

---

## Monitoring & Alerting

### Delivery Rates

#### Real-Time Dashboard Queries

```sql
-- Overall delivery rate (last 24 hours)
SELECT
    COUNT(*) as total_sent,
    COUNT(*) FILTER (WHERE status = 'delivered') as delivered,
    COUNT(*) FILTER (WHERE status = 'failed') as failed,
    COUNT(*) FILTER (WHERE status = 'bounced') as bounced,
    ROUND(
        COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL /
        NULLIF(COUNT(*), 0) * 100,
        2
    ) as delivery_rate_pct
FROM notifications
WHERE created_at > NOW() - INTERVAL '24 hours';

-- Delivery rate by channel
SELECT
    channel,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'delivered') as delivered,
    ROUND(
        COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL /
        NULLIF(COUNT(*), 0) * 100,
        2
    ) as rate_pct
FROM notifications
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY channel
ORDER BY total DESC;

-- Delivery rate by hour
SELECT
    DATE_TRUNC('hour', created_at) as hour,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'delivered') as delivered,
    ROUND(
        COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL /
        NULLIF(COUNT(*), 0) * 100,
        2
    ) as rate_pct
FROM notifications
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY hour
ORDER BY hour DESC;
```

#### API Endpoints for Monitoring

```bash
# Get delivery stats
curl http://localhost:3102/api/stats/delivery?period=24h

# Get engagement metrics
curl http://localhost:3102/api/stats/engagement?period=7d

# Get provider health
curl http://localhost:3102/api/stats/providers

# Get queue backlog
curl http://localhost:3102/api/stats/queue
```

### Failure Tracking

#### Detailed Failure Analysis

```sql
-- Failed notifications by error type
SELECT
    SUBSTRING(error FROM 1 FOR 50) as error_type,
    COUNT(*) as count,
    COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '1 hour') as last_hour
FROM notifications
WHERE status = 'failed'
GROUP BY error_type
ORDER BY count DESC
LIMIT 20;

-- Failed notifications by provider
SELECT
    provider,
    COUNT(*) as failures,
    ROUND(
        COUNT(*)::DECIMAL /
        (SELECT COUNT(*) FROM notifications WHERE status = 'failed') * 100,
        2
    ) as pct_of_failures
FROM notifications
WHERE status = 'failed'
  AND created_at > NOW() - INTERVAL '24 hours'
GROUP BY provider
ORDER BY failures DESC;

-- Bounce rate by domain
SELECT
    SUBSTRING(to_address FROM '@(.*)$') as domain,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'bounced') as bounced,
    ROUND(
        COUNT(*) FILTER (WHERE status = 'bounced')::DECIMAL /
        NULLIF(COUNT(*), 0) * 100,
        2
    ) as bounce_rate_pct
FROM notifications
WHERE channel = 'email'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY domain
HAVING COUNT(*) > 10
ORDER BY bounce_rate_pct DESC
LIMIT 20;
```

#### Failure Alerts

```sql
-- Alert if delivery rate drops below threshold
CREATE OR REPLACE FUNCTION check_delivery_rate() RETURNS VOID AS $$
DECLARE
    rate DECIMAL;
BEGIN
    SELECT
        COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL /
        NULLIF(COUNT(*), 0) * 100
    INTO rate
    FROM notifications
    WHERE created_at > NOW() - INTERVAL '1 hour';

    IF rate < 95.0 THEN
        -- Send alert notification
        INSERT INTO notifications (id, user_id, channel, template_name, to_address, status)
        VALUES (
            gen_random_uuid(),
            (SELECT id FROM users WHERE role = 'admin' LIMIT 1),
            'email',
            'system_alert',
            'admin@example.com',
            'queued'
        );
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Run every 5 minutes via cron
-- */5 * * * * psql $DATABASE_URL -c "SELECT check_delivery_rate();"
```

### Provider Health

#### Health Check Queries

```sql
-- Provider status overview
SELECT
    name,
    channel,
    enabled,
    priority,
    failure_count,
    last_success_at,
    last_failure_at,
    CASE
        WHEN last_success_at > NOW() - INTERVAL '5 minutes' THEN 'healthy'
        WHEN last_success_at > NOW() - INTERVAL '1 hour' THEN 'degraded'
        ELSE 'unhealthy'
    END as health_status
FROM notification_providers
ORDER BY channel, priority DESC;

-- Provider success rate (last 24 hours)
SELECT
    provider,
    COUNT(*) as total_sent,
    COUNT(*) FILTER (WHERE status IN ('delivered', 'sent')) as successful,
    COUNT(*) FILTER (WHERE status = 'failed') as failed,
    ROUND(
        COUNT(*) FILTER (WHERE status IN ('delivered', 'sent'))::DECIMAL /
        NULLIF(COUNT(*), 0) * 100,
        2
    ) as success_rate_pct,
    AVG(EXTRACT(EPOCH FROM (sent_at - created_at))) as avg_latency_sec
FROM notifications
WHERE created_at > NOW() - INTERVAL '24 hours'
  AND provider IS NOT NULL
GROUP BY provider
ORDER BY total_sent DESC;

-- Detect provider outages
SELECT
    provider,
    COUNT(*) as consecutive_failures,
    MIN(created_at) as started_at,
    MAX(created_at) as latest_at
FROM (
    SELECT
        provider,
        created_at,
        status,
        ROW_NUMBER() OVER (PARTITION BY provider ORDER BY created_at) -
        ROW_NUMBER() OVER (PARTITION BY provider, status ORDER BY created_at) as grp
    FROM notifications
    WHERE created_at > NOW() - INTERVAL '1 hour'
) t
WHERE status = 'failed'
GROUP BY provider, grp
HAVING COUNT(*) >= 10
ORDER BY consecutive_failures DESC;
```

#### Automated Health Checks

```typescript
// Health check worker
async function checkProviderHealth(): Promise<void> {
  const providers = await db.query(
    `SELECT name, channel, config FROM notification_providers WHERE enabled = true`
  );

  for (const provider of providers.rows) {
    try {
      // Send test notification
      await sendViaProvider(provider.name, provider.config, {
        to: 'healthcheck@example.com',
        subject: 'Health Check',
        body: 'This is an automated health check'
      });

      // Update success timestamp
      await db.execute(
        `UPDATE notification_providers
         SET last_success_at = NOW(), failure_count = 0
         WHERE name = $1`,
        [provider.name]
      );

    } catch (error) {
      // Update failure count
      await db.execute(
        `UPDATE notification_providers
         SET last_failure_at = NOW(), failure_count = failure_count + 1
         WHERE name = $1`,
        [provider.name]
      );
    }
  }
}

// Run every 5 minutes
setInterval(checkProviderHealth, 5 * 60 * 1000);
```

### Prometheus Metrics

```typescript
// Export metrics for Prometheus
import { Registry, Counter, Histogram, Gauge } from 'prom-client';

const register = new Registry();

// Notification counters
const notificationsSent = new Counter({
  name: 'notifications_sent_total',
  help: 'Total notifications sent',
  labelNames: ['channel', 'provider', 'status'],
  registers: [register]
});

const notificationLatency = new Histogram({
  name: 'notification_latency_seconds',
  help: 'Notification processing latency',
  labelNames: ['channel', 'provider'],
  buckets: [0.1, 0.5, 1, 2, 5, 10],
  registers: [register]
});

const queueSize = new Gauge({
  name: 'notification_queue_size',
  help: 'Current queue size',
  registers: [register]
});

// Update metrics
notificationsSent.inc({ channel: 'email', provider: 'resend', status: 'sent' });
notificationLatency.observe({ channel: 'email', provider: 'resend' }, 1.23);

// Expose metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});
```

### Alerting Rules

#### Grafana Alerts

```yaml
# grafana-alerts.yaml
groups:
  - name: notifications
    interval: 1m
    rules:
      - alert: HighFailureRate
        expr: |
          (
            sum(rate(notifications_sent_total{status="failed"}[5m]))
            /
            sum(rate(notifications_sent_total[5m]))
          ) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High notification failure rate
          description: Failure rate is {{ $value | humanizePercentage }}

      - alert: QueueBacklog
        expr: notification_queue_size > 10000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Notification queue backlog
          description: Queue has {{ $value }} pending notifications

      - alert: ProviderDown
        expr: |
          time() - notification_provider_last_success_timestamp > 600
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Notification provider down
          description: Provider {{ $labels.provider }} has not succeeded in 10+ minutes
```

#### PagerDuty Integration

```typescript
// Send critical alerts to PagerDuty
async function sendAlert(severity: 'info' | 'warning' | 'error' | 'critical', message: string): Promise<void> {
  if (severity === 'critical' || severity === 'error') {
    await fetch('https://events.pagerduty.com/v2/enqueue', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        routing_key: process.env.PAGERDUTY_INTEGRATION_KEY,
        event_action: 'trigger',
        payload: {
          summary: message,
          severity: severity,
          source: 'nself-notifications',
          timestamp: new Date().toISOString()
        }
      })
    });
  }
}

// Usage:
if (deliveryRate < 0.95) {
  await sendAlert('critical', `Delivery rate dropped to ${deliveryRate * 100}%`);
}
```

---

## Use Cases

### 1. Transactional Emails (Order Confirmations)

**Scenario:** E-commerce platform sends order confirmation immediately after purchase.

**Implementation:**

```typescript
// Triggered by webhook or after order creation
async function sendOrderConfirmation(orderId: string): Promise<void> {
  const order = await db.query('SELECT * FROM orders WHERE id = $1', [orderId]);
  const customer = await db.query('SELECT * FROM customers WHERE id = $1', [order.rows[0].customer_id]);

  await fetch('http://localhost:3102/api/notifications/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: customer.rows[0].id,
      channel: 'email',
      template: 'order_confirmation',
      to: { email: customer.rows[0].email },
      variables: {
        customer_name: customer.rows[0].name,
        order_number: order.rows[0].number,
        order_date: order.rows[0].created_at,
        items: order.rows[0].items,
        subtotal: order.rows[0].subtotal,
        tax: order.rows[0].tax,
        total: order.rows[0].total,
        shipping_address: order.rows[0].shipping_address
      }
    })
  });
}
```

**Key Features:**
- High priority (sent immediately)
- 99.9% delivery rate required
- Cannot be opted out (transactional)
- Multi-provider failover for reliability

### 2. Marketing Campaigns (Product Launches)

**Scenario:** SaaS company announces new feature to all active users.

**Implementation:**

```typescript
async function sendProductLaunch(): Promise<void> {
  // Segment users by engagement level
  const segments = [
    { name: 'power_users', query: 'SELECT * FROM users WHERE logins_last_30d > 20' },
    { name: 'regular_users', query: 'SELECT * FROM users WHERE logins_last_30d BETWEEN 5 AND 20' },
    { name: 'inactive_users', query: 'SELECT * FROM users WHERE logins_last_30d < 5' }
  ];

  for (const segment of segments) {
    const users = await db.query(segment.query);

    for (const user of users.rows) {
      // Check user preferences
      const prefs = await db.query(
        `SELECT enabled FROM notification_preferences
         WHERE user_id = $1 AND channel = 'email' AND category = 'marketing'`,
        [user.id]
      );

      if (prefs.rows[0]?.enabled === false) {
        continue; // User opted out
      }

      // Personalized template based on segment
      await fetch('http://localhost:3102/api/notifications/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          user_id: user.id,
          channel: 'email',
          template: `product_launch_${segment.name}`,
          to: { email: user.email },
          variables: {
            user_name: user.name,
            feature_name: 'AI Assistant',
            feature_description: 'Your new AI-powered coding companion',
            cta_url: `https://example.com/features/ai?user=${user.id}`
          }
        })
      });

      // Rate limit: 100 emails per second
      await new Promise(resolve => setTimeout(resolve, 10));
    }
  }
}
```

**Key Features:**
- Respects opt-out preferences
- Segmented messaging
- Rate limiting to avoid provider throttling
- Tracking clicks/opens for campaign analytics

### 3. System Alerts (Security Notifications)

**Scenario:** Banking app detects suspicious login and sends immediate alert.

**Implementation:**

```typescript
async function sendSecurityAlert(userId: string, event: string, details: any): Promise<void> {
  const user = await db.query('SELECT * FROM users WHERE id = $1', [userId]);

  // Send via multiple channels for critical alerts
  const channels = ['email', 'sms', 'push'];

  for (const channel of channels) {
    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: userId,
        channel: channel,
        template: 'security_alert',
        to: {
          email: user.rows[0].email,
          phone: user.rows[0].phone,
          device_token: user.rows[0].device_token
        }[channel],
        variables: {
          user_name: user.rows[0].name,
          event_type: event,
          event_time: new Date().toISOString(),
          ip_address: details.ip_address,
          location: details.location,
          device: details.device,
          action_url: `https://example.com/security/verify?token=${details.token}`
        }
      })
    });
  }

  // Log security event
  await db.execute(
    `INSERT INTO security_events (user_id, event_type, details, notified_at)
     VALUES ($1, $2, $3, NOW())`,
    [userId, event, JSON.stringify(details)]
  );
}
```

**Key Features:**
- Multi-channel delivery (email + SMS + push)
- Highest priority
- Cannot be opted out
- Immediate delivery (bypasses quiet hours)

### 4. Scheduled Reports (Weekly Analytics)

**Scenario:** Analytics platform sends weekly summary every Monday at 9 AM.

**Implementation:**

```typescript
// Cron job: 0 9 * * 1 (Every Monday at 9 AM)
async function sendWeeklyReport(): Promise<void> {
  const users = await db.query(
    `SELECT * FROM users WHERE subscription_tier IN ('pro', 'enterprise')`
  );

  for (const user of users.rows) {
    // Generate report data
    const stats = await db.query(
      `SELECT
         COUNT(*) as total_events,
         COUNT(DISTINCT session_id) as sessions,
         COUNT(DISTINCT user_id) as unique_users
       FROM analytics_events
       WHERE workspace_id = $1
         AND created_at > NOW() - INTERVAL '7 days'`,
      [user.workspace_id]
    );

    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: user.id,
        channel: 'email',
        template: 'weekly_analytics_report',
        to: { email: user.email },
        variables: {
          user_name: user.name,
          week_start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toLocaleDateString(),
          week_end: new Date().toLocaleDateString(),
          total_events: stats.rows[0].total_events,
          sessions: stats.rows[0].sessions,
          unique_users: stats.rows[0].unique_users,
          report_url: `https://example.com/reports/weekly/${user.workspace_id}`
        }
      })
    });
  }
}
```

**Key Features:**
- Scheduled delivery
- User timezone awareness
- Preference-based frequency (weekly/monthly)
- Rich data visualization in email

### 5. Reminder Notifications (Abandoned Cart)

**Scenario:** E-commerce site sends cart reminder 1 hour, 24 hours, and 7 days after abandonment.

**Implementation:**

```typescript
// Triggered by cron job checking abandoned carts
async function sendCartReminders(): Promise<void> {
  const intervals = [
    { hours: 1, template: 'cart_reminder_1h' },
    { hours: 24, template: 'cart_reminder_24h' },
    { hours: 168, template: 'cart_reminder_7d' }  // 7 days
  ];

  for (const interval of intervals) {
    const carts = await db.query(
      `SELECT c.*, u.email, u.name
       FROM shopping_carts c
       JOIN users u ON u.id = c.user_id
       WHERE c.status = 'abandoned'
         AND c.updated_at BETWEEN NOW() - INTERVAL '${interval.hours + 1} hours'
                             AND NOW() - INTERVAL '${interval.hours} hours'
         AND NOT EXISTS (
           SELECT 1 FROM notifications
           WHERE user_id = c.user_id
             AND template_name = '${interval.template}'
             AND created_at > c.updated_at
         )`
    );

    for (const cart of carts.rows) {
      await fetch('http://localhost:3102/api/notifications/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          user_id: cart.user_id,
          channel: 'email',
          template: interval.template,
          to: { email: cart.email },
          variables: {
            user_name: cart.name,
            cart_items: cart.items,
            cart_total: cart.total,
            cart_url: `https://example.com/cart/${cart.id}`,
            discount_code: interval.hours === 168 ? 'COMEBACK10' : null
          }
        })
      });
    }
  }
}
```

**Key Features:**
- Time-based triggers
- Deduplication (don't send twice)
- Progressive incentives (discount on final reminder)
- Tracking conversion rates

### 6. Social Notifications (New Followers)

**Scenario:** Social platform notifies users of new followers, batched hourly.

**Implementation:**

```typescript
// Cron job: 0 * * * * (Every hour)
async function sendFollowerNotifications(): Promise<void> {
  const users = await db.query(
    `SELECT
       u.id,
       u.email,
       u.name,
       COUNT(f.id) as new_followers
     FROM users u
     JOIN followers f ON f.following_id = u.id
     WHERE f.created_at > NOW() - INTERVAL '1 hour'
     GROUP BY u.id, u.email, u.name
     HAVING COUNT(f.id) > 0`
  );

  for (const user of users.rows) {
    // Get follower details
    const followers = await db.query(
      `SELECT u.name, u.username, u.avatar_url
       FROM followers f
       JOIN users u ON u.id = f.follower_id
       WHERE f.following_id = $1
         AND f.created_at > NOW() - INTERVAL '1 hour'
       ORDER BY f.created_at DESC
       LIMIT 5`,
      [user.id]
    );

    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: user.id,
        channel: 'push',  // Push notification for immediacy
        template: 'new_followers',
        to: { device_token: user.device_token },
        variables: {
          user_name: user.name,
          follower_count: user.new_followers,
          follower_names: followers.rows.map(f => f.name).join(', '),
          profile_url: `https://example.com/@${user.username}/followers`
        }
      })
    });
  }
}
```

**Key Features:**
- Batched hourly (not per-follower spam)
- Push notifications for mobile
- Collapsed format for multiple followers
- Deep linking to follower list

### 7. Billing Notifications (Payment Failed)

**Scenario:** SaaS platform notifies users of failed payment and retries.

**Implementation:**

```typescript
async function sendPaymentFailedNotification(subscriptionId: string): Promise<void> {
  const sub = await db.query(
    `SELECT s.*, u.email, u.name
     FROM subscriptions s
     JOIN users u ON u.id = s.user_id
     WHERE s.id = $1`,
    [subscriptionId]
  );

  const subscription = sub.rows[0];

  // Escalating notification sequence
  const attempts = subscription.retry_attempt;
  const channels = attempts === 1 ? ['email'] : ['email', 'push'];

  for (const channel of channels) {
    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: subscription.user_id,
        channel: channel,
        template: attempts === 4 ? 'payment_final_warning' : 'payment_failed',
        to: {
          email: subscription.email,
          device_token: subscription.device_token
        }[channel],
        variables: {
          user_name: subscription.name,
          plan_name: subscription.plan_name,
          amount: subscription.amount,
          retry_attempt: attempts,
          next_retry_date: subscription.next_retry_at,
          update_payment_url: `https://example.com/billing/update?sub=${subscriptionId}`,
          days_until_cancellation: 7 - attempts
        }
      })
    });
  }
}
```

**Key Features:**
- Escalating urgency (email → email+push)
- Clear call-to-action
- Grace period communication
- Dunning management integration

### 8. Onboarding Drip Campaign

**Scenario:** New users receive a series of emails over 14 days to guide them through features.

**Implementation:**

```typescript
// Triggered on user signup
async function startOnboardingCampaign(userId: string): Promise<void> {
  const drip = [
    { day: 0, template: 'welcome_email', subject: 'Welcome to Example!' },
    { day: 1, template: 'onboarding_step1', subject: 'Get started with your first project' },
    { day: 3, template: 'onboarding_step2', subject: 'Invite your team' },
    { day: 7, template: 'onboarding_step3', subject: 'Advanced features you\'ll love' },
    { day: 14, template: 'onboarding_complete', subject: 'You\'re all set!' }
  ];

  const user = await db.query('SELECT * FROM users WHERE id = $1', [userId]);

  for (const step of drip) {
    const sendAt = new Date(Date.now() + step.day * 24 * 60 * 60 * 1000);

    // Schedule for future delivery
    await db.execute(
      `INSERT INTO notification_queue (id, notification_id, next_retry_at)
       VALUES (
         gen_random_uuid(),
         (
           INSERT INTO notifications (id, user_id, channel, template_name, to_address, status)
           VALUES (gen_random_uuid(), $1, 'email', $2, $3, 'queued')
           RETURNING id
         ),
         $4
       )`,
      [userId, step.template, user.rows[0].email, sendAt]
    );
  }
}
```

**Key Features:**
- Scheduled sequence over time
- Progressive feature education
- Can be paused/resumed
- Tracks engagement to skip completed steps

### 9. Event-Driven Notifications (Comment Replies)

**Scenario:** Forum platform notifies users when someone replies to their comment.

**Implementation:**

```typescript
// Triggered by comment webhook
async function sendCommentReply(commentId: string): Promise<void> {
  const comment = await db.query(
    `SELECT
       c.id,
       c.content,
       c.parent_id,
       u1.id as author_id,
       u1.name as author_name,
       u2.id as parent_author_id,
       u2.email as parent_author_email,
       u2.name as parent_author_name,
       p.title as post_title
     FROM comments c
     JOIN users u1 ON u1.id = c.user_id
     JOIN comments pc ON pc.id = c.parent_id
     JOIN users u2 ON u2.id = pc.user_id
     JOIN posts p ON p.id = c.post_id
     WHERE c.id = $1`,
    [commentId]
  );

  const data = comment.rows[0];

  // Don't notify if replying to own comment
  if (data.author_id === data.parent_author_id) {
    return;
  }

  // Check user preferences for reply notifications
  const prefs = await db.query(
    `SELECT frequency FROM notification_preferences
     WHERE user_id = $1 AND channel = 'email' AND category = 'comments'`,
    [data.parent_author_id]
  );

  const frequency = prefs.rows[0]?.frequency || 'immediate';

  if (frequency === 'disabled') {
    return;
  }

  // Immediate or batched?
  if (frequency === 'immediate') {
    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: data.parent_author_id,
        channel: 'email',
        template: 'comment_reply',
        to: { email: data.parent_author_email },
        variables: {
          recipient_name: data.parent_author_name,
          author_name: data.author_name,
          post_title: data.post_title,
          comment_preview: data.content.substring(0, 100),
          comment_url: `https://example.com/posts/${data.post_id}#comment-${commentId}`
        }
      })
    });
  } else {
    // Add to daily digest
    await db.execute(
      `INSERT INTO notification_batches (id, user_id, batch_type, data)
       VALUES (gen_random_uuid(), $1, 'daily_comments', $2)`,
      [data.parent_author_id, JSON.stringify(data)]
    );
  }
}
```

**Key Features:**
- Real-time or batched based on preference
- Prevents self-notification
- Deep linking to comment
- Aggregates in digest mode

### 10. Location-Based Notifications (Nearby Events)

**Scenario:** Event app sends push notification when user is near an event they might like.

**Implementation:**

```typescript
// Triggered by geofence or periodic location check
async function sendNearbyEventNotification(userId: string, location: { lat: number, lon: number }): Promise<void> {
  // Find events within 5 miles
  const events = await db.query(
    `SELECT
       e.*,
       earth_distance(
         ll_to_earth(e.latitude, e.longitude),
         ll_to_earth($2, $3)
       ) / 1609.34 as distance_miles
     FROM events e
     WHERE e.start_time > NOW()
       AND e.start_time < NOW() + INTERVAL '7 days'
       AND earth_distance(
         ll_to_earth(e.latitude, e.longitude),
         ll_to_earth($2, $3)
       ) < 8046.72  -- 5 miles in meters
     ORDER BY distance_miles ASC
     LIMIT 3`,
    [userId, location.lat, location.lon]
  );

  if (events.rows.length === 0) {
    return;
  }

  const user = await db.query('SELECT * FROM users WHERE id = $1', [userId]);

  await fetch('http://localhost:3102/api/notifications/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: userId,
      channel: 'push',
      template: 'nearby_events',
      to: { device_token: user.rows[0].device_token },
      variables: {
        user_name: user.rows[0].name,
        event_count: events.rows.length,
        top_event_name: events.rows[0].name,
        top_event_distance: Math.round(events.rows[0].distance_miles * 10) / 10,
        events_url: `https://example.com/events/nearby?lat=${location.lat}&lon=${location.lon}`
      }
    })
  });

  // Rate limit: only send once per day
  await db.execute(
    `INSERT INTO notification_rate_limits (user_id, notification_type, last_sent_at)
     VALUES ($1, 'nearby_events', NOW())
     ON CONFLICT (user_id, notification_type) DO UPDATE SET last_sent_at = NOW()`,
    [userId]
  );
}
```

**Key Features:**
- Geolocation-based triggering
- Push notifications for immediacy
- Rate limiting (don't spam)
- Distance calculation and display

### 11. Weather Alerts (Severe Weather)

**Scenario:** Weather app sends urgent push notifications for severe weather in user's location.

**Implementation:**

```typescript
async function sendWeatherAlert(alertData: any): Promise<void> {
  // Find users in affected area
  const users = await db.query(
    `SELECT u.id, u.device_token, u.name, u.location
     FROM users u
     WHERE ST_Contains(
       ST_GeomFromGeoJSON($1),
       ST_SetSRID(ST_MakePoint(
         (u.location->>'lon')::float,
         (u.location->>'lat')::float
       ), 4326)
     )`,
    [JSON.stringify(alertData.affected_area)]
  );

  for (const user of users.rows) {
    await fetch('http://localhost:3102/api/notifications/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: user.id,
        channel: 'push',
        template: 'weather_alert',
        to: { device_token: user.device_token },
        variables: {
          user_name: user.name,
          alert_type: alertData.type,  // tornado, flood, etc.
          severity: alertData.severity,
          alert_message: alertData.message,
          expires_at: alertData.expires_at,
          safety_url: 'https://example.com/safety'
        }
      })
    });
  }
}
```

**Key Features:**
- Geospatial queries
- Critical priority (bypasses all preferences)
- Multi-channel (push + SMS for severe)
- Government integration (NOAA, etc.)

### 12. Compliance Notifications (GDPR Data Export)

**Scenario:** App notifies user when their GDPR data export is ready.

**Implementation:**

```typescript
async function sendDataExportReady(userId: string, exportId: string): Promise<void> {
  const user = await db.query('SELECT * FROM users WHERE id = $1', [userId]);
  const exportData = await db.query('SELECT * FROM data_exports WHERE id = $1', [exportId]);

  await fetch('http://localhost:3102/api/notifications/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: userId,
      channel: 'email',
      template: 'data_export_ready',
      to: { email: user.rows[0].email },
      variables: {
        user_name: user.rows[0].name,
        export_size: formatBytes(exportData.rows[0].file_size),
        download_url: `https://example.com/exports/${exportId}/download?token=${exportData.rows[0].download_token}`,
        expires_at: exportData.rows[0].expires_at,
        privacy_url: 'https://example.com/privacy'
      }
    })
  });

  // Log for compliance audit trail
  await db.execute(
    `INSERT INTO compliance_logs (user_id, action, details)
     VALUES ($1, 'data_export_notification_sent', $2)`,
    [userId, JSON.stringify({ export_id: exportId, sent_at: new Date() })]
  );
}
```

**Key Features:**
- Compliance tracking
- Secure download links
- Expiration handling
- Audit trail

---

## Troubleshooting

### Common Issues

#### "Notifications Not Sending"

**Solutions:**
1. Check worker is running: `ps aux | grep notifications`
2. Check queue status: `nself plugin notifications stats overview`
3. Check provider status: `nself plugin notifications test providers`
4. View logs: `tail -f ~/.nself/logs/plugins/notifications/worker.log`

#### "High Failure Rate"

**Solutions:**
1. Check provider health: `SELECT * FROM notification_provider_health;`
2. Review recent failures: `nself plugin notifications stats failures 50`
3. Test provider directly: `nself plugin notifications test email test@example.com`

#### "Queue Backlog Growing"

**Solutions:**
1. Increase worker concurrency: `WORKER_CONCURRENCY=20 nself plugin notifications worker`
2. Check database connection pool for bottlenecks
3. Monitor provider rate limits

#### "Database Connection Failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` format
3. Test connection: `psql $DATABASE_URL -c "SELECT 1"`

### Rate Limits

Default per-user per-channel limits:

| Channel | Default Limit |
|---------|---------------|
| Email | 100 per hour |
| Push | 200 per hour |
| SMS | 20 per hour |

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug nself plugin notifications server
```

### Health Checks

```bash
# Check server health
curl http://localhost:3102/health

# Check delivery stats
curl http://localhost:3102/api/stats/delivery
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)

---

*Last Updated: January 2026*
*Plugin Version: 1.0.0*
