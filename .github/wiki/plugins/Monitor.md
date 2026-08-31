# Monitor Plugin

> Keeps the bundled Grafana dashboards current with one upgrade command. **Free — MIT licensed.**

## Install

```bash
nself plugin install monitor
```

No license key required.

## Description

Monitoring stack management: upgrade the bundled Grafana dashboards.

This is a CLI plugin: it installs the `nself-monitor` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-monitor` subcommands (installed alongside the plugin):

- `nself-monitor upgrade-dashboards`

## Examples

### Upgrade-dashboards

```bash
nself-monitor upgrade-dashboards
```

## Source

[`plugins/monitor/`](https://github.com/nself-org/plugins/tree/main/monitor)

Manifest: [`plugins/monitor/plugin.json`](https://github.com/nself-org/plugins/tree/main/monitor/plugin.json)

## See Also

- [[Monitoring]] — the full observability stack
- [[Alerts]] — Prometheus alert rules and silences

← [[Home]] →
