# CI scheduling and cost

This page covers what `nself ci` spends and how it keeps score. Placement itself (eligibility, scoring, `ci plan`, `ci explain`) is described next to the scheduler; this page is about money.

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

## Handoff

- Not wired yet (debt D-1280): the dispatcher must bind the economics environment with `economics.Bind` (store, spend policy, clock, coordinator id) and copy the spend keys into the scheduler's policy with `economics.ApplyPolicy`. Until then the hooks are inert and nothing paid is placed.
- A job the scheduler holds with `approval.pending` never reaches the admission step. The dispatcher records the whole pipeline's worst case with `economics.NotePending`, so `ApproveSpend` has a figure to bind to and one approval releases the run. Without it, the figure is the pipeline total at the moment a job is held, and each job that grows the pipeline past it is held again.
- Burst (P7-SCHED-08) calls `economics.ReserveResource` and `economics.ReleaseResource` inside its own transactions. The GitHub billing importer (P7-HOST-10) writes through `economics.ReplaceUsage`.
