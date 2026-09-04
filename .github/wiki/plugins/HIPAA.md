# HIPAA Plugin

> HIPAA compliance add-on: PHI column registry, PHI access logging with 6-year retention, de-identification helpers (masking + tokenization), encryption-at-rest audit, and BAA workflow. **Free — MIT licensed.**

## Install

```bash
nself plugin install hipaa
```

No license key required.

## Description

HIPAA compliance add-on: PHI column registry, PHI access logging with 6-year retention, de-identification helpers (masking + tokenization), encryption-at-rest audit, and BAA workflow. Requires ɳSelf+ license.

Category: `compliance`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_HIPAA` | `-` | - |
| `NSELF_HIPAA_BAA` | `-` | - |
| `NSELF_HIPAA_VAULT` | `-` | - |
| `NSELF_HIPAA_VAULT_ADDR` | `-` | - |
| `NSELF_HIPAA_VAULT_TOKEN` | `-` | - |
| `NSELF_HIPAA_RETENTION_YEARS` | `-` | - |
| `NSELF_HIPAA_BAA_BUCKET` | `-` | - |
| `HIPAA_PLUGIN_PORT` | `3214` | - |
| `HIPAA_PLUGIN_HOST` | `-` | - |
| `HIPAA_API_KEY` | `-` | - |
| `HIPAA_LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3214 | HIPAA service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_phi_columns`
- `np_phi_audit_log`
- `np_baa_records`

## REST API

```
GET    /audit-log
GET    /audit-log/export
GET    /baa
POST   /baa/activate
POST   /baa/request
POST   /baa/terminate
POST   /deidentify
GET    /encryption-audit
GET    /health
GET    /phi-columns
POST   /phi-columns
DELETE /phi-columns/{id}
```

## Examples

### Health check

```bash
curl http://localhost:3214/health
```

## Source

[`plugins/hipaa/`](https://github.com/nself-org/plugins/tree/main/hipaa)

Manifest: [`plugins/hipaa/plugin.json`](https://github.com/nself-org/plugins/tree/main/hipaa/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
