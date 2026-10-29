# Monitor Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install monitor` |
| **Commands** | `nself monitor` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself monitor` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Keeps the bundled Grafana dashboards current with one upgrade command.

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
