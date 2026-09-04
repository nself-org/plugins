# nSelf Geo Plugin

> Forward and reverse geocoding with provider-agnostic caching layer. **Free — MIT licensed.**

## Install

```bash
nself plugin install nself-geo
```

No license key required.

## Description

Forward and reverse geocoding with provider-agnostic caching layer. Nominatim (free, OSM) is the default; Google Places and Mapbox are premium fallbacks. Exposes geocodeAddress, reverseGeocode, geocodeBatch, clearGeoCache via Hasura Remote Schema.

Category: `location`. Current version: `1.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `GEOCODING_PROVIDERS` | `nominatim` | - |
| `GEOCODING_GOOGLE_API_KEY` | `-` | - |
| `GEOCODING_MAPBOX_ACCESS_TOKEN` | `-` | - |
| `GEOCODING_NOMINATIM_URL` | `https://nominatim.openstreetmap.org` | - |
| `GEOCODING_NOMINATIM_EMAIL` | `-` | - |
| `GEOCODING_CACHE_TTL_DAYS` | `365` | - |
| `GEOCODING_CACHE_ENABLED` | `true` | - |
| `GEOCODING_MAX_BATCH_SIZE` | `100` | - |
| `GEOCODING_REDIS_URL` | `-` | - |
| `GEO_CACHE_TTL_SECONDS` | `86400` | - |
| `GEO_RATE_LIMIT_RPM` | `500` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3203 | nSelf Geo service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_geo.forward_cache`
- `np_geo.reverse_cache`

## REST API

```
POST   /geo/forward
POST   /geo/reverse
POST   /geo/batch
DELETE /geo/cache
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:3203/health
```

## Source

[`plugins/nself-geo/`](https://github.com/nself-org/plugins/tree/main/nself-geo)

Manifest: [`plugins/nself-geo/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-geo/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
