# Gauth Plugin

> Refreshes, checks, and revokes the Google OAuth tokens nSelf's AI services depend on. **Free — MIT licensed.**

## Install

```bash
nself plugin install gauth
```

No license key required.

## Description

Manage Google OAuth tokens for nSelf AI services: status, refresh, and revoke against plugin-gauth.

This is a CLI plugin: it installs the `nself-gauth` binary into your plugin path and runs as a command, not a background service.

Category: `integrations`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `GAUTH_URL` | *(see plugin.json)* | Optional. |
| `GAUTH_PORT` | *(see plugin.json)* | Optional. |

## Commands

`nself-gauth` subcommands (installed alongside the plugin):

- `nself-gauth status`
- `nself-gauth refresh`
- `nself-gauth revoke`

## Examples

### Status

```bash
nself-gauth status
```

### Refresh

```bash
nself-gauth refresh
```

## Source

[`plugins/gauth/`](https://github.com/nself-org/plugins/tree/main/gauth)

Manifest: [`plugins/gauth/plugin.json`](https://github.com/nself-org/plugins/tree/main/gauth/plugin.json)

## See Also

- [[Gateway]] — AI gateway key vault and quota
- [[AI-Studio]] — Google AI Studio bridge

← [[Home]] →
