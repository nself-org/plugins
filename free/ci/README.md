# nself-ci plugin

Local CI gate runner for nSelf repositories. Detects the repo stack and runs lint, test, and build checks. Posts a `nself-ci` GitHub commit status via `gh` OAuth so branch protection can require this check instead of billing-blocked GitHub Actions.

## What it does

1. Detects which stacks are present: Go (`go.mod`), Node/TS (`package.json`), Flutter (`pubspec.yaml`)
2. Runs stack-specific gates:
   - **Go:** `gofmt -l .` + `go vet ./...` + `go test ./...`
   - **Node:** `pnpm run lint` + `pnpm run typecheck` + `pnpm run test` + `pnpm run build` (skips missing scripts; a pnpm/npm workspace recurses into member packages so a root with no scripts of its own still runs real gates instead of reporting a false PASSED — see `internal/gate_runners.go`)
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

## Requiring nself-ci in branch protection

See the project's CI-LOCAL.md for the exact `gh api` command to configure branch protection to require `nself-ci`.

---

## Sentry-box operational scripts

These scripts live in `scripts/` and are deployed to `/opt/nself-ops/bin/` on each sentry box. Copy and make executable once:

```bash
cp scripts/db-watchdog.sh scripts/disk-prune.sh /opt/nself-ops/bin/
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
