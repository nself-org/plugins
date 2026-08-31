# storage-transform

On-the-fly image transformation CDN: resize, crop, format convert (WebP/AVIF/JPEG/PNG), quality, and device-pixel-ratio support. URL-param driven, Redis LRU cache, Nginx cache headers, rate limiting.

## Details

- **Category:** infrastructure
- **Tier:** pro
- **Language:** go
- **Port:** 3085
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `STORAGE_TRANSFORM_ENABLED` | No | — |
| `STORAGE_TRANSFORM_PORT` | No | — |
| `STORAGE_TRANSFORM_MAX_WIDTH` | No | — |
| `STORAGE_TRANSFORM_MAX_HEIGHT` | No | — |
| `STORAGE_TRANSFORM_CACHE_TTL_DAYS` | No | — |
| `STORAGE_TRANSFORM_RATE_LIMIT` | No | — |
| `STORAGE_TRANSFORM_SOURCE_STORAGE_URL` | No | — |
| `STORAGE_TRANSFORM_DISK_CACHE_DIR` | No | — |
| `STORAGE_TRANSFORM_SIGNING_SECRET` | No | — |
| `REDIS_URL` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/storage-transform/cache/purge` | bearer |  |
| `GET` | `/api/storage-transform/cache/stats` | bearer |  |
| `GET` | `/api/storage-transform/health` | bearer |  |
| `GET` | `/health` | bearer |  |
| `GET` | `/storage/v1/object/public/*` | bearer |  |
| `GET` | `/transform/*` | bearer |  |

## Install

```bash
nself plugin install storage-transform
```
