package economics

// Tests added for the second code review of P7-SCHED-07: free local work while
// the spend policy cannot be read, one approval for a whole pipeline when the
// dispatcher notes its full estimate, what the pipeline estimate counts, and a
// burst VM that belongs to no pipeline.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestBudgetUnreadablePolicyDoesNotBlockFreeLocalWork(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	env := Env{Store: st, Now: clk.now, Coordinator: "me", Spend: func(context.Context) (Policy, error) { return Policy{}, errors.New("policy store unreadable") }}
	c := WithEnv(ctx, env)
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, ClassLocal, "operator", 60_000, 0)); err != nil {
		t.Fatalf("a local run must still start: %v", err)
	}
	if row, ok, _ := st.LedgerAttempt(ctx, a); !ok || row.Confidence != ConfComputed || row.EstMicro != 0 {
		t.Fatalf("local ledger row: %+v", row)
	}
	// Anything that could cost money fails closed: remote, hosted, paid, or a GitHub runner.
	for _, tc := range []struct {
		name  string
		class string
		prov  string
		local bool
	}{{"remote owned", ClassOwned, "operator", false}, {"paid", ClassMetered, "cloudco", true}, {"unclassified", "", "cloudco", true}, {"github", ClassOwned, "github-hosted", true}} {
		b := queueAttempt(t, st, "pipe", "rev", "project")
		r := request(b, tc.class, tc.prov, 60_000, 0)
		r.Local = tc.local
		if err := place(c, st, r); err == nil || errors.As(err, new(*SpendError)) {
			t.Errorf("%s: want the raw policy error to fail the admission, got %v", tc.name, err)
		}
		if state, _, _ := st.AttemptStateEpoch(ctx, b); state != "queued" {
			t.Errorf("%s: attempt is %s, must stay queued", tc.name, state)
		}
	}
}

func TestBudgetOneApprovalReleasesAPipelineNotedInFull(t *testing.T) {
	r := newApprovalRig(t)
	// The dispatcher knows every job of the run: it notes the pipeline's whole worst case once.
	if err := NotePending(r.env, "pipe", 180_000); err != nil {
		t.Fatal(err)
	}
	var jobs []string
	for i := 0; i < 3; i++ {
		jobs = append(jobs, queueAttempt(t, r.st, "pipe", "sha-1", "project"))
	}
	if err := place(r.env, r.st, request(jobs[0], "", "cloudco", 60_000, 0)); err != nil {
		t.Fatalf("60 000 is under the 100 000 threshold: %v", err)
	}
	if ref := refusalOf(t, place(r.env, r.st, request(jobs[1], "", "cloudco", 60_000, 0))); ref.Reason != model.ApprovalPending {
		t.Fatalf("the second job takes the pipeline to 120 000: %+v", ref)
	}
	got, err := ApproveSpend(r.env, "pipe", r.admin)
	if err != nil || got.EstimateMicro != 180_000 {
		t.Fatalf("the noted whole-pipeline figure is the estimate: %+v %v", got, err)
	}
	for _, j := range jobs[1:] {
		if err = place(r.env, r.st, request(j, "", "cloudco", 60_000, 0)); err != nil {
			t.Fatalf("one approval covers the whole 180 000 pipeline: %v", err)
		}
	}
}

func TestBudgetPipelineEstimateCountsSettledActualsAndPaidRowsOnly(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyApproval: `400000`, "economics.rates.github-hosted": `{"class":"included","rate_per_minute_micro":100}`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	// A shadow-priced included attempt is not money: it does not count toward the threshold.
	_ = st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
		return tx.PutLedger(store.LedgerRow{Attempt: "inc", Pipeline: "pipe", Provider: "github-hosted", Class: ClassIncluded, EstMicro: 900_000, Confidence: ConfEstimated, WindowDay: "2026-10-19"})
	})
	first := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(first, "", "cloudco", 300_000, 0)); err != nil { // 300 000 <= 400 000 although "inc" holds 900 000
		t.Fatalf("included rows counted as spend: %v", err)
	}
	// The first job ends at 60 000: the pipeline's estimate is the actual, not the worst case.
	if _, err := st.LedgerSettle(ctx, first, 60_000, 60_000, ConfComputed); err != nil {
		t.Fatal(err)
	}
	second := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(second, "", "cloudco", 300_000, 0)); err != nil { // 60 000 + 300 000 <= 400 000
		t.Fatalf("a settled attempt must count at its actual: %v", err)
	}
	third := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(third, "", "cloudco", 60_000, 0))); ref.Reason != model.ApprovalPending { // 60 + 300 + 60 = 420 000
		t.Fatalf("%+v", ref)
	}
	// The attempt being admitted never counts against itself.
	_ = st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
		if got, _ := tx.PipelineHeld("pipe", second); got != 60_000 {
			t.Errorf("held without the second attempt: %d", got)
		}
		if got, _ := tx.PipelineHeld("pipe", "someone-else"); got != 360_000 {
			t.Errorf("held with everyone: %d", got)
		}
		return nil
	})
}

func TestBudgetBurstVMWithNoPipelineSkipsOnlyThePipelineBudget(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyBudgetPipeline: `1`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	err := st.LedgerWrite(c, func(tx store.LedgerTx) error {
		return ReserveResource(c, tx, Resource{Intent: "vm-free-floating", Provider: "cloudco", LifetimeMs: int64(5 * time.Minute / time.Millisecond)})
	})
	if err != nil {
		t.Fatalf("a VM that belongs to no pipeline is not held to the pipeline budget: %v", err)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 300_000 {
		t.Fatalf("it still counts against the day: %d", held)
	}
	if held, _ := st.Reserved(ctx, "pipeline", "", ""); held != 0 {
		t.Fatalf("and holds nothing in a pipeline scope: %d", held)
	}
}
