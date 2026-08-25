# watchdog

Self-healing container watchdog with circuit breaker.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install watchdog
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-watchdog`
binary at `~/.nself/plugins/bin/nself-watchdog`. From then on, `nself
watchdog ...` routes to it exactly as it did when `watchdog` was a core
command (pre-CLI-R11).

## Usage

```bash
nself watchdog status                    # circuit breaker states (exit 2 if any open)
nself watchdog status --json
nself watchdog reset-breakers            # reset all tripped breakers
nself watchdog reset <service>           # reset one service, including PERMANENT_OPEN
nself watchdog reset <service> --force   # required when NSELF_ENV=prod
nself watchdog history --since 24h
nself watchdog test-alert --service foo --severity critical
```

Escalation (Telegram + email) is configured via env vars:
`WATCHDOG_TG_BOT_TOKEN`, `WATCHDOG_TG_CHAT_ID`, `WATCHDOG_SMTP_HOST`,
`WATCHDOG_SMTP_PORT` (default 587), `WATCHDOG_SMTP_FROM`, `WATCHDOG_SMTP_TO`
(default `ops@nself.org`), `WATCHDOG_SMTP_USER`, `WATCHDOG_SMTP_PASS`.

## History

Extracted from `cli/cmd/commands/watchdog.go` and `cli/internal/watchdog/`
as a CLI-R11 thin-core extraction. Two dependencies could not move with it:

- `internal/health.DockerClient` (a Docker-CLI shell-exec adapter: `docker
  ps` / `docker inspect` / `docker restart`) lives in the same core file
  (`internal/health/restarter.go`) that `cli/cmd/commands/start.go` — a
  golden-path command — also constructs its `Restarter` from. Forking that
  file risked the two copies drifting on a path the CLI depends on to boot a
  stack, so instead this plugin reimplements just the narrow adapter it
  needs in `internal/watchdog/docker.go`: the same three `docker` subcommands,
  same flags, same output parsing, no shared state.
- `internal/httptimeout` (a scoped-`*http.Client` helper) is replaced with
  an inline client at the same 30s default the CLI falls back to.

Everything else — circuit-breaker state machine, escalation config,
Telegram/SMTP delivery, Prometheus metrics — moved unchanged. Only the cobra
wiring and terminal-output helpers were rebuilt as a standalone binary (the
plugin cannot import the CLI's `internal/ui` package across the module
boundary — see `internal/tui/tui.go`).

## Development

```bash
go build -o nself-watchdog ./cmd/
go test ./...
```
