# SIEM Plugin

> Forward nSelf audit logs and security events to external SIEM platforms: Datadog, Splunk HEC, Elastic, Loki, and custom webhooks. **Free — MIT licensed.**

## Install

```bash
nself plugin install siem
```

No license key required.

## Description

Forward nSelf audit logs and security events to external SIEM platforms: Datadog, Splunk HEC, Elastic, Loki, and custom webhooks. OCSF/ECS schema normalization. ɳSelf+ required for external destinations.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_SIEM` | `-` | - |
| `NSELF_SIEM_HF` | `-` | - |
| `NSELF_SIEM_DEFAULT_SCHEMA` | `-` | - |
| `NSELF_SIEM_BATCH_SIZE` | `-` | - |
| `NSELF_SIEM_FLUSH_INTERVAL` | `-` | - |
| `NSELF_SIEM_LOKI_URL` | `-` | - |
| `SIEM_PORT` | `8002` | - |
| `LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8002 | SIEM service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_siem_destinations`
- `np_siem_ship_log`

## REST API

```
GET    /health
GET    /siem/destinations
POST   /siem/destinations
DELETE /siem/destinations/{id}
PATCH  /siem/destinations/{id}
GET    /siem/destinations/{id}/ship-log
POST   /siem/destinations/{id}/test
POST   /siem/flush
GET    /siem/status
```

## Examples

### Health check

```bash
curl http://localhost:8002/health
```

## Source

[`plugins/siem/`](https://github.com/nself-org/plugins/tree/main/siem)

Manifest: [`plugins/siem/plugin.json`](https://github.com/nself-org/plugins/tree/main/siem/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
