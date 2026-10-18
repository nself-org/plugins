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
| `NGINX_RELOAD_MODE` | No | `exec` runs nginx here, `watch` leaves the reload to the conf.d watcher |
| `NSELF_CONTROLLER_ADOPT_LEGACY` | No | `true` once, to adopt a pre-v2 ledger |
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

## The container form

- Disabled by default. With `NSELF_CONTROLLER_ENABLED=false` (the default) the container logs one line and exits 0, and `restart: on-failure:5` leaves it stopped, so `nself add tenant-controller` does not restart-loop. It needs none of the vars below until you enable it. To run it, set `NSELF_CONTROLLER_ENABLED=true` (or `NSELF_FLAG_MULTI_TENANT_CONTROLLER=true`) plus `NSELF_CONTROLLER_ADMIN_TOKEN`, `NSELF_CONTROLLER_BASE_DOMAIN`, `HASURA_GRAPHQL_ENDPOINT` and `HASURA_GRAPHQL_ADMIN_SECRET`, then restart.
- Nginx. The controller writes `nself-projects.conf` into the stack's `nginx/conf.d/` (mounted at `/etc/nginx/conf.d`, which the stack nginx already includes). The image has no nginx binary, so `NGINX_RELOAD_MODE=watch` is set: the `plugin-tenant-controller-reloader` service polls that file every 2 seconds and sends SIGHUP to the stack nginx master. It joins nginx's PID namespace (`pid: service:nginx`); no Docker socket is mounted. A broken vhost file makes nginx keep its old config and log the error. Outside this fragment, `NGINX_RELOAD_MODE=exec` (the default) runs `nginx -s reload` and needs the binary.
- The `nself_controller` role is `NOLOGIN`: the daemon connects with `DATABASE_URL` and never as this role.
- Installs from before manifest v2 (schema `nself_controller`, own ledger, only `001` applied) are renamed in place, and boot then stops with `legacy ledger` instead of guessing. Restart once with `NSELF_CONTROLLER_ADOPT_LEGACY=true`: the controller records `001` only, applies `002` and `003`, and starts. Remove the flag afterwards. No such install is known. The role is locked to `NOLOGIN` on every start.
