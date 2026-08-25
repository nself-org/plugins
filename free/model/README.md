# model

Manage local AI models via Ollama: list, pull, remove, update, and benchmark,
plus the legacy `ollama` command tree.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install model
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-model`
binary at `~/.nself/plugins/bin/nself-model`. From then on, `nself model ...`
and `nself ollama ...` route to it exactly as they did when `model` was a
core command (pre-CLI-R11).

## Usage

```bash
nself model list
nself model pull llama3.2:3b
nself model remove gemma-3-4b
nself model update llama3.2:3b
nself model benchmark llama3.2:3b

# Legacy spelling — CLI-R09 rewrites bare `nself ollama` to `nself model ollama`.
nself ollama models list
nself ollama models pull llama3.2:3b
nself ollama status
```

## History

Extracted from `cli/cmd/commands/model*.go` and `ollama*.go` under CLI-R11.
Both were pure HTTP clients to the Ollama API with zero `internal/*`
imports, so this is a verbatim move — no adaptation was required. `ollama`
was already a subcommand of `model`, not a separate top-level command
(CLI-R09), so no separate plugin/binary is needed for it.

## Development

```bash
go build -o nself-model ./cmd/
go test ./...
```
