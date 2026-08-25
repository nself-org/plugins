# gdpr

GDPR data portability (Art. 20) and right-to-erasure (Art. 17) tools for
self-hosted nSelf instances.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install gdpr
```

This is a CLI-proxy plugin, not a long-running service: there is no port and
no HTTP server, but it reads and writes `np_gdpr_requests` directly via
`DATABASE_URL`. Installing it places the `nself-gdpr` binary at
`~/.nself/plugins/bin/nself-gdpr`. From then on, `nself gdpr ...` routes to
it exactly as it did when `gdpr` was a core command (pre-CLI-R11).

## Usage

```bash
nself gdpr export --user abc123 --output /tmp/export.zip
nself gdpr export --user abc123 --format csv --output /tmp/export.zip
nself gdpr delete --user abc123 --dry-run
nself gdpr delete --user abc123
nself gdpr forget --user abc123        # alias for delete
nself gdpr status --request <id>
nself gdpr list-requests
nself gdpr list-requests --status pending
```

All GDPR requests are logged to `np_gdpr_requests` for audit purposes. That
table is append-only: rows are never deleted.

## History

Extracted from `cli/cmd/commands/gdpr*.go` and `cli/internal/gdpr/` under
CLI-R11. `internal/gdpr` was exclusive to these commands (confirmed via
`grep -rn 'internal/gdpr"' cmd internal` before moving) and moved wholesale,
tests included. One small dependency, `internal/database.SanitizeIdentifier`,
was copied verbatim into this plugin's own `internal/database` package since
the CLI's `internal/database` is unreachable across the module boundary —
it is security-sensitive code (SQL identifier sanitization), so it is copied
byte-for-byte rather than reimplemented from a description. The terminal
output helpers (`internal/tui`) are the same simplification pattern used by
every other CLI-R11 plugin.

## Development

```bash
go build -o nself-gdpr ./cmd/
go test ./...
```
