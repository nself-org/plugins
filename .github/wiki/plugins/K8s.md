# K8s Plugin

> Installs and upgrades nSelf on any Kubernetes cluster through the official Helm chart. **Free — MIT licensed.**

## Install

```bash
nself plugin install k8s
```

No license key required.

## Description

Deploy and manage nSelf on any Kubernetes cluster via the official Helm chart: install, upgrade, and status commands wrapping helm.

This is a CLI plugin: it installs the `nself-k8s` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_PLUGIN_LICENSE_KEY` | — | Optional. |
| `KUBECONFIG` | *(see plugin.json)* | Optional. |

## Commands

`nself-k8s` subcommands (installed alongside the plugin):

- `nself-k8s install`
- `nself-k8s upgrade`
- `nself-k8s status`
- `nself-k8s values [--check]`

## Chart values from the compose model

`nself k8s values` makes Kubernetes a projection of the same source as `nself start`. It runs
`docker compose --env-file <each line of .nself/compose-env-files.txt> -f <each line of .nself/compose-files.txt> config --format json`
in the project directory, so `${VAR}` interpolation, `${VAR:-default}` defaults and plugin env files apply exactly as for
`nself restart`. It never parses compose syntax itself and never imports the core CLI. It needs a built project
(`nself build`) and either the Docker Compose plugin or the standalone `docker-compose` binary.

| Flag | Default | Meaning |
|------|---------|---------|
| `--project-dir` | `.` | Project directory (holds `.nself/`) |
| `--out` | `<project-dir>/.nself/generated/k8s` | Output directory |
| `--check` | off | Write nothing; compare an existing `values.yaml` with the compose model |

It writes two files:

- `values.yaml` (mode 0644): images and tags exactly as compose resolves them, env variable names, ports, volumes, readiness
  probes, the workload kind of each service, the Ingress rules, and the `unsupported` and `unmappedRoutes` lists. It holds no
  env value.
- `secrets.yaml` (mode 0600): every env value, per service. Never commit it, never log it. Values never appear on a process
  argument list or in command output.

Install with `helm install nself <chart> -f values.yaml -f secrets.yaml`. A render without `secrets.yaml` fails.

### Parity check

`nself k8s values --check` compares the existing `values.yaml` with the current compose model. It exits 1 and names every
service that is `missing` (in compose, not in the values), `extra` (in the values, not in compose) or `differs` (image, tag or
digest). A service listed under `unsupported` counts as accounted for. Run it in CI to catch drift after a compose change.

### Mapping

| Compose service | Kubernetes object |
|-----------------|-------------------|
| Long-running (default) | Deployment and a Service named like the compose service |
| `postgres` image or service | StatefulSet (one replica, PVC) and a headless Service |
| `restart: "no"` | Job, run as a Helm post-install and post-upgrade hook |
| `nginx` | Not deployed. The root location of each route in `.nself/generated/routes.json` becomes one Ingress |
| Build context and no pullable image | Unsupported, with a reason |
| Privileged, host or shared network, pid or ipc sharing, host devices, invalid DNS name | Unsupported, with a reason |

Every compose service appears in `services` or in `unsupported`. Compose keys with no Kubernetes counterpart on a mapped
service (bind mounts, tmpfs, `user`, `read_only`, `cap_add`, `cap_drop`, `security_opt`, `deploy`, `depends_on`) are listed in
that service's `notMapped`; nothing is approximated silently. A route whose upstream is not a mapped workload port (for
example a frontend on the host) is listed under `unmappedRoutes`.

Every named volume becomes one PersistentVolumeClaim `<release>-<claim>`. Service names equal compose names, so addresses such
as `postgres:5432` keep working; install one release per namespace.

### Chart values schema

The chart in `charts/nself` is generic over these keys. No image reference is typed in the chart.

| Key | Source | Meaning |
|-----|--------|---------|
| `project` | generated | Compose project name |
| `services.<name>.kind` | generated | `deployment`, `statefulset`, `job` or `ingress` |
| `services.<name>.image`, `tag`, `digest` | generated | Image as compose resolves it |
| `services.<name>.entrypoint`, `command` | generated | Container `command` and `args` |
| `services.<name>.env` | generated | Env variable names; values come from `secrets.<name>` |
| `services.<name>.ports`, `mounts`, `probe`, `notMapped` | generated | Container ports, volume mounts, readiness probe, unmapped keys |
| `services.<name>.replicas`, `resources` | user | Optional overrides for Deployments |
| `volumes.<volume>.claim` | generated | PVC name suffix; optional `storage` per volume |
| `ingress.rules` | generated | `name`, `host`, `service`, `port`, `scheme` |
| `ingress.className` | user | Ingress class (empty: cluster default) |
| `tls.enabled`, `tls.certManager`, `tls.issuerName` | user | TLS on each Ingress, optionally through cert-manager |
| `storage`, `storageClass` | user | PVC size (default `10Gi`) and storage class |
| `secrets.<name>.<ENV>` | `secrets.yaml` | Env values |

`tests/template-check.sh` renders the chart with the generated values of two fixtures through `helm template` and validates
every manifest with `kubeconform -strict`. Tool images are pinned by digest in `tests/tools.env`.

## Install and upgrade

`nself k8s install` and `nself k8s upgrade` run the Helm chart that is embedded in the `nself-k8s` binary. They extract it to a
private temporary directory (mode 0700), run `helm` against that directory and delete it afterwards. There is no chart
repository and no `helm repo add`.

Prerequisites: `helm` and `kubectl` on `PATH` (you install them), a reachable cluster, and a built project with generated
values:

```bash
nself build
nself k8s values                      # writes .nself/generated/k8s/values.yaml and secrets.yaml
nself k8s install --domain myapp.com  # waits until every workload is ready
nself k8s status
nself k8s upgrade --domain myapp.com  # after nself build and nself k8s values
```

Both commands refuse (exit 1, `run nself k8s values`) when `values.yaml` or `secrets.yaml` is missing.

| Flag | Default | Meaning |
|------|---------|---------|
| `--domain` | none | Domain of the deployment (required for install) |
| `--cluster` | none | Path to a kubeconfig. Without it helm reads `KUBECONFIG` or `~/.kube/config` |
| `--release` | `nself` | Helm release name |
| `--plugins` | none | Comma-separated plugins the release installs (`plugins.install`) |
| `--project-dir` | `.` | Project directory that holds `.nself/generated/k8s` |
| `--wait` | on | Wait until every workload is ready |
| `--timeout` | `10m` | How long helm waits |

What it passes to helm: `--values values.yaml --values secrets.yaml` and, when you set `--domain`, `--plugins` or
`NSELF_PLUGIN_LICENSE_KEY`, a third `--values` file written to the same private directory. Secrets and the licence key travel
in these files, never on the command line, and never through `--set`. `upgrade` applies the current values afresh (no
`--reuse-values`), so a service you removed from the compose model leaves the release; pass the same `--domain` and `--plugins`
as at install time.

## Try it on kind

`free/k8s/tests/kind/run.sh` is the proof the `k8s-kind` workflow runs: it builds `nself` from `nself-org/cli` at a pinned
commit, lays this plugin out the way `nself add` does, builds a minimal fixture project, runs `nself k8s values`,
`nself k8s install` and `nself k8s upgrade` on a kind cluster, waits for every pod to be Ready and POSTs `{__typename}` to
Hasura through `kubectl port-forward`. It needs docker, go and git; it downloads kind, kubectl and helm at the versions and
checksums pinned in `free/k8s/tests/kind/tools.env`.

```bash
bash free/k8s/tests/kind/run.sh
```

The workflow runs on pull requests that touch `free/k8s/**` and by `workflow_dispatch` with inputs `source` (`branch`, or
`registry` once the registry leg exists), `cli_ref` and `leg` (`install`; `day2` follows with backup and restore).

## Source

[`plugins/k8s/`](https://github.com/nself-org/plugins/tree/main/k8s)

Manifest: [`plugins/k8s/plugin.json`](https://github.com/nself-org/plugins/tree/main/k8s/plugin.json)

## See Also

- [[Infra]] — provision infrastructure via Terraform
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
