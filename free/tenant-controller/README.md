# tenant-controller

Multi-tenant master controller for nCloud. Manages N isolated nSelf project instances behind a single deploy: per-project Postgres schema, Hasura metadata namespace, Nginx vhost, JWT secret, Redis key prefix, and MinIO bucket. Enables 50 projects on one Hetzner CX21.

## Details

- **Category:** infrastructure
- **Tier:** max
- **Language:** go
- **Port:** 3750
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `HASURA_GRAPHQL_ENDPOINT` | Yes | — |
| `HASURA_GRAPHQL_ADMIN_SECRET` | Yes | — |
| `NSELF_CONTROLLER_BASE_DOMAIN` | Yes | — |
| `NSELF_CONTROLLER_ADMIN_TOKEN` | Yes | — |
| `DATABASE_URL` | Yes | — |
| `NSELF_CONTROLLER_ENABLED` | No | — |
| `NSELF_CONTROLLER_MAX_PROJECTS` | No | — |
| `NSELF_CONTROLLER_DB` | No | — |
| `NSELF_PROJECT_SCHEMA_PREFIX` | No | — |
| `NSELF_PROJECT_REDIS_PREFIX` | No | — |
| `NSELF_PROJECT_HASURA_PORT_START` | No | — |
| `NSELF_CONTROLLER_PORT` | No | — |
| `NSELF_CONTROLLER_HOST` | No | — |
| `REDIS_URL` | No | — |
| `MINIO_ENDPOINT` | No | — |
| `MINIO_ACCESS_KEY` | No | — |
| `MINIO_SECRET_KEY` | No | — |
| `NGINX_CONF_PATH` | No | — |
| `NSELF_AUDIT_RETENTION_DAYS` | No | — |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — |
| `LOG_LEVEL` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/controller/status` | bearer |  |
| `GET` | `/health` | bearer |  |
| `GET` | `/projects` | bearer |  |
| `POST` | `/projects/create` | bearer |  |
| `DELETE` | `/projects/{slug}` | bearer |  |
| `GET` | `/projects/{slug}/status` | bearer |  |

## Install

```bash
nself plugin install tenant-controller
```
