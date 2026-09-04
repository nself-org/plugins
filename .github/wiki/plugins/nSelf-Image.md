# nSelf Image Plugin

> Server-side image processing plugin for nSelf: resize, crop, format conversion (WebP/AVIF/JPEG/PNG), EXIF strip, and MinIO-integrated upload pipeline. **Free — MIT licensed.**

## Install

```bash
nself plugin install nself-image
```

No license key required.

## Description

Server-side image processing plugin for nSelf: resize, crop, format conversion (WebP/AVIF/JPEG/PNG), EXIF strip, and MinIO-integrated upload pipeline. Replaces per-app Sharp/Node.js usage across nFamily, nChat, and any consumer app needing image normalization.

Category: `media`. Current version: `0.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `IMAGE_PROCESSING_MINIO_ENDPOINT` | `(required)` | - |
| `IMAGE_PROCESSING_MINIO_ACCESS_KEY` | `(required)` | - |
| `IMAGE_PROCESSING_MINIO_SECRET_KEY` | `(required)` | - |
| `IMAGE_PROCESSING_SHARED_SECRET` | `(required)` | - |
| `IMAGE_PROCESSING_PORT` | `3852` | - |
| `IMAGE_PROCESSING_MAX_INPUT_BYTES` | `-` | - |
| `IMAGE_PROCESSING_MAX_CONCURRENT` | `-` | - |
| `IMAGE_AUDIT_ENABLED` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3852 | nSelf Image service port |

## Database Schema

1 table(s) added to your Postgres database:

- `np_image.jobs`

## REST API

```
POST   /process
GET    /health
GET    /metrics
```

## Examples

### Health check

```bash
curl http://localhost:3852/health
```

## Source

[`plugins/nself-image/`](https://github.com/nself-org/plugins/tree/main/nself-image)

Manifest: [`plugins/nself-image/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-image/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
