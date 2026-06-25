# ɳSelf Ollama Plugin

One-click offline LLM. Install Ollama, auto-pull `gemma-3-4b`, and route all nSelf AI features through a local model — zero cloud dependency, zero API key, zero usage cost.

## What it does

- Stands up an [Ollama](https://ollama.com) Docker container
- Auto-pulls `gemma-3-4b` on first install (4 GB, runs on CPU)
- Registers as an AI provider in `plugin-ai` — set `NSELF_AI_PROVIDER=ollama` to use it
- GPU passthrough optional: set `NSELF_OLLAMA_GPU=true` before `nself build`

## Quick start

```sh
nself install ollama
nself start ollama
```

After start, Ollama listens at `http://localhost:11434` (localhost-only, never exposed externally).

## Supported models

| Model | Size | Notes |
|---|---|---|
| `gemma-3-4b` | ~4 GB | Default — pulled automatically |
| `llama3.2:3b` | ~2 GB | Fast, lower quality |
| `mistral:7b` | ~4.1 GB | Strong reasoning |

Pull additional models:

```sh
nself ollama models pull llama3.2:3b
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `OLLAMA_MODEL` | `gemma-3-4b` | Model pulled on install |
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama endpoint |
| `OLLAMA_CONTAINER` | `nself-ollama` | Docker container name |
| `NSELF_OLLAMA_GPU` | `false` | Enable GPU passthrough |
| `OLLAMA_KEEP_ALIVE` | `5m` | Model keep-alive timeout |

## Privacy

All inference runs locally. No data leaves your machine.
