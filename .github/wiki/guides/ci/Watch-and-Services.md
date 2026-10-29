# Watch and services

<!-- docmap: cap:ci.watch -->
<!-- docmap: cap:ci.claims -->
<!-- docmap: cap:ci.demand -->
`nself ci serve watch` lets one host keep a repository's commit CI current without an inbound port or a webhook. This page covers what a pass does, how hosts share the work, and the keys that bound it. Tables of flags and codes are on [[Reference|guides/ci/Reference]].

## What `watch` does

```text
nself ci serve watch [--interval <duration>] [--repos o/r,...] [--once] [--json]
```

| Flag | Meaning |
|---|---|
| `--interval` | Time between passes. Default: `providers.github.poll_interval`. |
| `--repos` | Comma-separated `owner/repo` list. Default: this checkout's repository. |
| `--once` | Run one pass and exit. |
| `--json` | Print one JSON envelope per pass. |

The watcher is outbound-only: it reads from the provider and posts commit statuses. It needs a CI home (`NSELF_CI_HOME`) and refuses to start without one.

One pass:

1. Finds the heads of every watched repository.
2. Considers them newest default-branch head first, then recent branches.
3. Runs at most one commit.
4. When nothing ran, starts a due background pipeline.
5. Publishes the demand document and a heartbeat, ready or not.

A pass that hits a provider quota error stops and records the reset time; no scan is made before it. An unchanged answer from the provider reuses the previous heads. A host that is not ready claims nothing.

## Claims and the dead-claim rule

Several hosts can watch the same repository. They do not share a lock. The commit status history is the ledger: a host claims a sha by posting a status, and every host reads the same history to decide whether to take, wait, skip or treat the claim as lost.

- A claim from a creator the watcher does not trust never suppresses CI.
- Ordering uses the provider's `created_at`, never the local clock. With no provider time yet, no claim is declared dead.
- A claim is dead when its host has been silent for longer than the run limit plus ten minutes on the provider clock. Another host then takes the sha.
- Because the history is the ledger, a lost state file never reruns a commit that already has a result.

## Readiness and demand

Before it claims anything a host evaluates its own readiness: it samples the machine, runs the operator's readiness hook and checks the `yield_to` scan. Readiness only gates new work; it never pauses or kills a job that is already running. Each pass publishes a demand document, the work waiting and not yet claimed, even when the host was not ready to take it, so unmet demand stays visible.

## Keys

All `watch.*` keys belong to the operator. A repository cannot widen them, so a checkout can never change what a personal watcher runs or posts. A bad value is refused with the watch code listed on Reference.

| Key | Controls |
|---|---|
| `watch.branches` | Which branches are considered besides the default branch. |
| `watch.max_branches` | The cap on branches considered per pass. |
| `watch.branch_days` | How recent a branch head must be. |
| `watch.retry_max` | Attempts at a commit that fails. |
| `watch.retry_gap` | Minimum time between those attempts. |
| `watch.status_context` | The status context posted, `nself-ci` or `nself-ci/<name>`. |
| `watch.drain_sec` | Time allowed for a running job to finish when the watcher stops. |

Unset keys take the built-in defaults of the watcher. Read the effective values with `nself ci config show`.

## Running it

Try one pass in a foreground terminal first:

```text
nself ci serve watch --once --json
```

Then run the same command without `--once`. Stop it with `SIGTERM` or an interrupt; `watch.drain_sec` bounds how long it waits for work in hand.

## Related

- [[Auto placement and pools|guides/ci/Auto-Placement-and-Pools]] explains where each claimed commit runs.
- [[CI scheduling and cost|guides/ci/Scheduling-and-Cost]] explains scoring, budgets and limits.

This page covers only what ships today in `nself ci`; anything not listed here is not yet available.
