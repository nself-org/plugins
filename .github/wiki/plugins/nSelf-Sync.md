# nSelf Sync Plugin

> Event-log sync engine for ɳClaw. **Free — MIT licensed.**

## Install

```bash
nself plugin install nself-sync
```

No license key required.

## Description

Event-log sync engine for ɳClaw. Multi-device state sync via HLC + LWW; JWT-authenticated push/pull/snapshot/subscribe with Ed25519-signed events.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_SYNC_JWT_SECRET` | `(required)` | - |
| `NSELF_SYNC_BIND` | `127.0.0.1` | - |
| `NSELF_SYNC_PORT` | `3844` | - |
| `NSELF_SYNC_RATE_PUSH_PER_MIN` | `100` | - |
| `NSELF_SYNC_PULL_PAGE_SIZE` | `500` | - |
| `NSELF_SYNC_WS_AUTH_TIMEOUT_SEC` | `5` | - |
| `NSELF_SYNC_PLUGIN_ENABLED` | `false` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3853 | nSelf Sync service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_sync_events`
- `np_devices`
- `np_sync_cursors`

## Examples

```bash
nself plugin install nself-sync
```

## Source

[`plugins/nself-sync/`](https://github.com/nself-org/plugins/tree/main/nself-sync)

Manifest: [`plugins/nself-sync/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-sync/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
