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

The worst case of a job is its timeout rounded up to the next billing increment, priced at the rate, plus the minimum charge and extras. A one-minute job on a provider that bills by the hour is estimated at one hour plus the minimum charge and extras. A timeout above `economics.max_paid_timeout` has no bound (`cost.unbounded`) and cannot run on a paid class. If a figure would overflow, it is reported as over budget (`budget.exceeded`), never as a smaller number.

## Paid placement is off by default

A metered or ephemeral placement needs both `economics.paid_enabled` and a day or month budget (overall, or for that provider). A per-job budget alone is not enough (`budget.unset`). With paid spend off, nothing paid is ever placed (`paid.disabled`).

Each paid provider account has an owner coordinator: the one named in `economics.providers.<id>.owner_coordinator`, else the first coordinator that placed work on it. Any other coordinator sees `provider.owned_elsewhere`.

## Budgets and windows

Budgets exist per job, per pipeline, per UTC day, per UTC month and per provider (a provider budget counts per UTC month). The scheduler reserves the worst case of a job inside the same transaction that leases it, so the money is held with the lease or not at all. When the run closes, the reservation takes the actual cost and keeps the window it was made in.

The day key comes from the dispatcher's clock and is never earlier than the largest key recorded. If the clock steps back across a month boundary, work keeps being charged to the later month and a settle still charges the window of its admission.

A burst VM is reserved at its full lifetime when its intent is created and released only when its teardown is confirmed. A failed teardown keeps the money held.

## Spend approval

When a job's worst case is above `economics.approval_threshold_micro`, its pipeline waits in `approval.pending`. An admin approves it with `nself ci runs approve <run> --spend` (a TEAM member or release user is refused; the local operator of a PERSONAL coordinator holds admin). The approval is bound to the run, its head SHA, its config digest and the estimate the approver saw, and it is audited with the actor and amount. A new push is a new run and needs its own approval. A job that costs more than the approved estimate waits again.

## The ledger and its confidence

`nself ci policy usage [--json]` prints the ledger. Every figure carries a confidence and its source.

| Confidence | Where it comes from |
|---|---|
| `known` | A provider reported it (the GitHub billing importer, when you turn it on) |
| `computed` | A local actual from the coordinator's own timestamps |
| `estimated` | The worst case at admission, a remote actual that was clamped, any allowance without a provider balance |
| `unknown` | No basis (for example, a duration nobody recorded; it settles at the estimate) |

A remote attempt that reports an end before its start is clamped to the estimate and shown as `estimated`.

An allowance is the declared ceiling (`economics.ceilings.<provider>`) minus the consumption the provider reported minus what the local ledger spent since. Without a declared ceiling, or without any reported consumption, the allowance is unknown and private jobs do not run on that hosted provider (`allowance.unknown`). A job also needs the remaining allowance, minus `economics.margin_micro`, to be above its worst case (`allowance.below_margin`).

## Handoff

- The dispatcher wiring binds the economics environment with `economics.Bind` (store, spend policy, clock, coordinator id) and copies the spend keys into the scheduler's policy with `economics.ApplyPolicy`. Until a coordinator does that, the hooks are inert and nothing is priced.
- A job the scheduler holds with `approval.pending` never reaches the admission step; the dispatcher records its estimate with `economics.NotePending` so `ApproveSpend` has a figure to bind to.
- Burst (P7-SCHED-08) calls `economics.ReserveResource` and `economics.ReleaseResource` inside its own transactions. The GitHub billing importer (P7-HOST-10) writes through `economics.RecordUsage`.
