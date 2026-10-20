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
//          so approvalStep below mirrors it line for line over the same exported rules
//          (store.AdmitView.ApprovalState, source.RequiresApproval, source.VoidReasons); its single
//          deviation is that a run with no stored facts passes, so the other cases of this suite,
//          which never record facts, are not held. cmd/approve_test.go proves the production step.

import (
	"context"
	"database/sql"
	"errors"
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
	if !ok || !st.Facts {
		return nil
	}
	if !source.RequiresApproval(st.Requires, st.Facts, model.TrustClass(st.SourceTrust)) {
		return nil
	}
	hold := func(why string) error {
		return &dispatch.Refusal{Reason: model.PlacementReason(source.ReasonApprovalRequired), Detail: why}
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

// forkRun prepares a fork PR run that needs approval and records its source facts.
func forkRun(t *testing.T, r *rig, name string) (id string, submit func() (dispatch.Admitted, error), runID string) {
	t.Helper()
	j := newJob(name, "untrusted", "restricted")
	id, j = r.prepare(j, "untrusted")
	runID = "pipe-" + name
	if _, err := r.st.RecordApprovalFacts(context.Background(), store.ApprovalRunFacts{Run: runID, Author: "mallory", HeadRepo: "fork/app", RequiresApproval: true}, voidCheck); err != nil {
		t.Fatal(err)
	}
	return id, func() (dispatch.Admitted, error) { return r.enqueue(id, j, settle) }, runID
}

func heldBecause(t *testing.T, r *rig, id string, a dispatch.Admitted, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("run %s was admitted on %s, it must be held", id, a.RunnerID)
	}
	d, ok := r.sink.last(id)
	if !ok || d.Reason != source.ReasonApprovalRequired {
		t.Fatalf("decision: ok=%v reason=%q outcome=%q, want %s", ok, d.Reason, d.Outcome, source.ReasonApprovalRequired)
	}
}

// TestApprovalBoundToHeadSHA: held without an approval, admitted with one for the current head,
// held again after the head moves, admitted after a fresh approval of the new head.
func TestApprovalBoundToHeadSHA(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "fork1")

	a, err := submit()
	heldBecause(t, r, id, a, err)

	row, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: "rev", Check: voidCheck})
	if err != nil {
		t.Fatal(err)
	}
	if row.HeadSHA != "rev" || row.HeadRepo != "fork/app" || row.ConfigDigest != "i" {
		t.Fatalf("approval binding %+v", row.ApprovalBinding)
	}

	// The contributor pushes again: the run's head moves before the lease is granted.
	moveHead(t, r.db, runID, "rev2", "i")
	a, err = submit()
	heldBecause(t, r, id, a, err)
	if st, _, _ := r.st.ApprovalState(context.Background(), runID); !st.HasApproval || len(voidCheck(st.Approval.ApprovalBinding, st.Current())) != 1 {
		t.Fatalf("a push must leave a mismatch visible without any write: %+v", st)
	}
	voided, err := r.st.SweepApprovals(context.Background(), runID, voidCheck)
	if err != nil || len(voided) != 1 || voided[0].VoidReason != source.VoidHeadSHA {
		t.Fatalf("sweep after the push: %+v %v", voided, err)
	}
	// An approver who looked at the old head is refused; one who looked at the new head is accepted.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: "rev", Check: voidCheck}); !errors.Is(err, store.ErrApprovalStale) {
		t.Fatalf("a stale approver: %v", err)
	}
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: "rev2", Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	a, err = submit()
	if got := runnerOf(t, a, err); got != "gv1" {
		t.Fatalf("approved run placed on %q", got)
	}
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
	a, err := submitB()
	heldBecause(t, r, idB, a, err)
	// A changed config (the workflow file the PR edited) voids the approval of the same SHA.
	if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runB, Approver: "token:t1:bob", Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	moveHead(t, r.db, runB, "rev", "i-changed")
	a, err = submitB()
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

// TestApprovalBindsLeaseCheckout (D18, TM-34): the revision the lease checks out is the stored
// pipeline revision, and admission compares the approval with that revision inside the
// admission transaction. The delivery that goes out carries the approved revision.
func TestApprovalBindsLeaseCheckout(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), gvisorRunner("gv1", 4))
	id, submit, runID := forkRun(t, r, "forkc")
	if _, err := r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:bob", ExpectSHA: "rev", Check: voidCheck}); err != nil {
		t.Fatal(err)
	}
	a, err := submit()
	if got := runnerOf(t, a, err); got != "gv1" {
		t.Fatalf("approved run placed on %q", got)
	}
	for tries := 0; tries < 100 && len(r.tr.deliveries()) == 0; tries++ {
		time.Sleep(10 * time.Millisecond)
	}
	ds := r.tr.deliveries()
	if len(ds) != 1 || ds[0].AttemptID != id {
		t.Fatalf("deliveries %+v, want exactly the approved attempt", ds)
	}
}

// moveHead simulates a push to the pull request: the run's stored head SHA and config digest change.
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
