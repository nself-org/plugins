# nSelf Scan Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | planned ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install nself-scan` |
| **Requirements** | nSelf >=1.1.0 |

<!-- END GENERATED:catalog -->

> Server-side file scanning for MinIO uploads: magic-byte MIME validation, ClamAV virus/malware scanning (always free, Security-Always-Free Doctrine), and optional CSAM hash detection (deferred, requires partner agreement).

## Install

```bash
nself plugin install nself-scan
```

No license key required.

## Description

Server-side file scanning for MinIO uploads: magic-byte MIME validation, ClamAV virus/malware scanning (always free, Security-Always-Free Doctrine), and optional CSAM hash detection (deferred, requires partner agreement)

Category: `security`. Current version: `0.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `-` | PostgreSQL connection string |
| `SCAN_PORT` | `3829` | - |
| `SCAN_CLAMD_ADDR` | `-` | - |
| `SCAN_CLAMD_TIMEOUT_SECONDS` | `-` | - |
| `SCAN_CSAM_ENABLED` | `-` | - |
| `SCAN_CSAM_HASH_DB_PATH` | `-` | - |
| `SCAN_CSAM_PROVIDER` | `-` | - |
| `SCAN_MAX_SYNC_SIZE_MB` | `-` | - |
| `SCAN_MINIO_WEBHOOK_SECRET` | `-` | - |
| `SCAN_BLOCKED_MIME_TYPES` | `-` | - |
| `SCAN_FRESHCLAM_INTERVAL_HOURS` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3829 | nSelf Scan service port |

## Examples

```bash
nself plugin install nself-scan
```

## Source

[`plugins/nself-scan/`](https://github.com/nself-org/plugins/tree/main/nself-scan)

Manifest: [`plugins/nself-scan/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-scan/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
