package economics

// Purpose: builds the Report of `nself ci policy usage` from the store and the
// declared spend policy (Epic D11, X18).
// Inputs:  the store, the typed spend policy and a clock.
// Outputs: a Report in which budgets, allowances, ledger figures and approvals
// each carry a confidence and a source.
// Constraints: read only; the windows are the UTC day and month of the later of
// the clock and the highest window key recorded; a budget limit, rate and
// ceiling are printed as the owner declared them.

import (
	"context"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const recentAttempts = 20

func declared(micro int64, source string) Figure {
	return Figure{Micro: micro, Confidence: ConfDeclared, Source: source}
}

func declaredPtr(micro int64, source string) *Figure { f := declared(micro, source); return &f }

func reservedFigure(micro int64) *Figure {
	return &Figure{Micro: micro, Confidence: ConfEstimated, Source: "worst case reserved at admission, actual once settled"}
}

func remainingFigure(limit, held int64) *Figure {
	rem, _ := sched.Add(limit, -held)
	return &Figure{Micro: max(rem, 0), Confidence: ConfEstimated, Source: "limit minus reserved"}
}

// BuildReport reads the ledger.
func BuildReport(ctx context.Context, st *store.Store, p Policy, now time.Time) (Report, error) {
	high, err := st.WindowHigh(ctx, dayOf(now))
	if err != nil {
		return Report{}, err
	}
	day, month := peekWindows(high, now)
	rep := Report{Currency: p.Currency, Day: day, Month: month, PaidEnabled: p.PaidEnabled, Budgets: []BudgetLine{}, Providers: []ProviderLine{}, Recent: []AttemptLine{}, Approvals: []ApprovalLine{}}
	line := func(name, key, window string, limit int64) error {
		held, err := st.Reserved(ctx, name, key, window)
		if err != nil {
			return err
		}
		scope := name
		if name == store.ScopeProvider {
			scope = "provider:" + key
		}
		rep.Budgets = append(rep.Budgets, BudgetLine{Scope: scope, Window: window, Limit: declared(limit, "operator policy economics.budgets.*"), Reserved: reservedFigure(held), Remaining: remainingFigure(limit, held)})
		return nil
	}
	if p.Budgets.Job.Set {
		rep.Budgets = append(rep.Budgets, BudgetLine{Scope: store.ScopeJob, Limit: declared(p.Budgets.Job.Micro, "operator policy economics.budgets.job")})
	}
	if p.Budgets.Pipeline.Set {
		rep.Budgets = append(rep.Budgets, BudgetLine{Scope: store.ScopePipeline, Limit: declared(p.Budgets.Pipeline.Micro, "operator policy economics.budgets.pipeline")})
	}
	if p.Budgets.Day.Set {
		if err = line(store.ScopeDay, "", day, p.Budgets.Day.Micro); err != nil {
			return Report{}, err
		}
	}
	if p.Budgets.Month.Set {
		if err = line(store.ScopeMonth, "", month, p.Budgets.Month.Micro); err != nil {
			return Report{}, err
		}
	}
	ids := map[string]bool{}
	for id := range p.Rates {
		ids[id] = true
	}
	for id := range p.CeilingsMicro {
		ids[id] = true
	}
	for id, lim := range p.Budgets.Provider {
		ids[id] = true
		if err = line(store.ScopeProvider, id, month, lim); err != nil {
			return Report{}, err
		}
	}
	totals, err := st.LedgerMonth(ctx, month)
	if err != nil {
		return Report{}, err
	}
	for id := range totals {
		ids[id] = true
	}
	reported, err := st.UsageProviders(ctx, month)
	if err != nil {
		return Report{}, err
	}
	for _, id := range reported {
		ids[id] = true
	}
	order := make([]string, 0, len(ids))
	for id := range ids {
		order = append(order, id)
	}
	sort.Strings(order)
	for _, id := range order {
		pl, err := providerLine(ctx, st, p, id, month, totals[id])
		if err != nil {
			return Report{}, err
		}
		rep.Providers = append(rep.Providers, pl)
	}
	rows, err := st.LedgerRecent(ctx, recentAttempts)
	if err != nil {
		return Report{}, err
	}
	for _, r := range rows {
		al := AttemptLine{Attempt: r.Attempt, Provider: r.Provider, Class: r.Class, Estimate: Figure{Micro: r.EstMicro, Confidence: ConfEstimated, Source: "worst case at admission"}}
		if r.Class == ClassLocal || r.Class == ClassOwned {
			al.Estimate = Figure{Micro: 0, Confidence: ConfComputed, Source: "local and owned work moves no money"}
		}
		if r.ActMicro != nil {
			al.Actual = &Figure{Micro: *r.ActMicro, Confidence: r.Confidence, Source: "settled when the run closed"}
		}
		rep.Recent = append(rep.Recent, al)
	}
	apps, err := st.Approvals(ctx, 10)
	if err != nil {
		return Report{}, err
	}
	for _, a := range apps {
		rep.Approvals = append(rep.Approvals, ApprovalLine{Run: a.Run, SHA: a.SHA, ConfigDigest: a.ConfigDigest, Actor: a.Actor,
			Estimate: Figure{Micro: a.EstimateMicro, Confidence: ConfEstimated, Source: "pending estimate the approver saw"}})
	}
	return rep, nil
}

func providerLine(ctx context.Context, st *store.Store, p Policy, id, month string, t store.MonthTotals) (ProviderLine, error) {
	pl := ProviderLine{Provider: id, Class: classOf(p, "", id)}
	if r, ok := p.Rates[id]; ok {
		src := "operator policy economics.rates." + id
		if r.PerHourMicro > 0 {
			pl.RatePerHour = declaredPtr(r.PerHourMicro, src)
		}
		if r.PerMinuteMicro > 0 {
			pl.RatePerMinute = declaredPtr(r.PerMinuteMicro, src)
		}
	}
	if c, ok := p.CeilingsMicro[id]; ok {
		pl.Ceiling = declaredPtr(c, "operator policy economics.ceilings."+id)
	}
	local := Figure{Confidence: ConfEstimated, Source: "local ledger: settled actuals plus estimates of attempts not settled"}
	local.Micro, _ = sched.Add(t.ActualMicro, t.PendingMicro)
	if t.Attempts > 0 && t.PendingMicro == 0 && t.AllComputed {
		local.Confidence = ConfComputed
	}
	pl.Local = local
	if pl.Class != ClassIncluded {
		return pl, nil
	}
	in, err := st.Allowance(ctx, id, month)
	if err != nil {
		return pl, err
	}
	if in.HasImport {
		pl.Imported = &Figure{Micro: in.Imported, Confidence: ConfKnown, Source: "provider-reported usage"}
	}
	a := AllowanceOf(p, id, in)
	pl.Allowance = &Figure{Micro: a.Remaining, Confidence: a.Confidence, Source: a.Source}
	pl.AllowanceKnown = a.Known && a.CeilingKnown
	return pl, nil
}

// EmptyReport is the report of a machine with no ledger yet: the declared policy and nothing spent.
func EmptyReport(p Policy, now time.Time) Report {
	day, month := peekWindows("", now)
	rep := Report{Currency: p.Currency, Day: day, Month: month, PaidEnabled: p.PaidEnabled, Budgets: []BudgetLine{}, Providers: []ProviderLine{}, Recent: []AttemptLine{}, Approvals: []ApprovalLine{}}
	for _, b := range []struct {
		name, window string
		budget       schedpolicy.Budget
	}{{store.ScopeJob, "", p.Budgets.Job}, {store.ScopePipeline, "", p.Budgets.Pipeline}, {store.ScopeDay, day, p.Budgets.Day}, {store.ScopeMonth, month, p.Budgets.Month}} {
		if b.budget.Set {
			bl := BudgetLine{Scope: b.name, Window: b.window, Limit: declared(b.budget.Micro, "operator policy economics.budgets.*")}
			if b.window != "" {
				bl.Reserved, bl.Remaining = reservedFigure(0), remainingFigure(b.budget.Micro, 0)
			}
			rep.Budgets = append(rep.Budgets, bl)
		}
	}
	return rep
}
