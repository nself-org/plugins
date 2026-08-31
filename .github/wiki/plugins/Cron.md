# Cron Plugin

> Register jobs with standard cron syntax and dispatch them as signed HTTP callbacks. **Free — MIT licensed.**

## Install

```bash
nself plugin install cron
```

No license key required.

## Description

Cron job scheduler. Register jobs with standard cron syntax, execute via HTTP callbacks, track run history.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `automation`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |
| `PORT` | *(see plugin.json)* | Optional. |
| `CRON_TIMEOUT_SECS` | *(see plugin.json)* | Optional. |
| `CRON_RETENTION_DAYS` | *(see plugin.json)* | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3051 | Cron service |

## Database Schema

2 table(s) added to your Postgres database (prefix: `np_cron_`):

- `np_cron_jobs`
- `np_cron_runs`

## REST API

```
POST /jobs                — Register a cron job
GET  /jobs                — List registered jobs
GET  /jobs/{id}/runs      — Run history for a job
DELETE /jobs/{id}         — Remove a job
GET  /health              — Health check
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/cron/` | Cron plugin REST API |

## Examples

### Check health

```bash
curl http://localhost:3051/health
```

## Source

[`plugins/cron/`](https://github.com/nself-org/plugins/tree/main/cron)

Manifest: [`plugins/cron/plugin.json`](https://github.com/nself-org/plugins/tree/main/cron/plugin.json)

## See Also

- [[Queue]] — inspect background job queues
- [[DLQ]] — replay dead-lettered rows

← [[Home]] →
