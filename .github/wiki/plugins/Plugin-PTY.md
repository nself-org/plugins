# Plugin PTY Plugin

> Pseudo-terminal bridge for ClawDE AI sessions. **Free — MIT licensed.**

## Install

```bash
nself plugin install plugin-pty
```

No license key required.

## Description

Pseudo-terminal bridge for ClawDE AI sessions. Spawns, manages, and relays PTY processes with per-tenant resource limits and WebSocket I/O.

Category: `development`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_DB_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_LICENSE_KEY` | `(required)` | nSelf license key |
| `PTY_MAX_PER_TENANT` | `5` | Max concurrent PTY sessions per tenant |
| `PTY_SESSION_TIMEOUT_SECS` | `3600` | PTY session idle timeout in seconds |
| `PTY_PORT` | `9100` | HTTP server port |

## Ports

| Port | Purpose |
|------|---------|
| 9100 | Plugin PTY service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_pty_sessions`
- `np_pty_audit_log`

## Examples

```bash
nself plugin install plugin-pty
```

## Source

[`plugins/plugin-pty/`](https://github.com/nself-org/plugins/tree/main/plugin-pty)

Manifest: [`plugins/plugin-pty/plugin.json`](https://github.com/nself-org/plugins/tree/main/plugin-pty/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
