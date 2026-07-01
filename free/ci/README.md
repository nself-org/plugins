# nself-ci plugin

Local CI gate runner for nSelf repositories. Detects the repo stack and runs lint, test, and build checks. Posts a `nself-ci` GitHub commit status via `gh` OAuth so branch protection can require this check instead of billing-blocked GitHub Actions.

## What it does

1. Detects which stacks are present: Go (`go.mod`), Node/TS (`package.json`), Flutter (`pubspec.yaml`)
2. Runs stack-specific gates:
   - **Go:** `gofmt -l .` + `go vet ./...` + `go test ./...`
   - **Node:** `pnpm run lint` + `pnpm run typecheck` + `pnpm run test` + `pnpm run build` (skips missing scripts)
   - **Flutter:** `flutter analyze` + `flutter test`
3. Scans for secrets with `gitleaks` (uses repo `.github/gitleaks.toml` if present)
4. Posts a `nself-ci` commit status to GitHub so it appears in PR checks

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

# Via nself CLI proxy (once registered)
nself ci [repo-root]
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
