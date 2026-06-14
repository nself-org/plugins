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
