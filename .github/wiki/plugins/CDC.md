# CDC Plugin

> Change Data Capture, streams Postgres WAL events to downstream consumers via webhooks or message queues. **Free — MIT licensed.**

## Install

```bash
nself plugin install cdc
```

No license key required.

## Description

Change Data Capture, streams Postgres WAL events to downstream consumers via webhooks or message queues.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `CDC_PORT` | `8209` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8209 | CDC service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_cdc_streams`
- `np_cdc_events`

## REST API

```
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:8209/health
```

## Source

[`plugins/cdc/`](https://github.com/nself-org/plugins/tree/main/cdc)

Manifest: [`plugins/cdc/plugin.json`](https://github.com/nself-org/plugins/tree/main/cdc/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
