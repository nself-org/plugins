//go:build security

package security

// Purpose: SHA-bound run approvals (D18, TM-34, scenario 7). A first-time fork contributor's run
// is held by the `approval` admission step until a valid approval exists; the approval binds the
// run, head SHA, config digest and head repo; a push (the head moves) voids it with an audit row;
// a stale approver is refused; an approval cannot be replayed for another run; the step compares
// the approved SHA with the SHA the lease checks out, inside the admission transaction.
// Inputs:  the real store and dispatcher; the pure rules of trust/source.
// Outputs: pass or fail.
// Constraints: the production step lives in `package main` of cmd/approve.go and cannot be imported,
//          so approvalStep below mirrors it branch for branch over the same exported rules
//          (store.AdmitView.ApprovalState, source.RequiresApproval, source.VoidReasons). Every run of
//          this suite records its facts like the source hook (rig.prepare), so no test relies on a
//          bypass; a run with no facts is held when untrusted, as in production.
//          cmd/approve_test.go proves the production step itself.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func init() {
	if err := dispatch.RegisterStep("approval", approvalStep); err != nil {
		panic(err)
	}
}

func voidCheck(approved, cur store.ApprovalBinding) []string {
	return source.VoidReasons(source.Binding(approved), source.Binding(cur))
}

func approvalStep(ctx context.Context, v dispatch.View) error {
	st, ok, err := v.ApprovalState(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("approval: run %s has no pipeline row", v.PipelineID)
	}
	if !source.RequiresApproval(st.Requires, st.Facts, model.TrustClass(st.SourceTrust)) {
		return nil
	}
	hold := func(why string) error {
		return &dispatch.Refusal{Reason: model.PlacementReason(source.ReasonApprovalRequired), Detail: why}
	}
	if !strings.EqualFold(v.Request.Revision, st.Revision) {
		return hold("the lease checks out " + v.Request.Revision + " but run " + st.Run + " is at " + st.Revision)
	}
	switch reasons := voidCheck(st.Approval.ApprovalBinding, st.Current()); {
	case !st.HasApproval:
		return hold("run " + st.Run + " waits for an approval")
	case st.Approval.VoidedAt != 0:
		return hold("the approval was voided (" + st.Approval.VoidReason + ")")
	case len(reasons) > 0:
		return hold("the approval no longer matches the run (" + strings.Join(reasons, ",") + ")")
	}
	return nil
}

// forkRun prepares a fork PR run that needs approval and is not approved yet.
// submit takes the wait: settle for a run that must stay held, a longer one for a run that must be admitted.
func forkRun(t *testing.T, r *rig, name string) (id string, submit func(time.Duration) (dispatch.Admitted, error), runID string) {
	t.Helper()
	r.noAutoApprove = true
	j := newJob(name, "untrusted", "restricted")
	id, j = r.prepare(j, "untrusted")
	return id, func(wait time.Duration) (dispatch.Admitted, error) { return r.enqueue(id, j, wait) }, "pipe-" + name
}

func heldBecause(t *testing.T, r *rig, id string, a dispatch.Admitted, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("run %s was admitted on %s, it must be held", id, a.RunnerID)
	}
	d := awaitDecision(t, r, id)
	if d.Reason != source.ReasonApprovalRequired {
		t.Fatalf("decision: reason=%q outcome=%q, want %s", d.Reason, d.Outcome, source.ReasonApprovalRequired)
	}
}

// TestApprovalBoundToHeadSHA: held without an approval, admitted with one for the current head,
// held again after the head moves, admitted after a fresh approval of the new head.
func TestApprovalBoundToHeadSHA(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "fork1")

	a, err := submit(settle)
	heldBecause(t, r, id, a, err)

	row, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: voidCheck})
	if err != nil {
		t.Fatal(err)
	}
	if row.HeadSHA != baseRev || row.HeadRepo != "fork/app" || row.ConfigDigest != "i" {
		t.Fatalf("approval binding %+v", row.ApprovalBinding)
	}

	// The contributor pushes again: the run's head moves before the lease is granted.
	moveHead(t, r.db, runID, pushedRev, "i")
	a, err = submit(settle)
	heldBecause(t, r, id, a, err)
	if st, _, _ := r.st.ApprovalState(context.Background(), runID); !st.HasApproval || len(voidCheck(st.Approval.ApprovalBinding, st.Current())) != 1 {
		t.Fatalf("a push must leave a mismatch visible without any write: %+v", st)
	}
	voided, err := r.st.SweepApprovals(context.Background(), runID, voidCheck)
	if err != nil || len(voided) != 1 || voided[0].VoidReason != source.VoidHeadSHA {
		t.Fatalf("sweep after the push: %+v %v", voided, err)
	}
	// An approver who looked at the old head is refused; one who looked at the new head is accepted.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: voidCheck}); !errors.Is(err, store.ErrApprovalStale) {
		t.Fatalf("a stale approver: %v", err)
	}
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: pushedRev, Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	a, err = submit(2 * time.Second)
	if got := runnerOf(t, a, err); got != "gv1" {
		t.Fatalf("approved run placed on %q", got)
	}
	// The lease checks out the head that was approved, not the one that was approved before it.
	if ds := awaitDeliveries(r, 1); len(ds) != 1 || ds[0].Revision != pushedRev {
		t.Fatalf("deliveries %+v, want one carrying the approved head %s", ds, pushedRev)
	}
}

// awaitDeliveries waits (bounded) until the transport has seen n deliveries and returns them.
func awaitDeliveries(r *rig, n int) []dispatch.Delivery {
	for tries := 0; tries < 200 && len(r.tr.deliveries()) < n; tries++ {
		time.Sleep(10 * time.Millisecond)
	}
	return r.tr.deliveries()
}

// TestUntrustedRunWithNoFactsIsHeld mirrors the production step: a stranger's run whose source facts
// were never recorded needs an approval too (an unknown run is never waved through).
func TestUntrustedRunWithNoFactsIsHeld(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	r.noFacts = true
	id, a, err := r.submit(newJob("nofacts", "untrusted", "restricted"), "untrusted", settle)
	heldBecause(t, r, id, a, err)
}

// TestApprovalDoesNotTravel: an approval is bound to its run, config digest and head repository.
func TestApprovalDoesNotTravel(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	_, _, runA := forkRun(t, r, "forka")
	idB, submitB, runB := forkRun(t, r, "forkb")
	if _, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runA, Approver: "token:t1:bob", Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	// Run B has no approval of its own: A's does not unlock it.
	a, err := submitB(settle)
	heldBecause(t, r, idB, a, err)
	// A changed config (the workflow file the PR edited) voids the approval of the same SHA.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runB, Approver: "token:t1:bob", Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	moveHead(t, r.db, runB, baseRev, "i-changed")
	a, err = submitB(settle)
	heldBecause(t, r, idB, a, err)
	// The binding rule itself, over the pure type: a run id alone is never a valid approval.
	cur := source.Binding{Run: "r", HeadSHA: "abc", ConfigDigest: "d", HeadRepo: "fork/app"}
	for name, mutate := range map[string]func(*source.Binding){
		"other run": func(b *source.Binding) { b.Run = "r2" }, "other sha": func(b *source.Binding) { b.HeadSHA = "abd" },
		"other digest": func(b *source.Binding) { b.ConfigDigest = "e" }, "other head repo": func(b *source.Binding) { b.HeadRepo = "evil/app" },
		"empty run": func(b *source.Binding) { b.Run = "" },
	} {
		approved := cur
		mutate(&approved)
		if source.Valid(approved, cur) {
			t.Fatalf("approval with %s counted as valid", name)
		}
	}
	if !source.Valid(cur, source.Binding{Run: "r", HeadSHA: strings.ToUpper("abc"), ConfigDigest: "d", HeadRepo: "FORK/app"}) {
		t.Fatal("control: a SHA or repository that differs only in case must still match")
	}
}

// TestApprovalBindsLeaseCheckout (D18, TM-34): the admission step compares the approval with the stored
// pipeline head inside the admission transaction, the run dispatcher takes the lease's checkout revision
// from that same stored head (the rig does what run/dispatch_hook.go does), and the delivery that goes out
// carries exactly the approved head. A run whose head moved after the approval is held and delivers
// nothing, for the old head or the new one. A caller-supplied Job.Revision that differs from the stored head is
// held too (TestApprovalRefusesForeignCheckoutRevision, D-1360).
func TestApprovalBindsLeaseCheckout(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "forkc")
	row, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: voidCheck})
	if err != nil {
		t.Fatal(err)
	}
	a, err := submit(2 * time.Second)
	if got := runnerOf(t, a, err); got != "gv1" {
		t.Fatalf("approved run placed on %q", got)
	}
	ds := awaitDeliveries(r, 1)
	if len(ds) != 1 || ds[0].AttemptID != id || ds[0].Revision != row.HeadSHA || ds[0].Revision != baseRev {
		t.Fatalf("deliveries %+v, want exactly the approved attempt at the approved head %s", ds, row.HeadSHA)
	}
	// A second run, approved and then pushed: nothing is delivered for it.
	id2, submit2, run2 := forkRun(t, r, "forkd")
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: run2, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	moveHead(t, r.db, run2, pushedRev, "i")
	a2, err2 := submit2(settle)
	heldBecause(t, r, id2, a2, err2)
	for _, d := range r.tr.deliveries() {
		if d.AttemptID == id2 {
			t.Fatalf("a run whose head moved after its approval was delivered at %s", d.Revision)
		}
	}
}

// TestApprovalRefusesForeignCheckoutRevision (D-1360, TM-34): a caller that queues revision X for a run whose
// row and approval say Y gets nothing delivered, even though the approval is valid for Y.
func TestApprovalRefusesForeignCheckoutRevision(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	r.noAutoApprove = true
	j := newJob("forke", "untrusted", "restricted")
	id, j := r.prepare(j, "untrusted")
	if _, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: "pipe-forke", Approver: "token:t1:bob", ExpectSHA: baseRev, Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	a, err := r.enqueueAt(id, j, strings.Repeat("e", 40), settle)
	heldBecause(t, r, id, a, err)
	for _, d := range r.tr.deliveries() {
		if d.AttemptID == id {
			t.Fatalf("a lease for a foreign revision was delivered at %s", d.Revision)
		}
	}
}

// moveHead moves the stored head SHA and config digest of a run. Production starts a new run for a push (the
// new run has no approval: TestApprovalDoesNotTravel); no code updates a pipeline's revision in place. The raw
// UPDATE builds the one state the sweep and the admission step must judge, an approval that no longer matches
// the stored head, so the binding rule is proven where it decides.
func moveHead(t *testing.T, dbPath, run, sha, digest string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec("UPDATE pipeline SET revision=?,input_digest=? WHERE id=?", sha, digest, run)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("moveHead touched %d rows", n)
	}
}
