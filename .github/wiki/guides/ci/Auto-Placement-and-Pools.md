# Auto placement and pools

<!-- docmap: cap:ci.auto-placement -->
<!-- docmap: cap:ci.where -->
<!-- docmap: cap:ci.pools -->
<!-- docmap: cap:ci.race -->
<!-- docmap: cap:ci.hosted-tier -->
`nself ci` can run a job on this machine, on a registered node, on a pool of nodes, or on a hosted provider. This page covers how you choose, how a project limits the choice, and what happens when the chosen place is busy or unavailable. How a runner is scored is on [[CI scheduling and cost|guides/ci/Scheduling-and-Cost]]. Command, flag and code tables are on [[Reference|guides/ci/Reference]]; this page names them and does not repeat them.

## Tiers and the `auto` preset

Every runner belongs to one tier: `hosted`, `vps`, `lan` or `local`. Under the `auto` preset the tier of a runner turns into a small score bias, so a better tier wins when scores are close. The bias never overrides eligibility. A runner that is not ready, is out of capacity, fails trust, or would break a budget is still dropped, whatever its tier.

Two operator keys tune the order:

| Key | Meaning |
|---|---|
| `sched.auto.order` | The tiers `auto` may use, best first. You can drop tiers; you cannot add one. |
| `sched.auto.step_sec` | How long a busy better tier is waited for before the next tier is tried. A project can only lower it. |

Nothing here spends money. A hosted runner is in the choice only when its provider is registered, and the paid and budget rules on the scheduling page still decide whether it may run.

## Choosing a place with `--where`

`--where` is a flag of `nself ci` and of `nself ci runs rerun`. It maps to the same placement path as every other setting, so it can only narrow what policy allows.

| Value | Runs on |
|---|---|
| `local` | This machine only. |
| a node id | That registered node. |
| a pool id | Any ready member of that pool. |
| `hosted` | Registered hosted providers only. Refused when none is registered. |
| `auto` | Whatever the `auto` preset picks. |
| `all` | Several places at once (see [Race](#race)). Refused until race is available. |

Names are matched in that order: the reserved words, then a node id, then a pool id. An unknown value is refused with the invalid-`--where` code from the Reference, and the refusal lists the valid targets. Add `--json` to a plan to see the chosen place under `data.placement`; `nself ci explain <job>` names the reason each runner was dropped.

## Pools

A pool is not stored anywhere. It is the set of runners that carry the coordinator-assigned label `pool.<id>`. Two pools are implicit: `local` (this machine) and `hosted` (every provider runner). A node cannot join a pool by declaring a label itself; only the label the coordinator assigns counts.

List and inspect pools with the `nodes pools` verbs:

- `nself ci nodes pools list` shows each pool and whether the project may use it.
- `nself ci nodes pools show <pool>` shows one pool and its members.
- `nself ci nodes pools explain` shows the project's effective set and every runner a pool term drops.

### The effective-set rule

Three keys decide which pools a project may use.

| Key | Set by | Effect |
|---|---|---|
| `pools.allow` | The protected default branch's `.nself/ci.yaml` | The pools that may run the project. Unset means every pool. A revision can shrink this list and never widen it. |
| `pools.dev` | The operator only | The pools a development run may use. A value from a branch, revision, pipeline or job is ignored and reported. |
| `pools.record` | The operator | The pools allowed to produce a status of record. |

The effective set is the operator's pools, intersected with `pools.allow`. A runner outside it is dropped with the `pool.excluded` reason. `--where <pool>` is checked against the same set: naming a pool outside it is refused, not widened.

### Worked examples

**One laptop.** No node is registered, so the only pool is `local`. Leave every key unset. `--where local` and `--where auto` do the same thing, and `--where hosted` is refused because there is no provider.

**A person with a VPS and two Macs.** Register the three nodes and let the coordinator label the two Macs `pool.macs` and the VPS `pool.vps`. Leave `pools.allow` unset so the project may use all of them. Use `--where macs` for builds that need macOS, and `--where auto` for everything else; with the default tier order the VPS is preferred and the Macs are used when the VPS is busy past `sched.auto.step_sec`.

**A team pool.** The team coordinator labels shared runners `pool.team`. A repository that must stay on team hardware sets `pools.allow` to `[team]` on its protected default branch. A contributor's revision may shrink that list further but cannot add `local`, and `--where local` is refused for the project. Set `pools.record` to `[team]` so only team runners produce a status of record; a personal machine can still run a development check.

## Status of record

A status of record is the commit status other people and merge rules rely on. It is posted only from a pool listed in `pools.record`, from commit CI, not from a dev run. A dev run uses `pools.dev` for placement instead and never posts a status of record. If the labels of a runner cannot be read, the runner is excluded rather than assumed to qualify.

## Dirty-tree offload

To run uncommitted work on another node, `nself ci` takes a snapshot: a commit of the tracked changes (staged or not, including files added with `git add` and not yet committed) plus untracked files that git does not ignore, whose parent is `HEAD`. The snapshot lives under a private ref in the `refs/nself-ci/offload` namespace and is deleted when the run finishes, fails or is cancelled. Your index, branches and working tree are not touched. A clean tree offloads `HEAD` unchanged. The snapshot is only what the node checks out: the run records the commit it started from, marked dirty, so `ci runs reproduce` never depends on a snapshot that no ref keeps. The snapshot is taken for `--where <node>`, and for a pool, `auto` or `all` when at least one node is registered. A run with `--where local` or `--where hosted`, or with only hosted providers registered, keeps `HEAD`: a hosted runner fetches the revision from the forge, where a snapshot never exists.

Secrets are kept out by path and size, not by reading content. Untracked files that look like secrets or are too large are left out and named in the output. A tracked file of that kind refuses the offload. You can preview what would be included:

```bash
echo "draft" > notes.txt
git status --porcelain
```

## Race

`--where all` starts sibling attempts of a job on different places and keeps the first good result. The others are cancelled through the lease. A race never fails a run: a job it cannot race runs as a single attempt. Release-class jobs and jobs marked `idempotent: false` are never raced. The width is the resolved `sched.race.width`.

## Hosted tier and billing-blocked behaviour

Under `auto`, the hosted tier reads the result of the repository's own hosted workflow run and chooses one of three things: use that result, wait for it, or hand the commit to local nodes. A red hosted result stays red. The timers (`hosted.queued_sec`, `hosted.running_sec`) bound the wait. nSelf never plans paid GitHub minutes.

The operator key `hosted.mode` is `off`, `free` or `paid`. An unknown value reads as `free`. `paid` applies only when paid capacity is enabled and budgeted (see the scheduling page); otherwise it is downgraded to `free` and the output carries a note saying so.

When GitHub reports that billing blocks Actions, the provider is marked unavailable. That is never treated as a code failure: the commit is handed to the next tier in `sched.auto.order` instead. An unreadable hosted state has its own code on Reference. `nself ci explain <job>` shows the reason on each hosted runner.

This page covers only what ships today in `nself ci`; anything not listed here is not yet available.
