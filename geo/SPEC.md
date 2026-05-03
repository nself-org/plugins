# nself-geo Plugin Specification

**Plugin:** `geo`
**Port:** 3203 (grandfather port — predates 3820-3849 block; intentionally outside range, locked per PPI port registry)
**Language:** Go
**Category:** location
**License:** MIT (free plugin)
**Bundle:** None — free, no license required
**Status:** Planned — v1.1.0
**SPEC version:** 1.0.0 (authored P98 S98-04)

---

## 1. Overview

`nself-geo` provides provider-agnostic geocoding and reverse-geocoding for any nSelf-powered application. It abstracts over multiple upstream geocoding APIs (Nominatim/OSM by default; Google Geocoding API and Mapbox optionally) behind a single stable HTTP surface, with optional Redis caching to avoid rate limits and reduce costs.

**Primary use cases:**
- Forward geocoding: resolve address string → (lat, lng)
- Reverse geocoding: resolve (lat, lng) → formatted address + components
- Location-aware search enrichment (nclaw, nchat user profiles, ntv regional content)
- nFamily location tagging on photo albums and social posts

**Non-goals (deferred to v2):**
- Routing / directions (use a dedicated routing plugin)
- Distance matrix computations
- Static map tile serving

---

## 2. Architecture

```
Client App
    │
    ▼
Nginx (HTTPS)
    │  /geo/* → http://127.0.0.1:3203
    ▼
nself-geo service (Go, port 3203)
    │             │
    ▼             ▼
Redis cache   Upstream geocoding API
(optional)    (Nominatim default / Google / Mapbox)
```

The service is **stateless by design** — no PostgreSQL tables. All persistence is optional Redis caching. This keeps the plugin zero-migration (no Drizzle migrations required) and trivially deployable.

### Key components

| Component | Purpose |
|-----------|---------|
| `cmd/server/main.go` | HTTP server bootstrap, graceful shutdown |
| `internal/geo/handler.go` | HTTP handlers for `/geocode`, `/reverse`, `/health` |
| `internal/geo/provider.go` | Provider interface + factory (`nominatim`, `google`, `mapbox`) |
| `internal/geo/cache.go` | Redis cache layer with TTL and key normalization |
| `internal/geo/ratelimit.go` | Per-provider rate limiting (token bucket) |
| `internal/geo/types.go` | Shared request/response structs |
| `plugin.json` | Plugin manifest |

---

## 3. Multi-Tenant Convention Wall Compliance

**Classification:** Stateless service — no `np_*` tables.

Per PPI Hard Rule (Multi-Tenant Convention Wall):
- `source_account_id` (multi-app isolation) — **NOT APPLICABLE**: no DB tables exist.
- `tenant_id` (Cloud customer isolation) — **NOT APPLICABLE**: stateless service with no persistent rows.

**Wall compliance:** DECLARED COMPLIANT (stateless). Any future addition of DB tables MUST declare both columns per PPI convention, with Hasura row filters on `tenant_id` for Cloud deployments.

Cache keys include `source_account_id` as a namespace segment when the plugin operates in a multi-app deploy:

```
geo:{provider}:{source_account_id}:{lat_4dp}_{lng_4dp}
geo:{provider}:{source_account_id}:{address_hash}
```

When `GEO_MULTI_APP_MODE=false` (single-app default), `source_account_id` segment is omitted from cache keys.

---

## 4. Data Model

No PostgreSQL tables. Zero migrations required.

### Optional Redis cache

| Key pattern | Value | TTL |
|-------------|-------|-----|
| `geo:{provider}:{account}:{lat_4dp}_{lng_4dp}` | JSON `ReverseResult` | `GEO_CACHE_TTL_SECONDS` (default 86400 = 24h) |
| `geo:{provider}:{account}:{address_sha256_8char}` | JSON `ForwardResult` | `GEO_CACHE_TTL_SECONDS` (default 86400 = 24h) |
| `geo:ratelimit:{provider}:{minute_bucket}` | int counter | 60s |

**Lat/lng normalization:** truncated to 4 decimal places (~11m precision) before cache key construction. Prevents cache fragmentation from trivially different coordinates.

**Address hash:** first 8 hex chars of SHA-256 of the lowercased, trimmed address string. Collision risk negligible for typical installation sizes.

### Cache eviction policy

Redis must be configured with `maxmemory-policy allkeys-lru` for this plugin. If Redis is unavailable, the plugin falls through to the upstream API — Redis is never a hard dependency.

---

## 5. API Endpoints

All endpoints require `Authorization: Bearer <nself-internal-token>` when `GEO_REQUIRE_AUTH=true` (default: false for backward compat with existing open deployments; recommended: true for production).

### GET /geocode

Forward geocoding: address string → coordinates.

**Query parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `address` | string | Yes | Free-form address string |
| `language` | string | No | IETF BCP-47 language code for result labels (default: `en`) |
| `provider` | string | No | Override default provider: `nominatim`, `google`, `mapbox` |
| `source_account_id` | string | No | Multi-app isolation namespace (default: `primary`) |

**Response 200:**

```json
{
  "lat": 40.7128,
  "lng": -74.0060,
  "formatted_address": "New York, NY, USA",
  "confidence": 0.95,
  "provider": "nominatim",
  "cached": false,
  "components": {
    "city": "New York",
    "state": "New York",
    "country": "United States",
    "country_code": "US"
  }
}
```

**Response 404:** address not found.
**Response 429:** upstream rate limit exceeded (cache miss + API limit).
**Response 502:** upstream provider unreachable.

---

### GET /reverse

Reverse geocoding: coordinates → address.

**Query parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `lat` | float64 | Yes | Latitude (-90 to 90) |
| `lng` | float64 | Yes | Longitude (-180 to 180) |
| `language` | string | No | IETF BCP-47 language code (default: `en`) |
| `provider` | string | No | Override default provider |
| `source_account_id` | string | No | Multi-app isolation namespace (default: `primary`) |

**Response 200:**

```json
{
  "formatted_address": "20 W 34th St, New York, NY 10001, USA",
  "provider": "nominatim",
  "cached": true,
  "components": {
    "house_number": "20",
    "road": "West 34th Street",
    "city": "New York",
    "postcode": "10001",
    "state": "New York",
    "country": "United States",
    "country_code": "US"
  }
}
```

**Response 404:** no address found at coordinates.
**Response 422:** invalid lat/lng range.

---

### GET /health

**Response 200:**

```json
{
  "status": "ok",
  "provider": "nominatim",
  "cache": "connected",
  "uptime_seconds": 3600
}
```

`cache` field is `"disabled"` when Redis is not configured, `"connected"` or `"disconnected"` when Redis is configured.

---

### POST /batch (v1.1 — deferred to v2)

Batch geocoding for up to 100 addresses in a single call. Deferred to avoid upstream rate limit complexity on initial launch.

---

## 6. Provider Strategy

### Nominatim / OpenStreetMap (default)

- **Cost:** Free, open data
- **Rate limit:** 1 req/sec per IP per Nominatim usage policy
- **Config:** `GEO_NOMINATIM_URL` (default: `https://nominatim.openstreetmap.org`)
- **Self-host option:** operators may run their own Nominatim instance; set `GEO_NOMINATIM_URL` to the local address
- **Privacy:** queries contain address text — users should self-host Nominatim if PII geocoding is a concern

### Google Geocoding API (optional)

- **Cost:** $5 per 1,000 requests after free tier
- **Rate limit:** 50 req/sec with standard quota
- **Config:** `GEO_PROVIDER=google`, `GEO_GOOGLE_API_KEY`
- **When to use:** higher accuracy for ambiguous addresses; better international support

### Mapbox Geocoding API (optional)

- **Cost:** $0.50 per 1,000 requests after free tier
- **Rate limit:** 600 req/min
- **Config:** `GEO_PROVIDER=mapbox`, `GEO_MAPBOX_ACCESS_TOKEN`
- **When to use:** tight Mapbox ecosystem integration (e.g., rendering Mapbox tiles alongside)

### Fallback chain

When `GEO_FALLBACK_ENABLED=true` (default: false), the provider chain cascades:

```
Primary provider → Secondary provider (on error) → Cache stale-serve (on both errors)
```

Primary and secondary providers configured via `GEO_PROVIDER` (primary) and `GEO_FALLBACK_PROVIDER` (secondary).

---

## 7. Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `GEO_PORT` | `3203` | No | HTTP listen port |
| `GEO_PROVIDER` | `nominatim` | No | Default geocoding provider |
| `GEO_NOMINATIM_URL` | `https://nominatim.openstreetmap.org` | No | Nominatim base URL |
| `GEO_GOOGLE_API_KEY` | — | When provider=google | Google Maps Geocoding API key |
| `GEO_MAPBOX_ACCESS_TOKEN` | — | When provider=mapbox | Mapbox secret token |
| `GEO_CACHE_ENABLED` | `false` | No | Enable Redis geocode cache |
| `GEO_REDIS_URL` | `redis://127.0.0.1:6379` | When cache enabled | Redis connection URL |
| `GEO_CACHE_TTL_SECONDS` | `86400` | No | Cache TTL (24h default) |
| `GEO_REQUIRE_AUTH` | `false` | No | Require Bearer token on all endpoints |
| `GEO_RATE_LIMIT_PER_MIN` | `60` | No | Per-client rate limit (0 = disabled) |
| `GEO_MULTI_APP_MODE` | `false` | No | Namespace cache keys by source_account_id |
| `GEO_FALLBACK_ENABLED` | `false` | No | Enable provider fallback chain |
| `GEO_FALLBACK_PROVIDER` | — | When fallback enabled | Secondary provider name |
| `GEO_LOG_LEVEL` | `info` | No | Logging level: debug/info/warn/error |

---

## 8. Plugin Manifest

```json
{
  "name": "geo",
  "displayName": "Geocoding",
  "version": "1.0.0",
  "category": "location",
  "description": "Provider-agnostic geocoding and reverse-geocoding with optional Redis caching. Supports Nominatim (default, free), Google Maps, and Mapbox.",
  "isCommercial": false,
  "licenseType": "free",
  "requires_license": false,
  "requiredEntitlements": [],
  "language": "go",
  "port": 3203,
  "portNote": "Grandfather port — assigned before 3820-3849 block; locked permanently per PPI port registry",
  "dependencies": {
    "redis": "optional"
  },
  "envVars": [
    "GEO_PORT",
    "GEO_PROVIDER",
    "GEO_NOMINATIM_URL",
    "GEO_GOOGLE_API_KEY",
    "GEO_MAPBOX_ACCESS_TOKEN",
    "GEO_CACHE_ENABLED",
    "GEO_REDIS_URL",
    "GEO_CACHE_TTL_SECONDS",
    "GEO_REQUIRE_AUTH",
    "GEO_RATE_LIMIT_PER_MIN",
    "GEO_MULTI_APP_MODE",
    "GEO_FALLBACK_ENABLED",
    "GEO_FALLBACK_PROVIDER",
    "GEO_LOG_LEVEL"
  ],
  "endpoints": [
    "GET /geocode",
    "GET /reverse",
    "GET /health"
  ],
  "hasura": {
    "remoteSchema": true,
    "actions": ["geocodeAddress", "reverseGeocode"]
  },
  "source_account_id_support": "cache_namespace_only",
  "tags": ["geocoding", "location", "maps", "nominatim", "google-maps", "mapbox"]
}
```

---

## 9. Hasura Integration

The plugin registers as a Hasura Remote Schema, exposing two GraphQL actions:

### Action: geocodeAddress

```graphql
type Query {
  geocodeAddress(
    address: String!
    language: String
    provider: GeoProvider
    source_account_id: String
  ): GeoForwardResult
}

type GeoForwardResult {
  lat: Float!
  lng: Float!
  formatted_address: String!
  confidence: Float!
  provider: GeoProvider!
  cached: Boolean!
  components: GeoComponents!
}
```

### Action: reverseGeocode

```graphql
type Query {
  reverseGeocode(
    lat: Float!
    lng: Float!
    language: String
    provider: GeoProvider
    source_account_id: String
  ): GeoReverseResult
}

type GeoReverseResult {
  formatted_address: String!
  provider: GeoProvider!
  cached: Boolean!
  components: GeoComponents!
}
```

### Shared types

```graphql
enum GeoProvider {
  nominatim
  google
  mapbox
}

type GeoComponents {
  house_number: String
  road: String
  city: String
  district: String
  postcode: String
  state: String
  country: String
  country_code: String
}
```

### Hasura permissions

All roles (user, admin, public) can call geocoding actions — no data at rest to protect. The plugin performs no row-level authorization because it is stateless.

| Role | geocodeAddress | reverseGeocode |
|------|---------------|----------------|
| admin | Allow | Allow |
| user | Allow | Allow |
| anonymous | Allow (when `GEO_REQUIRE_AUTH=false`) | Allow |

---

## 10. Security Controls

### Input validation

- `address` parameter: max 512 characters, stripped of HTML/script tags, URL-decoded before processing
- `lat` range: -90.0 to 90.0 (422 on violation)
- `lng` range: -180.0 to 180.0 (422 on violation)
- Provider override: enum validation against known providers (nominatim, google, mapbox)
- `language` code: BCP-47 format validation (2-5 chars, letters only)

### Rate limiting (Security-Always-Free)

Per-client IP rate limiting is FREE and default-on when `GEO_RATE_LIMIT_PER_MIN > 0`. Uses token bucket algorithm backed by in-memory store (no Redis required for rate limiting). Protects against upstream API quota exhaustion and scraping.

```go
// Rate limit implementation — no license required
func (h *Handler) rateLimitMiddleware(next http.Handler) http.Handler {
    // token bucket per client IP
    // GEO_RATE_LIMIT_PER_MIN tokens per minute
    // burst: 2× rate limit
}
```

### Upstream API key protection

When Google or Mapbox keys are configured:
- Keys stored only in env vars, never logged
- No key exposure in health endpoint or API responses
- Key used server-side only (never proxied to client)

### Privacy note for PII addresses

Nominatim default: address strings are sent to `nominatim.openstreetmap.org`. For deployments where geocoded addresses are PII (e.g., user home addresses), operators SHOULD:
1. Self-host Nominatim (`GEO_NOMINATIM_URL=http://localhost:8088`)
2. Or use Google/Mapbox with appropriate DPA coverage

This note is surfaced in `nself doctor --plugins geo`.

---

## 11. Doctor Dependency Check

`nself doctor --plugins geo` validates:

| Check ID | Severity | Condition | Message |
|----------|---------|-----------|---------|
| `GEO-CONN-01` | INFO | `GEO_CACHE_ENABLED=true` AND Redis unreachable | "Redis unavailable — geocoding will work but caching is disabled. Set GEO_CACHE_ENABLED=false to suppress this warning." |
| `GEO-KEY-01` | WARNING | `GEO_PROVIDER=google` AND `GEO_GOOGLE_API_KEY` empty | "Google geocoding selected but GEO_GOOGLE_API_KEY is not set. Requests will fail." |
| `GEO-KEY-02` | WARNING | `GEO_PROVIDER=mapbox` AND `GEO_MAPBOX_ACCESS_TOKEN` empty | "Mapbox geocoding selected but GEO_MAPBOX_ACCESS_TOKEN is not set. Requests will fail." |
| `GEO-RATE-01` | INFO | `GEO_PROVIDER=nominatim` AND `GEO_RATE_LIMIT_PER_MIN` > 60 | "Nominatim usage policy requires max 1 req/sec. Current rate limit exceeds policy. Consider enabling cache." |
| `GEO-PRIV-01` | INFO | `GEO_PROVIDER=nominatim` AND `GEO_NOMINATIM_URL` contains `openstreetmap.org` | "Addresses are sent to public Nominatim. For PII addresses, self-host Nominatim or switch to Google/Mapbox." |
| `GEO-PORT-01` | INFO | Always | "Port 3203 is a grandfather port — assigned before the 3820-3849 reserved block. Never reassign." |

---

## 12. Competitive Parity

| Feature | nself-geo | Google Maps Geocoding | Mapbox | OpenCage |
|---------|-----------|----------------------|--------|----------|
| Forward geocoding | Yes | Yes | Yes | Yes |
| Reverse geocoding | Yes | Yes | Yes | Yes |
| Address components | Yes | Yes | Yes | Yes |
| Batch geocoding | v2 | Yes (50/req) | Yes (50/req) | Yes (100/req) |
| Multi-provider | Yes | No | No | No |
| Self-hostable | Yes (Nominatim) | No | No | No |
| Redis cache | Yes | No (client-side) | No | No |
| Cost (10k req/day) | $0 (Nominatim) | ~$5/day | ~$1.50/day | ~$2/day |
| Language support | 40+ via Nominatim | 50+ | 60+ | 130+ |
| Confidence score | Provider-dependent | Yes | Yes | Yes |

**Key differentiators:**
1. Single stable API regardless of upstream provider
2. Built-in Redis cache eliminates upstream costs at scale
3. Self-hostable by default (Nominatim) with zero external dependencies
4. Provider fallback chain for reliability

---

## 13. Test Plan

### Tier 1 — Unit tests (≥40% coverage)

- Provider adapter tests: mock HTTP client, verify request/response mapping for each provider
- Cache layer tests: hit/miss/TTL expiry, key normalization (4dp truncation, address hashing)
- Input validation tests: lat/lng bounds, address length, provider enum, language format
- Rate limiter tests: token bucket refill, burst handling, per-IP isolation

### Tier 2 — Integration tests (≥20% coverage)

- `httptest.NewServer` for all three endpoints with real cache layer (mock Redis via `miniredis`)
- Provider fallback: primary returns 5xx → secondary called → cache stale-serve
- Multi-app cache namespace isolation: two source_account_ids produce separate cache entries

### Tier 3 — E2E scaffold (≥5% coverage)

- Nominatim live call (skipped in CI, opt-in via `GEO_RUN_LIVE_TESTS=true`)
- Health endpoint returns 200 with correct provider field

### Test prohibitions

- No real network calls in unit or integration tests
- No real Redis in unit tests (use miniredis or in-memory mock)
- No hardcoded lat/lng from PII locations

---

## 14. Port Registry — Grandfather Annotation

**Port 3203** is the canonical port for `nself-geo`.

**Grandfather rationale:** This port was assigned to the geocoding plugin before the 3820-3849 reserved block was established in P98. The port falls intentionally outside the block. It is LOCKED permanently to `nself-geo` — it MUST NOT be reassigned, recycled, or shifted into the 3820-3849 range.

This annotation must appear in:
- `plugin.json` (`portNote` field)
- PPI port registry table
- SPORT F09 (port inventory)
- `nself doctor` output (GEO-PORT-01 info check)

See also: T2-23 (grandfather port annotation requirement).

---

## 15. Bundle Classification

**Free MIT plugin — no license required.**

nself-geo ships as a free plugin in `plugins/geo/`. Any nSelf user can install it without a license key:

```bash
nself plugin install geo
```

It is intentionally free because:
1. Location-awareness is a baseline capability (like auth or storage)
2. Nominatim is free and self-hostable — charging for the plugin would be inconsistent with the free upstream
3. Differentiates nSelf vs paid-only geocoding SaaS competitors

**Paid extension:** A future `geo-cloud` extension in `plugins-pro/paid/geo-cloud/` could add managed-Nominatim-instance provisioning (Cloud MAX tier), batch geocoding, and SLA-backed uptime. This is deferred to v2.

---

## 16. Cross-Plugin Coordination

| Plugin | Relationship |
|--------|-------------|
| `photos` (nFamily bundle) | Uses `reverseGeocode` to tag photo albums with location names |
| `social` (nFamily bundle) | Uses `geocodeAddress` for location-aware posts |
| `cms` | Optional — CMS content can have geo-tagged location fields |
| `analytics` | Can consume anonymized lat/lng data for regional analytics |
| `realtime` | Proximity-based pub/sub use case — geo enriches subscription filters |

---

## 17. Migration Plan

**No migrations required.** Stateless plugin.

Installation checklist:
1. `nself plugin install geo`
2. Configure `GEO_PROVIDER` in `.env.dev` / `.env.prod` (default: nominatim)
3. (Optional) Set `GEO_CACHE_ENABLED=true` and `GEO_REDIS_URL` if Redis is running
4. (Optional) Set provider API key for Google/Mapbox
5. `nself build` to regenerate docker-compose
6. `nself start` to launch

---

## 18. Rollout Plan

| Phase | Action |
|-------|--------|
| v1.1.0 | Ship geo plugin with Nominatim + Google + Mapbox providers; Redis cache; rate limiting |
| v1.1.x | Bug fixes; address `GET /batch` community demand signal |
| v1.2.0 | Batch geocoding (up to 100 addresses); provider streaming |
| v2.0.0 | `geo-cloud` extension — managed Nominatim; SLA uptime |

---

## 19. Observability

### Metrics (Prometheus)

| Metric | Type | Labels |
|--------|------|--------|
| `geo_requests_total` | counter | `endpoint`, `provider`, `status` |
| `geo_cache_hits_total` | counter | `endpoint`, `provider` |
| `geo_cache_misses_total` | counter | `endpoint`, `provider` |
| `geo_upstream_latency_seconds` | histogram | `provider` |
| `geo_rate_limit_rejections_total` | counter | `endpoint` |

### Structured logging

```json
{
  "level": "info",
  "ts": "2026-05-03T12:00:00Z",
  "msg": "geocode_request",
  "endpoint": "/geocode",
  "provider": "nominatim",
  "cached": false,
  "latency_ms": 145,
  "source_account_id": "primary"
}
```

No address strings in structured logs (privacy). Lat/lng logged at 2dp precision only.

---

## 20. Docs to Create

| Document | Location | Purpose |
|----------|---------|---------|
| `geo/README.md` | `plugins/geo/README.md` | Installation, env vars, quick start |
| Geocoding guide | `.github/wiki/plugins/geo.md` | Full provider configuration, caching setup, self-host Nominatim |
| SPORT F09 update | `.claude/temp/p98-sport-update-notes.md` | Add port 3203 to port inventory with grandfather annotation |
| SPORT F04 update | `.claude/temp/p98-sport-update-notes.md` | Add `geo` to free plugin count (+1) |
