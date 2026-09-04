# Event Bus Plugin

> Internal event bus with pub/sub, fan-out delivery, dead-letter queue, and replay for inter-plugin messaging. **Free — MIT licensed.**

## Install

```bash
nself plugin install event-bus
```

No license key required.

## Description

Internal event bus with pub/sub, fan-out delivery, dead-letter queue, and replay for inter-plugin messaging.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `EVENT_BUS_PORT` | `8212` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8212 | Event Bus service port |

## Database Schema

1 table(s) added to your Postgres database:

- `np_event_bus_stats`

## REST API

```
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:8212/health
```

## Source

[`plugins/event-bus/`](https://github.com/nself-org/plugins/tree/main/event-bus)

Manifest: [`plugins/event-bus/plugin.json`](https://github.com/nself-org/plugins/tree/main/event-bus/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
