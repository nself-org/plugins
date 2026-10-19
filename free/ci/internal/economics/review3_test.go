package economics

// Regression tests for the adversarial review of P7-SCHED-07 (each reproduces a
// defect found there): the allowance forgetting local spend, a burst VM that
// skipped the approval threshold or was counted twice, a VM overrun that
// reopened the budget, an empty coordinator id, a project-scoped approver, one
// forward clock jump pinning the window, and provider ids that spell GitHub oddly.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func ephemeralKV(extra map[string]string) map[string]string {
	kv := paidKV(map[string]string{"economics.rates.cloudco": `{"class":"ephemeral","rate_per_hour_micro":3600000,"billing_increment":"1m"}`})
	for k, v := range extra {
		kv[k] = v
	}
	return kv
}

func TestLedgerAllowanceKeepsLocalSpendWhenALateRowArrives(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-15T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{"economics.rates.other-hosted": `{"class":"included","rate_per_minute_micro":300000}`, "economics.ceilings.other-hosted": `1000000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := RecordUsage(c, Usage{Key: "acct/2026-10-14/minutes", Provider: "other-hosted", Date: "2026-10-14"}); err != nil {
		t.Fatal(err)
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "included", "other-hosted", 180_000, 0)); err != nil { // 900 000 of 1 000 000
		t.Fatal(err)
	}
	// A month-end line (or a lagging sku) dated later than every local attempt must not make them vanish.
	if err := RecordUsage(c, Usage{Key: "acct/2026-10-16/storage", Provider: "other-hosted", Date: "2026-10-16"}); err != nil {
		t.Fatal(err)
	}
	in, _ := st.Allowance(ctx, "other-hosted", "2026-10")
	if in.Local != 900_000 {
		t.Fatalf("local spend after a later-dated row: %d, want 900 000", in.Local)
	}
	b := queueAttempt(t, st, "pipe2", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(b, "included", "other-hosted", 180_000, 0))); ref.Reason != model.AllowanceBelowMargin {
		t.Fatalf("a second 900 000 job against a 1 000 000 ceiling: %+v", ref)
	}
	if err := RecordUsage(c, Usage{Key: "far", Provider: "other-hosted", Date: "2026-10-31"}); err == nil {
		t.Fatal("a usage line dated two weeks ahead was accepted")
	}
}

func TestBudgetBurstVMHonoursTheApprovalThreshold(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, ephemeralKV(map[string]string{schedpolicy.KeyApproval: `100000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	_ = queueAttempt(t, st, "pipe", "sha-1", "project")
	var se *SpendError
	err := reserveVMFor(c, st, "vm1", "pipe", 15*time.Minute) // 900 000 against a 100 000 threshold
	if !asSpend(err, &se) || se.Reason != model.ApprovalPending {
		t.Fatalf("an unapproved pipeline got a VM: %v", err)
	}
	if _, ok, _ := st.Resource(ctx, "vm1"); ok {
		t.Fatal("the refused VM left a resource row")
	}
	if err = reserveVMFor(c, st, "vm0", "pipe", time.Minute); err != nil { // 60 000, under the threshold
		t.Fatal(err)
	}
	// A VM's money counts in the pipeline estimate, so the pipeline's next paid job sees it.
	a := queueAttempt(t, st, "pipe", "sha-1", "project")
	if ref := refusalOf(t, place(c, st, request(a, "", "cloudco", 60_000, 0))); ref.Reason != model.ApprovalPending {
		t.Fatalf("60 000 (VM) + 60 000 (job) is over the threshold: %+v", ref)
	}
	// With the VM torn down at 30 s (billed one minute = 60 000) the estimate is its actual.
	clk.set(clk.now().Add(30 * time.Second))
	if err = releaseVM(c, st, "vm0", clk.now()); err != nil {
		t.Fatal(err)
	}
}

func reserveVMFor(ctx context.Context, st *store.Store, intent, pipeline string, life time.Duration) error {
	return st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
		return ReserveResource(ctx, tx, Resource{Intent: intent, Provider: "cloudco", Pipeline: pipeline, LifetimeMs: life.Milliseconds()})
	})
}

func TestBudgetAttemptOnABurstVMIsPaidByTheVM(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, ephemeralKV(map[string]string{schedpolicy.KeyBudgetDay: `1000000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := reserveVMFor(c, st, "vm1", "pipe", 15*time.Minute); err != nil { // 900 000 of 1 000 000
		t.Fatal(err)
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	r := request(a, ClassEphemeral, "ephemeral", 300_000, 0) // the runner's provider is the capability enum, not the account id
	r.Runner.Capability.Identity.Labels = []string{IntentLabelPrefix + "vm1"}
	if err := place(c, st, r); err != nil {
		t.Fatalf("the job the VM was bought for must run on it: %v", err)
	}
	row, _, _ := st.LedgerAttempt(ctx, a)
	if row.EstMicro != 0 || row.BudgetRefs != "resource:vm1" || row.Class != ClassEphemeral {
		t.Fatalf("ledger row: %+v", row)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 900_000 {
		t.Fatalf("the day holds %d: the VM's 900 000 once, not twice", held)
	}
	// Without the label (or after the VM is gone) the attempt is priced like any other.
	b := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(b, ClassEphemeral, "cloudco", 300_000, 0))); ref.Reason != model.BudgetExceeded {
		t.Fatalf("an unlabelled ephemeral attempt: %+v", ref)
	}
	if err := releaseVM(c, st, "vm1", clk.now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	d := queueAttempt(t, st, "pipe", "rev", "project")
	rd := request(d, ClassEphemeral, "ephemeral", 300_000, 0)
	rd.Runner.Capability.Identity.Labels = []string{IntentLabelPrefix + "vm1"}
	if ref := refusalOf(t, place(c, st, rd)); ref.Reason != model.CostUnknownRate {
		t.Fatalf("a job on a VM that was torn down is not free: %+v", ref)
	}
}

func TestBudgetVMOverrunIsRecordedAndReopensNothing(t *testing.T) {
	ctx := context.Background()
	t0 := at("2026-10-19T10:00:00Z")
	clk := &clock{t: t0}
	st := openStore(t, clk)
	p := spendPolicy(t, ephemeralKV(map[string]string{schedpolicy.KeyBudgetDay: `1000000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := reserveVMFor(c, st, "vm1", "pipe", 10*time.Minute); err != nil { // 600 000
		t.Fatal(err)
	}
	teardown := t0.Add(5 * time.Hour) // the failed teardown was retried for hours: 18 000 000 of real billing
	clk.set(teardown)
	if err := releaseVM(c, st, "vm1", teardown); err != nil {
		t.Fatal(err)
	}
	row, _, _ := st.Resource(ctx, "vm1")
	if row.ActualMicro == nil || *row.ActualMicro != 18_000_000 {
		t.Fatalf("the real cost of 5 h is 18 000 000, recorded %v", row.ActualMicro)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 18_000_000 {
		t.Fatalf("the day shows the overrun: %d", held)
	}
	var se *SpendError
	if err := reserveVMFor(c, st, "vm2", "pipe2", 6*time.Minute); !asSpend(err, &se) || se.Reason != model.BudgetExceeded {
		t.Fatalf("a budget that is already blown admits nothing more: %v", err)
	}
}

func TestBudgetCoordinatorWithoutAnIdCannotPlacePaidWork(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{"economics.providers.cloudco": `{"owner_coordinator":"other"}`}))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(WithEnv(ctx, envFor(st, p, clk, "")), st, request(a, "", "cloudco", 60_000, 0))); ref.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("%+v", ref)
	}
	if err := reserveVMFor(WithEnv(ctx, envFor(st, p, clk, "")), st, "vm", "", time.Minute); err == nil {
		t.Fatal("a coordinator with no id reserved a VM")
	}
}

func TestBudgetProjectScopedAdminApprovesWithTheProjectID(t *testing.T) {
	r := newApprovalRig(t)
	tok, _, err := r.svc.IssueToken(r.ctx, r.operator, roles.Request{Principal: "padm", Role: roles.Admin, Projects: []string{"acme"}})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := r.svc.ActorFromToken(r.ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	a := queueAttempt(t, r.st, "pipe", "sha-1", "/Users/dev/src/acme") // the pipeline row holds the repository root
	_ = place(r.env, r.st, request(a, "", "cloudco", 300_000, 0))
	if _, err = ApproveSpend(r.env, "pipe", scoped); err == nil {
		t.Fatal("a project-scoped admin approved with no project named")
	}
	if _, err = ApproveSpend(WithProject(r.env, "other-project"), "pipe", scoped); err == nil {
		t.Fatal("approved for a project the admin has no role on")
	}
	if _, err = ApproveSpend(WithProject(r.env, "acme"), "pipe", scoped); err != nil {
		t.Fatalf("the admin of acme, with the project id named: %v", err)
	}
	_ = audit.Log{}
}

func TestBudgetOneForwardClockJumpDoesNotPinTheWindow(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2099-01-01T00:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	clk.set(at("2026-10-20T10:00:00Z"))
	b := queueAttempt(t, st, "pipe2", "rev", "project")
	if err := place(c, st, request(b, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	if row, _, _ := st.LedgerAttempt(ctx, b); row.WindowDay != "2026-10-20" {
		t.Fatalf("after a one-off jump to 2099 and back, today's work is charged to %s", row.WindowDay)
	}
	entries, _ := st.AuditEntries(ctx, 0, 0)
	var reset bool
	for _, e := range entries {
		reset = reset || e.Action == "ledger.window.reset"
	}
	if !reset {
		t.Fatal("the replaced window mark is not audited")
	}
}

func TestBudgetJobBudgetCountsEveryAttemptOfTheJob(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyBudgetJob: `150000`}))
	// Two attempts of one job (a retry) share the job budget: 100 000 + 100 000 > 150 000.
	try := func(attempt, job string) error {
		return st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
			_, err := reserve(tx, p, spend{Owner: attempt, Pipeline: "p", Provider: "cloudco", Job: job, Micro: 100_000}, "2026-10-19", "2026-10")
			return err
		})
	}
	if err := try("att-1", "p/build"); err != nil {
		t.Fatal(err)
	}
	var se *SpendError
	if err := try("att-2", "p/build"); !asSpend(err, &se) || se.Reason != model.BudgetExceeded {
		t.Fatalf("a retry may not spend the job budget twice: %v", err)
	}
	if err := try("att-3", "p/other-job"); err != nil {
		t.Fatalf("another job has its own budget: %v", err)
	}
}

func TestEconomicsGitHubSpelledOddlyIsStillGitHub(t *testing.T) {
	p := spendPolicy(t, paidKV(nil))
	for _, id := range []string{"GitHub-Hosted", " github-hosted", "github-hosted\t", "GITHUB", "Gh", " hosted "} {
		if got := classOf(p, ClassMetered, id); got != ClassIncluded {
			t.Errorf("%q priced as %s", id, got)
		}
	}
}
