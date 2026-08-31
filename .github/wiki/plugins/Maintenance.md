# Maintenance Plugin

> Schedules disk cleanup and log rotation so a self-hosted instance doesn't fill its disk. **Free — MIT licensed.**

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
