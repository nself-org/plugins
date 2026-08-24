# dlq

Manage dead-letter queues (DLQ) for nSelf plugins: re-enqueue rows that
failed processing back to the work queue, with safe row limits and dry-run
preview.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install dlq
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table of its own. Installing it places the
`nself-dlq` binary at `~/.nself/plugins/bin/nself-dlq`. From then on,
`nself dlq ...` routes to it exactly as it did when `dlq` was a core command
(pre-CLI-R11).

## Usage

```bash
nself dlq replay mux
nself dlq replay mux --dry-run
nself dlq replay mux --max-rows 50
nself dlq replay mux --filter status=quarantined
```

Only replay after fixing the upstream bug that caused the failures —
replaying before that causes the rows to re-DLQ immediately. Operator-level
authentication is required (`NSELF_API_TOKEN` or `NSELF_API_URL`).

## History

Extracted from `cli/cmd/commands/dlq_replay.go` under CLI-R11. The command
had zero dependencies on the core CLI's `internal/*` packages beyond
`internal/dlq` itself (it only ever talks to the running stack's REST API
over HTTP), so this is a straight file move with no shim code needed.

## Development

```bash
go build -o nself-dlq ./cmd/
go test ./...
```
