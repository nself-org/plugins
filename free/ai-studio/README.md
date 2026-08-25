# ai-studio

Google AI Studio integration for local nSelf instances: a secure Cloudflare
Tunnel bridge with Postgres schema-context injection and read-only
enforcement, so Gemini can query your local schema without any cloud
deployment.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install ai-studio
```

This is a CLI-proxy plugin, not a long-running service managed by
`nself build`/`nself start`: it has no port in the port registry and no
database tables. Installing it places the `nself-ai-studio` binary at
`~/.nself/plugins/bin/nself-ai-studio`. From then on, `nself ai-studio ...`
routes to it exactly as it did when `ai-studio` was a core command
(pre-CLI-R11).

## Usage

```bash
nself ai-studio bridge
nself ai-studio bridge --no-context
nself ai-studio bridge --dry-run
nself ai-studio bridge --ip-allowlist 192.168.1.0/24
nself ai-studio bridge --idle-timeout 60
```

The bridge issues a short-lived session token, enforces schema READ-ONLY (no
mutations, DDL, or DML through the tunnel), injects the Postgres schema as
`X-Nself-Schema-Context`, and auto-closes after 30 minutes of inactivity.

## History

Extracted from `cli/cmd/aistudio/*` under CLI-R11. The original package had
zero `internal/*` imports (a fully self-contained Cloudflare Tunnel bridge),
so this is a verbatim move — only the package name and the root command's
`Use`/silence fields changed.

## Development

```bash
go build -o nself-ai-studio ./cmd/
go test ./...
```
