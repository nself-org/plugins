# nself-scan

Server-side file scanning for MinIO uploads: magic-byte MIME validation, ClamAV virus/malware scanning (always free, Security-Always-Free Doctrine), and optional CSAM hash detection (deferred — requires partner agreement)

> **Status: planned.** This plugin is specced (see `plugin.json`) but not yet implemented.

## Details

- **Category:** compliance
- **Tier:** free
- **Language:** go
- **Port:** 3829
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `DATABASE_URL` | — | — |
| `SCAN_PORT` | — | — |
| `SCAN_CLAMD_ADDR` | — | — |
| `SCAN_CLAMD_TIMEOUT_SECONDS` | — | — |
| `SCAN_CSAM_ENABLED` | — | — |
| `SCAN_CSAM_HASH_DB_PATH` | — | — |
| `SCAN_CSAM_PROVIDER` | — | — |
| `SCAN_MAX_SYNC_SIZE_MB` | — | — |
| `SCAN_MINIO_WEBHOOK_SECRET` | — | — |
| `SCAN_BLOCKED_MIME_TYPES` | — | — |
| `SCAN_FRESHCLAM_INTERVAL_HOURS` | — | — |

## Dependencies

Requires: `object-storage` · Optional: `database`

## Install

```bash
nself plugin install nself-scan
```
