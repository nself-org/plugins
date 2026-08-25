# gateway

Manage the nSelf AI gateway (nself-ai-gateway, port 3761): service health,
provider key vault, quota usage, and routing rules.

**Tier:** Free (MIT) — no license required. This is a thin HTTP client; the
nself-ai-gateway backend it talks to is a separate service.

## Installation

```bash
nself plugin install gateway
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table of its own. Installing it places the
`nself-gateway` binary at `~/.nself/plugins/bin/nself-gateway`. From then on,
`nself gateway ...` routes to it exactly as it did when `gateway` was a core
command (pre-CLI-R11).

## Usage

```bash
nself gateway status
nself gateway keys list
nself gateway keys add --provider anthropic --label my-key
nself gateway keys remove <id>
nself gateway quota
nself gateway quota --provider anthropic --model claude-sonnet
nself gateway routes
```

## History

Extracted from `cli/cmd/commands/gateway_cmd*.go` and `cli/internal/gateway/`
under CLI-R11. `internal/gateway` was only ever imported by these command
files, so it moved wholesale. Its `internal/ports` dependency (3 int
constants) and the `gatewayToken()` helper's `internal/auth.ReadAuthFile()`
call were narrowed to small local copies, since neither core package is
reachable from a plugin module.

## Development

```bash
go build -o nself-gateway ./cmd/
go test ./...
```
