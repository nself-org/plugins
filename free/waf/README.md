# waf

Web Application Firewall management for nSelf: enable Coraza with the OWASP
Core Rule Set, switch between detection and blocking mode, and review recent
WAF events from the nginx audit log.

**Tier:** Free (MIT) — no license required. WAF is Security-Always-Free per
the nSelf product doctrine.

## Installation

```bash
nself plugin install waf
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-waf`
binary at `~/.nself/plugins/bin/nself-waf`. From then on, `nself waf ...`
routes to it exactly as it did when `waf` was a core command (pre-CLI-R11).

## Usage

```bash
nself waf enable              # write nginx/waf/{coraza,custom}.conf in DetectionOnly mode
nself waf mode blocking       # enforce OWASP CRS rules
nself waf mode detection      # log only, do not block
nself waf report              # show recent WAF events (last 24h by default)
nself waf report --since 7d
```

`enable` and `mode` operate on the current nSelf project (found by walking up
from the working directory looking for `.env`/`.env.dev`/`.env.staging`/
`.env.prod`); `report` shells out to `docker compose exec nginx` to tail the
WAF audit log.

## History

Extracted from `cli/cmd/commands/waf.go` under CLI-R11. The check logic and
config-writing behavior are unchanged from the in-core version; only the
cobra wiring and two small helpers were rebuilt as standalone packages
(`internal/tui` in place of the CLI's `internal/ui`, `internal/projectroot`
in place of `internal/config.FindNSelfRoot`) — the plugin cannot import the
CLI's `internal/*` packages across the module boundary.

## Development

```bash
go build -o nself-waf ./cmd/
go test ./...
```
