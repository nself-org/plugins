# Gateway Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install gateway` |
| **Commands** | `nself gateway` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Manages the nSelf AI gateway's provider key vault, quota usage, and routing rules.

## Install

```bash
nself plugin install gateway
```

No license key required.

## Description

Manage the nSelf AI gateway (nself-ai-gateway, port 3761): service health, provider key vault, quota usage, and routing rules.

This is a CLI plugin: it installs the `nself-gateway` binary into your plugin path and runs as a command, not a background service.

Category: `integrations`. Current version: `1.0.0`.

## Commands

`nself-gateway` subcommands (installed alongside the plugin):

- `nself-gateway status`
- `nself-gateway keys`
- `nself-gateway add`
- `nself-gateway remove <id>`
- `nself-gateway quota`
- `nself-gateway routes`
- `nself-gateway list`

## Examples

### Status

```bash
nself-gateway status
```

### Keys

```bash
nself-gateway keys
```

## Source

[`plugins/gateway/`](https://github.com/nself-org/plugins/tree/main/gateway)

Manifest: [`plugins/gateway/plugin.json`](https://github.com/nself-org/plugins/tree/main/gateway/plugin.json)

## See Also

- [[Gauth]] — Google OAuth token management
- [[Model]] — manage local Ollama models

← [[Home]] →
