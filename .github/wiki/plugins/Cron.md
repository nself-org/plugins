# Cron Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) and Licensed, in the Claw Bundle |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | free: 1.2.13; licensed: 1.1.2 |
| **Install** | `nself plugin install cron` |
| **Requirements** | free: nSelf >=0.9.9; licensed: nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> Register jobs with standard cron syntax and dispatch them as signed HTTP callbacks.

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
POST   /v1/jobs                — Register a cron job
GET    /v1/jobs                — List registered jobs
GET    /v1/jobs/{id}           — One job with its recent runs (?history_limit=N)
PUT    /v1/jobs/{id}           — Update a job
DELETE /v1/jobs/{id}           — Remove a job
POST   /v1/jobs/{id}/trigger   — Run a job now
GET    /health                 — Health check
```

## Nginx Routes

| Route | Target | Access |
|-------|--------|--------|
| `cron.<BASE_DOMAIN>/v1/` | `http://plugin-cron:3051` | nginx container loopback only; everyone else gets `403` |

## Access (private by default)

The API has no authentication: anyone who reaches it can create jobs with any callback URL, trigger them and delete them. It is therefore private by default:

- Services on the project network call `http://plugin-cron:3051`.
- On the host the port is published on `127.0.0.1:3051` only. Remote operators tunnel: `ssh -L 3051:127.0.0.1:3051 <server>`.
- The `cron.<BASE_DOMAIN>` server block that `nself build` installs carries `allow 127.0.0.1; allow ::1; deny all;`. Private ranges are not allowed: behind Docker's userland proxy (IPv6 clients, Docker Desktop, rootless Docker) or a tunnel container, internet traffic arrives from a private address.

To expose it on purpose, add your own server block under a different name in your own file in `nginx/conf.d/` (hand-managed; `nself build` leaves files it did not write alone) with an allow list and `auth_basic` in front of `http://plugin-cron:3051`. The full example is in the [plugin README](https://github.com/nself-org/plugins/blob/main/free/cron/README.md#access-private-by-default). Do not reuse `cron.<BASE_DOMAIN>`: the plugin's block already serves that name. Keep `auth_basic` even with the allow line. Behind Docker's userland proxy nginx sees the Docker gateway address instead of your admin address, so do not "fix" a refused allow line by widening it to private ranges.

## Examples

### Check health

```bash
curl http://127.0.0.1:3051/health   # on the host
```

## Source

[`plugins/cron/`](https://github.com/nself-org/plugins/tree/main/cron)

Manifest: [`plugins/cron/plugin.json`](https://github.com/nself-org/plugins/tree/main/cron/plugin.json)

## See Also

- [[Queue]] — inspect background job queues
- [[DLQ]] — replay dead-lettered rows

← [[Home]] →
