package economics

// Purpose: the `budget` admission step (Epic D7/D11, P7-SCHED-04's RegisterStep).
// Inside AdmitAndLease's transaction it prices the attempt, refuses what the
// policy does not allow, reserves the worst case against every budget scope
// and writes the attempt's ledger row, so the money is held in the same commit
// as the lease or not at all.
// Inputs:  dispatch.View (the request: runner, job, placement) and the Env.
// Outputs: nil (admit), a *dispatch.Refusal (the job stays queued with that
// reason), or a store error.
// Constraints: inert when no Env is bound; local and owned work costs nothing
// and writes only its ledger row; paid classes (metered, ephemeral) need
// schedpolicy.PaidAllowed, a rate, a timeout within max_paid_timeout, an owner
// coordinator equal to this one, room in every set budget and, above the
// approval threshold, an approval still valid for this SHA and config digest;
// `included` private hosted capacity needs a known allowance above the margin;
// a GitHub runner is never paid. The step reads the clock through Env.Now and
// the window high-water mark through the store, so a clock stepped back keeps
// charging the later window. It calls no Store method (the writer is held).

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// PendingDetailPrefix starts the detail of an approval.pending refusal: the
// estimate ApproveSpend binds an approval to.
const PendingDetailPrefix = "estimate_micro="

const publicLabel = "nself.visibility=public"

func init() {
	if err := dispatch.RegisterStep("budget", step); err != nil {
		panic(err)
	}
}

func step(ctx context.Context, v dispatch.View) error {
	env, ok := envOf(ctx)
	if !ok {
		// Unbound: nothing is priced, so nothing paid may be placed. Free work is untouched.
		if c := v.Request.Runner.Class; isPaid(c) {
			return &dispatch.Refusal{Reason: model.PaidDisabled, Detail: "economics is not bound, so a " + c + " placement cannot be priced"}
		}
		return nil
	}
	err := admit(ctx, env, v)
	var se *SpendError
	if errors.As(err, &se) {
		return &dispatch.Refusal{Reason: se.Reason, Detail: se.Detail}
	}
	return err
}

func admit(ctx context.Context, env Env, v dispatch.View) error {
	r := v.Request
	t := v.Ledger(ctx)
	p, err := env.policy(ctx) // from memory; a failure fails the admission closed
	if err != nil {
		return err
	}
	row := store.LedgerRow{Attempt: v.Demand.AttemptID, Pipeline: v.PipelineID, Project: v.Project, Provider: r.Runner.Provider, Class: classOf(p, r.Runner.Class, r.Runner.Provider),
		Remote: !r.Local, Confidence: ConfEstimated, AdmittedAt: t.Now()}
	day, month, err := windows(t, env.Now())
	if err != nil {
		return err
	}
	row.WindowDay = day
	switch {
	case row.Class == ClassIncluded:
		err = includedCheck(t, p, v, month, &row)
	case isPaid(row.Class):
		err = paidCheck(t, env, p, v, day, month, &row)
	default: // local or owned: no money moves
		row.Confidence = ConfComputed
	}
	if err != nil {
		return err
	}
	return t.PutLedger(row)
}

// paidGate is what any paid placement needs before it is priced: not GitHub,
// paid spend on, a day or month budget, and a provider account this coordinator owns.
func paidGate(t store.LedgerTx, env Env, p Policy, provider string) error {
	switch {
	case isGitHub(provider):
		return refuse(model.PaidDisabled, "GitHub-hosted capacity is never paid")
	case !p.PaidEnabled:
		return refuse(model.PaidDisabled, "economics.paid_enabled is off")
	case !schedpolicy.PaidAllowed(p, provider):
		return refuse(model.BudgetUnset, "a paid placement needs a day or month budget, overall or for %s", provider)
	}
	owner := p.OwnerCoordinator[provider]
	if owner == "" {
		var err error
		if owner, err = t.ProviderOwner(provider, env.Coordinator); err != nil {
			return err
		}
	}
	if owner != "" && env.Coordinator != "" && owner != env.Coordinator {
		return refuse(model.ProviderOwnedElsewhere, "%s belongs to coordinator %s", provider, owner)
	}
	return nil
}

// paidCheck is the admission rule of metered and ephemeral work.
func paidCheck(t store.LedgerTx, env Env, p Policy, v dispatch.View, day, month string, row *store.LedgerRow) error {
	r := v.Request
	provider := r.Runner.Provider
	if err := paidGate(t, env, p, provider); err != nil {
		return err
	}
	q := QuoteJob(p.Rates[provider], r.Job.TimeoutMs, p.MaxPaidTimeoutMs)
	if q.Reason != "" {
		return refuse(q.Reason, "%s: timeout %d ms, cap %d ms", provider, r.Job.TimeoutMs, p.MaxPaidTimeoutMs)
	}
	cost := max(q.Micro, r.Placement.CostMicro)
	if v.PipelineID == "" {
		return refuse(model.BudgetUnset, "the attempt has no pipeline, so a pipeline budget cannot be checked")
	}
	// The approval threshold is about the pipeline's spend estimate: what its other paid
	// attempts hold plus this job, so twenty jobs under the threshold cannot add up past it.
	if thr := p.ApprovalThresholdMicro; thr.Set && thr.Micro > 0 {
		held, err := t.PipelineHeld(v.PipelineID, v.Demand.AttemptID)
		if err != nil {
			return err
		}
		total, overflow := addSat(held, cost)
		if overflow || total > thr.Micro {
			est, ok, err := t.ValidApproval(v.PipelineID)
			if err != nil {
				return err
			}
			if !ok || overflow || est < total {
				return refuse(model.ApprovalPending, "%s%s over the %d micro threshold", PendingDetailPrefix, strconv.FormatInt(total, 10), thr.Micro)
			}
		}
	}
	refs, err := reserve(t, p, spend{Owner: v.Demand.AttemptID, Pipeline: v.PipelineID, Provider: provider, Micro: cost}, day, month)
	if err != nil {
		return err
	}
	row.EstMs, row.EstMicro, row.Rate, row.BudgetRefs = q.BilledMs, cost, q.Snap, refsString(refs)
	return nil
}

// includedCheck applies the `included` rule: private hosted capacity is used
// only while remaining_estimated - margin > the job's worst case.
func includedCheck(t store.LedgerTx, p Policy, v dispatch.View, month string, row *store.LedgerRow) error {
	r := v.Request
	if slices.Contains(r.Runner.Capability.Identity.Labels, publicLabel) {
		return nil // public repositories are free on GitHub-hosted runners
	}
	in, err := t.Allowance(r.Runner.Provider, month)
	if err != nil {
		return err
	}
	a := AllowanceOf(p, r.Runner.Provider, in)
	if !a.Known || !a.CeilingKnown {
		return refuse(model.AllowanceUnknown, "%s: consumption or ceiling unknown", r.Runner.Provider)
	}
	// Included capacity is not paid, so max_paid_timeout does not apply: a job with no timeout is
	// priced at the longest run the policy allows anywhere.
	timeout := r.Job.TimeoutMs
	if timeout <= 0 {
		timeout = schedpolicy.MaxPaidTimeoutCap
	}
	q := QuoteJob(p.Rates[r.Runner.Provider], timeout, schedpolicy.MaxPaidTimeoutCap)
	if q.Reason == model.CostUnknownRate || q.Reason == model.CostUnbounded {
		return refuse(q.Reason, "%s: no usable rate or bound for a %d ms job", r.Runner.Provider, r.Job.TimeoutMs)
	}
	worst := max(q.Micro, r.Placement.CostMicro)
	left, of := addSat(a.Remaining, -p.MarginMicro)
	if q.Reason != "" || p.MarginMicro < 0 || of || left <= worst {
		return refuse(model.AllowanceBelowMargin, "%s: %d micro left, margin %d, job worst case %d", r.Runner.Provider, a.Remaining, p.MarginMicro, worst)
	}
	row.EstMs, row.EstMicro, row.Rate = q.BilledMs, worst, q.Snap
	return nil
}
