# k8s

Deploy and manage nSelf on any Kubernetes cluster via the official Helm
chart: install, upgrade, and status commands wrapping `helm`.

**Tier:** Free (MIT) — no license required.

**Status:** PLANNED — deferred (requires UD-12 minor release approval). The
Helm chart under `charts/nself/templates/` has Postgres (StatefulSet +
Service), Hasura (Deployment + Service), Auth (Deployment), and an nginx
Ingress (+ optional cert-manager `Certificate`), but has not been
helm-lint/template validated against a real cluster since extraction. This
plugin is a pure move of the unfinished in-core command, not a completion of
it.

## Installation

```bash
nself plugin install k8s
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table. Installing it places the `nself-k8s`
binary at `~/.nself/plugins/bin/nself-k8s`. From then on, `nself k8s ...`
routes to it exactly as it did when `k8s` was a core command (pre-CLI-R11).

Requires `helm` to be installed: https://helm.sh

## Usage

```bash
nself k8s install --domain myapp.com
nself k8s install --domain myapp.com --cluster ~/.kube/config --release my-nself
nself k8s status
nself k8s upgrade
```

The official chart is published at https://charts.nself.org (source lives
in `charts/nself/` in this repo).

## History

Extracted from `cli/cmd/commands/k8s.go`, `cli/internal/k8s/`, and
`cli/charts/nself/` as a CLI-R11 thin-core extraction. The Helm-wrapping
logic in `internal/k8s/` is unchanged from the in-core version; only the
cobra wiring and terminal-output helpers were rebuilt as a standalone binary
(the plugin cannot import the CLI's `internal/ui` package across the module
boundary — see `internal/tui/tui.go`).

## Development

```bash
go build -o nself-k8s ./cmd/
go test ./...
```
