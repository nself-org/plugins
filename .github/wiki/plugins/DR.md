# DR Plugin

> Promotes a standby, fences the old primary, and runs failover drills. **Free — MIT licensed.**

## Install

```bash
nself plugin install dr
```

No license key required.

## Description

Disaster recovery: promote a standby, fence the old primary, run drills, and install the systemd units DR needs.

This is a CLI plugin: it installs the `nself-dr` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PROJECT_NAME` | *(see plugin.json)* | Optional. |
| `BASE_DOMAIN` | *(see plugin.json)* | Optional. |
| `BACKUP_DIR` | *(see plugin.json)* | Optional. |
| `DR_STANDBY_HOST` | *(see plugin.json)* | Optional. |
| `REDIS_ENABLED` | *(see plugin.json)* | Optional. |

## Commands

`nself-dr` subcommands (installed alongside the plugin):

- `nself-dr promote-standby`
- `nself-dr fence`
- `nself-dr drill`
- `nself-dr rollback`
- `nself-dr reconfigure-dns`

## Examples

### Promote-standby

```bash
nself-dr promote-standby
```

### Fence

```bash
nself-dr fence
```

## Source

[`plugins/dr/`](https://github.com/nself-org/plugins/tree/main/dr)

Manifest: [`plugins/dr/plugin.json`](https://github.com/nself-org/plugins/tree/main/dr/plugin.json)

## See Also

- [[Region]] — multi-region replica management
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
