//go:build scenarios

package scenarios

// Purpose: the shared rig of the economics scenarios (Epic SCHED acceptance 5, P7-SCHED-15): a spend
// policy resolved the way the real loader resolves it (operator scope plus untrusted scopes), a store
// with an injected clock, a paid runner, a dispatcher world that carries the economics contributors'
// fields, direct admission requests and the roles actors that approve spend.
// Inputs:  key/value maps of policy keys (JSON values), an injected clock.
// Outputs: helpers the economics_*_test.go files share. All values are synthetic.
// Constraints: no sleeps, no network; the contributors in internal/economics run only inside
//          world.Build, so econWorld rebuilds the same four fields from the exported API.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// econStore opens a store whose clock is clk.
func econStore(t testing.TB, clk *clock) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// econResolve merges operator values (project scope, the trusted one) with planted values at the
// scope kind (an untrusted source), exactly as spendPolicy in the economics tests does.
func econResolve(kind trust.Source, operator, planted map[string]string) (*policy.Resolved, error) {
	scope := func(k trust.Source, m map[string]string) trust.Scope {
		v := map[string]json.RawMessage{}
		for key, raw := range m {
			v[key] = json.RawMessage(raw)
		}
		return trust.Scope{Kind: k, Values: v}
	}
	scopes := []trust.Scope{scope(trust.SourceProject, operator)}
	if planted != nil {
		scopes = append(scopes, scope(kind, planted))
	}
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		return nil, err
	}
	eff, digest, err := reg.Merge("", scopes...)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, s := range policy.Default.Keys() {
		names[s.Key] = true
	}
	for _, sc := range scopes {
		for k := range sc.Values {
			names[k] = true
		}
	}
	values := map[string]any{}
	for k := range names {
		if v, _, ok := eff.Value(k); ok {
			values[k] = v
		}
	}
	return &policy.Resolved{Effective: eff, Values: values, Digest: digest}, nil
}

// econPolicy is the typed spend policy of operator values with planted values at scope kind.
func econPolicy(t testing.TB, kind trust.Source, operator, planted map[string]string) economics.Policy {
	t.Helper()
	res, err := econResolve(kind, operator, planted)
	if err != nil {
		t.Fatal(err)
	}
	p, err := schedpolicy.Spend(res)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// econKV is a paid operator policy for provider "cloudco": 3.6 million micro per hour with a
// one-minute increment, so a 60 s job costs 60 000 micro. extra overrides or adds keys.
func econKV(extra map[string]string) map[string]string {
	kv := map[string]string{
		schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `1000000`, schedpolicy.KeyBudgetMonth: `5000000`,
		"economics.rates.cloudco": `{"class":"metered","rate_per_hour_micro":3600000,"billing_increment":"1m"}`,
	}
	for k, v := range extra {
		kv[k] = v
	}
	return kv
}

// econEnv is the economics environment of coordinator "me" over st with the injected clock.
func econEnv(st *store.Store, p economics.Policy, clk *clock) economics.Env {
	return economics.Env{Store: st, Spend: economics.StaticSpend(p), Now: clk.Now, Coordinator: "me"}
}

// econPaid is a LAN runner priced as class (metered or ephemeral) from provider.
func econPaid(id, class, provider string) sched.Runner {
	r := runner(id, "lan", "linux/amd64", 8)
	r.Class, r.Provider = class, provider
	if class == economics.ClassEphemeral {
		r.Ephemeral, r.CoordinatorMode = true, trust.Team
	}
	return r
}

// econWorld is liveWorld plus the economics fields: the spend policy and costs from the exported calls the
// contributors make, budgets, allowances and approvals from the real contributors through world.Build.
func econWorld(t testing.TB, st *store.Store, p economics.Policy, clk *clock, runners ...sched.Runner) func(context.Context, sched.Pipeline) (sched.World, string, error) {
	return liveWorld(t, st, func(w *sched.World) {
		ctx := context.Background()
		economics.ApplyPolicy(&w.Policy, p, "me")
		for _, r := range w.Runners {
			w.Costs[r.ID] = economics.CostFor(p, r.Class, r.Provider)
		}
		// budgets still open, allowances and valid approvals come from the production contributors, run by
		// world.Build with the economics environment bound; the cost of each runner is the same CostFor call
		// the costs contributor makes (the runners here are not registry rows, so the contributor cannot see them)
		bw, _, err := world.Build(economics.WithEnv(ctx, econEnv(st, p, clk)), world.Deps{Store: st, Clock: clk.Now,
			Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil }}, sched.Pipeline{ID: "pipe"})
		if err != nil {
			t.Error(err)
			return
		}
		w.Budgets, w.Approvals, w.Allowances = bw.Budgets, bw.Approvals, bw.Allowances
	}, runners...)
}

// econRequest is a direct admission request for attempt on a runner of class and provider.
func econRequest(attempt, class, provider string, timeoutMs int64) dispatch.Request {
	return dispatch.Request{AttemptID: attempt, Placement: sched.Placement{JobKey: attempt, RunnerID: "r-" + provider, CPU: 1, MemMB: 256},
		Runner: sched.Runner{ID: "r-" + provider, Class: class, Provider: provider}, Job: sched.Job{Key: attempt, TimeoutMs: timeoutMs}, Local: true,
		Host: "host-" + attempt, CapacityCPU: 8, CapacityMemMB: 8192}
}

// econAdmit admits through the real step chain with env in the context.
func econAdmit(env economics.Env, st *store.Store, r dispatch.Request) error {
	_, err := dispatch.Admit(economics.WithEnv(context.Background(), env), st, r)
	return err
}

// econRefusal returns the reason of an admission refusal, or "" when it was admitted.
func econRefusal(t testing.TB, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	ref, ok := err.(*dispatch.Refusal)
	if !ok {
		t.Fatalf("want a refusal or success, got %v", err)
	}
	return string(ref.Reason)
}

// econActors are the roles actors the approval and policy scenarios use.
type econActors struct {
	svc                     *roles.Service
	operator, admin, member roles.Actor
}

func newEconActors(t testing.TB, st *store.Store, clk *clock) econActors {
	t.Helper()
	ctx := context.Background()
	svc := roles.New(st, &audit.Log{St: st})
	svc.Now, svc.Self = clk.Now, "alice"
	op := svc.Operator("cli:alice")
	token := func(who string, role roles.Role) roles.Actor {
		tok, _, err := svc.IssueToken(ctx, op, roles.Request{Principal: who, Role: role, Projects: []string{roles.AllProjects}, TTL: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		a, err := svc.ActorFromToken(ctx, tok)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	return econActors{svc: svc, operator: op, admin: token("adm", roles.Admin), member: token("mem", roles.Member)}
}

// econPlaceWorld is an in-memory world for sched.Place: the spend policy applied, every runner priced
// by economics.CostFor and every set budget fully open (limit as remaining).
func econPlaceWorld(p economics.Policy, runners ...sched.Runner) sched.World {
	w := sched.World{Runners: runners, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}}
	economics.ApplyPolicy(&w.Policy, p, "me")
	open := func(b schedpolicy.Budget) sched.Budget { return sched.Budget{Set: b.Set, Remaining: b.Micro} }
	w.Budgets = sched.Budgets{Job: open(p.Budgets.Job), Pipeline: open(p.Budgets.Pipeline), Day: open(p.Budgets.Day), Month: open(p.Budgets.Month)}
	for _, r := range runners {
		w.Costs[r.ID] = economics.CostFor(p, r.Class, r.Provider)
	}
	return w
}

// econRefusalOf is the refusal reason of err, or "" when err is nil or not a refusal.
func econRefusalOf(err error) string {
	if ref, ok := err.(*dispatch.Refusal); ok {
		return string(ref.Reason)
	}
	return ""
}

// ctxT is a background context (kept short at call sites).
func ctxT(testing.TB) context.Context { return context.Background() }

// mustSpend reads the spend policy of env.
func mustSpend(t testing.TB, env economics.Env) economics.Policy {
	t.Helper()
	p, err := env.Spend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
