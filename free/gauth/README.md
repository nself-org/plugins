# gauth

Manage Google OAuth tokens for nSelf AI services: status, refresh, and revoke
against the plugin-gauth backend service.

**Tier:** Free (MIT) — no license required. This is a thin HTTP client; the
plugin-gauth backend it talks to (port 3762) is a separate, paid service.

## Installation

```bash
nself plugin install gauth
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-gauth`
binary at `~/.nself/plugins/bin/nself-gauth`. From then on, `nself gauth ...`
routes to it exactly as it did when `gauth` was a core command (pre-CLI-R11).

## Usage

```bash
nself gauth status
nself gauth status --account work --json
nself gauth refresh --account work
nself gauth refresh --account work --force
nself gauth revoke --account work
```

## History

Extracted from `cli/cmd/commands/gauth.go` under CLI-R11. The original file
had zero `internal/*` imports (a pure HTTP client to plugin-gauth), so this
is a verbatim move — no adaptation was required.

## Development

```bash
go build -o nself-gauth ./cmd/
go test ./...
```
