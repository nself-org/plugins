package economics

// Tests added for the first code review of P7-SCHED-07: the approval threshold
// is about the pipeline's spend, an unbound process places nothing paid, the
// owner's rate table beats a runner's own class, included work is not capped like
// paid work, a closed run settles every open attempt, and the boundaries of the
// money comparisons.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestBudgetApprovalCountsThePipelineNotTheJob(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyApproval: `1000000`, schedpolicy.KeyBudgetDay: `100000000`, schedpolicy.KeyBudgetMonth: `100000000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	first := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(first, "", "cloudco", 900_000, 0)); err != nil { // 15 minutes = 900 000, under the threshold
		t.Fatalf("one job under the threshold: %v", err)
	}
	second := queueAttempt(t, st, "pipe", "rev", "project")
	ref := refusalOf(t, place(c, st, request(second, "", "cloudco", 900_000, 0)))
	if ref.Reason != model.ApprovalPending || !strings.HasPrefix(ref.Detail, PendingDetailPrefix+"1800000") {
		t.Fatalf("two jobs of 900 000 are a 1 800 000 pipeline: %+v", ref)
	}
	other := queueAttempt(t, st, "other-pipe", "rev", "project")
	if err := place(c, st, request(other, "", "cloudco", 900_000, 0)); err != nil {
		t.Fatalf("another pipeline starts from zero: %v", err)
	}
}

func TestBudgetApprovalThresholdBoundary(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyApproval: `120000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	exact := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(exact, "", "cloudco", 120_000, 0)); err != nil { // two minutes = 120 000 = the threshold
		t.Fatalf("a pipeline exactly at the threshold needs no approval: %v", err)
	}
	more := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(more, "", "cloudco", 60_000, 0))); ref.Reason != model.ApprovalPending {
		t.Fatalf("one micro-unit over: %+v", ref)
	}
}

func TestBudgetIncludedMarginBoundary(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, map[string]string{
		"economics.rates.github-hosted":    `{"class":"included","rate_per_minute_micro":100,"billing_increment":"1m"}`,
		"economics.ceilings.github-hosted": `1000000`, schedpolicy.KeyMargin: `50000`,
	})
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	// worst case of a one-minute job is 100; remaining after the import is 50 100, margin 50 000: left == 100.
	if err := RecordUsage(c, Usage{Key: "u1", Provider: "github-hosted", Date: "2026-10-18", CostMicro: 949_900}); err != nil {
		t.Fatal(err)
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(a, "", "github-hosted", 60_000, 0))); ref.Reason != model.AllowanceBelowMargin {
		t.Fatalf("remaining - margin equal to the worst case must refuse: %+v", ref)
	}
	if err := RecordUsage(c, Usage{Key: "u1", Provider: "github-hosted", Date: "2026-10-18", CostMicro: 949_899}); err != nil {
		t.Fatal(err)
	}
	if err := place(c, st, request(a, "", "github-hosted", 60_000, 0)); err != nil {
		t.Fatalf("one micro-unit more room admits: %v", err)
	}
}

func TestBudgetIncludedIsNotCappedLikePaidWork(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, map[string]string{
		"economics.rates.github-hosted":    `{"class":"included","rate_per_minute_micro":1,"billing_increment":"1m"}`,
		"economics.ceilings.github-hosted": `100000000`,
	})
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := RecordUsage(c, Usage{Key: "u1", Provider: "github-hosted", Date: "2026-10-18", CostMicro: 1}); err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []int64{3 * 3600 * 1000, 0} { // above the 2 h paid cap, and no timeout at all
		a := queueAttempt(t, st, "pipe", "rev", "project")
		if err := place(c, st, request(a, "", "github-hosted", timeout, 0)); err != nil {
			t.Fatalf("an included job with timeout %d ms: %v", timeout, err)
		}
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(a, "", "github-hosted", 200*3600*1000, 0))); ref.Reason != model.CostUnbounded {
		t.Fatalf("past the hard cap: %+v", ref)
	}
}

func TestBudgetUnboundProcessPlacesNothingPaid(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, &clock{t: at("2026-10-19T10:00:00Z")})
	for _, class := range []string{ClassMetered, ClassEphemeral} {
		a := queueAttempt(t, st, "pipe", "rev", "project")
		if ref := refusalOf(t, place(ctx, st, request(a, class, "cloudco", 60_000, 0))); ref.Reason != model.PaidDisabled {
			t.Fatalf("%s with economics unbound: %+v", class, ref)
		}
	}
	for _, class := range []string{"", ClassOwned, ClassLocal} {
		a := queueAttempt(t, st, "pipe", "rev", "project")
		if err := place(ctx, st, request(a, class, "operator", 60_000, 0)); err != nil {
			t.Fatalf("free work with class %q must not be touched: %v", class, err)
		}
	}
}

func TestBudgetRateTableBeatsTheRunnersOwnClass(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	for _, claimed := range []string{ClassOwned, ClassLocal, ClassIncluded} {
		a := queueAttempt(t, st, "pipe", "rev", "project")
		if err := place(c, st, request(a, claimed, "cloudco", 120_000, 0)); err != nil {
			t.Fatal(err)
		}
		row, _, _ := st.LedgerAttempt(ctx, a)
		if row.Class != ClassMetered || row.EstMicro != 120_000 {
			t.Fatalf("a runner claiming %q on a provider with a declared metered rate: %+v", claimed, row)
		}
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 360_000 {
		t.Fatalf("all three were priced and reserved: %d", held)
	}
	if got := CostFor(p, ClassOwned, "cloudco"); got.Class != ClassMetered || got.RateMicro == 0 {
		t.Fatalf("the world prices it the same way: %+v", got)
	}
}

func TestLedgerRunCloseSettlesEveryOpenAttempt(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	retried, final, other := queueAttempt(t, st, "pipe", "rev", "project"), queueAttempt(t, st, "pipe", "rev", "project"), queueAttempt(t, st, "pipe2", "rev", "project")
	for _, a := range []string{retried, final, other} {
		if err := place(c, st, request(a, "", "cloudco", 300_000, 0)); err != nil {
			t.Fatal(err)
		}
	}
	// Only the final attempt of the job is in the outcome; the earlier try is not.
	info := cirun.RunInfo{RunID: "pipe", Jobs: []cirun.JobOutcome{{AttemptID: final, Duration: time.Minute}}}
	if err := (finalizer{}).Finalize(c, info); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id       string
		settled  bool
		micro    int64
		conf     string
		whatItIs string
	}{{final, true, 60_000, ConfComputed, "in the outcome"}, {retried, true, 300_000, ConfUnknown, "an earlier try, no span recorded: settles at its estimate"}, {other, false, 0, "", "another run"}} {
		row, _, _ := st.LedgerAttempt(ctx, tc.id)
		if (row.ActMicro != nil) != tc.settled || (tc.settled && (*row.ActMicro != tc.micro || row.Confidence != tc.conf)) {
			t.Errorf("%s (%s): %+v act=%v", tc.id, tc.whatItIs, row, row.ActMicro)
		}
	}
}

func TestBudgetRepeatedResourceReservationIsOneReservation(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	for i := 0; i < 3; i++ {
		if err := reserveVM(c, st, "vm-retry", 5*time.Minute); err != nil {
			t.Fatalf("provision retry %d: %v", i+1, err)
		}
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 300_000 {
		t.Fatalf("held %d, want one 5-minute reservation", held)
	}
}

func TestBudgetApproveSpendDoesNotRevealWhichRunsExist(t *testing.T) {
	r := newApprovalRig(t)
	queueAttempt(t, r.st, "pipe", "sha-1", "project")
	_, errKnown := ApproveSpend(r.env, "pipe", r.member)
	_, errUnknown := ApproveSpend(r.env, "no-such-run", r.member)
	if errKnown == nil || errUnknown == nil || roles.ReasonOf(errKnown) != roles.ReasonRole || roles.ReasonOf(errUnknown) != roles.ReasonRole {
		t.Fatalf("a member gets the same refusal either way: %v / %v", errKnown, errUnknown)
	}
}

func TestLedgerAllowanceCountsLocalWorkFromTheNewestImportedDay(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, nil)
	put := func(attempt, day string, micro int64) {
		_ = st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
			return tx.PutLedger(store.LedgerRow{Attempt: attempt, Pipeline: "p", Provider: "gh", Class: ClassIncluded, EstMicro: micro, Confidence: ConfEstimated, WindowDay: day})
		})
	}
	put("early", "2026-10-17", 1)
	put("on-the-day", "2026-10-18", 10)
	put("later", "2026-10-19", 100)
	if err := st.PutUsage(ctx, store.UsageRow{Key: "k", Provider: "gh", Period: "2026-10", Day: "2026-10-18", CostMicro: 1000, ReportedAt: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC).UnixNano()}); err != nil {
		t.Fatal(err)
	}
	in, err := st.Allowance(ctx, "gh", "2026-10")
	if err != nil || in.Local != 110 {
		t.Fatalf("a provider that lags by hours may not yet count the newest day: local %d, want 10+100 %v", in.Local, err)
	}
}
