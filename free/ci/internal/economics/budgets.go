package economics

// Purpose: reserving the worst case against every budget scope (Epic D11):
// per job (all attempts of it), per pipeline, per UTC day, per UTC month and per provider. Used by
// the admission step (an attempt) and ReserveResource (an ephemeral VM).
// Inputs:  the spend policy, the window keys and a spend (owner, pipeline, provider, amount).
// Outputs: reservation rows in the caller's transaction and the scope references
// recorded on the ledger row, or a *SpendError (budget.exceeded).
// Constraints: a scope is checked only when its budget is set (a budget of 0 is
// set and refuses everything); held money is the reserved amount, or the actual
// once settled; a sum that saturates is budget.exceeded; every check happens
// before the first write, so a refusal leaves nothing behind (the admission
// transaction would roll back anyway); the provider budget is counted per UTC month.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// spend is one reservation request.
type spend struct {
	Owner, Pipeline, Provider string
	// Job names the job whose attempts share the job budget (pipeline/job name); empty means the owner alone.
	Job   string
	Micro int64
}

type scope struct {
	name, key, window string
	limit             int64
	held              bool // the scope adds to money already held in it (a job budget is per attempt)
}

func scopesOf(p Policy, s spend, day, month string) []scope {
	var out []scope
	if p.Budgets.Job.Set {
		key := s.Job
		if key == "" {
			key = s.Owner
		}
		out = append(out, scope{store.ScopeJob, key, "", p.Budgets.Job.Micro, true})
	}
	if p.Budgets.Pipeline.Set && s.Pipeline != "" {
		out = append(out, scope{store.ScopePipeline, s.Pipeline, "", p.Budgets.Pipeline.Micro, true})
	}
	if p.Budgets.Day.Set {
		out = append(out, scope{store.ScopeDay, "", day, p.Budgets.Day.Micro, true})
	}
	if p.Budgets.Month.Set {
		out = append(out, scope{store.ScopeMonth, "", month, p.Budgets.Month.Micro, true})
	}
	if lim, ok := p.Budgets.Provider[s.Provider]; ok {
		out = append(out, scope{store.ScopeProvider, s.Provider, month, lim, true})
	}
	return out
}

func (sc scope) ref() string {
	switch sc.name {
	case store.ScopeDay, store.ScopeMonth:
		return sc.name + ":" + sc.window
	case store.ScopeProvider:
		return "provider:" + sc.key
	}
	return sc.name
}

// reserve checks every set budget and writes the reservations. It returns the scope references.
func reserve(t store.LedgerTx, p Policy, s spend, day, month string) ([]string, error) {
	scopes := scopesOf(p, s, day, month)
	refs := make([]string, 0, len(scopes))
	for _, sc := range scopes {
		var held int64
		if sc.held {
			var err error
			if held, err = t.ReservedExcept(sc.name, sc.key, sc.window, s.Owner); err != nil {
				return nil, err
			}
		}
		total, overflow := addSat(held, s.Micro)
		if overflow || total > sc.limit {
			return nil, refuse(model.BudgetExceeded, "%s budget %d micro: %d held plus %d reserved does not fit", sc.ref(), sc.limit, held, s.Micro)
		}
		refs = append(refs, sc.ref())
	}
	for _, sc := range scopes {
		if err := t.Reserve(store.Reservation{Owner: s.Owner, Scope: sc.name, ScopeKey: sc.key, WindowKey: sc.window, Micro: s.Micro}); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func refsString(refs []string) string { return strings.Join(refs, ",") }
