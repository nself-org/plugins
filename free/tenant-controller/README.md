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

## Limitations of the container form

- Disabled by default: set `NSELF_CONTROLLER_ENABLED=true` (or `NSELF_FLAG_MULTI_TENANT_CONTROLLER=true`) plus `NSELF_CONTROLLER_ADMIN_TOKEN`, `NSELF_CONTROLLER_BASE_DOMAIN`, `HASURA_GRAPHQL_ENDPOINT` and `HASURA_GRAPHQL_ADMIN_SECRET`.
- The controller writes the project nginx config to `NGINX_CONF_PATH` and reloads nginx. The image carries no nginx and the fragment mounts no shared nginx directory, so creating a project fails at the nginx step until a shared config directory and reload path are wired (tracked in hq quality debt).
- The `nself_controller` role is `NOLOGIN`: the daemon connects with `DATABASE_URL` and never as this role.
- Installs from before manifest v2 (schema `nself_controller` with its own ledger) are renamed in place. Their ledger has no checksum column and the old runner applied only `001`, so boot stops with the SDK's `legacy ledger` error instead of guessing: record `001` by hand, then restart. No such install is known. The `nself_controller` role is locked to `NOLOGIN` on every start.
