# soak

Manage soak testing lifecycle for nSelf environments: abort an active soak
and roll back to a prior version.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install soak
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-soak`
binary at `~/.nself/plugins/bin/nself-soak`. From then on, `nself soak ...`
routes to it exactly as it did when `soak` was a core command (pre-CLI-R11).

## Usage

```bash
nself soak abort --rollback v1.0.8
nself soak abort --rollback v1.0.8 --dry-run
nself soak abort --rollback v1.0.8 --yes
nself soak abort --rollback v1.0.8 --env staging
nself soak abort --rollback v1.0.8 --env prod --prod-i-mean-it --yes
```

Per the RISK doctrine, `soak abort` never executes without explicit user
confirmation unless `--yes` is passed. Production requires
`--prod-i-mean-it` in addition.

## History

Extracted from `cli/cmd/commands/soak_abort.go` and `cli/internal/soak/` under
CLI-R11. The rollback workflow in `internal/soak/` is unchanged from the
in-core version; only the cobra wiring was rebuilt as a standalone binary.

## Development

```bash
go build -o nself-soak ./cmd/
go test ./...
```
