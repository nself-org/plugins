# nself-ci plugin

Local CI gate runner for nSelf repositories. Detects the repo stack and runs lint, test, and build checks. Posts a `nself-ci` GitHub commit status via `gh` OAuth so branch protection can require this check instead of billing-blocked GitHub Actions.

## What it does

1. Detects which stacks are present: Go (`go.mod`), Node/TS (`package.json`), Flutter (`pubspec.yaml`)
2. Runs stack-specific gates:
   - **Go:** `gofmt -l .` (files under `vendor/` ignored) + `go vet ./...` + `go test ./...` (per-package `-timeout` at least 600 s)
   - **Node:** `pnpm run lint` + `pnpm run typecheck` + `pnpm run test` + `pnpm run build` (skips missing scripts; a pnpm/npm workspace recurses into member packages so a root with no scripts of its own still runs real gates instead of reporting a false PASSED — see `internal/legacy/gate_runners.go`)
   - **Flutter:** `flutter analyze` + `flutter test`
3. Scans for secrets with `gitleaks` (uses repo `.github/gitleaks.toml` if present). Defaults to git-mode (respects `.gitignore`, scans tracked content); pass `--filesystem` to force the old `--no-git` filesystem-scan behavior for a non-checkout source tree.
4. Posts a `nself-ci` commit status to GitHub so it appears in PR checks
5. `nself-ci build --artifact android` produces a signed release APK locally and can attach it to a GitHub release — see "Artifact builds" below.

## What it explicitly does NOT do

`nself ci` detects stacks and runs equivalent gates/artifact builds directly — it does **not** parse or execute arbitrary `.github/workflows/*.yml` files, and it is not a GitHub Actions emulator. Repos with custom Actions steps beyond lint/test/build/artifact (E2E, Lighthouse, axe-a11y, CodeQL, Trivy, SBOM, license audit, coverage ratchet, bundle-size, commitlint, i18n-check, deploy) still need those specific jobs run somewhere, per the PPI's Two-Servers-Only split:

- **Public repos** keep those jobs on free GitHub-hosted runners (Actions minutes are free for public repos).
- **Private repos** run them via a dedicated additional `nself ci` gate (a new gate type — not implemented by this plugin today), never a third nSelf server.

See `.claude/docs/doctrines/nself-ci-runner-ceiling.md` for the full runner-ceiling policy this restates.

## Usage

```bash
# Run gates + post nself-ci status to GitHub (standard usage)
nself-ci [repo-root]

# Run gates only, no status posted (local check)
nself-ci --check [repo-root]

# With explicit SHA / remote
nself-ci --owner nself-org --repo plugins --sha abc1234 .

# Skip gitleaks (if not installed)
nself-ci --no-gitleaks .

# Force a filesystem-mode gitleaks scan (--no-git) even inside a git checkout.
# Opt-in only, for a non-checkout source tree (e.g. an exported tarball) where
# the automatic non-repo fallback doesn't apply.
nself-ci --filesystem .

# Via nself CLI proxy (once registered)
nself ci [repo-root]
nself ci --filesystem [repo-root]
```

## Environment variables

| Var | Description |
|---|---|
| `NSELF_CI_REPO` | Repo root path (alternative to positional arg) |
| `NSELF_CI_SHA` | Commit SHA to report on (alternative to --sha) |
| `NSELF_CI_SKIP_STATUS` | Set to `1` to skip posting GitHub status |

## Prerequisites

- `gh` CLI with repo scope (`gh auth login`)
- `gitleaks` for secret scanning (`brew install gitleaks` or [releases](https://github.com/zricethezav/gitleaks/releases))
- Stack tools present: `go`, `pnpm`/`npm`, `flutter` as needed

## Build

```bash
cd plugins/free/ci
go build -o nself-ci ./cmd/
```

## Container mode: the pinned toolchain image

`nself-ci run --container` runs each job's checks in one pinned, multi-arch (linux/amd64 and linux/arm64) toolchain image instead of the tools installed on the host. Every tool in it (Go, Node, pnpm, Rust, gitleaks, govulncheck, hadolint, syft, trivy) is fetched by URL and checked against a sha256 pin; the base image is pinned by digest. The pins live in `toolchain/pins/*.yaml`; `toolchain/versions.yaml` is generated from them (`go generate ./internal/toolchain/...`) and embedded in the binary. A test fails if the `Dockerfile` drifts from the pins.

| Var | Description |
|---|---|
| `NSELF_CI_IMAGE` | The image `--container` runs in, as `repo[:tag]@sha256:<64 hex>`. Takes precedence over the pinned row. A reference by tag alone is refused. Read from the operator's own environment, never from job env. Until the `--image` flag is registered this is how to pass it. |
| `NSELF_CI_TOOLCHAIN_VERSIONS` | Path to another `versions.yaml` to use instead of the embedded one (fixtures, air-gapped mirrors). |

The image row `toolchain-image` is `pending` until the first publish, so `--container` without `NSELF_CI_IMAGE` fails with **E645**. There is no silent fallback to host tools.

**E645** (class usage): the pinned toolchain image is not available. Causes: the row is still `pending`, the `NSELF_CI_IMAGE` value is not `ref@digest`, the digest cannot be pulled, or a pin source is invalid. Fix: pass `NSELF_CI_IMAGE=<ref@digest>`, or run without `--container`.

`nself-ci doctor` has a `toolchain-pins` section: **CI-DI01** the embedded table matches the pins, **CI-DI02** the image row state (pending or pinned), **CI-DI03** one warning per installed tool whose version differs from its pin (a pass otherwise; tools that are not installed are not listed, and version differences never fail the section).

The weekly `scripts/drill/toolchain-pins` drill checks every row of `versions.yaml`: a pinned artifact that no longer resolves fails, a newer upstream release or a moved tag is advisory. Base-image rows include the two helper images of `.github/workflows/ci-toolchain-image.yml` (`binfmt`, the qemu helper, and `buildkit`), so a newer digest of either is reported too.

## Artifact builds

`nself-ci build --artifact android [android-dir]` produces a signed Android release APK on the developer's own machine — closing the private-repo "gates but no release artifact" gap without a GitHub-hosted runner or a third nSelf server. It mirrors the exact signing steps `nchat/.github/workflows/build-react-native.yml` and `deploy-mobile-android.yml` already use, so the same keystore secrets work in both places:

```bash
export ANDROID_KEYSTORE_BASE64="$(cat release.keystore | base64)"
export ANDROID_KEYSTORE_PASSWORD=...
export ANDROID_KEY_ALIAS=...
export ANDROID_KEY_PASSWORD=...

# Build only (prints the produced APK path)
nself-ci build --artifact android frontend/platforms/react-native/android

# Build + attach to a GitHub release
nself-ci build --artifact android --upload --tag v1.2.3 frontend/platforms/react-native/android

# Via nself CLI proxy
nself ci build --artifact android frontend/platforms/react-native/android
```

Steps: base64-decode the keystore into `<android-dir>/app/release.keystore` → `./gradlew assembleRelease` (keystore env vars inherited by gradle's signing config) → locate the signed APK under `<android-dir>/app/build/outputs/apk/`.

**Android only.** macOS/Windows/TV/WearOS artifacts explicitly stay on GitHub-hosted runners — this lane does not attempt them (see P6-E11-W2-S1-T6; Ummat's own gap report never asked for them either).

## Command surface (core `nself ci` parity)

The `nself-ci` binary takes the exact argv of core `nself ci` (P7-CANON-09), so `nself ci …` and `nself-ci …` are interchangeable:

| Command | What it does |
|---|---|
| `nself-ci [flags] [repo-root]` | The gate. Flags: `--check`, `--no-status`, `--no-gitleaks`, `--sha`, `--owner`, `--repo`, `-v`/`--verbose`, `--filesystem` (plus the existing `--env`, `--gateway`). Flags and the repo-root may be mixed. |
| `nself-ci build [flags] [dir]` | Local signed Android artifact build (see "Artifact builds"). `--artifact` defaults to `android`. |
| `nself-ci forgejo [--url U]` | Forgejo server health for the ops profile (default URL `http://localhost:3844`). Always exits 0, as in core. |
| `nself-ci serve [flags]` | Webhook listener daemon on port 3845 (HMAC-verified, Docker-sandboxed jobs, fail-closed defaults). Each job runs the gate by exec of this same executable. |
| `nself-ci run [flags] [search-root]` | Pipeline over `.ci.yaml` manifests (unchanged). |

`--help` of `ci`, `build`, `forgejo` and `serve` uses the core text captured by `scripts/capture-core-surface.sh` into `testdata/core-surface/` and embedded (`internal/surface`). The capture normalizes Forgejo help to remove the in-stack runner retired by P7-TRUST-21. `ci eval` is not part of this binary (P7-CI-44/45).

Core framing: with `--check` the binary prints core's section header first and `Error: gate failed` on a failing gate, so `nself-ci --no-status --check REPO` matches `nself ci --no-status --check REPO` byte for byte. The argv that deployed 1.4.x CLIs send (`--no-status` and friends, never `--check`) keeps its exact output, because those CLIs print the header themselves.

Parity proof: `bash scripts/parity-with-core.sh` (needs `$NSELF`, `$TMPDIR`, `gitleaks`) builds core `nself` from cli origin/main and the plugins origin/main and head `nself-ci`, then diffs stdout, stderr and exit codes for core-vs-head and for deployed-1.4.x (base cli with each binary). `--self-test` proves the comparator flags a planted difference. Forgejo is excluded from frozen-core parity: its server-only health check and removal of `--runner` are covered by the `internal/forgejo` and `cmd` tests.

## Experimental placement-plan v1 cost totals

In `ci.placement-plan/v1`, `totals.estimated_cost_micro` is an integer sum of
the known costs of placed jobs. Costs that are unknown and jobs that are not
placed contribute nothing to that sum. Read `totals.cost_confidence` first:
it is the worst confidence among placed costs, and is `unknown` when no job
is placed or any placed cost is unknown. For example, a known cost of 5 and
an unknown cost produce `estimated_cost_micro: 5` with `cost_confidence: unknown`.
No placed jobs produce `0` with `unknown`; this does not establish a free run.
Individual `placement.estimate.cost_micro` values can be `null`. The v1 total
retains its integer representation for compatibility.

## Obsolete deploy-host exclusion roots

With `NSELF_V15=1`, run `nself ci nodes forget-project /absolute/project/root`
on the coordinator host to remove a deleted or retired project's learned root,
cached exclusions and refresh failure. The local operation is audited and does
not require the project directory to exist. Use the absolute path recorded in
the cache. Remove the same root from `ci.project_roots` in the CI home's
`config.yaml` if configured; otherwise the next refresh learns it again.
Node commands run in an active `.nself` project can also learn its root again.

## macOS system uninstall after upgrades

`nself-ci-agent uninstall --system` prints the agent version whose bundled
layout it removes. `nodes upgrade` replaces the agent-owned binary under
`/opt`, while the root-owned copy at
`/Library/PrivilegedHelperTools/org.nself.ci-agent` retains the version installed
by `nodes add --system`. Refresh that trusted copy by running
`sudo /Library/PrivilegedHelperTools/org.nself.ci-agent upgrade --version <x.y.z>`
on the node before uninstalling an upgraded system node. Run uninstall from the trusted copy.

## Manifest

`plugin.json` is manifest v2: `pluginType: cli`, `service.kind: cli` (no container), and a `commands` block (`command: ci`, `binary: nself-ci`, subcommands `build`, `forgejo`, `serve`, `run` with their `side_effect`, `output` and `json`). The v1 compatibility keys (`pluginType`, `binaryName`, `cliCommands`) are generated by `manifestv2migrate -compat` and stay until v1.6.0. The old `actions` and `config` v1 keys are gone (their v2 home is `commands.subcommands`).

## Requiring nself-ci in branch protection

See the project's CI-LOCAL.md for the exact `gh api` command to configure branch protection to require `nself-ci`.

---

## Sentry-box operational scripts

These scripts live in the repo-root `.github/scripts/ops/` (not shipped in the ci tarball) and are deployed to `/opt/nself-ops/bin/` on each sentry box. Copy and make executable once:

```bash
cp .github/scripts/ops/db-watchdog.sh .github/scripts/ops/disk-prune.sh /opt/nself-ops/bin/
chmod +x /opt/nself-ops/bin/db-watchdog.sh /opt/nself-ops/bin/disk-prune.sh
```

### `db-watchdog.sh`

Checks postgres and redis container health on every invocation. On failure: emits a deduped MD alert to `REPORT_DIR` (picked up by Claude inbox sync) and issues `docker restart`. DB-independent — never calls `pg_isready` or `redis-cli` on the host; always via `docker exec`.

**Cron entry (every 2 minutes):**
```
*/2 * * * * /opt/nself-ops/bin/db-watchdog.sh >>/opt/nself-ops/errors/.db-watchdog.log 2>&1
```

| Env var | Default | Description |
|---|---|---|
| `REPORT_DIR` | `/opt/nself-ops/errors` | Where to write MD alert files |
| `POSTGRES_CONTAINER` | `ops-postgres` | Docker container name for postgres |
| `REDIS_CONTAINER` | `ops-redis` | Docker container name for redis |

Dedup: one alert per service per 10 minutes (lockfile in `/tmp`). The 2-minute cron aligns with the 30-second internal check interval from the Unity workaround pattern — cron IS the loop.

### `disk-prune.sh`

Three-step hourly housekeeping: (1) checks `/` disk usage and emits a deduped alert if over threshold, (2) runs `docker system prune -af --volumes` to reclaim stopped containers / dangling images / unused volumes, (3) cleans GitHub Actions runner `_work` directories that are not currently in use (checked via `lsof`).

**Cron entry (hourly):**
```
0 * * * * /opt/nself-ops/bin/disk-prune.sh >>/opt/nself-ops/errors/.disk-prune.log 2>&1
```

| Env var | Default | Description |
|---|---|---|
| `REPORT_DIR` | `/opt/nself-ops/errors` | Where to write MD alert files |
| `DISK_WARN_PCT` | `80` | Alert threshold (percent of `/` used) |
| `RUNNER_WORK_DIR` | auto-detect | Runner `_work` path; auto-detects `/home/runner/_work` or `/opt/actions-runner/_work` |

Dedup: one disk-full alert per 6 hours (lockfile in `/tmp`).
