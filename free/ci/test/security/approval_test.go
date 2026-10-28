//go:build security

package security

// Purpose: SHA-bound run approvals (D18, TM-34, scenario 7). A first-time fork contributor's run
// is held by the `approval` admission step until a valid approval exists; the approval binds the
// run, head SHA, config digest and head repo; a push (the head moves) voids it with an audit row;
// a stale approver is refused; an approval cannot be replayed for another run; the step compares
// the approved SHA with the SHA the lease checks out, inside the admission transaction.
// Inputs:  the real store and dispatcher; the production approval step.
// Outputs: pass or fail.
// Constraints: the suite registers and runs the production step (internal/approval) and its void
//          check, not a copy, so a regression in the step fails here. Every run of this suite
//          records its facts like the source hook (rig.prepare), so no test relies on a bypass; a
//          run with no facts is held when untrusted, as in production.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/approval"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func init() {
	if err := approval.Register(); err != nil {
		panic(err)
	}
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

	row, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: approval.VoidCheck})
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
	if st, _, _ := r.st.ApprovalState(context.Background(), runID); !st.HasApproval || len(approval.VoidCheck(st.Approval.ApprovalBinding, st.Current())) != 1 {
		t.Fatalf("a push must leave a mismatch visible without any write: %+v", st)
	}
	voided, err := r.st.SweepApprovals(context.Background(), runID, approval.VoidCheck)
	if err != nil || len(voided) != 1 || voided[0].VoidReason != source.VoidHeadSHA {
		t.Fatalf("sweep after the push: %+v %v", voided, err)
	}
	// An approver who looked at the old head is refused; one who looked at the new head is accepted.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: approval.VoidCheck}); !errors.Is(err, store.ErrApprovalStale) {
		t.Fatalf("a stale approver: %v", err)
	}
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: pushedRev, Check: approval.VoidCheck}); err != nil {
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

// TestVoidedApprovalStaysVoidedWhenTheHeadReturns: once a push voided the approval, a force push back to
// the approved head makes the binding match again, but the voided approval must not admit the run.
func TestVoidedApprovalStaysVoidedWhenTheHeadReturns(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "fork-back")
	ctx := context.Background()
	if _, err := r.st.ApproveRun(ctx, store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: approval.VoidCheck}); err != nil {
		t.Fatal(err)
	}
	moveHead(t, r.db, runID, pushedRev, "i")
	if voided, err := r.st.SweepApprovals(ctx, runID, approval.VoidCheck); err != nil || len(voided) != 1 {
		t.Fatalf("sweep after the push: %+v %v", voided, err)
	}
	moveHead(t, r.db, runID, baseRev, "i")
	st, _, err := r.st.ApprovalState(ctx, runID)
	if err != nil || !st.HasApproval || st.Approval.VoidedAt == 0 || len(approval.VoidCheck(st.Approval.ApprovalBinding, st.Current())) != 0 {
		t.Fatalf("setup: want a voided approval whose binding matches again, got %+v (%v)", st, err)
	}
	a, err := submit(settle)
	heldBecause(t, r, id, a, err)
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
	if _, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runA, Approver: "token:t1:bob", Check: approval.VoidCheck}); err != nil {
		t.Fatal(err)
	}
	// Run B has no approval of its own: A's does not unlock it.
	a, err := submitB(settle)
	heldBecause(t, r, idB, a, err)
	// A changed config (the workflow file the PR edited) voids the approval of the same SHA.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runB, Approver: "token:t1:bob", Check: approval.VoidCheck}); err != nil {
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
// nothing, for the old head or the new one. What this does not show: admission does not compare the
// caller-supplied Job.Revision with the stored head; D-1360 files that production gap.
func TestApprovalBindsLeaseCheckout(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "forkc")
	row, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: approval.VoidCheck})
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
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: run2, Approver: "token:t1:bob", ExpectSHA: baseRev, Check: approval.VoidCheck}); err != nil {
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
