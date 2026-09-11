# Adopting the `nself-ci` Gate (Dogfood Pattern)

**Status:** wired into `plugins` (this repo) 2026-09-11 — the first of 13 nself-org repos to adopt it. Zero repos had it wired before this, including `plugins` itself, which owns the source at `free/ci`.

This guide is the copy-paste pattern for the other 12 repos. It also records, honestly, what the gate does and does not check today — read "Known gaps" before requiring it as a sole merge gate.

---

## Why `plugins` first

- It owns the `nself-ci` source (`free/ci/`) and the wrapper (`scripts/nself-ci.sh`) — self-hosting the gate here is the purest dogfood: a regression in the gate breaks its own repo's checks first.
- It is public, so the workflow runs on free GitHub-hosted Actions — no runner-fleet dependency to prove the pattern.
- Its own `.claude/docs/CI-LOCAL.md` (project-level) already documents the branch-protection recipe; this guide is the repo-level companion showing the wiring actually in place.

## The pattern

1. **Workflow** — `.github/workflows/nself-ci.yml`: checkout → `setup-go` (builds the binary) → `setup-node`/`pnpm` (for the Node gate path) → install `gitleaks` → run `scripts/nself-ci.sh .` with `permissions: statuses: write` and `GH_TOKEN: ${{ github.token }}` so `gh api` can post the status without a PAT.
2. **Wrapper** — `scripts/nself-ci.sh` builds `free/ci/nself-ci` if missing/stale, then `exec`s it. Same script works locally and in CI.
3. **Status** — the binary posts a `nself-ci` commit status via `gh api repos/{owner}/{repo}/statuses/{sha}` (OAuth, no token in a URL).
4. **Branch protection (NOT applied by this PR — owner's call)** — add `nself-ci` to `required_status_checks.contexts` per the exact `gh api` command in `~/Sites/nself/.claude/docs/CI-LOCAL.md` § "Requiring nself-ci in branch protection". Do this as an **additional** required check alongside the repo's existing checks, not a replacement — see gaps below.

Copy `.github/workflows/nself-ci.yml` from this repo into each of the other 12 (`cli`, `admin`, `web`, `nchat`, `nclaw`, `nsentry`, `ntask`, `nfamily`, `clawde`, `homebrew-nself`, `packages`, `plugins-pro`), adjusting the toolchain setup steps to match that repo's stack (e.g. Flutter repos need `subosito/flutter-action` instead of `setup-node`).

---

## Verified locally (2026-09-11, this repo, HEAD `5c17916`)

```
$ scripts/nself-ci.sh --check -v .
[nself-ci] building gate binary...
[nself-ci] built .../free/ci/nself-ci
[nself-ci] running: gitleaks detect --source . --exit-code 1 --config .github/gitleaks.toml

nself-ci gate results — .
Stacks: node
────────────────────────────────────────────────────────────
  secrets:gitleaks                PASS  (3.617s)
────────────────────────────────────────────────────────────
  Overall: PASSED  (4s)
```

That is the real, complete output — one gate ran (`secrets:gitleaks`), it passed, in under 4 seconds. Read that literally: **no lint, typecheck, test, or build gate ran**, for the reason below.

---

## Known gaps — read before requiring this as the only gate

**1. This repo's Node gate is a no-op.** `nself-ci` detects Node stacks by checking the root `package.json` for `lint` / `typecheck` / `test` / `build` scripts (or workspace members' scripts, if `pnpm-workspace.yaml` exists). This repo's root `package.json` has neither: it has one custom script, `ci:local` (a stub — `tsc --noEmit --allowJs || echo skip; echo passed`), and no `pnpm-workspace.yaml`, even though `shared/` has its own `package.json`. Result: the workspace-recursion path in `free/ci/internal/gate_runners.go` (`runNodeGates`) never triggers, the non-workspace path finds no matching scripts, and the gate silently runs nothing but the secret scan — while still printing `Overall: PASSED`. This is exactly the shape of failure flagged in `.claude/memory/lesson_hollow_ci_gates.md`: a gate that passes while checking nothing.
   - **Fix options (not applied by this PR):** (a) add real `lint`/`typecheck`/`test`/`build` scripts to root `package.json` (or add `pnpm-workspace.yaml` so member scripts are picked up), or (b) extend `gate_runners.go` to also recognize this repo's `ci:local` convention. Until one of these lands, `nself-ci` on this repo is a secrets-only gate, not a lint/test/build gate.
2. **No per-plugin checks.** The real quality bar for this repo is per-plugin (schema validation, duplicate-port checks, registry consistency — see `validate.yml`, `registry-check.yml`, `hygiene.yml`, `license-gate.yml`). `nself-ci` only inspects the repo root; it does not walk `free/*/plugin.json` or run plugin-level TypeScript builds. It does not replace any of those workflows.
3. **No lint/typecheck/test/build for Go-only subpackages either.** `free/ci` itself (and other Go-based plugins) has a `go.mod`, but stack detection only looks at the *repo root*, which has no root `go.mod`. A regression inside `free/ci`'s own Go code would not be caught by `nself-ci` running against repo root — only by its own `go test ./...` if invoked directly in that subdirectory, or by `build-test.yml`.
4. **Per the plugin's own README** ("What it explicitly does NOT do"): `nself ci` is not a GitHub Actions emulator. E2E, Lighthouse, axe-a11y, CodeQL, Trivy, SBOM, license audit, coverage ratchet, bundle-size, commitlint, i18n-check, and deploy jobs all still need to run somewhere else — free-runner Actions for public repos (this repo), or a dedicated additional gate for private ones.
5. **Fork PRs:** `GH_TOKEN: ${{ github.token }}` on a fork PR is read-only for status writes; the binary treats a failed status post as a warning (non-fatal), so the job's pass/fail still reflects the real gate result, but the `nself-ci` check may not appear on fork PRs.

**Bottom line:** this PR adds `nself-ci` as evidence the wiring works end-to-end (workflow → build → gates → status), not as proof the gate is a complete substitute for this repo's existing checks. Recommend requiring it in branch protection **in addition to**, not instead of, `validate.yml` / `build-test.yml` / `gitleaks.yml` / `registry-check.yml` until gap 1 is closed.

---

## Rollout checklist for the other 12 repos

- [ ] Copy `.github/workflows/nself-ci.yml`, adjust toolchain setup for the repo's stack.
- [ ] Run `scripts/nself-ci.sh --check -v .` (or `plugins/free/ci/nself-ci` directly) locally first — confirm which stacks it detects and which gates actually execute. Do not assume; read the printed gate table.
- [ ] If the repo's real lint/typecheck/test/build live under a non-standard script name (as `ci:local` does here) or in workspace members without `pnpm-workspace.yaml`, fix that *before* treating a green `nself-ci` as meaningful.
- [ ] Open a PR, let Actions run it once, confirm the `nself-ci` commit status appears on the PR.
- [ ] Propose (do not silently apply) the branch-protection addition per `CI-LOCAL.md` — owner approves it.
