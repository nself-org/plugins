# Functions V8 Plugin

> Edge Functions V8 Runtime. **Free — MIT licensed.**

## Install

```bash
nself plugin install functions-edge
```

No license key required.

## Description

Edge Functions V8 Runtime. Deploy short-lived TypeScript functions with a Deno V8 isolate pool. HTTP-trigger, <50ms cold-start, allowlist-only env injection, Prometheus metrics, SSE log streaming.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `FUNCTIONS_EDGE_ENABLED` | `-` | - |
| `FUNCTIONS_EDGE_PORT` | `3088` | - |
| `FUNCTIONS_EDGE_MAX_DURATION_MS` | `-` | - |
| `FUNCTIONS_EDGE_POOL_SIZE` | `-` | - |
| `FUNCTIONS_EDGE_MAX_MEMORY_MB` | `-` | - |
| `FUNCTIONS_EDGE_LOG_RETENTION_DAYS` | `-` | - |
| `DENO_BINARY_PATH` | `-` | - |
| `PLUGIN_INTERNAL_SECRET` | `-` | Shared secret for plugin-to-plugin HTTP calls (`X-Internal-Token` header) |

## Ports

| Port | Purpose |
|------|---------|
| 3088 | Functions V8 service port |

## Database Schema

4 table(s) added to your Postgres database:

- `np_edge_functions`
- `np_edge_function_versions`
- `np_edge_function_env`
- `np_function_logs`

## REST API

```
GET    /
POST   /
GET    /functions/v1/{name}
POST   /functions/v1/{name}
GET    /health
DELETE /{name}
GET    /{name}
PATCH  /{name}/activate
GET    /{name}/env
POST   /{name}/env
DELETE /{name}/env/{key}
GET    /{name}/logs
```

## Examples

### Health check

```bash
curl http://localhost:3088/health
```

## Source

[`plugins/functions-v8/`](https://github.com/nself-org/plugins/tree/main/functions-v8)

Manifest: [`plugins/functions-v8/plugin.json`](https://github.com/nself-org/plugins/tree/main/functions-v8/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
