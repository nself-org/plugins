# Region Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install region` |
| **Commands** | `nself region` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself region` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Adds, inspects, and promotes replica regions for multi-region deployments.

## Install

```bash
nself plugin install region
```

No license key required.

## Description

Multi-region management: add replica regions, list and inspect their status, and promote a region to primary.

This is a CLI plugin: it installs the `nself-region` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-region` subcommands (installed alongside the plugin):

- `nself-region add`
- `nself-region list`
- `nself-region status`
- `nself-region promote`

## Examples

### Add

```bash
nself-region add
```

### List

```bash
nself-region list
```

## Source

[`plugins/region/`](https://github.com/nself-org/plugins/tree/main/region)

Manifest: [`plugins/region/plugin.json`](https://github.com/nself-org/plugins/tree/main/region/plugin.json)

## See Also

- [[DR]] — disaster recovery: promote, fence, drill
- [[Infra]] — provision infrastructure via Terraform

← [[Home]] →
