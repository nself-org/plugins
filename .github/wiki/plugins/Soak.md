# Soak Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install soak` |
| **Commands** | `nself soak` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Aborts an in-progress soak test and rolls the deployment back to its prior version.

## Install

```bash
nself plugin install soak
```

No license key required.

## Description

Manage soak testing lifecycle: abort an active soak and roll back to a prior version.

This is a CLI plugin: it installs the `nself-soak` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_CURRENT_VERSION` | *(see plugin.json)* | Optional. |

## Commands

`nself-soak` subcommands (installed alongside the plugin):

- `nself-soak abort --rollback <version>`

## Examples

### Abort

```bash
nself-soak abort --rollback <version>
```

## Source

[`plugins/soak/`](https://github.com/nself-org/plugins/tree/main/soak)

Manifest: [`plugins/soak/plugin.json`](https://github.com/nself-org/plugins/tree/main/soak/plugin.json)

## See Also

- [[Release]] — the release cascade
- [[Region]] — multi-region replica management

← [[Home]] →
