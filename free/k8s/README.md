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
nself k8s install --domain myapp.com --context my-context
nself k8s install --domain myapp.com --plugins cron --license-secret nself-license
nself k8s status
nself k8s upgrade
nself k8s uninstall --yes
```

`--domain` replaces the project's `BASE_DOMAIN` in the ingress hosts. The licence
key is never sent to helm: create a Secret that holds `NSELF_PLUGIN_LICENSE_KEY`
and pass its name with `--license-secret` (see `nself k8s install --help`).

The chart is embedded in the plugin binary (source in `charts/nself/` in this
repo); there is no chart repository.

Install and upgrade accept `--context` to select an explicit kube context and
override inherited Helm cluster settings. Failed commands show upgrade recovery
advice and bounded, sanitized Helm status diagnostics in human and JSON output.
`nself k8s values --check` detects changes between mapped and unsupported
services. Generated `pullPolicy` values render as container `imagePullPolicy`.

Plugin workloads keep their compose Service name and also expose the
`plugin-<name>` hostname recorded by `nself k8s values`. Dependency URLs such as
`PLUGIN_NOTIFY_INTERNAL_URL=http://plugin-notify:8080` therefore resolve even
when the compose service is named `notify`. Both Services select the same pods
and expose the same ports. Conflicting aliases fail chart rendering.

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

With the pinned tools from `tests/kind/tools.sh` on PATH, run
`KIND_WORK=<scratch-directory> bash tests/kind/plugin-alias.sh` to prove a
dependent plugin reaches `notify` through its injected dependency URL on kind.
