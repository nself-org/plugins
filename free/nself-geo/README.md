# nself-geo

Forward and reverse geocoding with provider-agnostic caching layer. Nominatim (free, OSM) is the default; Google Places and Mapbox are premium fallbacks. Exposes geocodeAddress, reverseGeocode, geocodeBatch, clearGeoCache via Hasura Remote Schema.

> **Status: planned.** This plugin is specced (see `plugin.json`) but not yet implemented.

## Details

- **Category:** integrations
- **Tier:** pro
- **Language:** go
- **Port:** 3203
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | — |
| `GEOCODING_PROVIDERS` | No | Default: `nominatim` |
| `GEOCODING_GOOGLE_API_KEY` | No | Default: `` |
| `GEOCODING_MAPBOX_ACCESS_TOKEN` | No | Default: `` |
| `GEOCODING_NOMINATIM_URL` | No | Default: `https://nominatim.openstreetmap.org` |
| `GEOCODING_NOMINATIM_EMAIL` | No | Default: `` |
| `GEOCODING_CACHE_TTL_DAYS` | No | Default: `365` |
| `GEOCODING_CACHE_ENABLED` | No | Default: `true` |
| `GEOCODING_MAX_BATCH_SIZE` | No | Default: `100` |
| `GEOCODING_REDIS_URL` | No | Default: `` |
| `GEO_CACHE_TTL_SECONDS` | No | Default: `86400` |
| `GEO_RATE_LIMIT_RPM` | No | Default: `500` |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/geo/forward` | bearer | Forward geocode: address to lat/lng |
| `POST` | `/geo/reverse` | bearer | Reverse geocode: lat/lng to city/state/country |
| `POST` | `/geo/batch` | bearer | Batch geocode up to 100 addresses |
| `DELETE` | `/geo/cache` | admin | Clear stale cache entries (admin only) |
| `GET` | `/health` | none | Liveness probe |

## Install

```bash
nself plugin install nself-geo
```
