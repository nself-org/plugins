# AI-CLI Plugin

> Chat, local Ollama model management, and Gemini key-pool provisioning from the command line. **Free — MIT licensed.**

## Install

```bash
nself plugin install ai-cli
```

No license key required.

## Description

AI operations for nSelf: chat, local Ollama model management, and Gemini API key pool provisioning and rotation.

This is a CLI plugin: it installs the `nself-ai-cli` binary into your plugin path and runs as a command, not a background service.

Category: `automation`. Current version: `1.0.0`.

## Commands

`nself-ai-cli` subcommands (installed alongside the plugin):

- `nself-ai-cli chat <message>`
- `nself-ai-cli models list`
- `nself-ai-cli models add <model>`
- `nself-ai-cli models remove <model>`
- `nself-ai-cli models swap <model>`
- `nself-ai-cli pool status`
- `nself-ai-cli pool rotate`
- `nself-ai-cli pool add`
- `nself-ai-cli pool remove`
- `nself-ai-cli pool test`
- `nself-ai-cli pool daily-reset`
- `nself-ai-cli health`

## Examples

### Chat

```bash
nself-ai-cli chat <message>
```

### Models

```bash
nself-ai-cli models list
```

## Source

[`plugins/ai-cli/`](https://github.com/nself-org/plugins/tree/main/ai-cli)

Manifest: [`plugins/ai-cli/plugin.json`](https://github.com/nself-org/plugins/tree/main/ai-cli/plugin.json)

## See Also

- [[Model]] — manage local Ollama models
- [[Gateway]] — AI gateway key vault and quota

← [[Home]] →
