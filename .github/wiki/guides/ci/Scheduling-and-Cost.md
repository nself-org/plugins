# CI scheduling and cost

Paid capacity is off by default. Nothing that costs money is placed until an operator sets `economics.paid_enabled` and a day or month budget above zero. GitHub-hosted capacity is included only: nSelf never plans paid GitHub minutes.

This page is the one place for how `nself ci` chooses a runner, what it may spend, and how much work it lets run at once. Figures in the tables are read from code. Figures in the text carry a confidence label: `declared` for a registered default or a value you set, `computed` for a local measurement, `estimated` for a bound, `unknown` for no basis. The labels are explained under [the ledger](#the-ledger-and-its-confidence).

## How a runner is chosen

Placement has two steps. First, every runner is tested for eligibility. A runner that fails is dropped and the failing check is recorded as a reason code. Second, each eligible runner gets a score in milliseconds and the lowest score wins. A job with no eligible runner is held or refused, and `nself ci explain <job>` says which reason code stopped each runner.

The score is the sum of nine components, each a value in milliseconds times its preset weight divided by 1000, plus a bias for the runner's cost class: `wait`, `runtime`, `cold_start`, `transfer`, `cache`, `contention`, `cost`, `scarcity` and `reliability`. Each component carries its own confidence in the plan output. A component with no basis is shown as `unknown`, not as zero.

Reasons that are static cannot clear without a configuration change (for example a missing tool or a disabled paid class). The others, such as `capacity.busy`, clear on their own.

<!-- BEGIN GENERATED:sched-reasons -->
| Reason | Clears without a configuration change |
|---|---|
| `allowance.below_margin` | yes |
| `allowance.unknown` | no (static) |
| `approval.pending` | yes |
| `approval.required` | yes |
| `background.contention` | yes |
| `background.interactive_queued` | yes |
| `backpressure.queue_full` | yes |
| `budget.exceeded` | yes |
| `budget.unset` | no (static) |
| `burst.team_only` | no (static) |
| `capacity.busy` | yes |
| `cost.unbounded` | no (static) |
| `cost.unknown_rate` | no (static) |
| `ephemeral.used` | yes |
| `labels.mismatch` | no (static) |
| `limit.global` | yes |
| `limit.job` | yes |
| `limit.pipeline` | yes |
| `limit.project` | yes |
| `limit.provider` | yes |
| `limit.runner` | yes |
| `limit.team` | yes |
| `limit.user` | yes |
| `node.deploy_host` | no (static) |
| `node.discovered` | no (static) |
| `node.draining` | yes |
| `node.interactive` | yes |
| `node.maintenance` | yes |
| `node.not_ready` | yes |
| `node.offline` | yes |
| `node.on_battery` | yes |
| `node.protocol` | no (static) |
| `node.reservation` | yes |
| `node.revoked` | no (static) |
| `override.excluded` | no (static) |
| `override.max_cost` | no (static) |
| `override.max_queue` | yes |
| `override.min_isolation` | no (static) |
| `paid.disabled` | no (static) |
| `platform.mismatch` | no (static) |
| `platform.none_available` | yes |
| `pool.excluded` | no (static) |
| `provider.down` | yes |
| `provider.owned_elsewhere` | no (static) |
| `queue.timeout` | yes |
| `resources.insufficient` | no (static) |
| `retry.anti_affinity` | yes |
| `runner.remote_only` | no (static) |
| `tier.waiting` | yes |
| `tools.missing` | no (static) |
<!-- END GENERATED:sched-reasons -->

The full reason list, including job reasons and the other typed reasons, is generated in [Reference](Reference.md).

## Presets

`sched.preset` picks the weights. The default is `balanced` (`declared`). A run can override it with `--prefer <preset>` (`local` is short for `prefer-local`). Weights are per mille; the bias columns are added by cost class.

<!-- BEGIN GENERATED:sched-presets -->
| Preset | `wait` | `runtime` | `cold_start` | `transfer` | `cache` | `contention` | `cost` | `scarcity` | `reliability` | ms per cost micro-unit | bias `local` (ms) | bias `owned` (ms) | bias `included` (ms) | bias `metered` (ms) | bias `ephemeral` (ms) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `auto` | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1 | 0 | 0 | 0 | 0 | 0 |
| `balanced` | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1 | 0 | 0 | 0 | 0 | 0 |
| `cheapest` | 250 | 250 | 250 | 250 | 250 | 250 | 4000 | 2000 | 250 | 10 | 0 | 0 | 0 | 0 | 0 |
| `fastest` | 1000 | 1000 | 1000 | 1000 | 1000 | 250 | 0 | 0 | 1000 | 0 | 0 | 0 | 0 | 0 | 0 |
| `prefer-included` | 1000 | 1000 | 1000 | 1000 | 1000 | 2000 | 1000 | 0 | 1000 | 1 | 40000 | 40000 | 0 | 60000 | 60000 |
| `prefer-local` | 1000 | 1000 | 1000 | 2000 | 1000 | 500 | 1000 | 1000 | 1000 | 1 | 0 | 10000 | 30000 | 60000 | 60000 |
| `preserve-hosted-quota` | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 1000 | 4000 | 1000 | 1 | 0 | 0 | 60000 | 120000 | 120000 |
<!-- END GENERATED:sched-presets -->

## Overrides

Overrides narrow where work may go. They sit under `sched.overrides.*`: `local_only`, `lan_only`, `private_only`, `providers.allow`, `providers.deny`, `labels`, `runner`, `os`, `arch`, `max_cost_micro`, `max_queue` and `min_isolation`. An override can only tighten: a project cannot loosen what an operator set. `--runner <id>` places every job of a run on one runner and is meant for exceptions.

Limits cap how much runs at once: `sched.limits.pipeline`, `job_matrix`, `project`, `user`, `team`, `global` and `provider.<id>`. No limit is the default. A limit of zero is refused, because the scheduler reads zero as no limit.

## Which keys spend money

Spend keys are `economics.*` and `burst.*`: currency, rates, ceilings, every budget, the approval threshold, `paid_enabled`, `max_paid_timeout`, `margin_micro`, provider ownership, and the burst provider, concurrency, margin, trust and cloud VM settings. They are read from operator scope only, and an admin writes them. A pull request, a branch or a pipeline file cannot set them. Loading and `nself ci policy set` belong to the trust layer; see [policy sources](Trust-Security-and-Privacy.md#policy-sources-and-nself-ci-policy).

```sh
nself ci policy show
```

```text
economics.paid_enabled	false	default
sched.preset	"balanced"	default
sched.queue.max_depth	500	default
```

Each row is key, value and the scope it came from. This is an excerpt of real output from a fresh repository.

## Plan and explain

`nself ci plan` shows the layers, the selection and a placement per job. This is real output for a two-job fixture where the second job needs a tool that is not installed:

```text
LAYER  JOB    PATH  REQ       STATE        REASON              PLACEMENT
0      build  deep  required  planned      selection.no_base   local (local, local) cost 0 micro [known]
0      test   deep  required  unplaceable  selection.no_base   unplaceable: unplaceable.tool
```

The cost cell ends in the confidence of the figure. `nself ci explain test` prints what the job needs and why it cannot be placed:

```text
job test  unplaceable  band interactive  source plan
  needs  platform darwin/arm64  cpu 0  mem_mb 0  isolation_min process  network internet  secret classes none  tools no-such-tool-xyz
  reason  unplaceable.tool  tool no-such-tool-xyz is not on PATH
```

`plan --json` carries the same data. Note: `--prefer`, `--runner` and `--max-queue` are accepted by `plan` but not yet applied to its placement (debt D-0991); the plan shows the policy a run uses.

## Job requirements and labels

A job declares what it needs under `requires`: `labels`, `tools`, `accelerators`, `cpu` and `mem_mb`. A runner is eligible only when it matches all of them. The reasons are `labels.mismatch`, `tools.missing` and `resources.insufficient`. To pin a project to certain runners, set `sched.overrides.labels` at project scope; the job then needs those labels on top of its own.

## Cost classes

Every runner is priced as one class.

| Class | Meaning | Costs money |
|---|---|---|
| `local` | This machine | No |
| `owned` | A node you own (LAN, VPS you already pay for) | No |
| `included` | Capacity inside an allowance you already have (GitHub-hosted runners) | No, but it draws down the allowance |
| `metered` | Pay per use (a rented runner) | Yes |
| `ephemeral` | A burst VM created for the job | Yes |

GitHub-hosted capacity is `included` and nothing else. Its overage is never a placement option, and no GitHub provider can be declared `metered` or `ephemeral`. A policy that tries is refused.

## Rates are what you declare

Rates are never fetched. You declare them at operator scope with `economics.rates.<provider>` (a rate per hour or per minute, a billing increment, a minimum charge and extras). A rate in a pull request, a branch or a pipeline file is ignored and recorded.

The worst case of a job is its timeout rounded up to the next billing increment, priced at the rate, plus the minimum charge and extras. With `billing_increment: 1h`, a one-minute job is estimated at one hour plus the minimum charge and extras. A paid rate (metered or ephemeral) must declare its `billing_increment`; the policy is refused without it, so a provider that bills whole hours cannot be under-estimated by a default. A job with no timeout, or one above `economics.max_paid_timeout`, has no bound (`cost.unbounded`) and cannot run on a paid class. If a figure would overflow, it is reported as over budget (`budget.exceeded`), never as a smaller number.

## Paid placement is off by default

A metered or ephemeral placement needs both `economics.paid_enabled` and a day or month budget above zero (overall, or for that provider). A per-job or per-pipeline budget alone is not enough (`budget.unset`). A day or month budget of zero does not enable paid spend (`budget.unset`). Once a budget is set, a job that does not fit is refused (`budget.exceeded`). With paid spend off, nothing paid is ever placed (`paid.disabled`).

A provider with a declared paid rate is paid, whatever class a runner says it is. Provider ownership is recorded in each coordinator's own store, so two coordinators with separate data directories can both claim an account that has no declared owner; declare `economics.providers.<id>.owner_coordinator` for any paid account more than one coordinator can reach. Each paid provider account has an owner coordinator: the one named in `economics.providers.<id>.owner_coordinator`, else the first coordinator that asked (a coordinator with no id cannot place paid work at all: `provider.owned_elsewhere`) to place work on it (recorded even if that placement is then refused). Any other coordinator sees `provider.owned_elsewhere`.

## Budgets and windows

Budgets exist per job (all attempts of one job share it, so retries count), per pipeline, per UTC day, per UTC month and per provider (a provider budget counts per UTC month). The scheduler reserves the worst case of a job inside the same transaction that leases it, so the money is held with the lease or not at all. When the run closes, the reservation takes the actual cost and keeps the window it was made in.

The day key comes from the dispatcher's clock and is never earlier than the largest key recorded. If the clock steps back across a month boundary, work keeps being charged to the later month and a settle still charges the window of its admission. A recorded day more than three days ahead of today is a clock that jumped forward once, not a step back: it is replaced by today and the replacement is written to the audit log, so one bad reading cannot freeze the budgets on a day that never ends.

A burst VM is reserved at its full lifetime when its intent is created and released only when its teardown is confirmed. A failed teardown keeps the money held, and a VM that outlives its reservation is settled at what it really cost (not capped), so the budgets show the overrun. A VM held for a pipeline counts in that pipeline's spend estimate and is subject to the approval threshold like any paid job. The job a VM was bought for runs on it without reserving again: the runner carries the label `nself.burst.intent=<intent>` (set by the burst code that enrols it). The shortcut is narrow: the runner must be ephemeral, the intent must still hold money, the intent's pipeline must be the job's (or none), and the job's worst case at the VM's rate must fit inside what was reserved; otherwise the job is priced and reserved like any other. A refused VM reservation (`approval.pending`) hands its estimate back in the error, and the caller notes it with `economics.NotePending` so one approval can release it.

## Spend approval

When a pipeline's spend estimate would go above `economics.approval_threshold_micro`, the job that crosses it waits in `approval.pending`. The estimate is what the pipeline's paid attempts already hold (their actual once settled, else their worst case) plus the job being placed, so many small jobs add up. An admin approves it through `economics.ApproveSpend` (the roles check uses the project id the caller names with `economics.WithProject`; without it the actor needs the role on every project) (P7-TRUST-24 adds the `nself ci runs approve <run> --spend` command that calls it; a TEAM member or release user is refused, and the local operator of a PERSONAL coordinator holds admin). The approval is bound to the run, its head SHA, its config digest and the estimate the approver saw, and it is audited with the actor and amount. A new push is a new run and needs its own approval. Earlier tries of a job keep their worst case in the pipeline's estimate until the run closes. A job that takes the pipeline past the approved estimate waits again.

## The ledger and its confidence

`nself ci policy usage [--repo dir] [--json]` prints the ledger. Every figure carries a confidence and its source.

| Confidence | Where it comes from |
|---|---|
| `known` | A provider reported it (the GitHub billing importer, when you turn it on) |
| `computed` | A local actual from the coordinator's own timestamps |
| `estimated` | The worst case at admission, a remote actual that was clamped, any allowance that has a declared ceiling but no provider balance |
| `declared` | A value you set in policy (a rate, a ceiling, a budget limit); an input, not a measurement |
| `unknown` | No basis (for example, a duration nobody recorded; a priced attempt settles at its estimate, a local or owned one at zero) |

A remote attempt that reports an end before its start, and any priced attempt whose duration is longer than the bound it was admitted under, is clamped to the estimate and shown as `estimated`. When a run closes, every attempt of it that is still open (an earlier try of a retried job, a cancelled job) is settled too, on the span the coordinator recorded or, with none, at its estimate as `unknown` (zero for local or owned work).

An allowance is the declared ceiling (`economics.ceilings.<provider>`) minus the consumption the provider reported minus what the local ledger spent from the newest day the provider's report covers (providers report with a delay, and streams (skus) lag by different amounts, so the count starts at the newest day of the stream that lags most; that day is counted twice rather than missed; the start is never later than the day before the newest report was made, and a usage line dated more than one day ahead of today is refused). Without a declared ceiling, or without any reported consumption, the allowance is unknown and private jobs do not run on that hosted provider (`allowance.unknown`). A balance the provider reports replaces the ceiling and the imported consumption, and is shown as `known`. Jobs of public repositories on GitHub-hosted runners (label `nself.visibility=public`) do not draw on the allowance. A job also needs the remaining allowance, minus `economics.margin_micro`, to be above its worst case (`allowance.below_margin`). An included provider with no declared rate has no usable bound (`cost.unknown_rate`), and a job past the longest run the policy allows is `cost.unbounded`; included work has no `max_paid_timeout`, and a job with no timeout is priced at that longest run.

## GitHub-hosted capacity is included only

GitHub-hosted runners are class `included` and nothing else. Their overage is never a placement option and no GitHub provider can be declared `metered` or `ephemeral`. nSelf never plans paid GitHub minutes. Jobs of public repositories on GitHub-hosted runners do not draw on the allowance. Private jobs need a known allowance (`allowance.unknown` otherwise) and a remaining balance above the job's worst case plus `economics.margin_micro` (`allowance.below_margin`).

## Burst

Burst creates a machine for a queued job when waiting costs more than provisioning. It is off until an operator enables a provider (`burst.providers.<id>.enabled`) and paid spend is allowed. Safety rules:

- Cloud burst is for TEAM coordinators only (`burst.team_only` elsewhere).
- Intent first: the intent row is committed, and its money reserved, before any provider call.
- One job per machine. The machine is revoked at teardown, and teardown is retried until the resource is gone.
- The engine deletes only machines labelled `role=ci-ephemeral` with its own coordinator id, and never a host in the deploy-host exclusion set. An unreadable exclusion set stops it.
- The sweeper runs on start and every five minutes. A machine with no intent is alerted once and deleted after `max_lifetime` plus a grace period. `--dry-run` lists what it would delete and changes nothing.
- Defaults (`declared`): at most 2 machines at once (`burst.max_concurrent`), a 60 s wait margin (`burst.margin`), a 120 s provisioning latency assumed when a provider has none recorded, and a breaker that opens after 3 join failures for 15 minutes.
- A decision refuses with the first failing rule and records every input.

## Affected selection

A run may select only the jobs a change can touch. The selector maps changed paths to packages, adds the packages that depend on them, and selects the jobs whose working directory covers one. It answers `full` whenever it cannot prove a narrower set: no base to compare with, an empty diff, a release pipeline, a default-branch run, a path in a generated or opaque directory, a path no package or job owns, an uncertain package graph, or no job left. It never selects fewer jobs on a guess. The selection mode and its reason are the first line of `nself ci plan`.

## Capacity

- Order: strict bands (interactive, normal, background). Inside a band, projects share capacity fairly with equal weights. Inside a project, higher priority goes first, then age.
- Starvation ceiling: a job that waited longer than `sched.fairness.starvation_after` (30 minutes, `declared`) moves up one band, never above normal.
- Backpressure: `sched.queue.max_depth` (500, `declared`) bounds the queue. A full queue refuses with `backpressure.queue_full`.
- Queue wait: `sched.overrides.max_queue` bounds how long a job waits for a runner (`queue.timeout`). A foreground run defaults to 10 minutes (`declared`) unless `--max-queue` is given.
- Reservations: a node can hold capacity back (`node.reservation`). A placement reserves its slot privately, so two jobs never take the same slot.
- Background CI (scheduled and housekeeping work) runs in the background band and waits behind interactive work (`background.interactive_queued`, `background.contention`).
- Runners: local and cloud ephemeral runners are used once per job; a used ephemeral runner is reported as `ephemeral.used`.

## CI health

`nself ci runs stats [--since <duration>] [--project <owner/repo>] [--json]` prints queue, lease, outbox, cache and node health over a window (24 hours by default). A figure with no samples is shown as `unavailable` with the cause, never as zero:

```text
window 24h0m0s
leases active 0, stale 0, self-fenced 0
outbox pending 0, oldest -
unavailable queue: no_samples
```

`nself ci doctor` includes the same health section as checks (`CI-STAT-*`).

## Measured overhead

No scheduling overhead figure is published yet (`unknown`). The benchmarks exist (`Place` over 1 to 100 runners and 10 to 1000 jobs; `world.Build`, `Place` and `Admit` against an on-disk store), but a baseline with a performance budget has not been recorded, so this page makes no claim. The status file `free/ci/bench/sched/SCHED-14-status.md` lists what is missing.

## Known gaps

- Not wired yet (debt D-1280): the dispatcher must bind the economics environment with `economics.Bind` (store, spend policy, clock, coordinator id) and copy the spend keys into the scheduler's policy with `economics.ApplyPolicy`. Until then the hooks are inert and nothing paid is placed.
- A job the scheduler holds with `approval.pending` never reaches the admission step. The dispatcher records the whole pipeline's worst case with `economics.NotePending`, so `ApproveSpend` has a figure to bind to and one approval releases the run. Without it, the figure is the pipeline total at the moment a job is held, and each job that grows the pipeline past it is held again.
- Burst (P7-SCHED-08) calls `economics.ReserveResource` and `economics.ReleaseResource` inside its own transactions. The GitHub billing importer (P7-HOST-10) writes through `economics.ReplaceUsage`.
