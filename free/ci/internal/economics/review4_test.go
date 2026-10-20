package economics

// Tests added for the third code review of P7-SCHED-07: the burst-VM shortcut is
// narrow (a runner's own label cannot buy a free pass), a refused VM hands its
// estimate back so one approval can release it, an empty coordinator id is
// refused when nothing else would refuse it, and a runaway window mark is no mark.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

func TestBudgetBurstVMShortcutIsNarrow(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, ephemeralKV(map[string]string{schedpolicy.KeyBudgetDay: `10000000`, "economics.rates.meteredco": `{"class":"metered","rate_per_hour_micro":3600000,"billing_increment":"1m"}`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := reserveVMFor(c, st, "vm1", "pipeA", 10*time.Minute); err != nil { // 600 000 for pipeA
		t.Fatal(err)
	}
	labelled := func(attempt, class, provider string, timeoutMs int64) error {
		r := request(attempt, class, provider, timeoutMs, 0)
		r.Runner.Capability.Identity.Labels = []string{IntentLabelPrefix + "vm1"}
		return place(c, st, r)
	}
	// 1. Another pipeline's job cannot ride on pipeA's VM.
	other := queueAttempt(t, st, "pipeB", "rev", "project")
	if row := labelled(other, ClassEphemeral, "ephemeral", 300_000); row == nil {
		t.Error("a job of another pipeline ran on pipeA's VM for free")
	} else if ref := refusalOf(t, row); ref.Reason != model.CostUnknownRate {
		t.Errorf("another pipeline: %+v", ref)
	}
	// 2. A runner that is not ephemeral (a rated, metered provider) gets no shortcut from a label.
	metered := queueAttempt(t, st, "pipeA", "rev", "project")
	if err := labelled(metered, ClassMetered, "meteredco", 300_000); err != nil {
		t.Fatalf("a metered runner is priced and reserved as usual: %v", err)
	}
	if row, _, _ := st.LedgerAttempt(ctx, metered); row.BudgetRefs == "resource:vm1" || row.EstMicro == 0 {
		t.Errorf("the label bought a free pass on a metered runner: %+v", row)
	}
	// 3. A job longer than the VM's reservation covers is not free on it; no timeout at all is not either.
	long := queueAttempt(t, st, "pipeA", "rev", "project")
	if err := labelled(long, ClassEphemeral, "ephemeral", 100*3600*1000); err == nil {
		t.Error("a 100 h job ran on a 10-minute reservation")
	}
	none := queueAttempt(t, st, "pipeA", "rev", "project")
	if err := labelled(none, ClassEphemeral, "ephemeral", 0); err == nil {
		t.Error("a job with no timeout ran on a VM for free")
	}
	// 4. The job the VM was bought for does run on it.
	good := queueAttempt(t, st, "pipeA", "rev", "project")
	if err := labelled(good, ClassEphemeral, "ephemeral", 300_000); err != nil {
		t.Fatalf("its own pipeline's job within the reservation: %v", err)
	}
	if row, _, _ := st.LedgerAttempt(ctx, good); row.BudgetRefs != "resource:vm1" || row.EstMicro != 0 {
		t.Errorf("%+v", row)
	}
}

func TestBudgetRefusedVMHandsBackItsEstimateForOneApproval(t *testing.T) {
	r := newApprovalRig(t)
	_ = queueAttempt(t, r.st, "pipe", "sha-1", "project")
	err := reserveVMFor(r.env, r.st, "vm1", "pipe", 15*time.Minute) // 900 000 against a 100 000 threshold
	var se *SpendError
	if !asSpend(err, &se) || se.Reason != model.ApprovalPending || se.Estimate != 900_000 {
		t.Fatalf("%v (estimate %d)", err, estimateOf(se))
	}
	// The VM's transaction rolled back with its refusal; the caller notes the estimate for the run.
	if err = NotePending(r.env, "pipe", se.Estimate); err != nil {
		t.Fatal(err)
	}
	if _, err = ApproveSpend(r.env, "pipe", r.admin); err != nil {
		t.Fatal(err)
	}
	if err = reserveVMFor(r.env, r.st, "vm1", "pipe", 15*time.Minute); err != nil {
		t.Fatalf("after one approval of the noted estimate: %v", err)
	}
}

func estimateOf(se *SpendError) int64 {
	if se == nil {
		return -1
	}
	return se.Estimate
}

func TestBudgetCoordinatorWithoutAnIdAndNoOwnerAnywhere(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil)) // no declared owner, nothing stored: only the empty id can refuse
	c := WithEnv(ctx, envFor(st, p, clk, ""))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(a, "", "cloudco", 60_000, 0))); ref.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("%+v", ref)
	}
	var se *SpendError
	if err := reserveVMFor(c, st, "vm", "", time.Minute); !asSpend(err, &se) || se.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("a VM: %v", err)
	}
	if owners, _ := st.ProviderOwners(ctx); len(owners) != 0 {
		t.Fatalf("an unnamed coordinator became an owner: %v", owners)
	}
}

func TestLedgerRunawayWindowMarkIsNotPeeked(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2099-01-01T00:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	clk.set(at("2026-10-20T10:00:00Z"))
	rep, err := BuildReport(ctx, st, spendPolicy(t, paidKV(nil)), clk.now())
	if err != nil || rep.Day != "2026-10-20" || rep.Month != "2026-10" {
		t.Fatalf("a mark from a jumped clock must not move the report's window: %+v %v", rep.Day, err)
	}
}

func TestLedgerReportTimeIsNeverInTheFuture(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-15T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{"economics.rates.other-hosted": `{"class":"included","rate_per_minute_micro":300000}`, "economics.ceilings.other-hosted": `1000000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := RecordUsage(c, Usage{Key: "k1", Provider: "other-hosted", Date: "2026-10-14"}); err != nil {
		t.Fatal(err)
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "included", "other-hosted", 180_000, 0)); err != nil {
		t.Fatal(err)
	}
	// A report claiming to have been made next year must not push the cut-off past the local attempt.
	if err := RecordUsage(c, Usage{Key: "k2", Provider: "other-hosted", Date: "2026-10-16", ReportedAt: at("2027-01-01T00:00:00Z")}); err != nil {
		t.Fatal(err)
	}
	if in, _ := st.Allowance(ctx, "other-hosted", "2026-10"); in.Local != 900_000 {
		t.Fatalf("local spend %d, want 900 000", in.Local)
	}
}
