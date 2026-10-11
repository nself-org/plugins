# cron

Cron job scheduler for nself. Register jobs with standard cron syntax, execute via HTTP callbacks, and track run history.

## Overview

The `cron` plugin provides a persistent cron scheduler backed by PostgreSQL. Jobs are registered via REST API with standard 5-field cron expressions and executed by making HTTP callbacks to configured endpoints. Run history is retained for auditing and debugging, with configurable retention.

The plugin is written in Go (per `plugin.json`) and ships as a single binary `nself-cron`. Its port is published on the host loopback only (`127.0.0.1:3051`), and the API is private by default: see [Access](#access-private-by-default).

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
POST   /v1/jobs                — Register a new cron job
GET    /v1/jobs                — List all registered jobs
GET    /v1/jobs/{id}           — Fetch one job with its recent runs (?history_limit=N)
PUT    /v1/jobs/{id}           — Update a job
DELETE /v1/jobs/{id}           — Remove a job
POST   /v1/jobs/{id}/trigger   — Run a job now
GET    /health                 — Health check (returns 200 OK)
```

### Register a job

```bash
curl -X POST http://127.0.0.1:3051/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nightly-report",
    "cron_expr": "0 2 * * *",
    "callback_url": "http://my-service:8080/cron/nightly-report"
  }'
```

### Inspect run history

```bash
curl "http://127.0.0.1:3051/v1/jobs/<job-id>?history_limit=20"
```

## Access (private by default)

The API has no authentication: whoever reaches it can create jobs with any callback URL, trigger them and delete them. So it is private by default:

- Services on the project network call it by service name: `http://plugin-cron:3051`.
- On the host, it listens on `127.0.0.1:3051` only. From your machine, tunnel to it: `ssh -L 3051:127.0.0.1:3051 <server>`, then use `http://127.0.0.1:3051`.
- `nself build` installs `nginx/cron.conf` as the `cron.<BASE_DOMAIN>` server block. That block answers `403` to every caller except the nginx container's own loopback (`allow 127.0.0.1; allow ::1; deny all;`). Private address ranges are not allowed: behind Docker's userland proxy (IPv6 clients, Docker Desktop, rootless Docker) or a tunnel container, internet traffic arrives from a private address.

To expose the API on purpose, add your own server block under another name in `nginx/conf.d/` (hand-managed; `nself build` never touches it) and put authentication in front of it. Do not reuse `cron.<BASE_DOMAIN>`: the plugin's block already serves that name. For example, `nginx/conf.d/cron-admin.conf`:

```nginx
server {
    listen 443 ssl;
    server_name cron-admin.example.com;
    ssl_certificate     /etc/nginx/ssl/<your-cert-dir>/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/<your-cert-dir>/privkey.pem;

    satisfy all;                       # both checks must pass
    allow 203.0.113.10;                # your admin address
    deny all;
    auth_basic "cron admin";
    auth_basic_user_file /etc/nginx/conf.d/cron-admin.htpasswd;

    resolver 127.0.0.11 valid=10s;
    set $upstream http://plugin-cron:3051;
    location /v1/ { proxy_pass $upstream; }
}
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

- **Jobs do not fire**: verify the scheduler is running (`curl http://127.0.0.1:3051/health` on the host) and that the job's `enabled` flag is `true`.
- **Callbacks time out**: raise `CRON_TIMEOUT_SECS` if your endpoint legitimately needs more than 30 seconds.
- **History grows unbounded**: confirm `CRON_RETENTION_DAYS` is set; the pruner runs once per day and deletes older `np_cron_runs` rows.

## License

MIT
