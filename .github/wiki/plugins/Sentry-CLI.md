# Sentry-CLI Plugin

> Operates a self-hosted ɳSentry instance: monitors, incidents, status pages, and alerts. **Free — MIT licensed.**

## Install

```bash
nself plugin install sentry-cli
```

No license key required.

## Description

ɳSentry operations: monitors, incidents, status pages, alerts, cloud login, and provisioning a self-hosted sentry server.

This is a CLI plugin: it installs the `nself-sentry-cli` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `HETZNER_NSELF_TOKEN` | — | Optional. |
| `HCLOUD_TOKEN` | — | Optional. |

## Commands

`nself-sentry-cli` subcommands (installed alongside the plugin):

- `nself-sentry-cli login`
- `nself-sentry-cli logout`
- `nself-sentry-cli whoami`
- `nself-sentry-cli monitors`
- `nself-sentry-cli incidents`
- `nself-sentry-cli alerts`
- `nself-sentry-cli channels`
- `nself-sentry-cli status-pages`
- `nself-sentry-cli status`
- `nself-sentry-cli provision <project>`
- `nself-sentry-cli sentry-server`

## Examples

### Login

```bash
nself-sentry-cli login
```

### Logout

```bash
nself-sentry-cli logout
```

## Source

[`plugins/sentry-cli/`](https://github.com/nself-org/plugins/tree/main/sentry-cli)

Manifest: [`plugins/sentry-cli/plugin.json`](https://github.com/nself-org/plugins/tree/main/sentry-cli/plugin.json)

## See Also

- [[Watchdog]] — self-healing container watchdog
- [[Alerts]] — Prometheus alert rules and silences

← [[Home]] →
