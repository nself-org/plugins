# Federation Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install federation` |
| **Commands** | `nself federation` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself federation` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Composes an Apollo Router supergraph from every installed plugin's GraphQL subgraph.

## Install

```bash
nself plugin install federation
```

No license key required.

## Description

Manage GraphQL Federation: compose an Apollo Router supergraph from installed plugin subgraphs, check subgraph health, and introspect the composed schema.

This is a CLI plugin: it installs the `nself-federation` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_FEDERATION` | *(see plugin.json)* | Optional. |
| `NSELF_PLUGIN_DIR` | *(see plugin.json)* | Optional. |
| `HASURA_PORT` | *(see plugin.json)* | Optional. |
| `NSELF_LEGACY_ENV_ORDER` | *(see plugin.json)* | Optional. |

## Commands

`nself-federation` subcommands (installed alongside the plugin):

- `nself-federation compose`
- `nself-federation status`
- `nself-federation introspect`

## Examples

### Compose

```bash
nself-federation compose
```

### Status

```bash
nself-federation status
```

## Source

[`plugins/federation/`](https://github.com/nself-org/plugins/tree/main/federation)

Manifest: [`plugins/federation/plugin.json`](https://github.com/nself-org/plugins/tree/main/federation/plugin.json)

## See Also

- [[API]] — plugin API surface + changelog
- [[Gateway]] — AI gateway routing rules

← [[Home]] →
