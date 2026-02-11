# CDN Plugin

CDN management and integration plugin - cache purging, signed URLs, analytics

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The CDN plugin provides comprehensive CDN management for the nself platform. It enables zone management, cache purging, signed URL generation, and analytics tracking across multiple CDN providers.

### Key Features

- **Multi-Provider Support** - Cloudflare, BunnyCDN, and extensible provider architecture
- **Zone Management** - Create and manage CDN zones with custom configurations
- **Cache Purging** - Purge by URL, cache tag, prefix, or entire zones
- **Signed URLs** - Generate time-limited, IP-restricted secure URLs
- **Analytics Tracking** - Monitor bandwidth, requests, cache hit rates
- **Batch Operations** - Efficient batch purging and URL signing
- **Provider Abstraction** - Unified API across different CDN providers
- **Multi-Account Support** - `source_account_id` isolation for multi-workspace deployments

### Supported CDN Providers

| Provider | Features | Status |
|----------|----------|--------|
| Cloudflare | Full support: purge, sign, analytics | Supported |
| BunnyCDN | Full support: purge, sign, analytics | Supported |

---

## Quick Start

```bash
# Install the plugin
nself plugin install cdn

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export CDN_PLUGIN_PORT=3204
export CDN_PROVIDER=cloudflare
export CDN_CLOUDFLARE_API_TOKEN=your_token_here

# Initialize database schema
nself plugin cdn init

# Start the server
nself plugin cdn server --port 3204

# Check status
nself plugin cdn status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `CDN_PLUGIN_PORT` | No | `3204` | HTTP server port |
| `CDN_PROVIDER` | No | `cloudflare` | Default CDN provider |
| `CDN_CLOUDFLARE_API_TOKEN` | No | - | Cloudflare API token |
| `CDN_CLOUDFLARE_ZONE_IDS` | No | - | Comma-separated Cloudflare zone IDs |
| `CDN_BUNNYCDN_API_KEY` | No | - | BunnyCDN API key |
| `CDN_BUNNYCDN_PULL_ZONE_IDS` | No | - | Comma-separated BunnyCDN pull zone IDs |
| `CDN_SIGNING_KEY` | No | - | Secret key for URL signing (HMAC-SHA256) |
| `CDN_SIGNED_URL_TTL` | No | `3600` | Default signed URL TTL in seconds |
| `CDN_ANALYTICS_SYNC_INTERVAL` | No | `86400` | Analytics sync interval in seconds (daily) |
| `CDN_PURGE_BATCH_SIZE` | No | `500` | Maximum URLs per purge batch |
| `CDN_API_KEY` | No | - | API key for authentication (optional) |
| `CDN_RATE_LIMIT_MAX` | No | `500` | Maximum requests per window |
| `CDN_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=nself
POSTGRES_PASSWORD=secure_password
POSTGRES_SSL=false

# Server
CDN_PLUGIN_PORT=3204
CDN_PLUGIN_HOST=0.0.0.0

# Provider Configuration
CDN_PROVIDER=cloudflare
CDN_CLOUDFLARE_API_TOKEN=your_cloudflare_api_token
CDN_CLOUDFLARE_ZONE_IDS=zone_id_1,zone_id_2
CDN_BUNNYCDN_API_KEY=your_bunnycdn_api_key
CDN_BUNNYCDN_PULL_ZONE_IDS=pull_zone_1,pull_zone_2

# Signing Configuration
CDN_SIGNING_KEY=your_secret_signing_key_here
CDN_SIGNED_URL_TTL=3600

# Analytics Configuration
CDN_ANALYTICS_SYNC_INTERVAL=86400

# Purge Configuration
CDN_PURGE_BATCH_SIZE=500

# Security (optional)
CDN_API_KEY=your_api_key_here
CDN_RATE_LIMIT_MAX=500
CDN_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin cdn init

# Start the server
nself plugin cdn server
nself plugin cdn server --port 3204 --host 0.0.0.0

# Check status and statistics
nself plugin cdn status
```

### Zone Commands

```bash
# List all zones
nself plugin cdn zones list

# Filter by provider
nself plugin cdn zones list --provider cloudflare

# Add a new zone
nself plugin cdn zones add \
  --provider cloudflare \
  --zone-id abc123def456 \
  --name "Production CDN" \
  --domain "cdn.example.com" \
  --origin "https://origin.example.com"
```

### Purge Commands

```bash
# Purge specific URLs
nself plugin cdn purge \
  --zone <zone-id> \
  --urls "https://cdn.example.com/image1.jpg,https://cdn.example.com/image2.jpg"

# Purge by cache tags
nself plugin cdn purge \
  --zone <zone-id> \
  --tags "blog,images"

# Purge by URL prefixes
nself plugin cdn purge \
  --zone <zone-id> \
  --prefixes "/images/,/css/"

# Purge entire zone
nself plugin cdn purge --zone <zone-id> --all
```

### Signed URL Commands

```bash
# Generate a signed URL
nself plugin cdn sign \
  --zone <zone-id> \
  --url "https://cdn.example.com/private/video.mp4" \
  --ttl 3600

# Generate with IP restriction
nself plugin cdn sign \
  --zone <zone-id> \
  --url "https://cdn.example.com/private/document.pdf" \
  --ttl 7200 \
  --ip "203.0.113.45"

# Generate multiple signed URLs
nself plugin cdn sign batch \
  --zone <zone-id> \
  --urls "url1.jpg,url2.jpg,url3.jpg" \
  --ttl 1800
```

### Analytics Commands

```bash
# View analytics
nself plugin cdn analytics --zone <zone-id>

# View analytics for date range
nself plugin cdn analytics \
  --zone <zone-id> \
  --from 2026-02-01 \
  --to 2026-02-10

# Sync analytics from provider
nself plugin cdn analytics sync --zone <zone-id>
```

---

## REST API

### Base URL

```
http://localhost:3204
```

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "cdn",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check endpoint.

**Response:**
```json
{
  "ready": true,
  "plugin": "cdn",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness endpoint with runtime stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "cdn",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 104857600,
    "heapTotal": 52428800,
    "heapUsed": 41943040
  },
  "stats": {
    "zones": 5,
    "pendingPurges": 2,
    "activeSignedUrls": 245,
    "totalRequestsTracked": 15234567
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Zone Endpoints

#### GET /api/zones
List all CDN zones.

**Query Parameters:**
- `provider` (optional): Filter by provider (cloudflare, bunnycdn)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "provider": "cloudflare",
      "zone_id": "abc123def456",
      "name": "Production CDN",
      "domain": "cdn.example.com",
      "origin_url": "https://origin.example.com",
      "ssl_enabled": true,
      "cache_ttl": 86400,
      "status": "active",
      "config": {
        "browser_cache_ttl": 14400,
        "development_mode": false
      },
      "metadata": {},
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 5
}
```

#### POST /api/zones
Create a new CDN zone.

**Request Body:**
```json
{
  "provider": "cloudflare",
  "zone_id": "abc123def456",
  "name": "Production CDN",
  "domain": "cdn.example.com",
  "origin_url": "https://origin.example.com",
  "ssl_enabled": true,
  "cache_ttl": 86400,
  "config": {
    "browser_cache_ttl": 14400
  }
}
```

**Response:** Returns created zone object (201 status).

#### GET /api/zones/:id
Get a specific zone by ID.

**Response:** Same format as single zone in list above.

### Purge Endpoints

#### POST /api/purge
Purge CDN cache.

**Request Body (by URLs):**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "urls",
  "urls": [
    "https://cdn.example.com/image1.jpg",
    "https://cdn.example.com/image2.jpg"
  ],
  "requested_by": "admin"
}
```

**Request Body (by tags):**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "tags",
  "tags": ["blog", "images"]
}
```

**Request Body (by prefixes):**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "prefixes",
  "prefixes": ["/images/", "/css/"]
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "urls",
  "urls": [
    "https://cdn.example.com/image1.jpg",
    "https://cdn.example.com/image2.jpg"
  ],
  "status": "completed",
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

#### POST /api/purge-all
Purge entire zone cache.

**Request Body:**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "requested_by": "admin"
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440002",
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "all",
  "status": "completed",
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

#### GET /api/purge/:id
Get purge request status.

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "purge_type": "urls",
  "urls": ["https://cdn.example.com/image1.jpg"],
  "status": "completed",
  "provider_request_id": "np_cf_purge_12345",
  "requested_by": "admin",
  "completed_at": "2026-02-11T10:00:05.000Z",
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

### Signed URL Endpoints

#### POST /api/sign
Generate a signed URL.

**Request Body:**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "url": "https://cdn.example.com/private/video.mp4",
  "ttl": 3600,
  "ip_restriction": "203.0.113.45",
  "max_access": 5
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440003",
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_url": "https://cdn.example.com/private/video.mp4",
  "signed_url": "https://cdn.example.com/private/video.mp4?expires=1707649200&sig=abc123def456&ip=203.0.113.45",
  "expires_at": "2026-02-11T11:00:00.000Z",
  "ip_restriction": "203.0.113.45",
  "max_access": 5,
  "access_count": 0,
  "created_at": "2026-02-11T10:00:00.000Z"
}
```

#### POST /api/sign/batch
Generate multiple signed URLs.

**Request Body:**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "urls": [
    "https://cdn.example.com/file1.pdf",
    "https://cdn.example.com/file2.pdf"
  ],
  "ttl": 1800
}
```

**Response:**
```json
{
  "data": [
    {
      "original_url": "https://cdn.example.com/file1.pdf",
      "signed_url": "https://cdn.example.com/file1.pdf?expires=1707646800&sig=xyz789",
      "expires_at": "2026-02-11T10:30:00.000Z"
    },
    {
      "original_url": "https://cdn.example.com/file2.pdf",
      "signed_url": "https://cdn.example.com/file2.pdf?expires=1707646800&sig=abc456",
      "expires_at": "2026-02-11T10:30:00.000Z"
    }
  ],
  "total": 2
}
```

### Analytics Endpoints

#### GET /api/analytics
Get analytics summary.

**Query Parameters:**
- `zone_id` (optional): Filter by zone ID
- `from` (optional): Start date (YYYY-MM-DD)
- `to` (optional): End date (YYYY-MM-DD)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440004",
      "zone_id": "550e8400-e29b-41d4-a716-446655440000",
      "date": "2026-02-10",
      "requests_total": 1523456,
      "requests_cached": 1234567,
      "bandwidth_total": 104857600000,
      "bandwidth_cached": 89478485504,
      "unique_visitors": 45678,
      "threats_blocked": 123,
      "status_2xx": 1450000,
      "status_3xx": 50000,
      "status_4xx": 20000,
      "status_5xx": 3456,
      "top_paths": [
        {
          "path": "/images/logo.png",
          "requests": 50000
        }
      ],
      "top_countries": [
        {
          "country": "US",
          "requests": 800000
        }
      ],
      "created_at": "2026-02-11T00:00:00.000Z"
    }
  ],
  "summary": {
    "total_requests": 1523456,
    "total_bandwidth": 104857600000,
    "cache_hit_rate": 81.0,
    "avg_requests_per_day": 1523456
  }
}
```

#### POST /api/analytics/sync
Sync analytics from CDN provider.

**Request Body:**
```json
{
  "zone_id": "550e8400-e29b-41d4-a716-446655440000",
  "from": "2026-02-01",
  "to": "2026-02-10"
}
```

**Response:**
```json
{
  "synced": true,
  "days": 10,
  "zone_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Stats Endpoint

#### GET /api/stats
Get overall plugin statistics.

**Response:**
```json
{
  "total_zones": 5,
  "active_zones": 5,
  "total_purge_requests": 234,
  "pending_purges": 2,
  "total_signed_urls": 1234,
  "active_signed_urls": 245,
  "np_analytics_days_tracked": 365,
  "total_requests_tracked": 15234567,
  "total_bandwidth_tracked": 10485760000000,
  "by_provider": {
    "cloudflare": 3,
    "bunnycdn": 2
  }
}
```

---

## Database Schema

### np_cdn_zones
Stores CDN zone configurations.

```sql
CREATE TABLE np_cdn_zones (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider VARCHAR(64) NOT NULL,
  zone_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  domain VARCHAR(255) NOT NULL,
  origin_url TEXT,
  ssl_enabled BOOLEAN DEFAULT TRUE,
  cache_ttl INTEGER DEFAULT 86400,
  status VARCHAR(32) DEFAULT 'active',
  config JSONB DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, provider, zone_id)
);

CREATE INDEX idx_cdn_zones_source_account ON np_cdn_zones(source_account_id);
CREATE INDEX idx_cdn_zones_provider ON np_cdn_zones(provider);
CREATE INDEX idx_cdn_zones_domain ON np_cdn_zones(domain);
```

### np_cdn_purge_requests
Tracks cache purge operations.

```sql
CREATE TABLE np_cdn_purge_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id UUID REFERENCES np_cdn_zones(id),
  purge_type VARCHAR(16) NOT NULL,
  urls JSONB DEFAULT '[]',
  tags JSONB DEFAULT '[]',
  prefixes JSONB DEFAULT '[]',
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  provider_request_id VARCHAR(255),
  requested_by VARCHAR(255),
  completed_at TIMESTAMPTZ,
  error TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_cdn_purge_source_account ON np_cdn_purge_requests(source_account_id);
CREATE INDEX idx_cdn_purge_status ON np_cdn_purge_requests(status);
CREATE INDEX idx_cdn_purge_zone ON np_cdn_purge_requests(zone_id);
```

### np_cdn_analytics
Stores daily CDN analytics.

```sql
CREATE TABLE np_cdn_analytics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id UUID REFERENCES np_cdn_zones(id),
  date DATE NOT NULL,
  requests_total BIGINT DEFAULT 0,
  requests_cached BIGINT DEFAULT 0,
  bandwidth_total BIGINT DEFAULT 0,
  bandwidth_cached BIGINT DEFAULT 0,
  unique_visitors BIGINT DEFAULT 0,
  threats_blocked BIGINT DEFAULT 0,
  status_2xx BIGINT DEFAULT 0,
  status_3xx BIGINT DEFAULT 0,
  status_4xx BIGINT DEFAULT 0,
  status_5xx BIGINT DEFAULT 0,
  top_paths JSONB DEFAULT '[]',
  top_countries JSONB DEFAULT '[]',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, zone_id, date)
);

CREATE INDEX idx_cdn_analytics_source_account ON np_cdn_analytics(source_account_id);
CREATE INDEX idx_cdn_analytics_date ON np_cdn_analytics(date);
CREATE INDEX idx_cdn_analytics_zone ON np_cdn_analytics(zone_id);
```

### np_cdn_signed_urls
Tracks generated signed URLs.

```sql
CREATE TABLE np_cdn_signed_urls (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  zone_id UUID REFERENCES np_cdn_zones(id),
  original_url TEXT NOT NULL,
  signed_url TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  ip_restriction VARCHAR(45),
  access_count INTEGER DEFAULT 0,
  max_access INTEGER,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_cdn_signed_source_account ON np_cdn_signed_urls(source_account_id);
CREATE INDEX idx_cdn_signed_expires ON np_cdn_signed_urls(expires_at);
CREATE INDEX idx_cdn_signed_zone ON np_cdn_signed_urls(zone_id);
```

### Analytics Views

#### np_cdn_bandwidth_by_zone
Bandwidth usage by zone.

```sql
CREATE OR REPLACE VIEW np_cdn_bandwidth_by_zone AS
SELECT z.source_account_id,
       z.name AS zone_name,
       z.domain,
       z.provider,
       a.date,
       a.bandwidth_total,
       a.bandwidth_cached,
       ROUND(100.0 * a.bandwidth_cached / NULLIF(a.bandwidth_total, 0), 1) AS cache_bandwidth_pct,
       a.requests_total,
       a.requests_cached,
       ROUND(100.0 * a.requests_cached / NULLIF(a.requests_total, 0), 1) AS cache_hit_rate
FROM np_cdn_zones z
JOIN np_cdn_analytics a ON z.id = a.zone_id
ORDER BY a.date DESC;
```

#### np_cdn_cache_hit_rate
Overall cache performance.

```sql
CREATE OR REPLACE VIEW np_cdn_cache_hit_rate AS
SELECT source_account_id,
       DATE(date) as day,
       SUM(requests_total) as total_requests,
       SUM(requests_cached) as cached_requests,
       ROUND(100.0 * SUM(requests_cached) / NULLIF(SUM(requests_total), 0), 1) as cache_hit_rate
FROM np_cdn_analytics
GROUP BY source_account_id, DATE(date)
ORDER BY day DESC;
```

#### np_cdn_top_paths
Most requested paths.

```sql
CREATE OR REPLACE VIEW np_cdn_top_paths AS
SELECT z.source_account_id,
       z.name AS zone_name,
       z.domain,
       a.date,
       path_item->>'path' AS path,
       (path_item->>'requests')::BIGINT AS requests
FROM np_cdn_zones z
JOIN np_cdn_analytics a ON z.id = a.zone_id,
     jsonb_array_elements(a.top_paths) AS path_item
ORDER BY (path_item->>'requests')::BIGINT DESC;
```

---

## Examples

### Example 1: Purge Cache After Deployment

```bash
# Purge all CSS and JS files after deploying new assets
curl -X POST http://localhost:3204/api/purge \
  -H "Content-Type: application/json" \
  -d '{
    "zone_id": "550e8400-e29b-41d4-a716-446655440000",
    "purge_type": "prefixes",
    "prefixes": ["/css/", "/js/"],
    "requested_by": "deployment-script"
  }'

# Purge specific files
curl -X POST http://localhost:3204/api/purge \
  -H "Content-Type: application/json" \
  -d '{
    "zone_id": "550e8400-e29b-41d4-a716-446655440000",
    "purge_type": "urls",
    "urls": [
      "https://cdn.example.com/css/main.css",
      "https://cdn.example.com/js/app.js"
    ]
  }'
```

### Example 2: Generate Signed URLs for Private Content

```javascript
// Generate a signed URL for a premium video
const response = await fetch('http://localhost:3204/api/sign', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    zone_id: '550e8400-e29b-41d4-a716-446655440000',
    url: 'https://cdn.example.com/premium/video.mp4',
    ttl: 3600, // 1 hour
    ip_restriction: '203.0.113.45', // Only accessible from this IP
    max_access: 3 // Can be accessed max 3 times
  })
});

const { signed_url, expires_at } = await response.json();
console.log(`Signed URL: ${signed_url}`);
console.log(`Expires at: ${expires_at}`);
```

### Example 3: Monitor Cache Hit Rates

```sql
-- Daily cache hit rate for last 30 days
SELECT
  zone_name,
  day,
  total_requests,
  cached_requests,
  cache_hit_rate
FROM np_cdn_bandwidth_by_zone
WHERE source_account_id = 'primary'
  AND day > CURRENT_DATE - INTERVAL '30 days'
ORDER BY day DESC, cache_hit_rate DESC;

-- Average cache hit rate by zone
SELECT
  zone_name,
  domain,
  AVG(cache_hit_rate) as avg_cache_hit_rate,
  SUM(total_requests) as total_requests
FROM np_cdn_bandwidth_by_zone
WHERE source_account_id = 'primary'
  AND day > CURRENT_DATE - INTERVAL '30 days'
GROUP BY zone_name, domain
ORDER BY avg_cache_hit_rate DESC;
```

### Example 4: Batch Sign URLs for Download

```javascript
// Generate signed URLs for multiple downloadable files
const files = [
  'report-january.pdf',
  'report-february.pdf',
  'report-march.pdf'
];

const urls = files.map(file => `https://cdn.example.com/reports/${file}`);

const response = await fetch('http://localhost:3204/api/sign/batch', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    zone_id: '550e8400-e29b-41d4-a716-446655440000',
    urls: urls,
    ttl: 7200 // 2 hours
  })
});

const { data } = await response.json();
data.forEach(item => {
  console.log(`Original: ${item.original_url}`);
  console.log(`Signed: ${item.signed_url}`);
  console.log(`Expires: ${item.expires_at}\n`);
});
```

### Example 5: Automated Analytics Sync

```bash
# Daily cron job to sync analytics
#!/bin/bash
ZONE_ID="550e8400-e29b-41d4-a716-446655440000"
YESTERDAY=$(date -d "yesterday" +%Y-%m-%d)

curl -X POST http://localhost:3204/api/analytics/sync \
  -H "Content-Type: application/json" \
  -d "{
    \"zone_id\": \"$ZONE_ID\",
    \"from\": \"$YESTERDAY\",
    \"to\": \"$YESTERDAY\"
  }"

# Query the results
psql $DATABASE_URL -c "
  SELECT
    date,
    requests_total,
    bandwidth_total / 1024 / 1024 as bandwidth_mb,
    ROUND(100.0 * requests_cached / requests_total, 1) as cache_hit_rate
  FROM np_cdn_analytics
  WHERE zone_id = '$ZONE_ID'
    AND date = '$YESTERDAY';
"
```

---

## Troubleshooting

### Purge not working

**Issue:** Cache purge completes but files still cached.

**Solution:**
1. Verify zone configuration:
   ```sql
   SELECT * FROM np_cdn_zones WHERE id = '<zone-id>';
   ```
2. Check purge request status:
   ```sql
   SELECT * FROM np_cdn_purge_requests
   WHERE zone_id = '<zone-id>'
   ORDER BY created_at DESC
   LIMIT 10;
   ```
3. Verify provider credentials:
   ```bash
   echo $CDN_CLOUDFLARE_API_TOKEN
   echo $CDN_BUNNYCDN_API_KEY
   ```
4. Test provider API directly:
   ```bash
   # Cloudflare
   curl -X POST "https://api.cloudflare.com/client/v4/zones/{zone_id}/purge_cache" \
     -H "Authorization: Bearer $CDN_CLOUDFLARE_API_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"files":["https://cdn.example.com/test.jpg"]}'
   ```

### Signed URLs not validating

**Issue:** Signed URLs return 403 or signature errors.

**Solution:**
1. Verify signing key is set:
   ```bash
   echo $CDN_SIGNING_KEY
   ```
2. Check URL hasn't expired:
   ```sql
   SELECT original_url, expires_at, NOW() > expires_at as expired
   FROM np_cdn_signed_urls
   WHERE id = '<signed-url-id>';
   ```
3. Verify IP restriction if set:
   ```sql
   SELECT ip_restriction FROM np_cdn_signed_urls WHERE id = '<signed-url-id>';
   ```
4. Check access count vs max_access:
   ```sql
   SELECT access_count, max_access
   FROM np_cdn_signed_urls
   WHERE id = '<signed-url-id>';
   ```
5. Test signature generation:
   ```javascript
   const crypto = require('crypto');
   const url = 'https://cdn.example.com/file.pdf';
   const expires = Math.floor(Date.now() / 1000) + 3600;
   const signature = crypto
     .createHmac('sha256', process.env.CDN_SIGNING_KEY)
     .update(`${url}${expires}`)
     .digest('hex');
   console.log(`${url}?expires=${expires}&sig=${signature}`);
   ```

### Analytics not syncing

**Issue:** Analytics data missing or stale.

**Solution:**
1. Check sync interval:
   ```bash
   echo $CDN_ANALYTICS_SYNC_INTERVAL  # Default: 86400 (daily)
   ```
2. Manually trigger sync:
   ```bash
   curl -X POST http://localhost:3204/api/analytics/sync \
     -H "Content-Type: application/json" \
     -d '{"zone_id": "<zone-id>"}'
   ```
3. Verify data in database:
   ```sql
   SELECT date, requests_total, bandwidth_total
   FROM np_cdn_analytics
   WHERE zone_id = '<zone-id>'
   ORDER BY date DESC
   LIMIT 7;
   ```
4. Check provider API access:
   ```bash
   # Cloudflare Analytics API
   curl "https://api.cloudflare.com/client/v4/zones/{zone_id}/analytics/dashboard?since=-7d" \
     -H "Authorization: Bearer $CDN_CLOUDFLARE_API_TOKEN"
   ```

### High purge batch size errors

**Issue:** Purge requests fail with batch size errors.

**Solution:**
1. Check batch size limit:
   ```bash
   echo $CDN_PURGE_BATCH_SIZE  # Default: 500
   ```
2. Split large purge requests:
   ```javascript
   const urls = [...]; // 1000+ URLs
   const batchSize = 500;

   for (let i = 0; i < urls.length; i += batchSize) {
     const batch = urls.slice(i, i + batchSize);
     await fetch('http://localhost:3204/api/purge', {
       method: 'POST',
       headers: { 'Content-Type': 'application/json' },
       body: JSON.stringify({
         zone_id: zoneId,
         purge_type: 'urls',
         urls: batch
       })
     });
     // Wait between batches to avoid rate limits
     await new Promise(resolve => setTimeout(resolve, 1000));
   }
   ```
3. Use prefixes instead:
   ```json
   {
     "zone_id": "...",
     "purge_type": "prefixes",
     "prefixes": ["/images/2026/"]
   }
   ```

### Zone creation failing

**Issue:** Cannot create new CDN zones.

**Solution:**
1. Verify required fields:
   ```bash
   # All fields are required
   provider: cloudflare or bunnycdn
   zone_id: Provider's zone/pull zone ID
   name: Descriptive name
   domain: CDN domain
   ```
2. Check zone_id uniqueness:
   ```sql
   SELECT * FROM np_cdn_zones
   WHERE provider = 'cloudflare'
     AND zone_id = '<zone-id>';
   ```
3. Verify provider credentials:
   ```bash
   # Cloudflare
   curl "https://api.cloudflare.com/client/v4/zones/<zone-id>" \
     -H "Authorization: Bearer $CDN_CLOUDFLARE_API_TOKEN"

   # BunnyCDN
   curl "https://api.bunny.net/pullzone/<zone-id>" \
     -H "AccessKey: $CDN_BUNNYCDN_API_KEY"
   ```

---

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- nself CLI: https://github.com/acamarata/nself
