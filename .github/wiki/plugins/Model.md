# Model Plugin

> Manages local Ollama models: pull, benchmark, update, and remove, from one command tree. **Free — MIT licensed.**

## Install

```bash
nself plugin install model
```

No license key required.

## Description

Manage local AI models via Ollama: list, pull, remove, update, benchmark, plus the legacy `ollama` command tree.

This is a CLI plugin: it installs the `nself-model` binary into your plugin path and runs as a command, not a background service.

Category: `integrations`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_OLLAMA_HOST` | *(see plugin.json)* | Optional. |
| `PLUGIN_AI_OLLAMA_URL` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_TIMEOUT_SECONDS` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_DEFAULT_MODEL` | *(see plugin.json)* | Optional. |

## Commands

`nself-model` subcommands (installed alongside the plugin):

- `nself-model list`
- `nself-model pull <model>`
- `nself-model remove <model>`
- `nself-model update <model>`
- `nself-model benchmark <model>`
- `nself-model status`
- `nself-model ollama`

## Examples

### List

```bash
nself-model list
```

### Pull

```bash
nself-model pull <model>
```

## Source

[`plugins/model/`](https://github.com/nself-org/plugins/tree/main/model)

Manifest: [`plugins/model/plugin.json`](https://github.com/nself-org/plugins/tree/main/model/plugin.json)

## See Also

- [[Ollama]] — one-click offline LLM stack
- [[AI-CLI]] — chat + model management

← [[Home]] →
