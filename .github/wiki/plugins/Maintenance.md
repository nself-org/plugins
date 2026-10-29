# Maintenance Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install maintenance` |
| **Commands** | `nself maintenance` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself maintenance` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Schedules disk cleanup and log rotation so a self-hosted instance doesn't fill its disk.

## Install

```bash
nself plugin install maintenance
```

No license key required.

## Description

Maintenance utilities: disk cleanup, log rotation and the maintenance scheduler.

This is a CLI plugin: it installs the `nself-maintenance` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-maintenance` subcommands (installed alongside the plugin):

- `nself-maintenance disk-cleanup`
- `nself-maintenance schedule`
- `nself-maintenance status`

## Examples

### Disk-cleanup

```bash
nself-maintenance disk-cleanup
```

### Schedule

```bash
nself-maintenance schedule
```

## Source

[`plugins/maintenance/`](https://github.com/nself-org/plugins/tree/main/maintenance)

Manifest: [`plugins/maintenance/plugin.json`](https://github.com/nself-org/plugins/tree/main/maintenance/plugin.json)

## See Also

- [[Watchdog]] — self-healing container watchdog
- [[Dogfood]] — production health checks

← [[Home]] →
