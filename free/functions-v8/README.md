# functions-edge

Edge Functions V8 Runtime. Deploy short-lived TypeScript functions with a Deno V8 isolate pool. HTTP-trigger, <50ms cold-start, allowlist-only env injection, Prometheus metrics, SSE log streaming.

## Details

- **Category:** infrastructure
- **Tier:** pro
- **Language:** go
- **Port:** 3088
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | — |
| `FUNCTIONS_EDGE_ENABLED` | No | — |
| `FUNCTIONS_EDGE_PORT` | No | — |
| `FUNCTIONS_EDGE_MAX_DURATION_MS` | No | — |
| `FUNCTIONS_EDGE_POOL_SIZE` | No | — |
| `FUNCTIONS_EDGE_MAX_MEMORY_MB` | No | — |
| `FUNCTIONS_EDGE_LOG_RETENTION_DAYS` | No | — |
| `DENO_BINARY_PATH` | No | — |
| `PLUGIN_INTERNAL_SECRET` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/` | bearer |  |
| `POST` | `/` | bearer |  |
| `GET` | `/functions/v1/{name}` | bearer |  |
| `POST` | `/functions/v1/{name}` | bearer |  |
| `GET` | `/health` | bearer |  |
| `DELETE` | `/{name}` | bearer |  |
| `GET` | `/{name}` | bearer |  |
| `PATCH` | `/{name}/activate` | bearer |  |
| `GET` | `/{name}/env` | bearer |  |
| `POST` | `/{name}/env` | bearer |  |
| `DELETE` | `/{name}/env/{key}` | bearer |  |
| `GET` | `/{name}/logs` | bearer |  |
| `GET` | `/{name}/logs/stream` | bearer |  |

## Install

```bash
nself plugin install functions-edge
```
