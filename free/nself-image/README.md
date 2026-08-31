# nself-image

Server-side image processing plugin for nSelf: resize, crop, format conversion (WebP/AVIF/JPEG/PNG), EXIF strip, and MinIO-integrated upload pipeline. Replaces per-app Sharp/Node.js usage across nFamily, nChat, and any consumer app needing image normalization.

> **Status: planned.** This plugin is specced (see `plugin.json`) but not yet implemented.

## Details

- **Category:** media
- **Tier:** pro
- **Language:** rust
- **Port:** 3852
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `IMAGE_PROCESSING_SHARED_SECRET` | Yes | — |
| `IMAGE_PROCESSING_MINIO_SECRET_KEY` | Yes | — |
| `IMAGE_PROCESSING_MINIO_ACCESS_KEY` | Yes | — |
| `IMAGE_PROCESSING_MINIO_ENDPOINT` | Yes | — |
| `IMAGE_PROCESSING_PORT` | No | — |
| `IMAGE_PROCESSING_MAX_INPUT_BYTES` | No | — |
| `IMAGE_PROCESSING_MAX_CONCURRENT` | No | — |
| `IMAGE_AUDIT_ENABLED` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/process` | hmac |  |
| `GET` | `/health` | none |  |
| `GET` | `/metrics` | none |  |

## Dependencies

Requires: `minio`

## Install

```bash
nself plugin install nself-image
```
