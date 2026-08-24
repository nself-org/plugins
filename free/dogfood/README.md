# dogfood

Production dogfood audit and reporting: 21 read-only checks covering backups,
disaster recovery, tenancy, licensing, secrets, migrations, monitoring,
security, the watchdog, and queue health.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install dogfood
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-dogfood`
binary at `~/.nself/plugins/bin/nself-dogfood`. From then on, `nself dogfood
...` routes to it exactly as it did when `dogfood` was a core command
(pre-CLI-R11).

## Usage

```bash
nself dogfood audit          # run all 21 checks
nself dogfood audit --json   # machine-readable output
nself dogfood report         # show the most recently saved report
```

Exit codes from `audit` (scripts/CI may depend on these):

| Code | Meaning |
|------|---------|
| 0 | All checks passed |
| 1 | One or more failures |
| 2 | Warnings only |

## History

Extracted from `cli/cmd/commands/dogfood.go` and `cli/internal/dogfood/` as
the first CLI-R11 thin-core extraction slice. The check logic in
`internal/dogfood/` is unchanged from the in-core version; only the cobra
wiring and terminal-output helpers were rebuilt as a standalone binary (the
plugin cannot import the CLI's `internal/ui` package across the module
boundary — see `internal/tui/tui.go`).

## Development

```bash
go build -o nself-dogfood ./cmd/
go test ./...
```
