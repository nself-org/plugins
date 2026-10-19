# K8s Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install k8s` |
| **Commands** | `nself k8s`: `backup`, `install`, `restore`, `status`, `uninstall`, `upgrade`, `values` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Installs and upgrades nSelf on any Kubernetes cluster through the official Helm chart.

## Install

```bash
nself plugin install k8s
```

No license key required.

## Description

Deploy and manage nSelf on any Kubernetes cluster via the official Helm chart: install, upgrade, and status commands wrapping helm, plus backup and restore of the postgres database with content fingerprints.

This is a CLI plugin: it installs the `nself-k8s` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_PLUGIN_LICENSE_KEY` | — | Optional. Never sent to the cluster; install and upgrade only warn when it is set. Put it in a Secret and pass `--license-secret` |
| `KUBECONFIG` | *(see plugin.json)* | Optional. |

## Commands

`nself-k8s` subcommands (installed alongside the plugin):

- `nself-k8s values [--check]`
- `nself-k8s install`
- `nself-k8s upgrade`
- `nself-k8s status`
- `nself-k8s uninstall [--yes]`
- `nself-k8s backup [--output FILE] [--tag TAG]`
- `nself-k8s restore <file> --yes`

Every subcommand accepts `--json` (see [JSON output and exit classes](#json-output-and-exit-classes)). The manifest declares
the same surface in its `commands` block, so `nself help --json k8s` lists the seven subcommands with canon `plugin`.

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

Every named volume becomes one PersistentVolumeClaim `<release>-<claim>`, annotated `helm.sh/resource-policy: keep`: helm never
deletes it, neither on uninstall nor on an upgrade that drops or renames the volume (see [Uninstall](#uninstall)). Service names
equal compose names, so addresses such as `postgres:5432` keep working; install one release per namespace.

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
every manifest with `kubeconform -strict`. Tool images are pinned by digest in `tests/tools.env`. `go test` fails when any
PersistentVolumeClaim template lacks `helm.sh/resource-policy: keep` (and checks the rendered fixtures too when helm is on PATH).

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
| `--domain` | none | Required for install. Replaces the project's `BASE_DOMAIN` in the ingress hosts and TLS hosts (see below) |
| `--cluster` | none | Path to a kubeconfig. Without it helm reads `KUBECONFIG` or `~/.kube/config` |
| `--release` | `nself` | Helm release name. A name that starts with `-` is refused in every mode (helm would read it as a flag) |
| `--plugins` | none | Comma-separated plugins (`plugins.install`). The chart checks each against the generated values and fails the render when one is missing |
| `--license-secret` | none | Name of an existing Secret that holds `NSELF_PLUGIN_LICENSE_KEY`. Plugin workloads read the key from it. Must be a valid Secret name |
| `--project-dir` | `.` | Project directory that holds `.nself/generated/k8s` |
| `--wait` | on | Wait until every workload is ready |
| `--timeout` | `10m` | How long helm waits |

What it passes to helm: `--values values.yaml --values secrets.yaml` and, when you set `--domain`, `--plugins` or `--license-secret`, a third
overlay (domain, plugins, the licence Secret name) that goes to helm on stdin (`--values -`), so it is never written to disk. Secrets travel in `secrets.yaml`, never on
the command line, and never through `--set`. helm runs with a cleaned environment: every `NSELF_*` variable is removed and
`HELM_DEBUG` is forced to `false` (with it set, helm prints every value and rendered Secret to the log). Before it runs, the
command prints which kubeconfig helm will use. `upgrade` applies the current values afresh (no `--reuse-values`), so a service
you removed from the compose model leaves the release; pass the same `--domain`, `--plugins` and `--license-secret` as at install time; `upgrade`
prints one line naming the values it is not passing.

`nself k8s status` prints only the release name, namespace, revision, chart version and status. It never prints helm's release
config or manifest, which hold every secret of the stack.

**What the chart reads.**

- `domain`: `nself k8s values` writes the project's `BASE_DOMAIN` as `baseDomain` (from the resolved compose environment: nginx's
  value, or the one value every service agrees on; it writes nothing when there is none or they conflict). With `--domain` set
  and different, every ingress host equal to `baseDomain` becomes the domain, and `api.<base>` becomes `api.<domain>`. Other
  hosts stay. The TLS hosts, the routes in the install notes and the hosts in the `install` success line follow the same rule.
- `plugins.install`: each name must be a plugin service in the generated values (a service called `plugin-<name>`, or one the
  CLI gave the hostname `plugin-<name>`), otherwise the render fails with `plugin <name> is not in the generated values: run
  nself plugin install <name>, then nself k8s values`. A plugin that has no Kubernetes mapping fails with its reason. The list
  adds no workload by itself.
- `license.secretName`: the licence key is **never** a chart value. helm stores every value in the release Secret
  (`sh.helm.release.v1.<release>.vN`, one per revision), so anyone who can read Secrets in the namespace could read the key.
  Create a Secret yourself, without the key on a command line, for example
  `kubectl create secret generic nself-license --from-env-file=<file holding NSELF_PLUGIN_LICENSE_KEY=...>`, then pass
  `--license-secret nself-license` on every install and upgrade. Every plugin workload gets `NSELF_PLUGIN_LICENSE_KEY` from
  that Secret (`secretKeyRef`, env name always `NSELF_PLUGIN_LICENSE_KEY`, key from `license.secretKey`). `nself k8s values`
  never writes `NSELF_PLUGIN_LICENSE_KEY` as a value: it drops the env from the service and lists it under `notMapped`.
  With the variable set and no `--license-secret`, install and upgrade warn that the key is not sent.

With `--domain` and no `baseDomain` in the generated values, install and upgrade warn that `--domain` has no effect
(run `nself k8s values` again). The success line of `install` lists the ingress hosts as URLs (with `--domain` applied), or no URL when the project routes none.

**Postgres init scripts.** The compose bind mount `./postgres/init:/docker-entrypoint-initdb.d` is carried. `nself k8s values`
reads the `*.sql`, `*.sql.gz` and `*.sh` files of that directory (not recursive, sorted by name), writes only their names to
`values.yaml` (`initdb`) and their contents to `secrets.yaml` (mode 0600, because an init script can hold a password). The
chart renders one Secret `<release>-postgres-initdb` and mounts it read-only at `/docker-entrypoint-initdb.d`, so postgres runs
the scripts on its first start (an existing data volume is not initialised again) and hasura-auth finds its schema. Other files
are listed under `notMapped` as `initdb:<name>`. The scripts of one service may total at most 512 KiB (helm keeps every value, `secrets.yaml` included, in one release Secret capped near 1 MiB, and a `.sql.gz`
is stored as base64);
`values` stops with the file name when they do not fit. Two such mounts on one service are an error. Pass `-f secrets.yaml` to helm as always: a name in `values.yaml`
without content in `secrets.yaml` fails the render. `values --check` compares the script names, `baseDomain` and the plugin marker of each service, not script contents.

## Uninstall

`nself k8s uninstall` removes the release with `helm uninstall <release>` (add `--cluster` and `--release` as for install). It is
destructive, so it needs confirmation:

- `--yes` (or `-y`) confirms.
- Without `--yes`, a terminal is asked to type the release name.
- Without `--yes` and without a terminal (a script, CI, stdin from `/dev/null`), or with `--json`, it refuses with E403 and
  exit status 4, and runs no helm command.

The confirmation answer is read up to 4096 bytes; a longer line never matches.

What uninstall keeps:

- Every PersistentVolumeClaim of the release (`<release>-<claim>`, the database and storage data) stays, because the chart marks
  it `helm.sh/resource-policy: keep`. helm lists them as kept resources. The bound volumes stay with them.
- A later `nself k8s install` with the same release name, in the same namespace, adopts the kept claims and finds the data again.
  The kind test proves this: it writes a row, uninstalls, reinstalls and reads the row back.
- To delete the data for good, remove the claims yourself after the uninstall:

  ```bash
  kubectl delete pvc -l app.kubernetes.io/instance=<release>   # add --kubeconfig and -n as for install
  ```

  With a `Delete` reclaim policy (the default of most storage classes, kind's `standard` included) this also deletes the volumes
  and their data. With `Retain` the volumes stay until you delete them.

## Backup and restore

`nself k8s backup` streams `pg_dump -Fc` out of the postgres pod through `kubectl exec` into the nSelf backup layout.
`nself k8s restore <file> --yes` streams it back with `pg_restore --clean --if-exists` and reports success only when the
data it finds equals what the backup recorded. Both need `kubectl` on `PATH` and one running postgres pod of the release
(found by the chart's labels `app.kubernetes.io/name=postgres` and `app.kubernetes.io/instance=<release>`).

| Flag | Command | Meaning |
|------|---------|---------|
| `--cluster` | both | Path to the kubeconfig (default: kubectl's own) |
| `--namespace` | both | Namespace of the release (default: the kubeconfig context's namespace) |
| `--release` | both | Helm release name (default `nself`) |
| `--output` | backup | Write the dump to exactly this file; it must not exist |
| `--tag` | backup | Label appended to the file name (`[A-Za-z0-9._-]`) |
| `--project-dir` | backup | Project directory that holds `.env` (default `.`); `BACKUP_DIR` is relative to it |
| `--yes` | restore | Required. Without it restore exits 4 (E403) in every mode and runs nothing |

### File layout and sidecar

The dump is `<BACKUP_DIR>/<PROJECT_NAME>_full_<YYYYMMDD_HHMMSS>[_<tag>].dump` (mode 0600, directory 0700). `PROJECT_NAME` and
`BACKUP_DIR` (default `./backups`) come from the process environment, else from the project `.env`. An existing file is never
replaced. The dump is first written to `<dir>/.meta/<name>.partial` (mode 0600) and gets its final name only after the archive check and the sidecar write succeed, so an interrupted backup (Ctrl-C or SIGTERM) leaves no file under a backup name that `nself backup restore latest` could pick. A killed process (SIGKILL) can leave a `.partial` file in `.meta/`; core ignores it and it is safe to delete. Next to the dump, `<dir>/.meta/<file>.json` (mode 0600) records the metadata:

```json
{"schema_version": "1", "kind": "k8s-backup", "project": "myapp", "file": "myapp_full_20261011_040506.dump",
 "bytes": 123456, "sha256": "<hex>", "created_at": "2026-10-11T04:05:06Z", "server_version": "160004",
 "release": "nself", "namespace": "default",
 "fingerprints": [{"schema": "auth", "table": "users", "rows": 2, "sum": "-123"}]}
```

It holds no password, no token and no host name. `nself backup list` and `nself backup restore latest` read only the files in
the backup directory, so the `.meta` subdirectory does not change them.

### Fingerprints

For every ordinary table (partitions included) in every schema except `pg_catalog`, `information_schema`, `pg_toast*` and
`pg_temp_*`, so `auth` and `storage` are covered, backup records `count(*)` and the sum of the first 64 bits of
`md5(<row as text>)`. The sum does not depend on row order and never builds one large value. The statements are generated
inside SQL from `pg_catalog` with `format('%I', ...)`, so odd table names are quoted by the database and never built from
input. The session pins `TimeZone`, `DateStyle`, `IntervalStyle`, `extra_float_digits` and `bytea_output` so a row has the same
text at backup and at restore.

Backup reads the fingerprints in a `REPEATABLE READ READ ONLY` session that exports a snapshot, takes `ACCESS SHARE` locks on
every covered table (as `pg_dump` does) before it reads any fingerprint, and `pg_dump --snapshot` streams from that snapshot, so dump and fingerprints describe the same moment even while the database is written. Restore reads the
fingerprints in one `REPEATABLE READ` transaction after `pg_restore` and compares them with the sidecar. A writer that changes
data during the restore makes them differ: stop the writers first (scale the apps down) or expect E737.

A `TRUNCATE` or table rewrite that commits between the snapshot export and the lock on its table is not seen by the backup (the same window `pg_dump --snapshot` has), so avoid running a backup while services run migrations or rewrite tables. Every psql session lifts `statement_timeout`, `lock_timeout` and `idle_in_transaction_session_timeout` first, so a database or role setting cannot cut the long-running sessions.

If the psql session that holds the snapshot ends while `pg_dump` is still running, the backup fails (E732). If it ends or its
`COMMIT` fails after `pg_dump` exited 0, the backup is kept and a warning is printed (`meta.warnings` with `--json`): the
snapshot was imported and the fingerprints were read before the dump started.

`kubectl exec` can exit 0 when the stream to the pod was cut off, so `pg_dump`, `pg_restore` and the check below run inside a pod script that prints `NSELF_RC=<status>` to stderr when the command ends. A step counts as done only with exactly one `NSELF_RC=0` line, last on stderr; a missing or non-zero marker is E732 (dump and check) or E736 (restore).

After the dump is written, backup streams the stored file back into the pod through `pg_restore -f /dev/null` (no database
connection). It must read the whole file. A truncated or corrupt archive fails there (E732), the dump is removed and no sidecar is written, so a recorded
checksum always belongs to a file that was proven readable when it was made.

### What the fingerprint does not prove

The fingerprint covers the row count and content of ordinary tables only. It does not cover sequence values, large objects,
the contents of materialized views, foreign tables, or the data of extension configuration tables that `pg_dump` does not
dump. It also compares every table of the database, so E737 can follow a good restore when the database holds tables the
backup does not have (for example tables an init script or another service created before the restore). Check the listed tables
before treating E737 as data loss.

### Restore order

1. The dump must exist and its sidecar must be valid (E734). A dump made by `nself backup create` has no sidecar and is refused.
2. The size and sha256 of the file must equal the sidecar's (E735). A tampered or truncated file stops here. Steps 1 and 2 run
   before kubectl is looked up, so a bad file never touches the cluster. If the backup's release or namespace differs from
   `--release` or the target pod's real namespace, restore prints a warning naming both and goes on. The sidecar records the namespace of the pod the backup read (not the `--namespace` flag).
3. `pg_restore --clean --if-exists` runs. If the file changes while it streams (its hash then differs), the result is E735
   with the message that `pg_restore` ran and the database state is not verified. If `pg_restore` ends cleanly without taking every byte of the archive, that is E736 ("did not read the whole archive"), not a changed file. Any `pg_restore: error:` line or a non-zero exit is E736.
4. The fingerprints are compared (E737 on any difference, up to 20 listed). Success is printed only after this step.

### Compatibility with `nself backup`

The dump is a plain `pg_dump -Fc` file in the core naming, so `nself backup restore <file>` restores it into a compose stack.
The other way round does not work with `nself k8s restore`: a core-made dump has no sidecar and is refused (core does not write
one yet). Postgres major versions must match as for any `pg_restore`.

## JSON output and exit classes

`--json` on any subcommand makes stdout carry exactly one v1 envelope (`schema_version`, `command`, then `data` or `error`, plus
`meta.warnings` when there are warnings), written through `sdk/go/output`. `command` is `k8s <subcommand>`. helm's own stdout and
the progress lines go to stderr, so a script can pipe stdout into `jq`. Without `--json` the output is unchanged.
`--help --json` (`--help` or `-h` as a flag, not as another flag's value and not after `--`, together with `--json`, on the
root or any subcommand) prints one ok envelope instead of help text: `command` is `k8s` or
`k8s <subcommand>`, `data` is `{"help": "<the text plain --help prints>"}`, and the exit status is 0. The `help` subcommand (`nself k8s help install --json`) is not covered and gives the usual E401
unknown-flag envelope.

| Subcommand | `data` |
|------------|--------|
| `values` | `mode` (`generate`), `values_file`, `secrets_file`, `mapped`, `unsupported[]`, `unmapped_routes[]` (`name`, `reason`); with `--check`: `mode` (`check`), `values_file`, `services`, `parity` (`ok`) |
| `install` | `release`, `installed`, `ingress_hosts[]` |
| `upgrade` | `release`, `upgraded` |
| `status` | `name`, `namespace`, `version`, `chart_version`, `status` (never helm's config or manifest) |
| `uninstall` | `release`, `uninstalled` |
| `backup` | `file`, `metadata_file`, `bytes`, `sha256`, `tables[]` (`schema`, `table`, `rows`) |
| `restore` | `file`, `verified` (always `true`), `tables[]` (`schema`, `table`, `rows`) |

Database error text never reaches an envelope: `cause` holds fixed text only. Human mode prints the first error line of `pg_dump` or `pg_restore` after the code, with every quoted part masked as `…`, `DETAIL`, `CONTEXT` and `HINT` dropped, at most 160 characters.

An error envelope carries `error.code`, `message`, optional `cause` and `remediation`, `exit_code` and `class`; the process exit
status equals `exit_code`. Messages never carry a secret, the licence key or helm's release JSON. Without `--json` every
failure still prints `Error: <message>` and exits 1, except the uninstall and restore refusals (exit 4) and the integrity failures E735 and E737 (exit 2).

| Code | Class (exit) | Meaning |
|------|--------------|---------|
| E401 | user (1) | Usage: unknown or missing flag (install without `--domain`), unexpected argument, a `--release` that is not a helm release name (always for `uninstall` and for a name starting with `-`; otherwise under `--json` only). Core code |
| E403 | destructive_blocked (4) | `uninstall` or `restore` without confirmation; nothing was changed. Core code |
| E720 | infra (2) | `helm` is not on `PATH` |
| E721 | user (1) | Generated chart values not found: run `nself k8s values` |
| E722 | infra (2) | A helm command failed (`cause` is the exit status). For install, upgrade and uninstall helm's output is on stderr; `status` does not show it (run `helm status` yourself) |
| E723 | user (1) | `values --check` found differences (`cause` lists them: service, kind, image refs) |
| E724 | infra (2) | The compose model could not be resolved (`docker compose config` failed) |
| E725 | user (1) | helm reports `release: not found` (`status`, `uninstall`): install it, or fix `--release` and `--cluster`. An unreachable cluster or unknown kube context is E722 |
| E726 | infra (2) | `helm status` printed something that is not a release document |
| E727 | user (1) | Generated values unusable: `secrets.yaml` readable by others, unreadable, or an invalid `values.yaml` |
| E729 | infra (2) | Unexpected failure (a temp directory, a write) |
| E730 | infra (2) | `kubectl` is not on `PATH` (`backup`, `restore`) |
| E731 | user (1) | The postgres pod of the release was not found, is not unique, or is not running and ready; the message says how many were found. Check `--cluster`, `--namespace`, `--release` |
| E732 | infra (2) | The backup stream failed, or the finished dump is not a complete archive (read back with `pg_restore -f /dev/null`), or the snapshot session ended before `pg_dump` finished. The dump is removed and no sidecar is written |
| E733 | infra (2) | The fingerprint query failed (before the dump, or after a restore), or its output ended without the completion marker (a cut-off stream) |
| E734 | user (1) | The backup file or its `.meta` sidecar is missing, unreadable or invalid. Nothing ran in the pod |
| E735 | infra (2) | The dump does not match the checksum or size in its sidecar; nothing was restored. Or the file changed while it streamed: `pg_restore` ran and the database state is not verified |
| E736 | infra (2) | `pg_restore` printed an error line, exited non-zero, or did not report `NSELF_RC=0`, or did not read the whole archive (`cause` is fixed text: exit status, marker state, count of error lines). The database state is not verified |
| E737 | infra (2) | After the restore the fingerprints differ from the backup's (`cause` lists up to 20 tables). Never reported as success |
| E738 | user (1) | The backup target cannot be resolved: `PROJECT_NAME` missing or not a plain name, a bad `--tag`, or the output file already exists (its own remediation: choose another `--output` or `--tag`) |

The plugin owns E720 to E739; E728 and E739 are free. E401 and E403 are the core registry's codes, reused because a
plugin cannot register new ones.

### Troubleshooting

| Symptom | Cause and fix |
|---------|---------------|
| `cannot re-use a name that is still in use` on install, or `has no deployed releases` on upgrade | An earlier install failed or timed out and left the release in state `failed`. Run `helm uninstall nself` (use your `--release` name and `--kubeconfig`), then `nself k8s install` again. `upgrade` works only after a revision has deployed. A failed install prints this hint |
| `run nself k8s values` | `values.yaml` or `secrets.yaml` is missing. Run `nself build` then `nself k8s values` |
| `readable by group or others` | `chmod 600 .nself/generated/k8s/secrets.yaml`, or run `nself k8s values` again |

## Try it on kind

`free/k8s/tests/kind/run.sh` is the proof the `k8s-kind` workflow runs: it builds `nself` from `nself-org/cli` at a pinned
commit, lays this plugin out the way `nself add` does, builds a minimal fixture project, runs `nself k8s values`,
`nself k8s install` and `nself k8s upgrade` on a kind cluster, waits for every pod to be Ready and POSTs `{__typename}` to
Hasura through `kubectl port-forward`. It then checks the machine contract with `jq`: `nself help --json k8s` lists the
subtree with canon `plugin`, `nself k8s status --json` is a v1 envelope, `nself k8s uninstall` without `--yes` and without a
terminal exits 4 and leaves the release, and `nself k8s uninstall --yes --json` removes it. It needs docker, go and git; it downloads kind, kubectl and helm at the versions and
checksums pinned in `free/k8s/tests/kind/tools.env`.

```bash
bash free/k8s/tests/kind/run.sh
```

The workflow runs on pull requests that touch `free/k8s/**` and by `workflow_dispatch` with inputs `source` (`branch`, or
`registry` once the registry leg exists), `cli_ref` and `leg` (`install` or `day2`). A pull request runs both legs.

The `day2` leg (`LEG=day2 bash free/k8s/tests/kind/run.sh`) installs into a dedicated namespace, seeds three tables with 1,000 rows,
runs `nself k8s backup --json` (the envelope names the dump and every table's count), deletes the namespace, installs again,
checks that `restore` without `--yes` exits 4, runs `restore --yes --json` (`verified: true`) and then compares the row counts and
a content digest read directly with `psql` in the pod. The unit step of the workflow also runs the integration test
`internal/ops/pg_integration_test.go` (`NSELF_K8S_PG_TEST=1`, needs docker) against a postgres:16 container with a 5-million-row table.

## Source

[`plugins/k8s/`](https://github.com/nself-org/plugins/tree/main/k8s)

Manifest: [`plugins/k8s/plugin.json`](https://github.com/nself-org/plugins/tree/main/k8s/plugin.json)

## See Also

- [[Infra]] — provision infrastructure via Terraform
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
