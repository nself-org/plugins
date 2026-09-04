# Job Queue Plugin

> Durable background job queue with priorities, retries, scheduled execution, and per-job progress tracking. **Free — MIT licensed.**

## Install

```bash
nself plugin install job-queue
```

No license key required.

## Description

Durable background job queue with priorities, retries, scheduled execution, and per-job progress tracking.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `JOB_QUEUE_PORT` | `8213` | - |
| `JOB_QUEUE_WORKERS` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8213 | Job Queue service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_jobqueue_jobs`
- `np_jobqueue_runs`

## REST API

```
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:8213/health
```

## Source

[`plugins/job-queue/`](https://github.com/nself-org/plugins/tree/main/job-queue)

Manifest: [`plugins/job-queue/plugin.json`](https://github.com/nself-org/plugins/tree/main/job-queue/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
