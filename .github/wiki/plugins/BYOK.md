# BYOK Plugin

> Bring Your Own Key (BYOK) per-tenant encryption. **Free — MIT licensed.**

## Install

```bash
nself plugin install byok
```

No license key required.

## Description

Bring Your Own Key (BYOK) per-tenant encryption. Envelope encryption with customer-managed keys (CMK) via AWS KMS, GCP Cloud KMS, or HashiCorp Vault Transit. DEK wrapped by CMK. Satisfies HIPAA, FedRAMP High, FFIEC, and DORA key-control requirements. Enterprise-only.

Category: `compliance`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `BYOK_PLUGIN_PORT` | `3743` | - |
| `NSELF_BYOK` | `-` | - |
| `NSELF_BYOK_DEK_CACHE` | `-` | - |
| `NSELF_BYOK_DEK_CACHE_TTL` | `-` | - |
| `NSELF_BYOK_ROTATE_BATCH_SIZE` | `-` | - |
| `NSELF_BYOK_ROTATE_RATE_LIMIT` | `-` | - |
| `NSELF_BYOK_MULTI_REGION` | `-` | - |
| `NSELF_LICENSE_KEY` | `-` | nSelf license key |
| `LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3743 | BYOK service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_kms_configs`
- `np_encrypted_values`
- `np_encryption_key_events`

## REST API

```
GET    /encryption/key-events
GET    /encryption/kms
POST   /encryption/kms
PUT    /encryption/kms
POST   /encryption/kms/verify
POST   /encryption/rotate
GET    /encryption/rotate/{job_id}
GET    /health
GET    /ready
```

## Examples

### Health check

```bash
curl http://localhost:3743/health
```

## Source

[`plugins/byok/`](https://github.com/nself-org/plugins/tree/main/byok)

Manifest: [`plugins/byok/plugin.json`](https://github.com/nself-org/plugins/tree/main/byok/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
