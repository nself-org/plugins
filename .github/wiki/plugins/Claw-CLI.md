# Claw-CLI Plugin

> Command-line client for the nClaw AI assistant: chat, memories, sessions, and an MCP server. **Free — MIT licensed.**

## Install

```bash
nself plugin install claw-cli
```

No license key required.

## Description

CLI client for the nClaw AI assistant: prompt, chat, pairing, keys, memories, topics, sessions, MCP server, a widely-compatible chat completion proxy, and schema migrations.

This is a CLI plugin: it installs the `nself-claw-cli` binary into your plugin path and runs as a command, not a background service.

Category: `integrations`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_CLAW_API_KEY` | — | Optional. |
| `NSELF_CLAW_SERVER` | *(see plugin.json)* | Optional. |
| `NSELF_EXTERNAL_URL` | *(see plugin.json)* | Optional. |
| `NSELF_AICC_URL` | *(see plugin.json)* | Optional. |
| `PLUGIN_CLAW_INTERNAL_URL` | *(see plugin.json)* | Optional. |
| `CLAW_PROXY_ALLOWED_ORIGINS` | *(see plugin.json)* | Optional. |
| `NSELF_HTTP_TIMEOUT_DEFAULT` | *(see plugin.json)* | Optional. |

## Commands

`nself-claw-cli` subcommands (installed alongside the plugin):

- `nself-claw-cli chat`
- `nself-claw-cli pair`
- `nself-claw-cli unlock`
- `nself-claw-cli keys`
- `nself-claw-cli config`
- `nself-claw-cli set <key> <value>`
- `nself-claw-cli memories`
- `nself-claw-cli search <query>`
- `nself-claw-cli topics`
- `nself-claw-cli session`
- `nself-claw-cli create`
- `nself-claw-cli attach <id>`
- `nself-claw-cli stop <id>`
- `nself-claw-cli list`
- `nself-claw-cli export`
- `nself-claw-cli revoke <id>`
- `nself-claw-cli migrate`
- `nself-claw-cli mcp`
- `nself-claw-cli status`

## Examples

### Chat

```bash
nself-claw-cli chat
```

### Pair

```bash
nself-claw-cli pair
```

## Source

[`plugins/claw-cli/`](https://github.com/nself-org/plugins/tree/main/claw-cli)

Manifest: [`plugins/claw-cli/plugin.json`](https://github.com/nself-org/plugins/tree/main/claw-cli/plugin.json)

## See Also

- [[AI-CLI]] — chat + model management
- [[Gateway]] — AI gateway key vault

← [[Home]] →
