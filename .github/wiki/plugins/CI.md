# CI Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install ci` |
| **Commands** | `nself ci`: `build`, `forgejo`, `nodes provision`, `nodes verify`, `run`, `serve` |
| **Requirements** | nSelf >=1.4.12 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself ci artifacts` | `nself_ci_artifacts` | `/v1/commands/ci/artifacts` | read | document | MCP and HTTP | none |
| `nself ci build` | - | - | remote | document | CLI only (json none) | none |
| `nself ci config ignore add` | - | - | write | document | CLI only (json none) | none |
| `nself ci config migrate` | `nself_ci_config_migrate` | `/v1/commands/ci/config/migrate` | write | document | MCP and HTTP | none |
| `nself ci config show` | `nself_ci_config_show` | `/v1/commands/ci/config/show` | read | document | MCP and HTTP | none |
| `nself ci doctor` | `nself_ci_doctor` | `/v1/commands/ci/doctor` | read | document | MCP and HTTP | none |
| `nself ci evidence` | `nself_ci_evidence` | `/v1/commands/ci/evidence` | read | document | MCP and HTTP | none |
| `nself ci evidence verify` | `nself_ci_evidence_verify` | `/v1/commands/ci/evidence/verify` | read | document | MCP and HTTP | none |
| `nself ci explain` | `nself_ci_explain` | `/v1/commands/ci/explain` | read | document | MCP and HTTP | none |
| `nself ci forgejo` | - | - | read | document | CLI only (json none) | none |
| `nself ci logs` | `nself_ci_logs` | `/v1/commands/ci/logs` | read | document | MCP and HTTP | none |
| `nself ci netproxy` | - | - | remote | interactive | CLI only (surface cli-only) | none |
| `nself ci nodes add` | `nself_ci_nodes_add` | `/v1/commands/ci/nodes/add` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes discover` | `nself_ci_nodes_discover` | `/v1/commands/ci/nodes/discover` | read | document | MCP and HTTP | none |
| `nself ci nodes drain` | `nself_ci_nodes_drain` | `/v1/commands/ci/nodes/drain` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes enable` | `nself_ci_nodes_enable` | `/v1/commands/ci/nodes/enable` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes forget-project` | - | - | destructive | document | CLI only (surface cli-only) | none |
| `nself ci nodes label` | `nself_ci_nodes_label` | `/v1/commands/ci/nodes/label` | write | document | MCP and HTTP | none |
| `nself ci nodes list` | `nself_ci_nodes_list` | `/v1/commands/ci/nodes/list` | read | document | MCP and HTTP | none |
| `nself ci nodes pair` | - | - | remote | interactive | CLI only (surface cli-only) | none |
| `nself ci nodes pools explain` | `nself_ci_nodes_pools_explain` | `/v1/commands/ci/nodes/pools/explain` | read | document | MCP and HTTP | none |
| `nself ci nodes pools list` | `nself_ci_nodes_pools_list` | `/v1/commands/ci/nodes/pools/list` | read | document | MCP and HTTP | none |
| `nself ci nodes pools show` | `nself_ci_nodes_pools_show` | `/v1/commands/ci/nodes/pools/show` | read | document | MCP and HTTP | none |
| `nself ci nodes projects add` | `nself_ci_nodes_projects_add` | `/v1/commands/ci/nodes/projects/add` | write | document | MCP and HTTP | none |
| `nself ci nodes projects list` | `nself_ci_nodes_projects_list` | `/v1/commands/ci/nodes/projects/list` | read | document | MCP and HTTP | none |
| `nself ci nodes projects remove` | `nself_ci_nodes_projects_remove` | `/v1/commands/ci/nodes/projects/remove` | write | document | MCP and HTTP | none |
| `nself ci nodes provision` | - | - | remote | document | CLI only (surface cli-only) | none |
| `nself ci nodes remove` | `nself_ci_nodes_remove` | `/v1/commands/ci/nodes/remove` | destructive | document | MCP and HTTP | nonce |
| `nself ci nodes revoke` | `nself_ci_nodes_revoke` | `/v1/commands/ci/nodes/revoke` | destructive | document | MCP and HTTP | nonce |
| `nself ci nodes rotate` | `nself_ci_nodes_rotate` | `/v1/commands/ci/nodes/rotate` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes secrets grant` | - | - | remote | interactive | CLI only (surface cli-only) | none |
| `nself ci nodes secrets revoke` | - | - | remote | interactive | CLI only (surface cli-only) | none |
| `nself ci nodes set` | `nself_ci_nodes_set` | `/v1/commands/ci/nodes/set` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes show` | `nself_ci_nodes_show` | `/v1/commands/ci/nodes/show` | read | document | MCP and HTTP | none |
| `nself ci nodes status` | `nself_ci_nodes_status` | `/v1/commands/ci/nodes/status` | read | document | MCP and HTTP | none |
| `nself ci nodes sync` | `nself_ci_nodes_sync` | `/v1/commands/ci/nodes/sync` | write | document | MCP and HTTP | none |
| `nself ci nodes test` | `nself_ci_nodes_test` | `/v1/commands/ci/nodes/test` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes upgrade` | `nself_ci_nodes_upgrade` | `/v1/commands/ci/nodes/upgrade` | remote | document | MCP and HTTP | nonce |
| `nself ci nodes verify` | - | - | read | document | CLI only (surface cli-only) | none |
| `nself ci plan` | `nself_ci_plan` | `/v1/commands/ci/plan` | read | document | MCP and HTTP | none |
| `nself ci policy pin` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci policy protect` | `nself_ci_policy_protect` | `/v1/commands/ci/policy/protect` | remote | document | MCP and HTTP | nonce |
| `nself ci policy set` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci policy show` | `nself_ci_policy_show` | `/v1/commands/ci/policy/show` | read | document | MCP and HTTP | none |
| `nself ci policy unset` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci policy usage` | `nself_ci_policy_usage` | `/v1/commands/ci/policy/usage` | read | document | MCP and HTTP | none |
| `nself ci providers` | - | - | read | document | CLI only (surface cli-only) | none |
| `nself ci providers authorize` | `nself_ci_providers_authorize` | `/v1/commands/ci/providers/authorize` | remote | document | MCP and HTTP | nonce |
| `nself ci providers github init` | `nself_ci_providers_github_init` | `/v1/commands/ci/providers/github/init` | write | document | MCP and HTTP | none |
| `nself ci providers gitlab init` | `nself_ci_providers_gitlab_init` | `/v1/commands/ci/providers/gitlab/init` | write | document | MCP and HTTP | none |
| `nself ci providers list` | `nself_ci_providers_list` | `/v1/commands/ci/providers/list` | read | document | MCP and HTTP | none |
| `nself ci providers revoke` | `nself_ci_providers_revoke` | `/v1/commands/ci/providers/revoke` | destructive | document | MCP and HTTP | nonce |
| `nself ci providers show` | `nself_ci_providers_show` | `/v1/commands/ci/providers/show` | read | document | MCP and HTTP | none |
| `nself ci providers test` | `nself_ci_providers_test` | `/v1/commands/ci/providers/test` | remote | document | MCP and HTTP | nonce |
| `nself ci providers usage import` | `nself_ci_providers_usage_import` | `/v1/commands/ci/providers/usage/import` | write | document | MCP and HTTP | none |
| `nself ci run` | - | - | remote | document | CLI only (json none) | none |
| `nself ci runs` | `nself_ci_runs` | `/v1/commands/ci/runs` | read | document | MCP and HTTP | none |
| `nself ci runs approve` | `nself_ci_runs_approve` | `/v1/commands/ci/runs/approve` | remote | document | MCP and HTTP | nonce |
| `nself ci runs cancel` | `nself_ci_runs_cancel` | `/v1/commands/ci/runs/cancel` | remote | document | MCP and HTTP | nonce |
| `nself ci runs decisions` | `nself_ci_runs_decisions` | `/v1/commands/ci/runs/decisions` | read | document | MCP and HTTP | none |
| `nself ci runs follow` | - | `/v1/commands/ci/runs/follow` | read | stream | HTTP NDJSON only (stream) | none |
| `nself ci runs list` | `nself_ci_runs_list` | `/v1/commands/ci/runs/list` | read | document | MCP and HTTP | none |
| `nself ci runs outbox` | `nself_ci_runs_outbox` | `/v1/commands/ci/runs/outbox` | read | document | MCP and HTTP | none |
| `nself ci runs pending` | `nself_ci_runs_pending` | `/v1/commands/ci/runs/pending` | read | document | MCP and HTTP | none |
| `nself ci runs prune` | `nself_ci_runs_prune` | `/v1/commands/ci/runs/prune` | destructive | document | MCP and HTTP | nonce |
| `nself ci runs queue` | `nself_ci_runs_queue` | `/v1/commands/ci/runs/queue` | read | document | MCP and HTTP | none |
| `nself ci runs reproduce` | `nself_ci_runs_reproduce` | `/v1/commands/ci/runs/reproduce` | remote | document | MCP and HTTP | nonce |
| `nself ci runs rerun` | `nself_ci_runs_rerun` | `/v1/commands/ci/runs/rerun` | remote | document | MCP and HTTP | nonce |
| `nself ci runs resolve` | `nself_ci_runs_resolve` | `/v1/commands/ci/runs/resolve` | write | document | MCP and HTTP | none |
| `nself ci runs stats` | `nself_ci_runs_stats` | `/v1/commands/ci/runs/stats` | read | document | MCP and HTTP | none |
| `nself ci runs wait` | `nself_ci_runs_wait` | `/v1/commands/ci/runs/wait` | read | document | MCP and HTTP | none |
| `nself ci serve` | - | - | write | stream | CLI only (surface cli-only) | none |
| `nself ci serve admin` | - | - | read | document | CLI only (surface cli-only) | none |
| `nself ci serve admin audit` | `nself_ci_serve_admin_audit` | `/v1/commands/ci/serve/admin/audit` | read | document | MCP and HTTP | none |
| `nself ci serve admin audit checkpoint` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci serve admin cert` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci serve admin roles` | - | - | destructive | document | CLI only (surface cli-only) | none |
| `nself ci serve admin token` | - | - | destructive | document | CLI only (surface cli-only) | none |
| `nself ci serve backup` | - | - | read | document | CLI only (surface cli-only) | none |
| `nself ci serve install` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself ci serve restore` | - | - | destructive | interactive | CLI only (surface cli-only) | none |
| `nself ci serve uninstall` | - | - | destructive | document | CLI only (surface cli-only) | none |
| `nself ci serve watch` | - | - | remote | stream | CLI only (surface cli-only) | none |

<!-- END GENERATED:machine-surface -->

> Runs the local lint/test/build/gitleaks gate and posts the result as a GitHub commit status.

## Install

```bash
nself plugin install ci
```

No license key required.

## Description

Local CI gate runner: detects repo stack (Go/Node/Flutter/Dart), runs lint+test+build, scans secrets with gitleaks, then posts a GitHub commit status (nself-ci) via gh OAuth. Replaces billing-blocked GitHub Actions as the merge gate.

This is a CLI plugin: it installs the `nself-ci` binary into your plugin path and runs as a command, not a background service.

Category: `development`. Current version: `1.0.1`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_CI_REPO` | *(see plugin.json)* | Optional. |
| `NSELF_CI_SHA` | *(see plugin.json)* | Optional. |
| `NSELF_CI_SKIP_STATUS` | *(see plugin.json)* | Optional. |
| `NSELF_CI_TIMEOUT` | *(see plugin.json)* | Optional. |

## Examples

### Run

```bash
nself-ci
```

## Commands

The `nself-ci` binary accepts the exact argv of core `nself ci`:

| Command | Description |
|---------|-------------|
| `nself-ci [flags] [repo-root]` | Run the gate. Flags: `--check`, `--no-status`, `--no-gitleaks`, `--sha`, `--owner`, `--repo`, `-v`, `--filesystem`. |
| `nself-ci build [flags] [dir]` | Build a signed Android release artifact locally; `--upload --tag T` attaches it to a GitHub release. |
| `nself-ci forgejo [--url U] [--runner R]` | Forgejo server and runner health (ops profile). |
| `nself-ci serve [flags]` | Webhook listener daemon on port 3845; fail-closed (secret, allowlist, Docker). |
| `nself-ci run [flags] [search-root]` | Run the `.ci.yaml` pipeline. |

The manifest is v2 with a `commands` block (`command: ci`, `binary: nself-ci`). `ci eval` is not part of this plugin. The runner host commands are below.

## Runner hosts: `nodes provision` and `nodes verify`

Ported from core `nself runner provision|verify` (P7-CANON-10) with the same flags, messages and exit codes. Both are dispatched as `nself ci nodes <verb>` or `nself-ci nodes <verb>`. The dependency set is one declarative manifest compiled into the binary, so a freshly provisioned host and a "should be identical" host are checked against the same list.

| Command | Description |
|---------|-------------|
| `nself-ci nodes verify [--host user@host]... [--ssh-key K] [--json]` | Check each host against the manifest (packages, a real `_work` directory, cached Chromium libraries) and print a parity matrix. No `--host` checks this machine. Exits 1 on a failing check, an unreachable host or drift between hosts. |
| `nself-ci nodes provision --github-url U [--host user@host] [--instances N] [--install-root D] [--labels a,b] [--ssh-key K] [--token T]` | Install the dependencies, create the runner user with passwordless sudo, ensure a real `_work` directory and register N runner instances as systemd services. Every step is idempotent. No `--host` provisions this machine. |

- The registration token comes from `--token` or the `GITHUB_RUNNER_TOKEN` environment variable, never from a project `.env`, and is never printed.
- On a remote host the token travels on the script's stdin, so it is absent from argv, `ps` and the plain sudo log file. If the host's sudoers sets `Defaults log_input` (or a `LOG_INPUT:` tag), sudo records the piped token in its I/O log; leave `log_input` off for the runner user on provisioned hosts.
- `provision` takes at most one `--host` per run, and refuses a `_work` that is a symlink.
- Every ssh call goes through the vendored cli `sdk/go/remote` (one exec funnel, destination allowlist, `--` before the destination). A test (`TestSingleSSHExecSite`) fails if free/ci gains a second ssh or scp exec site.
- Exit status follows core: 1 for a refusal or a failed step, or the status of the failed ssh or remote command (255 for a connection error).
- `scripts/parity-nodes.sh` compares stdout, stderr, exit code, the ssh argv and the commands a stubbed host recorded against a core build, using disposable local sshd containers only.

## Container mode: the pinned toolchain image

`nself-ci run --container` runs jobs in one pinned multi-arch toolchain image instead of the host's tools. Tools are checked against sha256 pins and the base image is pinned by digest (`free/ci/toolchain/pins/`). The image row is `pending` until its first publish.

| Env Var | Description |
|---------|-------------|
| `NSELF_CI_IMAGE` | Image for `--container` as `repo[:tag]@sha256:<64 hex>`. Overrides the pinned row; a tag-only reference is refused. |
| `NSELF_CI_TOOLCHAIN_VERSIONS` | Path to a different `versions.yaml` (fixtures, air-gapped mirrors). |
| `NSELF_CI_TEAM` | Team id of this coordinator. Its `team` policy rows (`nself ci policy set --scope team --team <id>`) apply to every run, plan and dispatch of the process. Unset: no team row applies. |

- **E645** (usage): the pinned toolchain image is not available (row still `pending`, `NSELF_CI_IMAGE` not `ref@digest`, digest not pullable, or an invalid pin source). Set `NSELF_CI_IMAGE=<ref@digest>` or run without `--container`. There is no silent fallback to host tools.
- `nself-ci doctor` section `toolchain-pins`: CI-DI01 table matches pins, CI-DI02 image row state, CI-DI03 installed tool versions that differ from the pins (warnings only).
- The weekly toolchain-pins drill reports newer upstream releases and moved tags for every pin row, including the `binfmt` and `buildkit` helper images of the image build workflow.

## Source

[`plugins/ci/`](https://github.com/nself-org/plugins/tree/main/ci)

Manifest: [`plugins/ci/plugin.json`](https://github.com/nself-org/plugins/tree/main/ci/plugin.json)

## See Also

- [[Forgejo]] — self-hosted git + Actions runner
- [[GitHub-Runner]] — self-hosted GitHub Actions runner

← [[Home]] →
