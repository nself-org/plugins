# Tenant Controller Plugin

> Multi-tenant master controller for nCloud. **Free — MIT licensed.**

## Install

```bash
nself plugin install tenant-controller
```

No license key required.

## Description

Multi-tenant master controller for nCloud. Manages N isolated nSelf project instances behind a single deploy: per-project Postgres schema, Hasura metadata namespace, Nginx vhost, JWT secret, Redis key prefix, and MinIO bucket. Enables 50 projects on one Hetzner CX21.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_CONTROLLER_ADMIN_TOKEN` | `(required)` | - |
| `NSELF_CONTROLLER_BASE_DOMAIN` | `(required)` | - |
| `HASURA_GRAPHQL_ADMIN_SECRET` | `(required)` | Hasura admin secret (for Hasura metadata registration) |
| `HASURA_GRAPHQL_ENDPOINT` | `(required)` | - |
| `NSELF_CONTROLLER_ENABLED` | `-` | - |
| `NSELF_CONTROLLER_MAX_PROJECTS` | `-` | - |
| `NSELF_CONTROLLER_DB` | `-` | - |
| `NSELF_PROJECT_SCHEMA_PREFIX` | `-` | - |
| `NSELF_PROJECT_REDIS_PREFIX` | `-` | - |
| `NSELF_PROJECT_HASURA_PORT_START` | `-` | - |
| `NSELF_CONTROLLER_PORT` | `3750` | - |
| `NSELF_CONTROLLER_HOST` | `-` | - |
| `REDIS_URL` | `-` | - |
| `MINIO_ENDPOINT` | `-` | - |
| `MINIO_ACCESS_KEY` | `-` | - |
| `MINIO_SECRET_KEY` | `-` | - |
| `NGINX_CONF_PATH` | `-` | - |
| `NSELF_AUDIT_RETENTION_DAYS` | `-` | - |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `-` | - |
| `LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3750 | Tenant Controller service port |

## Database Schema

3 table(s) added to your Postgres database:

- `nself_controller.projects`
- `nself_controller.project_audit_log`
- `nself_controller.port_map`

## REST API

```
GET    /controller/status
GET    /health
GET    /projects
POST   /projects/create
DELETE /projects/{slug}
GET    /projects/{slug}/status
```

## Examples

### Health check

```bash
curl http://localhost:3750/health
```

## Source

[`plugins/tenant-controller/`](https://github.com/nself-org/plugins/tree/main/tenant-controller)

Manifest: [`plugins/tenant-controller/plugin.json`](https://github.com/nself-org/plugins/tree/main/tenant-controller/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
