# cron

Cron job scheduler for nself. Register jobs with standard cron syntax, execute via HTTP callbacks, and track run history.

## Overview

The `cron` plugin provides a persistent cron scheduler backed by PostgreSQL. Jobs are registered via REST API with standard 5-field cron expressions and executed by making HTTP callbacks to configured endpoints. Run history is retained for auditing and debugging, with configurable retention.

The plugin is written in Go (per `plugin.json`) and ships as a single binary `nself-cron`. It binds to `127.0.0.1` by default and is reverse-proxied through Nginx by `nself build`.

## Installation

```bash
nself plugin install cron
```

No license key required. MIT-licensed.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PORT` | No | `3051` | Server port (also `defaultPort` in plugin.json) |
| `CRON_TIMEOUT_SECS` | No | `30` | HTTP callback timeout in seconds |
| `CRON_RETENTION_DAYS` | No | `90` | Days to retain run history before pruning |

## Usage

```bash
# Start the cron scheduler server
nself plugin run cron server
```

The scheduler polls the `np_cron_jobs` table every minute and dispatches due jobs by issuing the configured HTTP callback. Each attempt is recorded in `np_cron_runs` with status, duration, and response.

## REST API

```
POST   /jobs          — Register a new cron job
GET    /jobs          — List all registered jobs
GET    /jobs/:id      — Fetch a single job by ID
DELETE /jobs/:id      — Remove a job
GET    /runs          — View run history (paginated)
GET    /runs?job_id=X — Filter runs for a specific job
GET    /health        — Health check (returns 200 OK)
```

### Register a job

```bash
curl -X POST http://localhost:3051/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nightly-report",
    "schedule": "0 2 * * *",
    "callback_url": "https://api.example.com/cron/nightly-report",
    "callback_method": "POST"
  }'
```

### Inspect run history

```bash
curl "http://localhost:3051/runs?job_id=nightly-report&limit=20"
```

## Database Tables

Two tables added to your Postgres database (prefix `np_cron_`):

- `np_cron_jobs` — Registered cron jobs with schedule expression, callback URL, and enabled flag
- `np_cron_runs` — Execution history with timestamp, status code, duration, and response snippet

## Common Workflows

- **Periodic data sync**: register a job that pings a sync endpoint every hour with `0 * * * *`.
- **Daily reports**: schedule `0 2 * * *` to generate overnight reports at 02:00.
- **Health probes**: register a 5-minute job (`*/5 * * * *`) that hits an internal health endpoint.

## Troubleshooting

- **Jobs do not fire**: verify the scheduler is running (`curl http://localhost:3051/health`) and that the job's `enabled` flag is `true`.
- **Callbacks time out**: raise `CRON_TIMEOUT_SECS` if your endpoint legitimately needs more than 30 seconds.
- **History grows unbounded**: confirm `CRON_RETENTION_DAYS` is set; the pruner runs once per day and deletes older `np_cron_runs` rows.

## License

MIT
