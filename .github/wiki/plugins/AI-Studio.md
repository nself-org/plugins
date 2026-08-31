# AI-Studio Plugin

> Bridges a local nSelf instance to Google AI Studio over a secure Cloudflare Tunnel. **Free — MIT licensed.**

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
