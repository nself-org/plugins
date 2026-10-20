package run

// D-1280 regression: the run dispatcher resolves the spend policy into the scheduler's policy and
// binds the economics hooks before it starts; unbound, no paid placement is ever possible.

import (
	"context"
	"encoding/json"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func econSession(t *testing.T, run string) (*Session, store.PipelineRow) {
	t.Helper()
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for k, v := range map[string]string{schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `1000000`, schedpolicy.KeyApproval: `500`} {
		w := store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: k, Value: json.RawMessage(v), Actor: "test"}
		if err = st.ApplyWrite(context.Background(), w, func(*store.Tx) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	row := store.PipelineRow{ID: run, Revision: "rev", Project: t.TempDir(), Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	return &Session{Store: st, RunID: run, Revision: "rev", Host: "coord-1", Capacity: func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 8192, BatteryPercent: -1}
	}}, row
}

// fakeWiring records what the dispatcher asks of economics; internal/economics imports this package,
// so the real wiring cannot be used here (economics' own test covers the registration).
type fakeWiring struct {
	binds, restores int
	applied         schedpolicy.SpendPolicy
	coordinator     string
	spend           func(context.Context) (schedpolicy.SpendPolicy, error)
	noted           map[string]int64
}

func wireFake(t *testing.T) *fakeWiring {
	t.Helper()
	f := &fakeWiring{noted: map[string]int64{}}
	econMu.Lock()
	old := econWiring
	econWiring = SpendWiring{
		Bind: func(_ *store.Store, c string, spend func(context.Context) (schedpolicy.SpendPolicy, error)) func() {
			f.binds++
			f.coordinator, f.spend = c, spend
			return func() { f.restores++ }
		},
		ApplyPolicy: func(sp *sched.Policy, p schedpolicy.SpendPolicy, c string) {
			f.applied = p
			sp.PaidEnabled, sp.CoordinatorID = p.PaidEnabled, c
		},
		NotePending: func(_ context.Context, run string, est int64) error { f.noted[run] = est; return nil },
	}
	econMu.Unlock()
	t.Cleanup(func() { econMu.Lock(); econWiring = old; econMu.Unlock() })
	return f
}

func TestRunPolicyCarriesTheSpendKeys(t *testing.T) {
	f := wireFake(t)
	sess, row := econSession(t, "r1")
	rd := &runDispatch{sess: sess, row: row}
	p, err := rd.policy(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !p.PaidEnabled || p.CoordinatorID != "coord-1" || !f.applied.PaidEnabled || f.applied.ApprovalThresholdMicro != (schedpolicy.Budget{Micro: 500, Set: true}) {
		t.Fatalf("spend keys not applied: policy %+v, resolved spend %+v", p, f.applied)
	}
}

func TestRunDispatcherBindsEconomicsUntilTheLastReleases(t *testing.T) {
	f := wireFake(t)
	sess, _ := econSession(t, "r2")
	ctx := context.Background()
	if _, err := runDispatcher(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if f.binds != 1 || f.coordinator != "coord-1" {
		t.Fatalf("economics was not bound before the dispatcher started: %+v", f)
	}
	if got, err := f.spend(ctx); err != nil || !got.PaidEnabled {
		t.Fatalf("the bound spend policy is not the run's: %+v %v", got, err)
	}
	if err := notePending(ctx, "r2", 7); err != nil || f.noted["r2"] != 7 {
		t.Fatalf("the pending-spend hook does not reach economics: %v %+v", err, f.noted)
	}
	sess3, _ := econSession(t, "r3")
	if _, err := runDispatcher(ctx, sess3); err != nil {
		t.Fatal(err)
	}
	dispatchRelease("r2")
	if f.binds != 1 || f.restores != 0 {
		t.Fatalf("an earlier release unbound a live run: %+v", f)
	}
	dispatchRelease("r3")
	if f.restores != 1 {
		t.Fatalf("the last release did not restore the previous binding: %+v", f)
	}
}
