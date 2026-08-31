# Ollama Plugin

> Stands up a local Ollama container and routes nSelf's AI features through it at zero API cost. **Free — MIT licensed.**

## Install

```bash
nself plugin install ollama
```

No license key required.

## Description

One-click offline LLM stack. Stands up an Ollama Docker container, auto-pulls gemma-3-4b on first start, and registers as a provider in plugin-ai. All nSelf AI features route through Ollama when NSELF_AI_PROVIDER=ollama. Zero cloud dependency, zero API key, zero usage cost after install. GPU passthrough is optional and localhost-only (port 11434 binds to 127.0.0.1 and is never exposed externally).

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `integrations`. Current version: `1.1.1`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_AI_PROVIDER` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_HOST` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_DEFAULT_MODEL` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_AUTO_PULL` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_GPU` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_CONTEXT_WINDOW` | *(see plugin.json)* | Optional. |
| `NSELF_OLLAMA_TIMEOUT_SECONDS` | *(see plugin.json)* | Optional. |
| `OLLAMA_ENABLED` | *(see plugin.json)* | Optional. |
| `PLUGIN_AI_OLLAMA_MODEL` | *(see plugin.json)* | Optional. |
| `PLUGIN_AI_OLLAMA_URL` | *(see plugin.json)* | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 11434 | Ollama service |

## Database Schema

1 table(s) added to your Postgres database (prefix: `np_ollama_`):

- `np_ollama_model_registry`

## REST API

```
GET  /api/tags            — List pulled models (proxied Ollama API)
POST /api/pull            — Pull a model
POST /api/generate        — Generate a completion (proxied Ollama API)
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `(localhost only, no nginx route: 127.0.0.1:11434)` | Ollama never exposed externally |

## Examples

### Check health

```bash
curl http://localhost:11434/health
```

## Source

[`plugins/ollama/`](https://github.com/nself-org/plugins/tree/main/ollama)

Manifest: [`plugins/ollama/plugin.json`](https://github.com/nself-org/plugins/tree/main/ollama/plugin.json)

## See Also

- [[Model]] — manage local Ollama models
- [[AI-CLI]] — chat + model management

← [[Home]] →
