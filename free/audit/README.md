# audit

Ecosystem documentation audit: banned words, dead links, and missing anchors
across READMEs, wiki pages, `.claude/docs/`, SPORT (F01-F15), MASTER-LISTS,
FEATURES.md, PPI, and every PRI.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install audit
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-audit`
binary at `~/.nself/plugins/bin/nself-audit`. From then on, `nself audit ...`
routes to it exactly as it did when `audit` was a core command (pre-CLI-R11).

## Usage

```bash
nself audit docs                          # scan cwd, print JSON to stdout
nself audit docs --summary                # also print a human summary
nself audit docs --output report.json     # write to a file
nself audit docs --fix                    # apply safe auto-fixes in place
nself audit docs --quarter 2026-Q3        # override the quarter label
```

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | No findings |
| 1 | Findings present (non-fatal, report emitted) |
| 2 | Scan failed (I/O, unreadable tree) |

## History

Extracted from `cli/cmd/commands/audit.go` and the docs-audit files of
`cli/internal/audit/` (`docs.go`, `docs_scan.go`, `docs_output_fix.go`,
`autofix.go`) under CLI-R11. `internal/audit/table_audit.go` and
`event_log.go` stayed in the core CLI: they back `nself plugin audit-tables`
and the security event log respectively, not this command. The scan/auto-fix
logic is unchanged from the in-core version; only the cobra wiring and
terminal-output helpers were rebuilt as a standalone binary (the plugin
cannot import the CLI's `internal/ui` package across the module boundary —
see `internal/tui/tui.go`).

## Development

```bash
go build -o nself-audit ./cmd/
go test ./...
```
