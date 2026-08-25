# claw-cli

CLI client for the nClaw AI assistant: prompt, chat, pairing, keys,
memories, topics, sessions, an MCP server, an OpenAI-compatible proxy, and
schema migrations.

**Tier:** Free (MIT) — no license required. This is a thin client; the
ɳClaw backend it talks to (`plugin-claw`, plugins-pro/paid/claw) is a
separate, paid service.

Named `claw-cli` (not `claw`) to avoid colliding with that paid backend
service, which is already registered under the name `claw`. The command
surface is unaffected: `nself claw ...` still resolves here.

## Installation

```bash
nself plugin install claw-cli
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server of its own, and no database table. Installing it places the
`nself-claw` binary at `~/.nself/plugins/bin/nself-claw`. From then on,
`nself claw ...` routes to it exactly as it did when `claw` was a core
command (pre-CLI-R11).

## Usage

```bash
nself claw config set server https://claw.example.com
nself claw config set api-key <key>

nself claw prompt "What's the weather like?"
nself claw chat
nself claw pair
nself claw status
nself claw topics
nself claw memories
nself claw keys
nself claw session start
nself claw proxy
nself claw mcp
nself claw export --format json
nself claw migrate
```

## History

Extracted from `cli/cmd/commands/claw*.go` and `cli/internal/claw/*` under
CLI-R11. Most of the family is a pure move (no `internal/*` dependency
beyond stdlib+cobra+third-party). Three pieces needed adaptation because the
core packages they used are unreachable from a plugin module:

- `internal/errs.Exit` → `cmd/exit.go` (the one exit-code case, `claw keys
  create --bootstrap`'s validation gate).
- `internal/auth.ReadAuthFile` → `internal/auth/storage.go` (read-only copy,
  used only by `claw session`).
- `internal/config.Load` → `internal/projectenv.Load` (the six env vars
  `claw migrate`/`claw pair` actually read, not the full 7000+ line env
  cascade).

## Development

```bash
go build -o nself-claw ./cmd/
go test ./...
```
