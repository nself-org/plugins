# CI-Failure Bridge (gh-ci-failures-to-reports.sh)

Bridges GitHub Actions failures into enriched Markdown reports for
`.claude/inbox`, so a failure is triageable without opening the run URL.

## Where it actually runs (verified 2026-09-03)

There is no dedicated "sentry box" for GitHub-hosted CI failures. The `ci`
stream is a **local process on the developer machine**. Two launchers exist and
they run **different copies** of this script:

| Launcher | Copy executed | Interval |
|---|---|---|
| Cascade daemon (`cascaded`), config `~/.cascade/nsentry-sync.yaml`, one `ci` stream per project (nself, unyeco, ummeco, acamarata) | `~/.cascade/nsentry/scripts/gh-ci-failures-to-reports.sh`, **rewritten from a string embedded in the daemon binary at every daemon start** (`include_str!` of `cascade-v1/crates/cascade-daemon/assets/nsentry/gh-ci-failures-to-reports.sh`) | 900 s |
| launchd `com.acamarata.ghci-bridge` via `~/bin/acamarata-stopgap/acamarata-ghci-bridge.sh` (acamarata only) | this repo's working-tree file, `plugins/free/ci/scripts/gh-ci-failures-to-reports.sh`, directly | 300 s |

Both write to the same project inbox and share its `.gh-seen`, so they dedup
against each other. Output goes straight to `.claude/inbox`; rsync only carries
the separate uptime/box-error stream from a real sentry box, which is unrelated.

**Consequence:** updating the script in this repo changes the launchd lane
immediately (it reads the working tree) and does **not** change the daemon
lane. Copying the repo file over `~/.cascade/nsentry/scripts/` only lasts until
the next daemon restart, which overwrites it from the embedded asset (that is
what the `.bak-drift-20260830` file next to it is evidence of). To change the
daemon lane, change the asset in the cascade daemon crate and rebuild
`cascaded`. There is no automated sync between the two copies; this is a known
gap, not by design.

## Verifying which copy is deployed

```bash
diff ~/.cascade/nsentry/scripts/gh-ci-failures-to-reports.sh \
     plugins/free/ci/scripts/gh-ci-failures-to-reports.sh
```

A report written by the daemon copy currently has no `jobs_failed` /
`steps` / `first_error_line` fields; one written by the repo copy does.

## Stale-run guards (added 2026-09-03)

Incident: acamarata received a report on 2026-09-03 for a qibla lint failure
from 2026-05-30 that had been fixed on 2026-05-31; about twenty similar replays
of runs 79 to 182 days old had landed since 2026-08-15, roughly one a day.

Root cause: the script picks the newest failed run per workflow and skipped it
if seen, but only ever recorded the **emitted** key. Every older failure stayed
"unseen" forever. GitHub's status-filtered run listing is eventually consistent
and occasionally omits the newest run; whenever that happened the next-older,
never-recorded failure fell through as "new".

Three guards now apply, in this order, cheapest first:

1. **Record everything listed.** After each repo, every row of the failure
   listing is written to `.gh-seen`, whether reported, skipped, or shadowed by
   a newer failure in the same workflow. An old run can no longer resurface.
2. **`--max-age-days N`** (env `NSENTRY_MAX_AGE_DAYS`, default 14, `0`
   disables). Older runs are skipped and recorded. 14 rather than 3 so a
   machine that was asleep for a week still reports a failure that is
   genuinely still red; guard 3 handles the ones that were fixed meanwhile.
3. **Superseded check** (`--no-superseded-check` disables). If the latest run
   of that workflow is newer than the candidate and concluded `success`, the
   failure is already resolved: skip and record. Costs one `gh run list
   --workflow` call, paid only for candidates that passed guards 1 and 2. A
   lookup failure counts as "not superseded", so an outage can at worst report
   once too often, never drop a failure.

Skips are logged as `SKIP <key> (<reason>)` so the bridge log shows why a
failure did not become a report.

Note: the daemon's embedded copy carries its own, older `--max-age-days`
(default 3, added after a 2026-07-06 replay of a 2025-08-06 acamarata/ali run)
but none of the other guards and none of the report enrichment.

## Report contents

Each report has YAML frontmatter (`id`, `created_at`, `title`, `severity`,
`source`, `repo`, `workflow`, `sha`, `run_id`) plus enrichment fields:

- `jobs_failed`: list of job names that failed/were cancelled/timed out
- `steps`: per-job list of `{job, step, conclusion}` for the failing step(s)
- `first_error_line`: best-guess single most useful log line (`null` if the
  failure has no matching error-pattern line, e.g. an artifact-upload failure)

...followed by a `## What failed` table and a `## Log tail` section (last N
lines of the failing step's log, default 40, `--log-lines` to change).

## Triage rule for readers

Compare the frontmatter `created_at` with the `When:` event date in the body.
If they differ by more than a few days the report is a replay, not a live
failure: verify with `gh run list -R <org>/<repo> -L 5` before touching code.

## Known non-goals

GitHub Actions billing/quota/storage failures are not specially suppressed by
this bridge (out of scope for the 2026-08-30 enrichment pass) — they still
generate a report like any other failure. Per standing project policy those
reports are ignored by devs, not by the bridge.
