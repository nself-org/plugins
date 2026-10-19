# CI Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.7 |
| **Install** | `nself plugin install ci` |
| **Commands** | `nself ci`: `build`, `forgejo`, `nodes provision`, `nodes verify`, `run`, `serve` |
| **Requirements** | nSelf >=1.4.12 |

<!-- END GENERATED:catalog -->

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
