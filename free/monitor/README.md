# monitor

Monitoring stack management: upgrade the bundled Grafana dashboards.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install monitor
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-monitor`
binary at `~/.nself/plugins/bin/nself-monitor`. From then on, `nself monitor
...` routes to it exactly as it did when `monitor` was a core command
(pre-CLI-R11).

## Usage

```bash
nself monitor upgrade-dashboards          # check dashboards, provision only if stale
nself monitor upgrade-dashboards --force  # re-provision every dashboard
```

Re-provisions all 11 nSelf Grafana dashboards from the bundled templates:
System Overview, Postgres, Hasura, Nginx, Per-Plugin, Request Latency, AI Cost
Tracker, User Activity, Error Heatmap, Backups, Licenses.

## History

Extracted from `cli/cmd/commands/monitor.go` as a CLI-R11 thin-core
extraction. The command had no dedicated `internal/` package in core — the
dashboard list was inline — so the move is a straight copy with no logic
left behind. Only the cobra wiring and terminal-output helpers were rebuilt
as a standalone binary (the plugin cannot import the CLI's `internal/ui`
package across the module boundary — see `internal/tui/tui.go`).

## Development

```bash
go build -o nself-monitor ./cmd/
go test ./...
```
