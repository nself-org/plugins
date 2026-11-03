package run

// Purpose: the dispatcher's economics wiring (D-1280) with fake hooks: Bind before the dispatcher
// starts, the spend keys on the placement policy, and the pipeline worst case noted for a job held
// with approval.pending. Inputs: a hand-built runDispatch. Outputs: assertions only.
// Constraints: no store, no clock, no provider; the fake hooks record calls and change no spend value.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type captureSink struct{ got []dispatch.Decision }

func (c *captureSink) Record(_ context.Context, d dispatch.Decision) { c.got = append(c.got, d) }

func withHooks(t *testing.T, h *EconHooks) {
	t.Helper()
	old := econHooks
	econHooks = h
	t.Cleanup(func() { econHooks = old })
}

func TestEconomicsUnregisteredHooksAreInert(t *testing.T) {
	withHooks(t, nil)
	rd := &runDispatch{sess: &Session{}}
	if err := rd.bindEconomics(context.Background()); err != nil {
		t.Fatal(err)
	}
	var p sched.Policy
	rd.applySpend(&p)
	if rd.econRestore != nil || p.PaidEnabled {
		t.Fatalf("nothing registered: nothing bound or enabled (%v)", p)
	}
}

func TestEconomicsBindAndApplyUseTheResolvedSpend(t *testing.T) {
	var bound EconEnv
	var applied schedpolicy.SpendPolicy
	withHooks(t, &EconHooks{
		Bind:  func(e EconEnv) (func(), error) { bound = e; return func() {}, nil },
		Apply: func(_ *sched.Policy, sp schedpolicy.SpendPolicy, _ string) { applied = sp },
	})
	sp := schedpolicy.SpendPolicy{MaxPaidTimeoutMs: 42}
	rd := &runDispatch{sess: &Session{Coordinator: store.Coordinator{ID: "coord-1"}, Placement: &sched.Policy{}, Queue: &schedpolicy.QueuePolicy{}, Spend: &sp}}
	if err := rd.bindEconomics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rd.econRestore == nil || bound.Coordinator != "coord-1" {
		t.Fatalf("bound env %+v", bound)
	}
	if _, err := rd.policy(context.Background(), ""); err != nil || applied.MaxPaidTimeoutMs != 42 {
		t.Fatalf("the spend policy was not applied: %+v %v", applied, err)
	}
	if got, _ := bound.Spend(context.Background()); got.MaxPaidTimeoutMs != 42 {
		t.Fatalf("Spend answers from memory with the resolved policy, got %+v", got)
	}
}

func TestEconomicsNoSpendPolicyBindsTheDefaults(t *testing.T) {
	var bound EconEnv
	withHooks(t, &EconHooks{Bind: func(e EconEnv) (func(), error) { bound = e; return func() {}, nil }})
	rd := &runDispatch{sess: &Session{Placement: &sched.Policy{}, Queue: &schedpolicy.QueuePolicy{}}}
	if err := rd.bindEconomics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := bound.Spend(context.Background()); err != nil || got.PaidEnabled {
		t.Fatalf("no resolved spend policy is the default, which places nothing paid: %+v %v", got, err)
	}
}

func TestEconomicsSinkNotesThePipelineWorstCase(t *testing.T) {
	var run string
	var est int64
	var seen []int64
	withHooks(t, &EconHooks{
		Pending: func(_ context.Context, r string, e int64) error { run, est = r, e; return nil },
		Worst:   func(_ map[string]sched.Cost, ts []int64) int64 { seen = ts; return 777 },
	})
	pl := &plan.Plan{Jobs: []plan.PlannedJob{{ID: "a", Job: model.Job{Timeout: "5m"}}, {ID: "b"}}}
	inner := &captureSink{}
	s := econSink{DecisionSink: inner, rd: &runDispatch{sess: &Session{Plan: pl}}}
	d := dispatch.Decision{Job: sched.Job{Pipeline: sched.Pipeline{ID: "pipe-9"}}, World: &sched.World{}}
	s.Record(context.Background(), d) // not awaiting approval: nothing noted
	if run != "" || len(inner.got) != 1 {
		t.Fatalf("only an approval hold is noted (run %q, forwarded %d)", run, len(inner.got))
	}
	d.Outcome = dispatch.OutcomeAwaitingAppr
	s.Record(context.Background(), d)
	if run != "pipe-9" || est != 777 || len(inner.got) != 2 {
		t.Fatalf("noted %q %d, forwarded %d", run, est, len(inner.got))
	}
	if len(seen) != 2 || seen[0] != 300_000 || seen[1] != 30*60*1000 {
		t.Fatalf("the whole pipeline is priced, with the default timeout for a job that declares none: %v", seen)
	}
}
