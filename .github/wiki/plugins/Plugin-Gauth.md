# Plugin Gauth Plugin

> Headless server-side Google OAuth token refresh for nSelf AI services. **Free — MIT licensed.**

## Install

```bash
nself plugin install plugin-gauth
```

No license key required.

## Description

Headless server-side Google OAuth token refresh for nSelf AI services

Category: `authentication`. Current version: `0.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_DB_URL` | `(required)` | PostgreSQL connection string |
| `GAUTH_ENCRYPTION_KEY` | `(required)` | - |
| `GOOGLE_CLIENT_ID` | `(required)` | - |
| `GOOGLE_CLIENT_SECRET` | `(required)` | - |
| `GAUTH_PORT` | `3827` | - |
| `GOOGLE_TOKEN_URL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3827 | Plugin Gauth service port |

## Examples

```bash
nself plugin install plugin-gauth
```

## Source

[`plugins/plugin-gauth/`](https://github.com/nself-org/plugins/tree/main/plugin-gauth)

Manifest: [`plugins/plugin-gauth/plugin.json`](https://github.com/nself-org/plugins/tree/main/plugin-gauth/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
