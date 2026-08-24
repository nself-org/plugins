# costs

Show an itemized breakdown of estimated monthly costs for your nself
install: Hetzner VPS, Cloudflare, Vercel, Stripe transaction fees, and
installed paid plugin licenses.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install costs
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-costs`
binary at `~/.nself/plugins/bin/nself-costs`. From then on, `nself costs ...`
routes to it exactly as it did when `costs` was a core command (pre-CLI-R11).

## Usage

```bash
nself costs                          # auto-detect server type from env
nself costs --server-type cx23       # override server type
nself costs --format json
```

## History

Extracted from `cli/cmd/commands/costs.go` under CLI-R11. The core CLI's
`internal/plugin.ListInstalled()` call (used to find installed paid plugin
tiers) is unreachable across the module boundary and is genuinely
core-shared infrastructure, not something to fork wholesale. This plugin's
`internal/plugininfo` package reimplements only the narrow read this command
needs — a plugin's `tier`/`requires_license`/`licenseType` fields from its
`plugin.json` — not the full manifest validation `internal/plugin` performs
at install time. See that package's doc comment for the exact scope of the
simplification.

## Development

```bash
go build -o nself-costs ./cmd/
go test ./...
```
