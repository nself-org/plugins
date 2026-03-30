# cron

Cron job scheduler for nself. Register jobs with standard cron syntax, execute via HTTP callbacks, and track run history.

## Overview

The `cron` plugin provides a persistent cron scheduler backed by PostgreSQL. Jobs are registered via REST API with standard 5-field cron expressions and executed by making HTTP callbacks to configured endpoints.

## Installation

```bash
nself plugin install cron
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `PORT` | No | Server port (default: 3051) |
| `CRON_TIMEOUT_SECS` | No | HTTP callback timeout in seconds (default: 30) |
| `CRON_RETENTION_DAYS` | No | Days to retain run history (default: 90) |

## Usage

```bash
# Start the cron scheduler server
nself plugin run cron server
```

## REST API

- `POST /jobs` — Register a new cron job
- `GET /jobs` — List all registered jobs
- `DELETE /jobs/:id` — Remove a job
- `GET /runs` — View run history

## Database Tables

- `np_cron_jobs` — Registered cron jobs
- `np_cron_runs` — Execution history

## License

MIT
