# CI-Failure Bridge (gh-ci-failures-to-reports.sh)

Bridges GitHub Actions failures into enriched Markdown reports for
`.claude/inbox`, so a failure is triageable without opening the run URL.

## Where it actually runs (verified 2026-08-30)

There is no dedicated "sentry box" for the nSelf org's GitHub-hosted CI
failures. The `ci` stream is a **local process on the developer machine**,
owned by the Cascade daemon:

- Config: `~/.cascade/nsentry-sync.yaml` (project `nself`, org `nself-org`,
  `ci: { interval_secs: 900, per_repo: 10 }`)
- Script executed: `~/.cascade/nsentry/scripts/gh-ci-failures-to-reports.sh`
  — a **vendored copy** the Cascade binary owns, not a symlink into this repo
- Output: writes reports directly to the project's `.claude/inbox` (no rsync
  hop for this stream — rsync only carries the separate uptime/box-error
  stream from a real sentry box, which is unrelated and, as of 2026-08-30,
  broken/unreachable for `nself`/`unyeco`)

**Consequence:** updating `plugins/free/ci/scripts/gh-ci-failures-to-reports.sh`
in this repo does NOT update what runs. The Cascade daemon's vendored copy at
`~/.cascade/nsentry/scripts/gh-ci-failures-to-reports.sh` must be refreshed
separately (`cp` the repo file over it, verify with `sha256sum`) any time this
script changes. There is currently no automated sync between the two; this is
a known gap, not by design.

## Verifying the deployed copy matches HEAD

```bash
diff ~/.cascade/nsentry/scripts/gh-ci-failures-to-reports.sh \
     plugins/free/ci/scripts/gh-ci-failures-to-reports.sh
```

No output means they match. If they differ, copy HEAD over the vendored path
and re-verify.

## Report contents

Each report has YAML frontmatter (`id`, `created_at`, `title`, `severity`,
`source`, `repo`, `workflow`, `sha`, `run_id`) plus enrichment fields:

- `jobs_failed`: list of job names that failed/were cancelled/timed out
- `steps`: per-job list of `{job, step, conclusion}` for the failing step(s)
- `first_error_line`: best-guess single most useful log line (`null` if the
  failure has no matching error-pattern line, e.g. an artifact-upload failure)

...followed by a `## What failed` table and a `## Log tail` section (last N
lines of the failing step's log, default 40, `--log-lines` to change).

## Known non-goals

GitHub Actions billing/quota/storage failures are not specially suppressed by
this bridge (out of scope for the 2026-08-30 enrichment pass) — they still
generate a report like any other failure. Per standing project policy those
reports are ignored by devs, not by the bridge.
