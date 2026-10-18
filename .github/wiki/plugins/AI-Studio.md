# AI-Studio Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install ai-studio` |
| **Commands** | `nself ai-studio` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Bridges a local nSelf instance to Google AI Studio over a secure Cloudflare Tunnel.

## Install

```bash
nself plugin install ai-studio
```

No license key required.

## Description

Google AI Studio integration for local nSelf instances: secure Cloudflare Tunnel bridge with schema-context injection and read-only enforcement.

This is a CLI plugin: it installs the `nself-ai-studio` binary into your plugin path and runs as a command, not a background service.

Category: `integrations`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_AISTUDIO_AUTH_TOKEN` | — | Optional. |
| `NSELF_DEBUG` | *(see plugin.json)* | Optional. |
| `PROJECT_NAME` | *(see plugin.json)* | Optional. |

## Commands

`nself-ai-studio` subcommands (installed alongside the plugin):

- `nself-ai-studio bridge`

## Examples

### Bridge

```bash
nself-ai-studio bridge
```

## Source

[`plugins/ai-studio/`](https://github.com/nself-org/plugins/tree/main/ai-studio)

Manifest: [`plugins/ai-studio/plugin.json`](https://github.com/nself-org/plugins/tree/main/ai-studio/plugin.json)

## See Also

- [[AI-CLI]] — chat + model management
- [[Gauth]] — Google OAuth tokens

← [[Home]] →
