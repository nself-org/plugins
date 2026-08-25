# alerts

Manage Prometheus alert rules and Alertmanager silences.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install alerts
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-alerts`
binary at `~/.nself/plugins/bin/nself-alerts`. From then on, `nself alerts
...` routes to it exactly as it did when `alerts` was a core command
(pre-CLI-R11).

## Usage

```bash
nself alerts list                              # list all 16 default alert rules
nself alerts list --severity P1                # filter by severity
nself alerts list --json                       # machine-readable output
nself alerts silence ServiceDown --reason "planned maintenance"
nself alerts silence ServiceDown --duration 30m --reason "..."
nself alerts test ServiceDown                  # fire a synthetic test alert
nself alerts test ServiceDown --alertmanager-url http://127.0.0.1:9093
```

## History

Extracted from `cli/cmd/commands/alerts.go` and `cli/internal/alerts/` as a
CLI-R11 thin-core extraction. `internal/alerts` had no dependency beyond the
standard library (it shells out to `amtool`/`curl` as best-effort integrations)
so it moved wholesale, unchanged. Only the cobra wiring and terminal-output
helpers were rebuilt as a standalone binary (the plugin cannot import the
CLI's `internal/ui` package across the module boundary — see
`internal/tui/tui.go`).

One core-side file depended on the same package purely to describe two
static admin-specific Prometheus alert rules (`internal/admin/alerts.go`,
never actually called outside its own test); that file now defines its own
minimal type instead of importing the extracted package.

## Development

```bash
go build -o nself-alerts ./cmd/
go test ./...
```
