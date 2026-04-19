# CDN Plugin

CDN management and integration plugin with multi-provider support, cache purging, HMAC-SHA256 signed URLs, bandwidth analytics, and zone management. Supports Cloudflare, BunnyCDN, Fastly, and Akamai.

| Property | Value |
|----------|-------|
| **Port** | `3036` |
| **Category** | `infrastructure` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run cdn init
nself plugin run cdn server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CDN_PLUGIN_PORT` | `3036` | Server port |
| `CDN_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `CDN_PROVIDER` | `cloudflare` | Default CDN provider |
| `CDN_CLOUDFLARE_API_TOKEN` | - | Cloudflare API token |
| `CDN_CLOUDFLARE_ZONE_IDS` | - | Comma-separated Cloudflare zone IDs |
| `CDN_BUNNYCDN_API_KEY` | - | BunnyCDN API key |
| `CDN_BUNNYCDN_PULL_ZONE_IDS` | - | Comma-separated BunnyCDN pull zone IDs |
| `CDN_SIGNING_KEY` | - | HMAC-SHA256 key for signed URLs |
| `CDN_SIGNED_URL_TTL` | `3600` | Default signed URL TTL (seconds) |
| `CDN_ANALYTICS_SYNC_INTERVAL` | `3600000` | Analytics sync interval (ms) |
| `CDN_PURGE_BATCH_SIZE` | `30` | Maximum URLs per purge request |
| `CDN_API_KEY` | - | API key for plugin authentication |
| `CDN_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `CDN_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (4 tables, 3 views) |
| `server` | Start the HTTP API server |
| `status` | Show CDN plugin statistics |
| `zones` | Manage CDN zones (list, create, delete) |
| `purge` | Purge CDN cache (by URLs, tags, prefixes, or all) |
| `sign` | Generate signed URLs |
| `analytics` | View and sync analytics |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime |

### Zones

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/zones` | Create zone (body: `name`, `provider`, `domain`, `origin_url?`, `config?`, `enabled?`) |
| `GET` | `/api/zones` | List zones (query: `provider?`, `limit?`, `offset?`) |
| `GET` | `/api/zones/:id` | Get zone details |

### Cache Purging

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/purge` | Purge selective cache (body: `zone_id`, `purge_type`, `urls?`, `tags?`, `prefixes?`) |
| `POST` | `/api/purge/all` | Purge entire cache for a zone (body: `zone_id`, `confirm: true`) |
| `GET` | `/api/purge/:id` | Get purge request status |

### Signed URLs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/sign` | Generate signed URL (body: `url`, `ttl?`, `ip_restriction?`, `max_access?`, `metadata?`) |
| `POST` | `/api/sign/batch` | Generate multiple signed URLs (body: `urls[]`, `ttl?`) |

### Analytics

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/analytics` | Query analytics (query: `zone_id?`, `start?`, `end?`, `limit?`) |
| `GET` | `/api/analytics/summary` | Get analytics summary (query: `zone_id?`, `days?`) |

### Sync & Stats

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/sync` | Trigger analytics sync from provider |
| `GET` | `/api/stats` | Get plugin statistics |

---

## CDN Providers

| Provider | Features | Config Required |
|----------|----------|-----------------|
| `cloudflare` | Purge by URL/tag/prefix/all, analytics, page rules | `CDN_CLOUDFLARE_API_TOKEN`, `CDN_CLOUDFLARE_ZONE_IDS` |
| `bunnycdn` | Purge by URL/all, bandwidth analytics, storage zones | `CDN_BUNNYCDN_API_KEY`, `CDN_BUNNYCDN_PULL_ZONE_IDS` |
| `fastly` | Purge by URL/surrogate key/all, real-time analytics | Provider-specific config in zone `config` JSONB |
| `akamai` | Purge by URL/CP code/all, traffic reports | Provider-specific config in zone `config` JSONB |

---

## Purge Types

| Type | Description |
|------|-------------|
| `urls` | Purge specific URLs (up to `CDN_PURGE_BATCH_SIZE` per request) |
| `tags` | Purge by cache tags / surrogate keys |
| `prefixes` | Purge by URL prefix (path-based) |
| `all` | Purge entire zone cache (requires `confirm: true`) |

---

## Signed URLs

Signed URLs use HMAC-SHA256 with the `CDN_SIGNING_KEY` to generate time-limited, optionally IP-restricted access tokens for CDN resources.

### Signature Generation

The signed URL includes query parameters:

- `expires` -- Unix timestamp when the URL expires
- `signature` -- HMAC-SHA256 hash of the URL path and expiry
- `ip` -- (optional) Client IP restriction
- `max` -- (optional) Maximum access count

### Example Response

```json
{
  "original_url": "https://cdn.example.com/files/report.pdf",
  "signed_url": "https://cdn.example.com/files/report.pdf?expires=1707782400&signature=a1b2c3...",
  "expires_at": "2026-02-12T12:00:00Z",
  "ttl": 3600
}
```

### Batch Signing

Use `POST /api/sign/batch` to sign multiple URLs in a single request. All URLs share the same TTL and options.

---

## Analytics

The plugin syncs analytics data from CDN providers and stores it locally for querying. Analytics include:

| Metric | Description |
|--------|-------------|
| `requests` | Total HTTP requests |
| `bandwidth` | Total bandwidth (bytes) |
| `visitors` | Unique visitor count |
| `status_2xx` | Successful responses |
| `status_3xx` | Redirects |
| `status_4xx` | Client errors |
| `status_5xx` | Server errors |
| `top_paths` | Most requested paths |
| `top_countries` | Traffic by country |

### Analytics Summary

`GET /api/analytics/summary` returns aggregated metrics over a time period:

```json
{
  "total_requests": 1250000,
  "total_bandwidth": 5368709120,
  "avg_daily_requests": 178571,
  "cache_hit_rate": 94.2,
  "top_paths": [
    { "path": "/images/hero.jpg", "requests": 45000 },
    { "path": "/api/data.json", "requests": 32000 }
  ],
  "top_countries": [
    { "country": "US", "requests": 650000 },
    { "country": "GB", "requests": 180000 }
  ]
}
```

---

## Database Schema

### `np_cdn_zones`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Zone ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Zone display name |
| `provider` | `VARCHAR(50)` | `cloudflare`, `bunnycdn`, `fastly`, `akamai` |
| `provider_zone_id` | `VARCHAR(255)` | Provider-specific zone identifier |
| `domain` | `VARCHAR(255)` | Zone domain |
| `origin_url` | `TEXT` | Origin server URL |
| `config` | `JSONB` | Provider-specific configuration |
| `enabled` | `BOOLEAN` | Whether zone is active |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_cdn_purge_requests`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Purge request ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `zone_id` | `UUID` (FK) | References `np_cdn_zones` |
| `purge_type` | `VARCHAR(20)` | `urls`, `tags`, `prefixes`, `all` |
| `targets` | `TEXT[]` | URLs, tags, or prefixes to purge |
| `status` | `VARCHAR(20)` | `pending`, `processing`, `completed`, `failed` |
| `provider_response` | `JSONB` | Raw response from CDN provider |
| `error_message` | `TEXT` | Error details (if failed) |
| `requested_at` | `TIMESTAMPTZ` | Request timestamp |
| `completed_at` | `TIMESTAMPTZ` | Completion timestamp |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `np_cdn_analytics`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Analytics record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `zone_id` | `UUID` (FK) | References `np_cdn_zones` |
| `date` | `DATE` | Analytics date |
| `requests` | `BIGINT` | Total requests |
| `bandwidth` | `BIGINT` | Total bandwidth (bytes) |
| `visitors` | `INTEGER` | Unique visitors |
| `status_2xx` | `INTEGER` | 2xx responses |
| `status_3xx` | `INTEGER` | 3xx responses |
| `status_4xx` | `INTEGER` | 4xx responses |
| `status_5xx` | `INTEGER` | 5xx responses |
| `cache_hits` | `INTEGER` | Cache hit count |
| `cache_misses` | `INTEGER` | Cache miss count |
| `top_paths` | `JSONB` | Top requested paths |
| `top_countries` | `JSONB` | Top countries by traffic |
| `metadata` | `JSONB` | Additional provider-specific metrics |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `np_cdn_signed_urls`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `original_url` | `TEXT` | Original unsigned URL |
| `signed_url` | `TEXT` | Generated signed URL |
| `signature` | `VARCHAR(128)` | HMAC-SHA256 signature |
| `expires_at` | `TIMESTAMPTZ` | URL expiration |
| `ip_restriction` | `VARCHAR(45)` | Optional IP restriction |
| `max_access` | `INTEGER` | Maximum access count |
| `access_count` | `INTEGER` | Current access count |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

---

## Database Views

### `np_cdn_bandwidth_by_zone`

Aggregates total bandwidth per zone with daily breakdown.

### `np_cdn_cache_hit_rate`

Calculates cache hit rate percentage per zone: `cache_hits / (cache_hits + cache_misses) * 100`.

### `np_cdn_top_paths`

Aggregates most-requested paths across all zones.

---

## Troubleshooting

**"CDN provider API key not configured"** -- Set the appropriate `CDN_CLOUDFLARE_API_TOKEN` or `CDN_BUNNYCDN_API_KEY` for your provider.

**Purge request fails** -- Verify the zone exists and the provider API key has purge permissions. For Cloudflare, the token needs "Cache Purge" permission. Check `CDN_PURGE_BATCH_SIZE` if purging many URLs.

**Signed URLs rejected** -- Verify `CDN_SIGNING_KEY` is set and matches what the CDN edge is configured to validate. Check that the URL has not expired (`expires` parameter).

**Analytics empty** -- Run `POST /sync` to trigger an analytics sync from the provider. Verify the provider API key has analytics read permissions. Analytics sync runs automatically at `CDN_ANALYTICS_SYNC_INTERVAL`.

**Zone not syncing** -- Ensure the zone is `enabled: true` and the `provider_zone_id` matches the actual zone ID in the CDN provider dashboard.
