# Alerts Plugin

> Operate Prometheus alert rules and Alertmanager silences without leaving the terminal. **Free — MIT licensed.**

## Install

```bash
nself plugin install alerts
```

No license key required.

## Description

Manage Prometheus alert rules and Alertmanager silences: list, silence, and send synthetic test alerts.

This is a CLI plugin: it installs the `nself-alerts` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-alerts` subcommands (installed alongside the plugin):

- `nself-alerts list`
- `nself-alerts silence <alert-name>`
- `nself-alerts test <alert-name>`

## Examples

### List

```bash
nself-alerts list
```

### Silence

```bash
nself-alerts silence <alert-name>
```

## Source

[`plugins/alerts/`](https://github.com/nself-org/plugins/tree/main/alerts)

Manifest: [`plugins/alerts/plugin.json`](https://github.com/nself-org/plugins/tree/main/alerts/plugin.json)

## See Also

- [[Monitor]] — Grafana dashboard upgrades
- [[Monitoring]] — the full observability stack

← [[Home]] →
