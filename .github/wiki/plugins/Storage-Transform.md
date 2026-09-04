# Storage Transform Plugin

> On-the-fly image transformation CDN: resize, crop, format convert (WebP/AVIF/JPEG/PNG), quality, and device-pixel-ratio support. **Free — MIT licensed.**

## Install

```bash
nself plugin install storage-transform
```

No license key required.

## Description

On-the-fly image transformation CDN: resize, crop, format convert (WebP/AVIF/JPEG/PNG), quality, and device-pixel-ratio support. URL-param driven, Redis LRU cache, Nginx cache headers, rate limiting.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `STORAGE_TRANSFORM_ENABLED` | `-` | - |
| `STORAGE_TRANSFORM_PORT` | `3085` | - |
| `STORAGE_TRANSFORM_MAX_WIDTH` | `-` | - |
| `STORAGE_TRANSFORM_MAX_HEIGHT` | `-` | - |
| `STORAGE_TRANSFORM_CACHE_TTL_DAYS` | `-` | - |
| `STORAGE_TRANSFORM_RATE_LIMIT` | `-` | - |
| `STORAGE_TRANSFORM_SOURCE_STORAGE_URL` | `-` | - |
| `STORAGE_TRANSFORM_DISK_CACHE_DIR` | `-` | - |
| `STORAGE_TRANSFORM_SIGNING_SECRET` | `-` | - |
| `REDIS_URL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3085 | Storage Transform service port |

## REST API

```
POST   /api/storage-transform/cache/purge
GET    /api/storage-transform/cache/stats
GET    /api/storage-transform/health
GET    /health
GET    /storage/v1/object/public/*
GET    /transform/*
```

## Examples

### Health check

```bash
curl http://localhost:3085/health
```

## Source

[`plugins/storage-transform/`](https://github.com/nself-org/plugins/tree/main/storage-transform)

Manifest: [`plugins/storage-transform/plugin.json`](https://github.com/nself-org/plugins/tree/main/storage-transform/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
