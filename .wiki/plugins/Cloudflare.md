# Cloudflare

Cloudflare zone, DNS, R2, cache, and analytics management

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The Cloudflare plugin provides comprehensive management and monitoring capabilities for Cloudflare zones, DNS records, R2 object storage, CDN cache purging, and analytics. It syncs Cloudflare configuration and metrics to your local PostgreSQL database, enabling offline queries, historical analytics, and automation workflows.

This plugin is essential for applications that need to manage Cloudflare infrastructure programmatically, automate DNS management, monitor CDN performance, or integrate Cloudflare analytics into custom dashboards.

### Key Features

- **Zone Management** - Sync and manage Cloudflare zones with settings and status tracking
- **DNS Management** - Create, update, and delete DNS records (A, AAAA, CNAME, MX, TXT, etc.)
- **R2 Object Storage** - Manage R2 buckets with storage statistics and access control
- **Cache Purging** - Purge CDN cache by URL, tag, host, or prefix with audit logging
- **Analytics Sync** - Store historical analytics data for requests, bandwidth, threats, and visitors
- **Cache Performance** - Track cache hit rates and bandwidth savings
- **Multi-Zone Support** - Manage multiple zones across different Cloudflare accounts
- **REST API** - Full HTTP API for integration with external systems
- **Multi-App Support** - Isolate resources by application ID for multi-tenant architectures
- **Webhook Events** - Emit events for zone syncs, DNS changes, cache purges, and analytics updates

## Quick Start

```bash
# Install the plugin
nself plugin install cloudflare

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export CF_API_TOKEN="your-cloudflare-api-token"
export CF_PLUGIN_PORT=3024

# Initialize the database schema
nself plugin cloudflare init

# Start the server
nself plugin cloudflare server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `""` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL connection |
| `CF_API_TOKEN` | Yes | `""` | Cloudflare API token (recommended) |
| `CF_API_KEY` | No | `""` | Cloudflare Global API Key (legacy, use API token instead) |
| `CF_API_EMAIL` | No | `""` | Cloudflare account email (required if using API key) |
| `CF_ACCOUNT_ID` | No | `""` | Cloudflare account ID for R2 and Workers |
| `CF_PLUGIN_PORT` | No | `3024` | HTTP server port |
| `CF_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `CF_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `CF_APP_IDS` | No | `primary` | Comma-separated list of application IDs for multi-app support |
| `CF_ZONE_IDS` | No | `""` | Comma-separated list of zone IDs to sync (empty = all zones) |
| `CF_R2_ACCESS_KEY` | No | `""` | R2 access key ID for bucket management |
| `CF_R2_SECRET_KEY` | No | `""` | R2 secret access key |
| `CF_SYNC_INTERVAL` | No | `3600` | Automatic sync interval in seconds (default: 1 hour) |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
CF_API_TOKEN=your-cloudflare-api-token-here

# Server Configuration
CF_PLUGIN_PORT=3024
CF_PLUGIN_HOST=0.0.0.0
CF_LOG_LEVEL=info

# Cloudflare Account
CF_ACCOUNT_ID=your-account-id
CF_ZONE_IDS=zone-id-1,zone-id-2  # Optional: limit to specific zones

# R2 Object Storage (optional)
CF_R2_ACCESS_KEY=your-r2-access-key
CF_R2_SECRET_KEY=your-r2-secret-key

# Multi-App Support
CF_APP_IDS=primary,production,staging

# Sync Configuration
CF_SYNC_INTERVAL=3600  # Sync every hour

# Legacy API Key Authentication (not recommended)
# CF_API_KEY=your-global-api-key
# CF_API_EMAIL=your@email.com
```

## CLI Commands

### `init`

Initialize the Cloudflare database schema.

```bash
nself plugin cloudflare init
```

### `server`

Start the Cloudflare HTTP server.

```bash
nself plugin cloudflare server
```

### `sync`

Sync data from Cloudflare API to local database.

```bash
# Sync all resources
nself plugin cloudflare sync

# Sync specific resources
nself plugin cloudflare sync --resources zones,dns

# Multi-app support
nself plugin cloudflare sync --app-id production
```

**Available resources:** `zones`, `dns`, `r2`, `analytics`

### `zones`

List synced Cloudflare zones.

```bash
# List all zones
nself plugin cloudflare zones

# Multi-app support
nself plugin cloudflare zones --app-id staging

# Example output:
# {
#   "zones": [
#     {
#       "id": "abc123...",
#       "name": "example.com",
#       "status": "active",
#       "type": "full",
#       "ssl_status": "active"
#     }
#   ],
#   "total": 5
# }
```

### `dns`

List DNS records for a zone.

```bash
# List DNS records
nself plugin cloudflare dns --zone abc123...

# Example output:
# {
#   "records": [
#     {
#       "id": "dns_123...",
#       "type": "A",
#       "name": "www.example.com",
#       "content": "192.0.2.1",
#       "proxied": true
#     }
#   ],
#   "total": 15
# }
```

### `dns-add`

Add a DNS record.

```bash
# Add A record
nself plugin cloudflare dns-add \
  --zone abc123 \
  --type A \
  --name www \
  --content 192.0.2.1 \
  --proxied

# Add CNAME record
nself plugin cloudflare dns-add \
  --zone abc123 \
  --type CNAME \
  --name blog \
  --content example.com

# Add MX record
nself plugin cloudflare dns-add \
  --zone abc123 \
  --type MX \
  --name @ \
  --content mail.example.com \
  --priority 10

# Add TXT record
nself plugin cloudflare dns-add \
  --zone abc123 \
  --type TXT \
  --name _dmarc \
  --content "v=DMARC1; p=quarantine"
```

### `cache-purge`

Purge CDN cache for a zone.

```bash
# Purge everything
nself plugin cloudflare cache-purge --zone abc123 --all

# Purge specific URLs
nself plugin cloudflare cache-purge \
  --zone abc123 \
  --urls "https://example.com/page1.html,https://example.com/page2.html"

# Multi-app support
nself plugin cloudflare cache-purge --zone abc123 --all --app-id production
```

### `r2`

List R2 buckets.

```bash
# List all R2 buckets
nself plugin cloudflare r2

# Example output:
# {
#   "buckets": [
#     {
#       "name": "my-bucket",
#       "location": "WNAM",
#       "storage_class": "Standard",
#       "object_count": 1250,
#       "total_size_bytes": 524288000
#     }
#   ],
#   "total": 3
# }
```

### `analytics`

View zone analytics.

```bash
# Get analytics for last 30 days (default)
nself plugin cloudflare analytics --zone abc123

# Get analytics for specific date range
nself plugin cloudflare analytics \
  --zone abc123 \
  --from 2025-01-01 \
  --to 2025-01-31

# Example output:
# {
#   "analytics": [
#     {
#       "date": "2025-02-10",
#       "requests_total": 125000,
#       "requests_cached": 100000,
#       "bandwidth_total": 5368709120,
#       "threats_total": 42,
#       "unique_visitors": 8500
#     }
#   ],
#   "total": 30
# }
```

### `status`

Show sync status and statistics.

```bash
nself plugin cloudflare status

# Example output:
# {
#   "totalZones": 5,
#   "totalDnsRecords": 87,
#   "totalR2Buckets": 3,
#   "totalCachePurges": 12,
#   "totalAnalyticsRecords": 180,
#   "lastSyncedAt": "2025-02-11T10:30:00Z"
# }
```

### `stats`

Show Cloudflare plugin statistics.

```bash
nself plugin cloudflare stats
```

## REST API

### Health Check Endpoints

#### `GET /health`

Check if the server is running.

**Response:**
```json
{
  "status": "ok",
  "plugin": "cloudflare",
  "timestamp": "2025-02-11T10:30:00Z",
  "version": "1.0.0"
}
```

#### `GET /ready`

Check if the server is ready to accept requests.

**Response:**
```json
{
  "ready": true,
  "database": "ok",
  "timestamp": "2025-02-11T10:30:00Z"
}
```

#### `GET /live`

Get server liveness information.

**Response:**
```json
{
  "alive": true,
  "uptime": 3600.5,
  "memory": {
    "used": 104857600,
    "total": 536870912
  }
}
```

### Zone Endpoints

#### `GET /api/zones`

List all zones.

**Headers:**
- `X-App-Id` (optional): Application ID for multi-app support

**Response:**
```json
{
  "data": [
    {
      "id": "abc123...",
      "source_account_id": "primary",
      "name": "example.com",
      "status": "active",
      "type": "full",
      "name_servers": ["ns1.cloudflare.com", "ns2.cloudflare.com"],
      "plan": {
        "id": "free",
        "name": "Free Website"
      },
      "settings": {},
      "ssl_status": "active",
      "synced_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 5
}
```

#### `GET /api/zones/:id`

Get zone details by ID.

**Response:**
```json
{
  "id": "abc123...",
  "name": "example.com",
  "status": "active",
  "type": "full",
  "name_servers": ["ns1.cloudflare.com", "ns2.cloudflare.com"],
  "plan": {
    "id": "pro",
    "name": "Pro Website"
  },
  "settings": {
    "always_use_https": "on",
    "ssl": "full",
    "minify": {
      "css": "on",
      "html": "on",
      "js": "on"
    }
  },
  "ssl_status": "active"
}
```

#### `POST /api/zones/:id/settings`

Update zone settings.

**Request Body:**
```json
{
  "settings": {
    "always_use_https": "on",
    "ssl": "strict",
    "minify": {
      "css": "on",
      "html": "on",
      "js": "on"
    }
  }
}
```

### DNS Endpoints

#### `GET /api/zones/:id/dns`

List DNS records for a zone.

**Response:**
```json
{
  "data": [
    {
      "id": "dns_123...",
      "source_account_id": "primary",
      "zone_id": "abc123...",
      "type": "A",
      "name": "www.example.com",
      "content": "192.0.2.1",
      "ttl": 1,
      "proxied": true,
      "priority": null,
      "locked": false,
      "synced_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 15
}
```

#### `POST /api/zones/:id/dns`

Create a new DNS record.

**Request Body:**
```json
{
  "type": "A",
  "name": "www",
  "content": "192.0.2.1",
  "ttl": 1,
  "proxied": true
}
```

For MX records:
```json
{
  "type": "MX",
  "name": "@",
  "content": "mail.example.com",
  "priority": 10,
  "ttl": 3600
}
```

**Response:** `201 Created`
```json
{
  "id": "dns_123...",
  "type": "A",
  "name": "www.example.com",
  "content": "192.0.2.1",
  "proxied": true
}
```

#### `PUT /api/dns/:id`

Update a DNS record.

**Request Body:**
```json
{
  "content": "192.0.2.2",
  "proxied": false,
  "ttl": 3600
}
```

#### `DELETE /api/dns/:id`

Delete a DNS record.

**Response:** `204 No Content`

### R2 Bucket Endpoints

#### `GET /api/r2/buckets`

List R2 buckets.

**Response:**
```json
{
  "data": [
    {
      "id": "uuid...",
      "source_account_id": "primary",
      "name": "my-bucket",
      "location": "WNAM",
      "storage_class": "Standard",
      "object_count": 1250,
      "total_size_bytes": 524288000,
      "created_at": "2025-01-01T00:00:00Z",
      "synced_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 3
}
```

#### `POST /api/r2/buckets`

Create a new R2 bucket.

**Request Body:**
```json
{
  "name": "my-bucket",
  "location": "WNAM"
}
```

**Response:** `201 Created`

#### `DELETE /api/r2/buckets/:name`

Delete an R2 bucket.

**Response:** `204 No Content`

#### `GET /api/r2/buckets/:name/stats`

Get R2 bucket statistics.

**Response:**
```json
{
  "name": "my-bucket",
  "objectCount": 1250,
  "totalSizeBytes": 524288000,
  "location": "WNAM",
  "storageClass": "Standard"
}
```

### Cache Endpoints

#### `POST /api/zones/:id/cache/purge`

Purge CDN cache.

**Request Body (purge everything):**
```json
{
  "type": "all"
}
```

**Request Body (purge specific URLs):**
```json
{
  "type": "urls",
  "urls": [
    "https://example.com/page1.html",
    "https://example.com/page2.html"
  ]
}
```

**Request Body (purge by cache tag):**
```json
{
  "type": "tags",
  "tags": ["blog", "news"]
}
```

**Request Body (purge by host):**
```json
{
  "type": "hosts",
  "hosts": ["www.example.com", "blog.example.com"]
}
```

**Response:**
```json
{
  "purged": true,
  "id": "purge-uuid..."
}
```

#### `GET /api/zones/:id/cache/stats`

Get cache performance statistics (last 30 days).

**Response:**
```json
{
  "hitRate": 0.85,
  "totalRequests": 5000000,
  "cachedRequests": 4250000,
  "uncachedRequests": 750000
}
```

### Analytics Endpoints

#### `GET /api/zones/:id/analytics`

Get analytics data for a zone.

**Query Parameters:**
- `from` (optional, default: 30 days ago): Start date (YYYY-MM-DD)
- `to` (optional, default: today): End date (YYYY-MM-DD)

**Response:**
```json
{
  "daily": [
    {
      "date": "2025-02-10",
      "requests_total": 125000,
      "requests_cached": 100000,
      "requests_uncached": 25000,
      "bandwidth_total": 5368709120,
      "bandwidth_cached": 4294967296,
      "threats_total": 42,
      "unique_visitors": 8500,
      "status_codes": {
        "200": 120000,
        "404": 3000,
        "500": 50
      },
      "countries": {
        "US": 50000,
        "GB": 20000,
        "DE": 15000
      }
    }
  ],
  "totals": {
    "requests": 3750000,
    "bandwidth": 161061273600,
    "cached": 3187500,
    "threats": 1260,
    "uniqueVisitors": 255000
  }
}
```

### Sync Endpoints

#### `POST /api/sync`

Trigger manual sync from Cloudflare API.

**Request Body:**
```json
{
  "resources": ["zones", "dns", "r2", "analytics"]
}
```

**Response:**
```json
{
  "synced": {
    "zones": 5,
    "dns": 87,
    "r2": 3,
    "analytics": 30
  },
  "errors": [],
  "duration": 2345,
  "stats": {
    "totalZones": 5,
    "totalDnsRecords": 87,
    "totalR2Buckets": 3,
    "totalCachePurges": 12,
    "totalAnalyticsRecords": 210,
    "lastSyncedAt": "2025-02-11T10:30:00Z"
  }
}
```

#### `GET /api/status`

Get sync status and statistics.

**Response:**
```json
{
  "status": "ok",
  "totalZones": 5,
  "totalDnsRecords": 87,
  "totalR2Buckets": 3,
  "totalCachePurges": 12,
  "totalAnalyticsRecords": 210,
  "lastSyncedAt": "2025-02-11T10:30:00Z",
  "syncInterval": 3600
}
```

## Webhook Events

The Cloudflare plugin emits webhook events that are stored in the `cf_webhook_events` table.

| Event Type | Description | Payload |
|------------|-------------|---------|
| `cf.zone.synced` | Zone data synced from Cloudflare | `{ zoneId, zoneName }` |
| `cf.dns.created` | DNS record created | `{ recordId, type, name, content }` |
| `cf.dns.updated` | DNS record updated | `{ recordId, changes }` |
| `cf.dns.deleted` | DNS record deleted | `{ recordId }` |
| `cf.cache.purged` | Cache purge completed | `{ zoneId, purgeType, urls?, tags?, hosts? }` |
| `cf.r2.bucket.created` | R2 bucket created | `{ bucketName, location }` |
| `cf.analytics.synced` | Analytics data synced | `{ zoneId, dateRange }` |

## Database Schema

### cf_zones

Stores Cloudflare zones with configuration and status.

```sql
CREATE TABLE IF NOT EXISTS cf_zones (
  id VARCHAR(64) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  status VARCHAR(50),
  type VARCHAR(20),
  name_servers TEXT[],
  plan JSONB,
  settings JSONB DEFAULT '{}',
  ssl_status VARCHAR(50),
  synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cf_zones_source_app ON cf_zones(source_account_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | VARCHAR(64) | No | - | Cloudflare zone ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `name` | VARCHAR(255) | No | - | Domain name (e.g., example.com) |
| `status` | VARCHAR(50) | Yes | NULL | Zone status (active, pending, deactivated) |
| `type` | VARCHAR(20) | Yes | NULL | Zone type (full, partial) |
| `name_servers` | TEXT[] | Yes | NULL | Array of nameservers |
| `plan` | JSONB | Yes | NULL | Plan details (id, name, price) |
| `settings` | JSONB | No | `'{}'` | Zone settings (SSL, minify, etc.) |
| `ssl_status` | VARCHAR(50) | Yes | NULL | SSL/TLS status |
| `synced_at` | TIMESTAMPTZ | No | `NOW()` | Last sync timestamp |

### cf_dns_records

Stores DNS records for all zones.

```sql
CREATE TABLE IF NOT EXISTS cf_dns_records (
  id VARCHAR(64) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id VARCHAR(64) NOT NULL REFERENCES cf_zones(id),
  type VARCHAR(10) NOT NULL,
  name VARCHAR(255) NOT NULL,
  content TEXT NOT NULL,
  ttl INTEGER DEFAULT 1,
  proxied BOOLEAN DEFAULT true,
  priority INTEGER,
  locked BOOLEAN DEFAULT false,
  synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cf_dns_source_app ON cf_dns_records(source_account_id);
CREATE INDEX IF NOT EXISTS idx_cf_dns_zone ON cf_dns_records(zone_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | VARCHAR(64) | No | - | DNS record ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `zone_id` | VARCHAR(64) | No | - | Parent zone ID |
| `type` | VARCHAR(10) | No | - | Record type (A, AAAA, CNAME, MX, TXT, etc.) |
| `name` | VARCHAR(255) | No | - | Full record name (e.g., www.example.com) |
| `content` | TEXT | No | - | Record content (IP, domain, text) |
| `ttl` | INTEGER | No | `1` | TTL in seconds (1 = automatic) |
| `proxied` | BOOLEAN | No | `true` | Whether record is proxied through Cloudflare |
| `priority` | INTEGER | Yes | NULL | Priority (for MX and SRV records) |
| `locked` | BOOLEAN | No | `false` | Whether record is locked from editing |
| `synced_at` | TIMESTAMPTZ | No | `NOW()` | Last sync timestamp |

### cf_r2_buckets

Stores R2 object storage buckets.

```sql
CREATE TABLE IF NOT EXISTS cf_r2_buckets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(63) NOT NULL,
  location VARCHAR(50),
  storage_class VARCHAR(50) DEFAULT 'Standard',
  object_count BIGINT DEFAULT 0,
  total_size_bytes BIGINT DEFAULT 0,
  created_at TIMESTAMPTZ,
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX IF NOT EXISTS idx_cf_r2_buckets_source_app ON cf_r2_buckets(source_account_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Internal ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `name` | VARCHAR(63) | No | - | Bucket name (must be unique per account) |
| `location` | VARCHAR(50) | Yes | NULL | Bucket region (WNAM, ENAM, etc.) |
| `storage_class` | VARCHAR(50) | No | `'Standard'` | Storage class |
| `object_count` | BIGINT | No | `0` | Number of objects in bucket |
| `total_size_bytes` | BIGINT | No | `0` | Total storage size in bytes |
| `created_at` | TIMESTAMPTZ | Yes | NULL | Bucket creation timestamp |
| `synced_at` | TIMESTAMPTZ | No | `NOW()` | Last sync timestamp |

### cf_cache_purge_log

Stores cache purge history with audit trail.

```sql
CREATE TABLE IF NOT EXISTS cf_cache_purge_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id VARCHAR(64) NOT NULL,
  purge_type VARCHAR(20) NOT NULL,
  urls TEXT[],
  tags TEXT[],
  hosts TEXT[],
  prefixes TEXT[],
  status VARCHAR(20) NOT NULL,
  cf_response JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cf_cache_purge_source_app ON cf_cache_purge_log(source_account_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Purge request ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `zone_id` | VARCHAR(64) | No | - | Zone ID where cache was purged |
| `purge_type` | VARCHAR(20) | No | - | Purge type (all, urls, tags, hosts, prefixes) |
| `urls` | TEXT[] | Yes | NULL | Array of URLs purged (if type=urls) |
| `tags` | TEXT[] | Yes | NULL | Array of cache tags purged (if type=tags) |
| `hosts` | TEXT[] | Yes | NULL | Array of hosts purged (if type=hosts) |
| `prefixes` | TEXT[] | Yes | NULL | Array of URL prefixes purged (if type=prefixes) |
| `status` | VARCHAR(20) | No | - | Purge status (completed, failed) |
| `cf_response` | JSONB | Yes | NULL | Raw Cloudflare API response |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Purge request timestamp |

### cf_analytics

Stores daily analytics data per zone.

```sql
CREATE TABLE IF NOT EXISTS cf_analytics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id VARCHAR(64) NOT NULL,
  date DATE NOT NULL,
  requests_total BIGINT DEFAULT 0,
  requests_cached BIGINT DEFAULT 0,
  requests_uncached BIGINT DEFAULT 0,
  bandwidth_total BIGINT DEFAULT 0,
  bandwidth_cached BIGINT DEFAULT 0,
  threats_total BIGINT DEFAULT 0,
  unique_visitors BIGINT DEFAULT 0,
  status_codes JSONB DEFAULT '{}',
  countries JSONB DEFAULT '{}',
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, zone_id, date)
);

CREATE INDEX IF NOT EXISTS idx_cf_analytics_source_app ON cf_analytics(source_account_id);
CREATE INDEX IF NOT EXISTS idx_cf_analytics_date ON cf_analytics(source_account_id, zone_id, date);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Analytics record ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `zone_id` | VARCHAR(64) | No | - | Zone ID |
| `date` | DATE | No | - | Date of analytics data |
| `requests_total` | BIGINT | No | `0` | Total requests |
| `requests_cached` | BIGINT | No | `0` | Cached requests |
| `requests_uncached` | BIGINT | No | `0` | Uncached requests |
| `bandwidth_total` | BIGINT | No | `0` | Total bandwidth in bytes |
| `bandwidth_cached` | BIGINT | No | `0` | Cached bandwidth in bytes |
| `threats_total` | BIGINT | No | `0` | Total threats blocked |
| `unique_visitors` | BIGINT | No | `0` | Unique visitors |
| `status_codes` | JSONB | No | `'{}'` | HTTP status code distribution |
| `countries` | JSONB | No | `'{}'` | Request distribution by country |
| `synced_at` | TIMESTAMPTZ | No | `NOW()` | Last sync timestamp |

### cf_webhook_events

Stores webhook events for asynchronous processing.

```sql
CREATE TABLE IF NOT EXISTS cf_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  retry_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cf_webhook_events_source_app ON cf_webhook_events(source_account_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | VARCHAR(255) | No | - | Event ID (primary key) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `event_type` | VARCHAR(128) | No | - | Event type (e.g., cf.zone.synced) |
| `payload` | JSONB | No | - | Event payload data |
| `processed` | BOOLEAN | No | `false` | Whether the event has been processed |
| `processed_at` | TIMESTAMPTZ | Yes | NULL | When the event was processed |
| `error` | TEXT | Yes | NULL | Processing error message |
| `retry_count` | INTEGER | No | `0` | Number of processing retry attempts |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Event creation timestamp |

## Examples

### Example 1: Complete DNS Setup for New Domain

```bash
# 1. Sync zones from Cloudflare
nself plugin cloudflare sync --resources zones

# 2. Get zone ID
ZONE_ID=$(nself plugin cloudflare zones | jq -r '.zones[] | select(.name=="example.com") | .id')

# 3. Add root A record
nself plugin cloudflare dns-add \
  --zone $ZONE_ID \
  --type A \
  --name @ \
  --content 192.0.2.1 \
  --proxied

# 4. Add www CNAME
nself plugin cloudflare dns-add \
  --zone $ZONE_ID \
  --type CNAME \
  --name www \
  --content example.com

# 5. Add mail MX records
nself plugin cloudflare dns-add \
  --zone $ZONE_ID \
  --type MX \
  --name @ \
  --content mail.example.com \
  --priority 10

# 6. Add SPF TXT record
nself plugin cloudflare dns-add \
  --zone $ZONE_ID \
  --type TXT \
  --name @ \
  --content "v=spf1 mx include:_spf.google.com ~all"

# 7. Verify DNS records
nself plugin cloudflare dns --zone $ZONE_ID
```

### Example 2: Analytics Dashboard Query

```sql
-- 30-day traffic summary with cache efficiency
SELECT
  z.name as zone,
  SUM(a.requests_total) as total_requests,
  SUM(a.requests_cached) as cached_requests,
  ROUND(100.0 * SUM(a.requests_cached) / NULLIF(SUM(a.requests_total), 0), 2) as cache_hit_rate,
  pg_size_pretty(SUM(a.bandwidth_total)) as total_bandwidth,
  pg_size_pretty(SUM(a.bandwidth_cached)) as cached_bandwidth,
  SUM(a.threats_total) as threats_blocked,
  SUM(a.unique_visitors) as unique_visitors
FROM cf_analytics a
JOIN cf_zones z ON z.id = a.zone_id
WHERE a.source_account_id = 'primary'
  AND a.date >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY z.name
ORDER BY total_requests DESC;
```

### Example 3: Automated Cache Purging Workflow

```bash
#!/bin/bash
# Auto-purge cache when deploying new version

ZONE_ID="your-zone-id"
DEPLOY_TAG="v1.2.3"

# Deploy application
echo "Deploying ${DEPLOY_TAG}..."
# ... deployment commands ...

# Purge cached static assets
echo "Purging CDN cache..."
curl -X POST "http://localhost:3024/api/zones/${ZONE_ID}/cache/purge" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "prefixes",
    "prefixes": [
      "https://example.com/static/",
      "https://example.com/assets/"
    ]
  }'

echo "Cache purge completed!"
```

### Example 4: R2 Bucket Management

```bash
# Create R2 bucket for media storage
curl -X POST "http://localhost:3024/api/r2/buckets" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "media-storage",
    "location": "WNAM"
  }'

# Check bucket stats
curl "http://localhost:3024/api/r2/buckets/media-storage/stats"

# List all buckets
nself plugin cloudflare r2
```

### Example 5: Multi-Zone DNS Query

```sql
-- Find all A records across all zones
SELECT
  z.name as zone,
  d.name as record_name,
  d.content as ip_address,
  d.proxied,
  d.synced_at
FROM cf_dns_records d
JOIN cf_zones z ON z.id = d.zone_id
WHERE d.source_account_id = 'primary'
  AND d.type = 'A'
ORDER BY z.name, d.name;
```

### Example 6: Zone Settings Automation

```bash
#!/bin/bash
# Apply security settings to all zones

for ZONE_ID in $(nself plugin cloudflare zones | jq -r '.zones[].id'); do
  echo "Updating zone ${ZONE_ID}..."

  curl -X POST "http://localhost:3024/api/zones/${ZONE_ID}/settings" \
    -H "Content-Type: application/json" \
    -d '{
      "settings": {
        "always_use_https": "on",
        "ssl": "strict",
        "security_level": "high",
        "browser_check": "on"
      }
    }'
done
```

## Troubleshooting

### Common Issues

#### 1. Authentication Errors

**Symptom:** API requests fail with 401 or 403 errors.

**Solutions:**
- Verify API token is valid: `echo $CF_API_TOKEN`
- Check API token permissions in Cloudflare dashboard:
  - Zone:Read
  - DNS:Edit
  - Analytics:Read
  - Account:Read (for R2)
- Use API token instead of Global API Key for better security
- Verify account ID is correct for R2 operations

#### 2. Zone Not Found

**Symptom:** Zone operations fail with "Zone not found" error.

**Solutions:**
- Run sync to fetch latest zones: `nself plugin cloudflare sync --resources zones`
- Verify zone ID exists:
  ```sql
  SELECT id, name FROM cf_zones WHERE source_account_id = 'primary';
  ```
- Check if zone filtering is enabled: `echo $CF_ZONE_IDS`
- Verify multi-app isolation: use correct `--app-id` parameter

#### 3. DNS Record Creation Fails

**Symptom:** DNS record creation returns validation errors.

**Solutions:**
- Verify record name format (use @ for root, subdomain name without domain)
- Check content format for record type:
  - A/AAAA: Must be valid IP address
  - CNAME: Must be valid domain name
  - MX: Must include priority value
  - TXT: Enclose in quotes if contains spaces
- Ensure zone is active: check `status` column in `cf_zones` table

#### 4. Slow Analytics Queries

**Symptom:** Analytics endpoints are slow or timeout.

**Solutions:**
- Limit date range to reduce data volume
- Add custom index:
  ```sql
  CREATE INDEX idx_cf_analytics_zone_date
  ON cf_analytics(zone_id, date DESC)
  WHERE source_account_id = 'primary';
  ```
- Use aggregate queries instead of fetching all daily records
- Consider materialized views for frequently-accessed reports:
  ```sql
  CREATE MATERIALIZED VIEW cf_monthly_analytics AS
  SELECT
    zone_id,
    DATE_TRUNC('month', date) as month,
    SUM(requests_total) as requests,
    SUM(bandwidth_total) as bandwidth
  FROM cf_analytics
  GROUP BY zone_id, month;
  ```

#### 5. R2 Bucket Access Denied

**Symptom:** R2 operations fail with access denied errors.

**Solutions:**
- Verify R2 credentials are set:
  ```bash
  echo $CF_R2_ACCESS_KEY
  echo $CF_R2_SECRET_KEY
  ```
- Check account ID is correct: `echo $CF_ACCOUNT_ID`
- Verify R2 is enabled for your Cloudflare account
- Ensure R2 API token has required permissions

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
