# Configuration Guide

**Complete configuration reference for all nself plugins**

---

## Table of Contents

1. [Overview](#overview)
2. [Configuration File Locations](#configuration-file-locations)
3. [Environment Variables Reference](#environment-variables-reference)
4. [Database Configuration](#database-configuration)
5. [Multi-Environment Setup](#multi-environment-setup)
6. [Security Configuration](#security-configuration)
7. [Port Configuration](#port-configuration)
8. [Sync Intervals](#sync-intervals)
9. [Webhook Configuration](#webhook-configuration)
10. [Advanced Configuration](#advanced-configuration)
11. [Configuration Validation](#configuration-validation)
12. [Troubleshooting](#troubleshooting)

---

## Overview

All nself plugins are configured via environment variables, typically stored in `.env` files. This guide provides a complete reference for all configuration options across all 8 plugins.

### Configuration Principles

- **Environment Variables First**: All configuration uses environment variables
- **Sensible Defaults**: Most values have reasonable defaults for development
- **Production Requirements**: Some values (API keys, webhook secrets) are required in production
- **Overridable**: Plugin-specific variables can override global defaults
- **Type Safety**: Configuration is validated at startup with clear error messages

---

## Configuration File Locations

### Where to Place .env Files

Each plugin looks for environment variables in this order:

1. **Shell environment** - Exported variables take highest priority
2. **Local .env file** - `plugins/<name>/ts/.env` (plugin-specific)
3. **Root .env file** - `.env` in repository root (shared across all plugins)
4. **Default values** - Hardcoded defaults in `config.ts`

### Recommended Structure

```bash
# Development
plugins/stripe/ts/.env           # Stripe-specific configuration
plugins/github/ts/.env           # GitHub-specific configuration
plugins/shopify/ts/.env          # Shopify-specific configuration

# Production (use secrets management)
# Docker: Pass via -e flags or --env-file
# Kubernetes: Use ConfigMaps and Secrets
# Cloud: Use provider's secret manager (AWS Secrets Manager, etc.)
```

### .gitignore Protection

All `.env` files are automatically ignored by git. **Never commit credentials to version control.**

```gitignore
# Already in .gitignore
.env
.env.local
.env.*.local
**/.env
**/.env.local
.secrets/
```

---

## Environment Variables Reference

### Core Plugins (Stripe, GitHub, Shopify)

#### Stripe Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **API Configuration** | | | |
| `STRIPE_API_KEY` | Yes | - | Stripe API key (sk_test_* or sk_live_*) |
| `STRIPE_API_VERSION` | No | `2024-12-18.acacia` | Stripe API version |
| `STRIPE_WEBHOOK_SECRET` | Production | - | Webhook signing secret (whsec_*) |
| **Server** | | | |
| `STRIPE_PLUGIN_PORT` | No | `3001` | HTTP server port |
| `STRIPE_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `PORT` | No | `3001` | Fallback port (if STRIPE_PLUGIN_PORT not set) |
| `HOST` | No | `0.0.0.0` | Fallback host (if STRIPE_PLUGIN_HOST not set) |
| **Sync** | | | |
| `STRIPE_SYNC_INTERVAL` | No | `3600` | Auto-sync interval in seconds (0 to disable) |
| **Security** | | | |
| `STRIPE_API_KEY` | No | - | API key for REST endpoint authentication |
| `STRIPE_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `STRIPE_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in ms (1 minute) |
| `NSELF_API_KEY` | No | - | Global fallback API key |
| `RATE_LIMIT_MAX` | No | `100` | Global fallback rate limit |
| `RATE_LIMIT_WINDOW_MS` | No | `60000` | Global fallback window |

#### GitHub Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **API Configuration** | | | |
| `GITHUB_TOKEN` | Yes | - | GitHub personal access token (ghp_* or github_pat_*) |
| `GITHUB_ORG` | No | - | GitHub organization to sync (optional) |
| `GITHUB_REPOS` | No | - | Comma-separated list of repos (owner/repo) |
| `GITHUB_WEBHOOK_SECRET` | Production | - | Webhook secret for signature verification |
| **Server** | | | |
| `GITHUB_PLUGIN_PORT` | No | `3002` | HTTP server port |
| `GITHUB_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `PORT` | No | `3002` | Fallback port |
| `HOST` | No | `0.0.0.0` | Fallback host |
| **Sync** | | | |
| `GITHUB_SYNC_INTERVAL` | No | `3600` | Auto-sync interval in seconds |
| **Security** | | | |
| `GITHUB_API_KEY` | No | - | API key for REST endpoint authentication |
| `GITHUB_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `GITHUB_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in ms |

#### Shopify Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **API Configuration** | | | |
| `SHOPIFY_SHOP_DOMAIN` | Yes | - | Shop domain (myshop.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Admin API access token (shpat_*) |
| `SHOPIFY_API_VERSION` | No | `2024-01` | Shopify API version |
| `SHOPIFY_WEBHOOK_SECRET` | Production | - | Webhook HMAC secret |
| **Server** | | | |
| `PORT` | No | `3003` | HTTP server port |
| `HOST` | No | `0.0.0.0` | HTTP server bind address |
| **Sync** | | | |
| `SYNC_BATCH_SIZE` | No | `250` | Records per batch during sync |
| **Security** | | | |
| `SHOPIFY_API_KEY` | No | - | API key for REST endpoint authentication |
| `SHOPIFY_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `SHOPIFY_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in ms |

### Infrastructure Plugins

#### ID.me Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **OAuth Configuration** | | | |
| `IDME_CLIENT_ID` | Yes | - | ID.me OAuth client ID |
| `IDME_CLIENT_SECRET` | Yes | - | ID.me OAuth client secret |
| `IDME_REDIRECT_URI` | Yes | - | OAuth redirect URI |
| `IDME_SCOPES` | No | `openid,email,profile` | Comma-separated OAuth scopes |
| `IDME_SANDBOX` | No | `false` | Use sandbox environment |
| `IDME_WEBHOOK_SECRET` | No | - | Webhook verification secret |
| **Server** | | | |
| `PORT` | No | `3010` | HTTP server port |
| `HOST` | No | `0.0.0.0` | HTTP server bind address |

#### File Processing Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Storage Configuration** | | | |
| `FILE_STORAGE_PROVIDER` | No | `minio` | Storage provider (minio, s3, gcs, r2, b2, azure) |
| `FILE_STORAGE_BUCKET` | Yes | - | Storage bucket name |
| `FILE_STORAGE_ENDPOINT` | Conditional | - | Storage endpoint (required for MinIO, R2, B2) |
| `FILE_STORAGE_REGION` | No | `us-east-1` | Storage region |
| `FILE_STORAGE_ACCESS_KEY` | Conditional | - | Access key (required for S3, MinIO, R2, B2) |
| `FILE_STORAGE_SECRET_KEY` | Conditional | - | Secret key (required for S3, MinIO, R2, B2) |
| `AZURE_STORAGE_CONNECTION_STRING` | Conditional | - | Azure connection string (required for Azure) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Conditional | - | GCS credentials path (required for GCS) |
| **Processing** | | | |
| `FILE_THUMBNAIL_SIZES` | No | `100,400,1200` | Comma-separated thumbnail widths in pixels |
| `FILE_ENABLE_VIRUS_SCAN` | No | `false` | Enable ClamAV virus scanning |
| `FILE_ENABLE_OPTIMIZATION` | No | `true` | Enable image optimization |
| `FILE_MAX_SIZE` | No | `104857600` | Max file size in bytes (100MB default) |
| `FILE_ALLOWED_TYPES` | No | - | Comma-separated allowed MIME types (empty = all) |
| `FILE_STRIP_EXIF` | No | `true` | Strip EXIF data from images |
| `FILE_QUEUE_CONCURRENCY` | No | `3` | Concurrent processing jobs |
| **ClamAV** | | | |
| `CLAMAV_HOST` | No | `localhost` | ClamAV daemon host |
| `CLAMAV_PORT` | No | `3310` | ClamAV daemon port |
| **Queue** | | | |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis URL for job queue |
| **Server** | | | |
| `PORT` | No | `3104` | HTTP server port |
| `HOST` | No | `0.0.0.0` | HTTP server bind address |

#### Realtime Plugin (WebSockets)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Server** | | | |
| `REALTIME_PORT` | No | `3101` | WebSocket server port |
| `REALTIME_HOST` | No | `0.0.0.0` | Server bind address |
| `REALTIME_REDIS_URL` | Yes | - | Redis URL for pub/sub |
| `REALTIME_CORS_ORIGIN` | Yes | - | Comma-separated allowed origins |
| **Limits** | | | |
| `REALTIME_MAX_CONNECTIONS` | No | `10000` | Max concurrent WebSocket connections |
| `REALTIME_PING_TIMEOUT` | No | `60000` | Ping timeout in ms |
| `REALTIME_PING_INTERVAL` | No | `25000` | Ping interval in ms |
| **Authentication** | | | |
| `REALTIME_JWT_SECRET` | No | - | JWT secret for authentication (optional) |
| `REALTIME_ALLOW_ANONYMOUS` | No | `false` | Allow anonymous connections |
| **Features** | | | |
| `REALTIME_ENABLE_PRESENCE` | No | `true` | Enable presence tracking |
| `REALTIME_ENABLE_TYPING` | No | `true` | Enable typing indicators |
| `REALTIME_TYPING_TIMEOUT` | No | `3000` | Typing indicator timeout in ms |
| `REALTIME_PRESENCE_HEARTBEAT` | No | `30000` | Presence heartbeat interval in ms |
| **Performance** | | | |
| `REALTIME_ENABLE_COMPRESSION` | No | `true` | Enable WebSocket compression |
| `REALTIME_BATCH_SIZE` | No | `100` | Message batch size |
| `REALTIME_RATE_LIMIT` | No | `100` | Messages per second per connection |
| **Logging** | | | |
| `REALTIME_LOG_EVENTS` | No | `true` | Log connection events |
| `REALTIME_LOG_EVENT_TYPES` | No | `connect,disconnect,error` | Comma-separated event types to log |
| **Monitoring** | | | |
| `REALTIME_ENABLE_METRICS` | No | `true` | Enable Prometheus metrics |
| `REALTIME_METRICS_PATH` | No | `/metrics` | Metrics endpoint path |
| `REALTIME_ENABLE_HEALTH` | No | `true` | Enable health check endpoint |
| `REALTIME_HEALTH_PATH` | No | `/health` | Health check path |

#### Notifications Plugin

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Email Configuration** | | | |
| `NOTIFICATIONS_EMAIL_ENABLED` | No | `false` | Enable email notifications |
| `NOTIFICATIONS_EMAIL_PROVIDER` | No | `resend` | Email provider (resend, sendgrid, ses, postmark, smtp) |
| `NOTIFICATIONS_EMAIL_API_KEY` | Conditional | - | Provider API key |
| `NOTIFICATIONS_EMAIL_FROM` | No | `noreply@example.com` | Default sender address |
| `NOTIFICATIONS_EMAIL_DOMAIN` | No | - | Verified domain |
| `SMTP_HOST` | Conditional | - | SMTP host (required for smtp provider) |
| `SMTP_PORT` | No | `587` | SMTP port |
| `SMTP_SECURE` | No | `false` | Use TLS |
| `SMTP_USER` | Conditional | - | SMTP username |
| `SMTP_PASS` | Conditional | - | SMTP password |
| **Push Notifications** | | | |
| `NOTIFICATIONS_PUSH_ENABLED` | No | `false` | Enable push notifications |
| `NOTIFICATIONS_PUSH_PROVIDER` | No | - | Push provider (onesignal, fcm, apns, expo, webpush) |
| `NOTIFICATIONS_PUSH_API_KEY` | Conditional | - | Provider API key |
| `NOTIFICATIONS_PUSH_APP_ID` | Conditional | - | App ID |
| `NOTIFICATIONS_PUSH_PROJECT_ID` | Conditional | - | Project ID (FCM) |
| `NOTIFICATIONS_PUSH_VAPID_PUBLIC_KEY` | Conditional | - | VAPID public key (Web Push) |
| `NOTIFICATIONS_PUSH_VAPID_PRIVATE_KEY` | Conditional | - | VAPID private key (Web Push) |
| `NOTIFICATIONS_PUSH_VAPID_SUBJECT` | Conditional | - | VAPID subject (Web Push) |
| **SMS Configuration** | | | |
| `NOTIFICATIONS_SMS_ENABLED` | No | `false` | Enable SMS notifications |
| `NOTIFICATIONS_SMS_PROVIDER` | No | - | SMS provider (twilio, plivo, sns) |
| `NOTIFICATIONS_SMS_ACCOUNT_SID` | Conditional | - | Account SID (Twilio/Plivo) |
| `NOTIFICATIONS_SMS_AUTH_TOKEN` | Conditional | - | Auth token (Twilio) |
| `NOTIFICATIONS_SMS_AUTH_ID` | Conditional | - | Auth ID (Plivo) |
| `NOTIFICATIONS_SMS_FROM` | Conditional | - | Sender phone number |
| **Queue** | | | |
| `NOTIFICATIONS_QUEUE_BACKEND` | No | `redis` | Queue backend (redis, postgres) |
| `REDIS_URL` | Conditional | `redis://localhost:6379` | Redis URL (if queue backend is redis) |
| **Worker** | | | |
| `WORKER_CONCURRENCY` | No | `5` | Concurrent notification workers |
| `WORKER_POLL_INTERVAL` | No | `1000` | Worker poll interval in ms |
| **Retry** | | | |
| `NOTIFICATIONS_RETRY_ATTEMPTS` | No | `3` | Max retry attempts |
| `NOTIFICATIONS_RETRY_DELAY` | No | `1000` | Initial retry delay in ms |
| `NOTIFICATIONS_MAX_RETRY_DELAY` | No | `300000` | Max retry delay in ms (5 minutes) |
| **Rate Limits** | | | |
| `NOTIFICATIONS_RATE_LIMIT_EMAIL` | No | `100` | Email per user per hour |
| `NOTIFICATIONS_RATE_LIMIT_PUSH` | No | `200` | Push per user per hour |
| `NOTIFICATIONS_RATE_LIMIT_SMS` | No | `20` | SMS per user per hour |
| **Batch** | | | |
| `NOTIFICATIONS_BATCH_ENABLED` | No | `false` | Enable batch notifications |
| `NOTIFICATIONS_BATCH_INTERVAL` | No | `86400` | Batch interval in seconds (24 hours) |
| **Server** | | | |
| `PORT` | No | `3102` | HTTP server port |
| `HOST` | No | `0.0.0.0` | HTTP server bind address |
| **Features** | | | |
| `NOTIFICATIONS_TRACKING_ENABLED` | No | `true` | Track opens/clicks |
| `NOTIFICATIONS_QUIET_HOURS_ENABLED` | No | `true` | Respect quiet hours |
| **Security** | | | |
| `NOTIFICATIONS_ENCRYPT_CONFIG` | No | `false` | Encrypt stored configuration |
| `NOTIFICATIONS_ENCRYPTION_KEY` | Conditional | - | Encryption key (required if encrypt_config is true) |
| `NOTIFICATIONS_WEBHOOK_SECRET` | No | - | Webhook verification secret |
| `NOTIFICATIONS_WEBHOOK_VERIFY` | No | `true` | Verify webhook signatures |
| **Development** | | | |
| `NOTIFICATIONS_DRY_RUN` | No | `false` | Dry run mode (don't send) |
| `NOTIFICATIONS_TEST_MODE` | No | `false` | Test mode |

#### Jobs Plugin (Background Tasks)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Redis** | | | |
| `JOBS_REDIS_URL` | No | `redis://localhost:6379` | Redis URL for BullMQ |
| **Dashboard** | | | |
| `JOBS_DASHBOARD_ENABLED` | No | `true` | Enable Bull Board dashboard |
| `JOBS_DASHBOARD_PORT` | No | `3105` | Dashboard port |
| `JOBS_DASHBOARD_PATH` | No | `/dashboard` | Dashboard path |
| **Worker** | | | |
| `JOBS_DEFAULT_CONCURRENCY` | No | `5` | Default worker concurrency |
| `JOBS_RETRY_ATTEMPTS` | No | `3` | Default retry attempts |
| `JOBS_RETRY_DELAY` | No | `5000` | Retry delay in ms |
| `JOBS_JOB_TIMEOUT` | No | `60000` | Job timeout in ms (1 minute) |
| **Monitoring** | | | |
| `JOBS_ENABLE_TELEMETRY` | No | `true` | Enable telemetry/metrics |
| **Cleanup** | | | |
| `JOBS_CLEAN_COMPLETED_AFTER` | No | `86400000` | Clean completed jobs after ms (24 hours) |
| `JOBS_CLEAN_FAILED_AFTER` | No | `604800000` | Clean failed jobs after ms (7 days) |

### Global Configuration

These variables apply to ALL plugins:

| Variable | Default | Description |
|----------|---------|-------------|
| **Database** | | |
| `POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_DB` | `nself` | Database name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | - | Database password |
| `POSTGRES_SSL` | `false` | Enable SSL connection |
| **Logging** | | |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `NODE_ENV` | `development` | Environment (development, production, test) |
| **Security** | | |
| `NSELF_API_KEY` | - | Global API key for all plugins |
| `RATE_LIMIT_MAX` | `100` | Global rate limit max requests |
| `RATE_LIMIT_WINDOW_MS` | `60000` | Global rate limit window |

---

## Database Configuration

All plugins connect to PostgreSQL using the same set of environment variables.

### Basic Configuration

```bash
# Development
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password_here
POSTGRES_SSL=false
```

### Production Configuration

```bash
# Production with SSL
POSTGRES_HOST=db.production.example.com
POSTGRES_PORT=5432
POSTGRES_DB=nself_production
POSTGRES_USER=nself_app
POSTGRES_PASSWORD=strong_random_password_here
POSTGRES_SSL=true
```

### Docker Compose Configuration

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: nself
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  stripe-plugin:
    build: ./plugins/stripe/ts
    environment:
      POSTGRES_HOST: postgres
      POSTGRES_PORT: 5432
      POSTGRES_DB: nself
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
```

### Connection URL Format

Some tools accept connection URLs instead of individual variables:

```bash
# Format
DATABASE_URL=postgresql://user:password@host:port/database?sslmode=require

# Example
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
```

### Connection Pooling

Plugins use connection pooling automatically. Default pool size is 10 connections per plugin.

To customize (add to plugin config.ts if needed):

```typescript
{
  databaseMaxConnections: parseInt(process.env.POSTGRES_MAX_CONNECTIONS ?? '10', 10)
}
```

---

## Multi-Environment Setup

### Development Environment

Create `plugins/<name>/ts/.env.development`:

```bash
# Development mode
NODE_ENV=development
LOG_LEVEL=debug

# Local database
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself_dev
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_SSL=false

# Stripe Test Mode
STRIPE_API_KEY=sk_test_your_test_key_here
STRIPE_WEBHOOK_SECRET=whsec_test_secret_here
STRIPE_SYNC_INTERVAL=300  # Sync every 5 minutes for dev

# GitHub
GITHUB_TOKEN=ghp_your_personal_access_token
GITHUB_ORG=your-org

# No API authentication in dev
# STRIPE_API_KEY not set = no auth required
```

### Staging Environment

Create `plugins/<name>/ts/.env.staging`:

```bash
# Staging
NODE_ENV=staging
LOG_LEVEL=info

# Staging database
POSTGRES_HOST=staging-db.example.com
POSTGRES_PORT=5432
POSTGRES_DB=nself_staging
POSTGRES_USER=nself_staging
POSTGRES_PASSWORD=staging_password
POSTGRES_SSL=true

# Stripe Test Mode (use test keys in staging)
STRIPE_API_KEY=sk_test_your_test_key_here
STRIPE_WEBHOOK_SECRET=whsec_staging_secret_here
STRIPE_SYNC_INTERVAL=1800

# API authentication enabled
STRIPE_API_KEY=staging_api_key_here
RATE_LIMIT_MAX=200
```

### Production Environment

**DO NOT use .env files in production.** Use your platform's secret management:

#### Docker Swarm / Kubernetes

```yaml
# Kubernetes Secret
apiVersion: v1
kind: Secret
metadata:
  name: stripe-plugin-secrets
type: Opaque
stringData:
  POSTGRES_PASSWORD: production_password
  STRIPE_API_KEY: sk_live_your_live_key
  STRIPE_WEBHOOK_SECRET: whsec_production_secret
  STRIPE_API_KEY: production_api_key

---
# Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stripe-plugin
spec:
  template:
    spec:
      containers:
      - name: stripe-plugin
        envFrom:
        - secretRef:
            name: stripe-plugin-secrets
        env:
        - name: NODE_ENV
          value: "production"
        - name: POSTGRES_HOST
          value: "postgres-service"
```

#### AWS ECS

```json
{
  "containerDefinitions": [
    {
      "name": "stripe-plugin",
      "environment": [
        { "name": "NODE_ENV", "value": "production" },
        { "name": "POSTGRES_HOST", "value": "db.example.com" }
      ],
      "secrets": [
        {
          "name": "STRIPE_API_KEY",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:123456789:secret:stripe-api-key"
        },
        {
          "name": "POSTGRES_PASSWORD",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:123456789:secret:postgres-password"
        }
      ]
    }
  ]
}
```

### Environment Switching

Use shell scripts to switch environments:

```bash
#!/bin/bash
# scripts/use-dev.sh
export NODE_ENV=development
source .env.development
npm run dev

# scripts/use-staging.sh
export NODE_ENV=staging
source .env.staging
npm run build && npm start
```

---

## Security Configuration

### API Key Authentication

Protect REST API endpoints with API key authentication:

```bash
# Enable authentication
STRIPE_API_KEY=your_secret_api_key_here

# Clients must include header:
# Authorization: Bearer your_secret_api_key_here
# OR
# X-API-Key: your_secret_api_key_here
```

**Generate secure API keys:**

```bash
# Generate random 32-byte key
openssl rand -base64 32

# Example output
xK9vL2mP4nQ7wR8tY5sZ1aB6cD3eF0gH=
```

### Rate Limiting

Configure rate limits per plugin:

```bash
# Plugin-specific
STRIPE_RATE_LIMIT_MAX=100        # Max requests
STRIPE_RATE_LIMIT_WINDOW_MS=60000  # Per 1 minute

# Global fallback
RATE_LIMIT_MAX=100
RATE_LIMIT_WINDOW_MS=60000
```

### Webhook Secrets

Each service has its own webhook secret format:

```bash
# Stripe
STRIPE_WEBHOOK_SECRET=whsec_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd

# GitHub
GITHUB_WEBHOOK_SECRET=your_random_secret_here

# Shopify
SHOPIFY_WEBHOOK_SECRET=your_random_secret_here
```

**IMPORTANT**: Webhook secrets are **required in production** (NODE_ENV=production). Plugins will refuse to start without them.

### Security Best Practices

1. **Never commit secrets to git**
   - Use `.env` files locally (they're gitignored)
   - Use secret managers in production

2. **Use strong, random secrets**
   ```bash
   # Generate webhook secret
   openssl rand -hex 32
   ```

3. **Rotate credentials regularly**
   - Rotate API keys every 90 days
   - Rotate webhook secrets when compromised

4. **Use SSL in production**
   ```bash
   POSTGRES_SSL=true
   ```

5. **Limit API key permissions**
   - Use least-privilege access
   - Create separate keys for dev/staging/prod

6. **Enable rate limiting**
   - Prevents abuse
   - Recommended: 100-200 requests per minute

---

## Port Configuration

### Default Ports

Each plugin uses a unique default port to avoid conflicts:

| Plugin | Default Port | Override Variable |
|--------|--------------|-------------------|
| Stripe | 3001 | `STRIPE_PLUGIN_PORT` or `PORT` |
| GitHub | 3002 | `GITHUB_PLUGIN_PORT` or `PORT` |
| Shopify | 3003 | `SHOPIFY_PLUGIN_PORT` or `PORT` |
| Realtime | 3101 | `REALTIME_PORT` |
| Notifications | 3102 | `PORT` |
| File Processing | 3104 | `PORT` |
| Jobs Dashboard | 3105 | `JOBS_DASHBOARD_PORT` |
| ID.me | 3010 | `PORT` |

### Port Resolution Order

For Stripe, GitHub plugins:

1. `<PLUGIN>_PLUGIN_PORT` (highest priority)
2. `PORT` (fallback)
3. Default value (3001, 3002, etc.)

For other plugins:

1. `PORT`
2. Default value

### Running Multiple Plugins Simultaneously

Each plugin needs its own port:

```bash
# .env.stripe
STRIPE_PLUGIN_PORT=3001

# .env.github
GITHUB_PLUGIN_PORT=3002

# .env.shopify
PORT=3003
```

### Port Conflicts

If you get `EADDRINUSE` error:

```bash
# Find what's using the port
lsof -i :3001

# Kill the process
kill -9 <PID>

# Or use a different port
STRIPE_PLUGIN_PORT=3011 npm start
```

### Behind Reverse Proxy

When using nginx/Caddy/Traefik:

```bash
# Plugins listen on localhost only
STRIPE_PLUGIN_HOST=127.0.0.1
GITHUB_PLUGIN_HOST=127.0.0.1

# Proxy forwards to internal ports
# nginx.conf
upstream stripe {
    server 127.0.0.1:3001;
}

server {
    listen 80;
    server_name stripe.example.com;

    location / {
        proxy_pass http://stripe;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Sync Intervals

Configure how often plugins automatically sync data:

### Recommended Intervals

| Plugin | Recommended | Minimum | Maximum | Default |
|--------|-------------|---------|---------|---------|
| Stripe | 1800-3600s | 300s | 86400s | 3600s (1 hour) |
| GitHub | 1800-3600s | 300s | 86400s | 3600s (1 hour) |
| Shopify | 1800-3600s | 300s | 86400s | 3600s (1 hour) |

### Configuration

```bash
# Auto-sync every hour (recommended)
STRIPE_SYNC_INTERVAL=3600
GITHUB_SYNC_INTERVAL=3600

# More frequent for high-volume (every 30 minutes)
STRIPE_SYNC_INTERVAL=1800

# Less frequent for low-volume (every 6 hours)
STRIPE_SYNC_INTERVAL=21600

# Disable auto-sync (webhook-only)
STRIPE_SYNC_INTERVAL=0
```

### Sync Strategies

**Full Sync (default)**
- Fetches all records from service
- Updates local database
- Use for initial sync or recovery

**Incremental Sync**
- Only fetches changed records
- Uses `updated_at` or `since` parameter
- More efficient for regular syncs

**Webhook-Only**
- Set sync interval to 0
- Only updates via webhooks
- Most efficient, requires reliable webhooks

### Best Practices

1. **Start with webhooks + hourly sync**
   - Webhooks provide real-time updates
   - Hourly sync catches missed webhooks

2. **Increase interval after stable**
   - Once webhooks are reliable, reduce sync frequency
   - Save API quota and database load

3. **Monitor sync performance**
   - Check sync duration in logs
   - Adjust interval if syncs take too long

4. **Consider API rate limits**
   - Stripe: 100 req/sec (test), 25-100 req/sec (live)
   - GitHub: 5,000 req/hour (authenticated)
   - Shopify: 2 req/sec (REST), 1,000 points/sec (GraphQL)

---

## Webhook Configuration

### Webhook URLs

Each plugin exposes a webhook endpoint at `/webhook`:

| Plugin | URL Pattern | Default URL |
|--------|-------------|-------------|
| Stripe | `https://your-domain.com/webhook` | `http://localhost:3001/webhook` |
| GitHub | `https://your-domain.com/webhook` | `http://localhost:3002/webhook` |
| Shopify | `https://your-domain.com/webhook` | `http://localhost:3003/webhook` |

### Registering Webhooks

#### Stripe

1. Go to https://dashboard.stripe.com/webhooks
2. Click "Add endpoint"
3. Enter URL: `https://your-domain.com/webhook`
4. Select events to listen to (or choose "Select all events")
5. Copy webhook signing secret to `STRIPE_WEBHOOK_SECRET`

```bash
STRIPE_WEBHOOK_SECRET=whsec_1234567890abcdef...
```

#### GitHub

1. Go to repository/org settings > Webhooks
2. Click "Add webhook"
3. Payload URL: `https://your-domain.com/webhook`
4. Content type: `application/json`
5. Secret: Generate random secret
6. Select individual events or "Send me everything"

```bash
GITHUB_WEBHOOK_SECRET=your_random_secret_here
```

#### Shopify

1. Admin > Settings > Notifications > Webhooks
2. Create webhook subscription
3. URL: `https://your-domain.com/webhook`
4. Format: JSON
5. API version: Latest

```bash
SHOPIFY_WEBHOOK_SECRET=your_hmac_secret_here
```

### Webhook Security

All plugins verify webhook signatures:

**Stripe**: HMAC SHA-256 with timestamp
**GitHub**: HMAC SHA-256 with `sha256=` prefix
**Shopify**: HMAC SHA-256 with Base64 encoding

### Testing Webhooks Locally

Use ngrok or similar tunneling service:

```bash
# Install ngrok
brew install ngrok

# Start plugin
npm run dev

# Tunnel to local port
ngrok http 3001

# Use ngrok URL in webhook configuration
# https://abc123.ngrok.io/webhook
```

### Webhook Debugging

Enable debug logging:

```bash
LOG_LEVEL=debug npm run dev
```

Check webhook event table:

```sql
SELECT * FROM stripe_webhook_events
ORDER BY created_at DESC
LIMIT 10;
```

### Webhook Retry Logic

Plugins automatically:
- Return 200 OK immediately
- Process events asynchronously
- Store failed events for retry
- Log processing errors

---

## Advanced Configuration

### Connection Pooling

PostgreSQL connection pool configuration (add if needed):

```typescript
// In config.ts
{
  databaseMaxConnections: parseInt(process.env.POSTGRES_MAX_CONNECTIONS ?? '10', 10),
  databaseIdleTimeoutMs: parseInt(process.env.POSTGRES_IDLE_TIMEOUT ?? '30000', 10),
  databaseConnectionTimeoutMs: parseInt(process.env.POSTGRES_CONNECTION_TIMEOUT ?? '5000', 10),
}
```

### Timeouts

HTTP client and database timeouts:

```bash
# HTTP client timeout (milliseconds)
HTTP_TIMEOUT=30000

# Database query timeout (milliseconds)
POSTGRES_QUERY_TIMEOUT=10000
```

### Retry Configuration

Configure retry behavior for API calls:

```typescript
// Most plugins use exponential backoff
{
  maxRetries: parseInt(process.env.MAX_RETRIES ?? '3', 10),
  baseDelay: parseInt(process.env.RETRY_BASE_DELAY ?? '1000', 10),
  maxDelay: parseInt(process.env.RETRY_MAX_DELAY ?? '10000', 10),
}
```

### Custom Logging

Configure structured logging:

```bash
# Log format (json, pretty)
LOG_FORMAT=json

# Log destination (stdout, file)
LOG_DESTINATION=stdout

# Log file path (if destination is file)
LOG_FILE=/var/log/nself/stripe.log

# Disable colors in production
NO_COLOR=true
```

### Performance Tuning

```bash
# Increase Node.js memory limit
NODE_OPTIONS="--max-old-space-size=4096"

# Enable HTTP keep-alive
HTTP_KEEP_ALIVE=true
HTTP_KEEP_ALIVE_TIMEOUT=5000

# Batch size for bulk operations
SYNC_BATCH_SIZE=100

# Concurrent API requests
API_CONCURRENCY=5
```

### Health Checks

All plugins expose health check endpoints:

```bash
# Basic health check
curl http://localhost:3001/health

# Detailed health check
curl http://localhost:3001/api/status
```

Configure health check behavior:

```bash
# Include database check in health endpoint
HEALTH_CHECK_DB=true

# Health check timeout
HEALTH_CHECK_TIMEOUT=5000
```

---

## Configuration Validation

### Startup Validation

All plugins validate configuration at startup and exit with clear error messages:

```bash
# Example error
Error: Configuration invalid:
  - STRIPE_API_KEY is required
  - Invalid STRIPE_API_KEY format. Expected sk_test_* or sk_live_*
  - STRIPE_WEBHOOK_SECRET is required in production
```

### Manual Validation

Test configuration without starting the server:

```bash
# Dry run (will be implemented)
npm run config:validate

# Or check directly
node -e "require('./dist/config.js').loadConfig()"
```

### Type Validation

TypeScript ensures type safety:

```typescript
// Config interface validates types
interface Config {
  port: number;              // Must be number
  enabled: boolean;          // Must be boolean
  apiKey: string;           // Must be string
  tags?: string[];          // Optional array
}
```

### Environment-Specific Validation

Different requirements per environment:

| Config | Development | Staging | Production |
|--------|-------------|---------|------------|
| `WEBHOOK_SECRET` | Optional | Recommended | **Required** |
| `API_KEY` | Optional | Recommended | **Required** |
| `SSL` | Optional | Recommended | **Required** |
| `LOG_LEVEL` | `debug` | `info` | `warn` or `error` |

---

## Troubleshooting

### Common Configuration Errors

#### Missing Required Variables

```
Error: STRIPE_API_KEY is required
```

**Solution**: Add the required variable to your `.env` file

```bash
STRIPE_API_KEY=sk_test_your_key_here
```

#### Invalid Variable Format

```
Error: Invalid STRIPE_API_KEY format. Expected sk_test_* or sk_live_*
```

**Solution**: Check the variable matches expected format

```bash
# Wrong
STRIPE_API_KEY=pk_test_key

# Right
STRIPE_API_KEY=sk_test_key
```

#### Production Validation Failures

```
Error: STRIPE_WEBHOOK_SECRET is required in production
```

**Solution**: Either set the secret OR change environment to development

```bash
# Option 1: Set the secret
STRIPE_WEBHOOK_SECRET=whsec_...

# Option 2: Use development mode
NODE_ENV=development
```

#### Port Already in Use

```
Error: listen EADDRINUSE: address already in use :::3001
```

**Solution**: Use a different port or kill the existing process

```bash
# Use different port
STRIPE_PLUGIN_PORT=3011 npm start

# Or find and kill existing process
lsof -i :3001
kill -9 <PID>
```

### Database Connection Issues

#### Connection Refused

```
Error: connect ECONNREFUSED 127.0.0.1:5432
```

**Solution**: Ensure PostgreSQL is running

```bash
# Check PostgreSQL status
pg_isready -h localhost -p 5432

# Start PostgreSQL (macOS)
brew services start postgresql

# Start PostgreSQL (Linux)
sudo systemctl start postgresql
```

#### Authentication Failed

```
Error: password authentication failed for user "postgres"
```

**Solution**: Verify database credentials

```bash
# Test connection manually
psql -h localhost -U postgres -d nself

# Update .env with correct password
POSTGRES_PASSWORD=correct_password
```

#### SSL Required

```
Error: SSL required
```

**Solution**: Enable SSL or connect to correct host

```bash
# Enable SSL
POSTGRES_SSL=true

# Or connect to local database
POSTGRES_HOST=localhost
POSTGRES_SSL=false
```

### Configuration Loading Issues

#### .env File Not Loaded

If environment variables aren't loading:

1. Check `.env` file location (must be in plugin's `ts/` directory)
2. Verify file name is exactly `.env` (not `.env.txt`)
3. Ensure no trailing spaces in variable assignments
4. Check file permissions

```bash
# Verify .env exists
ls -la plugins/stripe/ts/.env

# Check permissions
chmod 600 plugins/stripe/ts/.env

# Validate syntax (no spaces around =)
# WRONG: STRIPE_API_KEY = sk_test_123
# RIGHT: STRIPE_API_KEY=sk_test_123
```

#### Variables Not Taking Effect

Check precedence order:

1. Shell exports override .env
2. Check for typos in variable names
3. Restart the process after changing .env

```bash
# Unset shell variable to use .env
unset STRIPE_API_KEY

# Restart process
npm run dev
```

### Validation Tools

#### Check All Variables

```bash
# Print all environment variables
env | grep -E '(POSTGRES|STRIPE|GITHUB|SHOPIFY|NOTIFICATIONS|REALTIME|JOBS|FILE)'

# Check specific plugin
env | grep STRIPE
```

#### Configuration Checklist

Before deploying:

- [ ] All required variables set
- [ ] API keys have correct format
- [ ] Webhook secrets configured (production)
- [ ] Database connection works
- [ ] Ports available/not conflicting
- [ ] SSL enabled for production database
- [ ] Rate limits appropriate
- [ ] Sync intervals reasonable
- [ ] Log level appropriate for environment

---

## Example Configurations

### Complete Development Setup

```bash
# Environment
NODE_ENV=development
LOG_LEVEL=debug

# Database
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself_dev
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_SSL=false

# Stripe
STRIPE_API_KEY=sk_test_51Abc...
STRIPE_API_VERSION=2024-12-18.acacia
STRIPE_WEBHOOK_SECRET=whsec_test...
STRIPE_PLUGIN_PORT=3001
STRIPE_SYNC_INTERVAL=300

# GitHub
GITHUB_TOKEN=ghp_your_token_here
GITHUB_ORG=your-org
GITHUB_REPOS=your-org/repo1,your-org/repo2
GITHUB_WEBHOOK_SECRET=dev_secret_123
GITHUB_PLUGIN_PORT=3002
GITHUB_SYNC_INTERVAL=300

# No API auth in dev (omit API_KEY variables)
```

### Complete Production Setup (Kubernetes)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nself-config
data:
  NODE_ENV: "production"
  LOG_LEVEL: "info"
  POSTGRES_HOST: "postgres-service"
  POSTGRES_PORT: "5432"
  POSTGRES_DB: "nself_production"
  POSTGRES_SSL: "true"
  STRIPE_PLUGIN_PORT: "3001"
  STRIPE_SYNC_INTERVAL: "3600"
  RATE_LIMIT_MAX: "200"

---
apiVersion: v1
kind: Secret
metadata:
  name: nself-secrets
type: Opaque
stringData:
  POSTGRES_PASSWORD: "production_db_password"
  STRIPE_API_KEY: "sk_live_your_live_key"
  STRIPE_WEBHOOK_SECRET: "whsec_production_secret"
  STRIPE_API_KEY: "production_api_key_for_endpoints"
```

---

## Additional Resources

- [Plugin Documentation](../plugins/)
- [Security Best Practices](../Security.md)
- [Development Guide](../DEVELOPMENT.md)
- [Troubleshooting](../troubleshooting/)

---

**Last Updated**: 2026-01-30
