# Plugin Spec: nself-geo

**Bundle:** shared (nTV + accessible from any pro tier)
**Tier:** pro
**Target version:** v1.1.0
**Port:** 3203 (grandfather assignment — pre-P98, outside 3820–3849 block; do NOT reassign)
**Language:** Go
**Multi-tenant decision:** `tenant_id` for Cloud customer isolation (geocoding is a Cloud-pay feature); `source_account_id` for multi-app isolation (each app maintains its own cache namespace)

---

## §1 Overview

`nself-geo` provides forward geocoding (address to coordinates) and reverse geocoding (coordinates to city/state/country) with a caching layer to avoid per-call provider billing. The plugin is provider-agnostic: OpenStreetMap Nominatim is the free default; Google Places and Mapbox are premium fallbacks. All operations are exposed as Hasura Remote Schema actions callable from any nSelf-powered app.

**Why it exists:** Ummat Masjid Scraper (Sprint M1) and the masjid claim wizard (Sprint M2) depend directly on the Google Places API, creating a hard billing dependency. Nominatim as default brings geocoding cost to zero at Ummat's scale. The caching layer prevents repeat API calls for the same coordinates during the dedup pipeline.

**Note:** The plugin directory `plugins-pro/paid/geocoding/` contains a prior partial implementation. This SPEC.md is the authoritative design document for the full v1.1.0 implementation. Implementation should be verified against this spec before shipping — any existing code that satisfies a section here needs no rewrite.

---

## §2 Architecture

- **Service shape:** Go binary running as a Docker container, port 3203.
- **Hasura Remote Schema:** Exposes `geocodeAddress`, `reverseGeocode`, `geocodeBatch`, `clearGeoCache` (admin-only).
- **Database schema:** Role `np_geo`, schema `np_geo`.
- **Provider chain:** Nominatim (default, free) → Google Places (if `GEOCODING_GOOGLE_API_KEY` set) → Mapbox (if `GEOCODING_MAPBOX_ACCESS_TOKEN` set). First provider returning a result wins; fallback triggers on HTTP error or empty result. Nominatim rate-limit: 1 req/sec (OSM Acceptable Use Policy) — batch requests must throttle accordingly.
- **Cache-first:** Every request checks the DB cache before calling a provider. Cache is keyed by `(tenant_id, source_account_id, address_normalized)` for forward geocoding and `(tenant_id, source_account_id, lat_rounded, lng_rounded)` for reverse geocoding.

### §2a Cache Invalidation Strategy

Geocoding results are cached in PostgreSQL (`np_geo.forward_cache`, `np_geo.reverse_cache`) with a configurable TTL.

- **Default TTL:** 24 hours (configurable via `GEO_CACHE_TTL_SECONDS`, default `86400`).
- **Cache key (forward):** normalized address text + tenant/account scope.
- **Cache key (reverse):** `geo:{source}:{lat_4dp}:{lng_4dp}` — coordinates rounded to 4 decimal places (~11 m precision) to maximize cache hits.
- **Redis layer (optional):** When `GEOCODING_REDIS_URL` is set, results are additionally cached in Redis using the key format `geo:{source}:{lat_4dp}:{lng_4dp}` with TTL = `GEO_CACHE_TTL_SECONDS`. Redis cache is checked before Postgres cache. Defaults to Postgres-only when Redis is not configured.
- **Manual invalidation:** `nself geo cache-clear [--tenant <id>] [--account <id>] [--older-than <duration>]`. Runs `DELETE FROM np_geo.forward_cache WHERE expires_at < now()` and the equivalent for reverse cache.
- **Expiry:** rows with `expires_at < now()` are ignored on read and cleaned up by the `DELETE /geo/cache` endpoint (triggered by `nself geo cache-clear`).

---

## §3 Data Model

```sql
-- Schema and role
CREATE SCHEMA np_geo;
CREATE ROLE np_geo;
GRANT USAGE ON SCHEMA np_geo TO np_geo;

-- Forward geocode cache
CREATE TABLE np_geo.forward_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID,                                      -- Cloud: separates paying tenants
  source_account_id TEXT NOT NULL DEFAULT 'primary',   -- Multi-app: separates apps in same deploy
  address_normalized TEXT NOT NULL,                    -- lowercased, stripped whitespace
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  display_name TEXT,
  country_code TEXT,
  provider_used TEXT NOT NULL,
  provider_response JSONB,
  hit_count INT NOT NULL DEFAULT 1,
  cached_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, source_account_id, address_normalized)
);
CREATE INDEX idx_geo_forward_tenant ON np_geo.forward_cache (tenant_id, source_account_id);
CREATE INDEX idx_geo_forward_expires ON np_geo.forward_cache (expires_at);

-- Reverse geocode cache
CREATE TABLE np_geo.reverse_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  lat_rounded DOUBLE PRECISION NOT NULL,               -- rounded to 4 decimal places (~11m)
  lng_rounded DOUBLE PRECISION NOT NULL,
  city TEXT,
  state TEXT,
  country TEXT,
  country_code TEXT,
  postal_code TEXT,
  provider_used TEXT NOT NULL,
  provider_response JSONB,
  hit_count INT NOT NULL DEFAULT 1,
  cached_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, source_account_id, lat_rounded, lng_rounded)
);
CREATE INDEX idx_geo_reverse_tenant ON np_geo.reverse_cache (tenant_id, source_account_id);
CREATE INDEX idx_geo_reverse_coords ON np_geo.reverse_cache (lat_rounded, lng_rounded, tenant_id);
```

**Rollback:** `DROP SCHEMA np_geo CASCADE; DROP ROLE np_geo;`

---

## §4 Hasura Permissions

### Hasura Row-Level Security (RLS)

- `tenant_id` tables: Hasura row filter `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}` required on every `np_geo.*` table for Cloud multi-tenancy.
- `source_account_id` tables: Hasura permission filter `{"source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}}` for multi-app isolation.

### Role Matrix

| Role | geocodeAddress | reverseGeocode | geocodeBatch | clearGeoCache |
|------|---------------|----------------|--------------|---------------|
| admin | Allow | Allow | Allow (unlimited) | Allow |
| user | Allow | Allow | Allow (max 20/call) | Deny |
| service | Allow | Allow | Allow | Deny |
| anonymous | Deny | Deny | Deny | Deny |

### Remote Schema Permissions

All `np_geo.*` tables:
- `user` role: SELECT only, filtered by `source_account_id = X-Hasura-Source-Account-Id` AND `tenant_id = X-Hasura-Tenant-Id`
- `admin` role: full SELECT/INSERT/UPDATE/DELETE
- `service` role: SELECT/INSERT only

---

## §5 Bundle Membership

`nself-geo` is classified as **shared pro** (accessible from any bundle holding a pro license). Primary association is the **nTV bundle** (F06 UD-14, 2026-04-22). Self-hosters with any paid bundle can install it. nCloud customers get it as part of the Cloud pay tier.

This is a **Cloud-pay plugin** — the `tenant_id` column is present and enforced because geocoding with Redis caching and high-request-rate provider fallback is a Cloud-tier feature. Self-hosters with a bundle license get the Postgres-cache path; Redis caching layer is available to all.

---

## §6 Pricing Tier

| Context | Access |
|---|---|
| Free core (no license) | Not available |
| Any bundle ($0.99/mo) | Available — Postgres cache only |
| ɳSelf+ ($3.99/mo) | Available — Postgres + optional Redis cache |
| nCloud MAX | Available — full feature set, tenant isolation enforced |

---

## §7 Port Assignment

**Port: 3203** — grandfather assignment. Assigned before the P98 3820–3849 plugin block was established. This port must NOT be reassigned. SPORT F10 must annotate: `3203 = nself-geo (geocoding) — grandfather assignment, pre-P98`.

---

## §8 Multi-Tenant Convention Wall

Per the PPI Multi-Tenant Convention Wall Hard Rule:

| Column | Applied to | Semantics |
|---|---|---|
| `source_account_id TEXT NOT NULL DEFAULT 'primary'` | All `np_geo.*` tables | Isolates cache entries across independent apps within one nSelf deploy |
| `tenant_id UUID` (nullable) | All `np_geo.*` tables | Isolates paying Cloud customers — never used for multi-app isolation |

**Decision:** Both columns apply. Forward/reverse cache entries are namespaced by `(tenant_id, source_account_id)` together. A self-hosted single-app deploy uses `tenant_id = NULL` and `source_account_id = 'primary'`. A Cloud tenant gets `tenant_id = <uuid>`.

Hasura row filter for Cloud: `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}` on all `np_geo.*` tables.
Hasura row filter for multi-app: `{"source_account_id": {"_eq": "X-Hasura-Source-Account-Id"}}` additionally applied.

**NEVER** use `source_account_id` alone to separate Cloud customers. **NEVER** use `tenant_id` to separate apps within one deploy.

**Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_geo.*` tables.

---

## §9 Competitive Parity

Self-hosted geocoding alternatives are rare. Commercial APIs (Google Maps Geocoding, Mapbox, Here) charge per-request. At Ummat's masjid-scraper scale (50k+ locations), this creates a significant ongoing billing dependency. `nself-geo` with Nominatim as the free default brings that cost to zero. The PostgreSQL caching layer ensures that repeated lookups (common in dedup pipelines) never hit an external API. Competitors like Pelias require running their own search cluster; nself-geo delegates to Nominatim and caches the results locally — far simpler to operate. For teams that need Google Places precision, the provider-fallback chain gives them that option without locking in any single vendor.

---

## §10 Plugin Manifest

```json
{
  "name": "geocoding",
  "displayName": "Geocoding & Reverse Geocoding",
  "version": "1.0.0",
  "tier": "pro",
  "bundle": ["nTV", "shared"],
  "port": 3203,
  "language": "go",
  "category": "infrastructure",
  "tags": ["geocoding", "maps", "location", "reverse-geocoding", "places", "nominatim"],
  "systemDependencies": [],
  "tables": [
    "np_geo.forward_cache",
    "np_geo.reverse_cache"
  ],
  "hasuraRemoteSchema": true,
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "defaultValue": "primary"
  },
  "multiTenant": {
    "supported": true,
    "isolationColumn": "tenant_id",
    "hasuraFilter": "X-Hasura-Tenant-Id"
  },
  "env": {
    "required": ["DATABASE_URL"],
    "optional": {
      "GEOCODING_PLUGIN_PORT": "3203",
      "GEO_CACHE_TTL_SECONDS": "86400",
      "GEOCODING_CACHE_ENABLED": "true",
      "GEOCODING_PROVIDERS": "nominatim",
      "GEOCODING_GOOGLE_API_KEY": "",
      "GEOCODING_MAPBOX_ACCESS_TOKEN": "",
      "GEOCODING_NOMINATIM_URL": "https://nominatim.openstreetmap.org",
      "GEOCODING_NOMINATIM_EMAIL": "",
      "GEOCODING_MAX_BATCH_SIZE": "100",
      "GEOCODING_RATE_LIMIT_MAX": "500",
      "GEOCODING_RATE_LIMIT_WINDOW_MS": "60000",
      "GEOCODING_REDIS_URL": ""
    }
  },
  "routes": [
    {"method": "POST", "path": "/geo/forward", "auth": "hasura-jwt"},
    {"method": "POST", "path": "/geo/reverse", "auth": "hasura-jwt"},
    {"method": "POST", "path": "/geo/batch", "auth": "hasura-jwt"},
    {"method": "DELETE", "path": "/geo/cache", "auth": "admin-jwt"},
    {"method": "GET", "path": "/health", "auth": "none"}
  ]
}
```

---

## §11 Test Plan

### Unit Tests
- Address normalization: lowercasing, whitespace stripping, unicode handling
- Coordinate rounding to 4 decimal places
- Cache TTL computation and expiry check
- Provider fallback chain: mock provider 1 errors → assert fallback to provider 2
- Batch chunking: 150 items chunked into 2 calls (100 + 50)
- Nominatim rate-limiter: assert 1 req/sec throttle is enforced

### Integration Tests
- Docker Compose Postgres; mock Nominatim HTTP server
- Cache miss → provider call → DB write → cache hit on identical repeat
- `tenant_id` isolation: tenant A's cache entries not visible to tenant B
- `source_account_id` isolation: app "app1" cache not accessible to "app2"
- `DELETE /geo/cache` clears expired rows, leaves unexpired rows

### E2E Tests
- Hasura Action `geocodeAddress("1600 Pennsylvania Ave NW, Washington DC")` → assert lat ~38.897, lng ~-77.037
- `reverseGeocode(38.897, -77.037)` → assert city = "Washington"
- Batch: 5 addresses → assert 5 results returned

### Security Tests
- Verify `anonymous` role cannot invoke any geocoding action
- Verify `user` role batch is limited to max 20 items
- Confirm address text is hashed before logging (no PII in logs)
- Confirm Nominatim requests carry `User-Agent` with valid email (Acceptable Use compliance)

---

## §12 Migration Path

`nself-geo` (this spec) and the existing `geocoding` plugin at `plugins-pro/paid/geocoding/` represent the same plugin. Canonical name in registry and manifests remains `geocoding`; internal code and directory may use either. Steps for P98:

1. Write `plugins-pro/paid/geocoding/SPEC.md` (or symlink to this file) as the canonical in-repo spec.
2. CR-A review: compare existing `plugins-pro/paid/geocoding/` implementation against this spec. Identify gaps.
3. Gaps become implementation tickets (post-P98 CRUNCH).
4. No schema migration needed — if tables already exist with matching DDL, skip; if not, run migration.
5. Backwards compatibility: existing `GEOCODING_*` env var names are preserved (no renaming).

**Feature flag:** `GEOCODING_CACHE_ENABLED=true` (default on). Can disable cache for testing.

**Nominatim AUP:** warn on startup if `GEOCODING_NOMINATIM_EMAIL` is empty and Nominatim is the primary provider. Log: `WARN: GEOCODING_NOMINATIM_EMAIL not set; Nominatim Acceptable Use Policy requires a valid contact email in User-Agent`.

---

## §13 Observability

- **Metrics:** `geo_requests_total{op,provider,cache_hit}`, `geo_request_duration_seconds{op,provider}`, `geo_cache_size_rows{table}`
- **Logs:** Structured JSON `{op, address_hash, provider, cache_hit, duration_ms}`. Address hashed (SHA-256 truncated to 12 hex chars) before logging. Never log raw addresses.
- **Traces:** OpenTelemetry spans on `cache_lookup`, `provider_call`, `cache_write`
- **Alerts:**
  - `provider_error_rate > 5%` → WARNING
  - All configured providers failing → CRITICAL
  - `geo_request_duration_seconds p99 > 2s` → WARNING

---

## §14 Security Notes

- All provider API keys stored as env vars; never logged, never in DB.
- Address inputs are hashed before logging to prevent PII leakage in log aggregators.
- Rate limit: 500 req/min per tenant, enforced at service level before provider call.
- HMAC auth on admin routes (`DELETE /geo/cache`).
- `nself doctor --deep` check `GEO-001`: verify `GEOCODING_NOMINATIM_EMAIL` set when Nominatim is primary.

---

## §15 Shippability

**Target:** v1.1.0

**Blockers (none at spec stage):**
- Implementation review against existing `plugins-pro/paid/geocoding/` required before CRUNCH.
- Nominatim rate-limiter must be verified in existing code.

**Dependencies:**
- Postgres (required)
- Redis (optional, for Redis cache layer)
- External provider (Nominatim is free/default; others require API keys)

**Doc sync required at ship:**
- Update `plugins-pro/.github/docs/geocoding.md`
- Update `web/docs/src/content/plugins/geocoding.mdx`
- Update `plugins-pro/paid/geocoding/README.md`
- SPORT F04: no count change (plugin already exists); F06: update `geocoding` entry notes
