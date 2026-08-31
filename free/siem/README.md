# siem

Forward nSelf audit logs and security events to external SIEM platforms: Datadog, Splunk HEC, Elastic, Loki, and custom webhooks. OCSF/ECS schema normalization. ɳSelf+ required for external destinations.

## Details

- **Category:** infrastructure
- **Tier:** pro
- **Language:** go
- **Port:** 8002
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | — |
| `NSELF_SIEM` | No | — |
| `NSELF_SIEM_HF` | No | — |
| `NSELF_SIEM_DEFAULT_SCHEMA` | No | — |
| `NSELF_SIEM_BATCH_SIZE` | No | — |
| `NSELF_SIEM_FLUSH_INTERVAL` | No | — |
| `NSELF_SIEM_LOKI_URL` | No | — |
| `SIEM_PORT` | No | — |
| `LOG_LEVEL` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | bearer |  |
| `GET` | `/siem/destinations` | bearer |  |
| `POST` | `/siem/destinations` | bearer |  |
| `DELETE` | `/siem/destinations/{id}` | bearer |  |
| `PATCH` | `/siem/destinations/{id}` | bearer |  |
| `GET` | `/siem/destinations/{id}/ship-log` | bearer |  |
| `POST` | `/siem/destinations/{id}/test` | bearer |  |
| `POST` | `/siem/flush` | bearer |  |
| `GET` | `/siem/status` | bearer |  |

## Install

```bash
nself plugin install siem
```
