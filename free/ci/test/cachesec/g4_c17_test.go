//go:build cachesec

package cachesec

// Purpose: TM-17 cases on the local carrier: a release job without passing dependency evidence, untrusted
// output promoted into a release, the positive case (dependencies that ran on a remote node, signed, admitted
// once the TRUST-20 verifier is registered), the same after that node is revoked, and a published release whose
// run was quarantined. Admission goes through the real dispatch.Admit with job.Step (the production release
// step) so the refusal is recorded the way the coordinator records it (sched_refusal, read with store.Refusal).
// The signed-remote cases (04, 05) call job.Evaluate directly, not dispatch.Admit: debt D-1454 (the TRUST-20 verifier's Consume
// waits for the writer lock the admission transaction holds, so signed-remote Facts through the step time out and fail closed).
// Inputs: newWorld, g4Setup, newG4Release. Outputs: registered cases C17-REL-01..06.
// Constraints: production packages only; nothing is weakened. Landed is 1 when a refused attempt was leased.

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const g4Manifest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// g4E715 judges facts with the production gate and returns the typed E715.
func g4E715(t *testing.T, f job.Facts) *job.Error {
	t.Helper()
	e := job.Evaluate(f)
	if e == nil {
		t.Fatal("the release facts were admitted; the matrix expects a refusal")
	}
	if e.Code != job.CodeNotAdmitted {
		t.Fatalf("code %s, want E715", e.Code)
	}
	return e
}

// g4Refused asserts the dispatcher refused attempt with "release.<reason>", recorded it for `ci explain`, and
// left the attempt queued; it returns the number of attempts that got a lease (0).
func g4Refused(t *testing.T, fx g4Fixture, attempt string, ref *dispatch.Refusal, reason string) int64 {
	t.Helper()
	if ref == nil {
		t.Fatal("the dispatcher admitted a release attempt that must be refused")
	}
	if want := model.PlacementReason("release." + reason); ref.Reason != want {
		t.Fatalf("dispatcher reason %q, want %q", ref.Reason, want)
	}
	got, _, ok, err := fx.w.St.Refusal(fx.w.ctx, attempt)
	if err != nil || !ok || got != string(ref.Reason) {
		t.Fatalf("the refusal is not recorded: %q ok=%v err=%v", got, ok, err)
	}
	if fx.State(attempt) != "queued" {
		return 1
	}
	return 0
}

// g4Admission is the shape of C17-REL-01..03: an honest attempt is admitted, the bent one is refused.
func g4Admission(t *testing.T, bend func(*job.RunState), mutate func(*job.Facts), reason string) Observed {
	w := newWorld(t)
	fx := g4Setup(w, "owner")
	ok := fx.Queue("rel-ok")
	if ref := fx.Admit(ok, fx.Facts(false, nil), true); ref != nil || fx.State(ok) != "leased" {
		t.Fatalf("control: the honest release attempt was not admitted: %v", ref)
	}
	facts := fx.Facts(false, bend)
	if mutate != nil {
		mutate(&facts)
	}
	e := g4E715(t, facts)
	bad := fx.Queue("rel-bad")
	landed := g4Refused(t, fx, bad, fx.Admit(bad, facts, true), e.Reason)
	return Observed{Outcome: Refused, Code: e.Code, Reason: e.Reason, Audit: "sched_refusal", Landed: landed}
}

func init() {
	register("C17-REL-01", func(t *testing.T) Observed {
		d := g4Digests()
		d.Lockfiles = append(d.Lockfiles, model.DigestRef{Path: "Cargo.lock", SHA256: "4444444444444444444444444444444444444444444444444444444444444444"})
		return g4Admission(t, func(s *job.RunState) { s.Now = &d }, nil, job.DepsStale) // a lockfile appeared after the evidence
	})
	register("C17-REL-02", func(t *testing.T) Observed {
		return g4Admission(t, nil, func(f *job.Facts) { f.Evidence.RepoID = "another-repo" }, job.DepsMismatch) // evidence of another repository
	})
	register("C17-REL-03", func(t *testing.T) Observed {
		return g4Admission(t, func(s *job.RunState) { s.SourceTrust = "untrusted" }, nil, job.Trust) // output of an untrusted source
	})

	// C17-REL-07: release at-most-once. A second submission of an attempt that is already being submitted is refused
	// E715 duplicate_submission; Guard refuses before the attempt queues, so production audits nothing (debt D-1453).
	register("C17-REL-07", func(t *testing.T) Observed {
		w := newWorld(t)
		fx := g4Setup(w, "owner")
		facts := fx.Facts(false, nil)
		stash := job.NewStash()
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- stash.Guard(w.ctx, "rel-dup", job.Static(facts), func() error { close(entered); <-release; return nil })
		}()
		<-entered // the first submission is in flight
		submitted := 0
		err := stash.Guard(w.ctx, "rel-dup", job.Static(facts), func() error { submitted++; return nil })
		close(release)
		if submitted != 0 {
			t.Fatal("the duplicate reached the dispatcher")
		}
		if first := <-done; first != nil { // control: the first submission was admitted and completed
			t.Fatalf("control: the first submission failed: %v", first)
		}
		var e *job.Error
		if !errors.As(err, &e) {
			t.Fatalf("want an E715, got %v", err)
		}
		if err = stash.Guard(w.ctx, "rel-dup", job.Static(facts), func() error { submitted++; return nil }); err != nil || submitted != 1 {
			t.Fatalf("after the first finished the attempt can be submitted again (a retry): err %v submitted %d", err, submitted)
		}
		return Observed{Outcome: Refused, Code: e.Code, Reason: e.Reason, Audit: g4None(w), Landed: 0}
	})

	register("C17-REL-04", func(t *testing.T) Observed {
		w := newWorld(t)
		fx := g4Setup(w, "owner")
		facts := fx.Facts(true, nil) // the build ran on node "storetest" and the node signed its evidence
		if e := job.Evaluate(facts); e != nil {
			t.Fatalf("signed remote evidence was refused: %v", e)
		}
		return Observed{Outcome: Admitted}
	})

	register("C17-REL-05", func(t *testing.T) Observed {
		w := newWorld(t)
		fx := g4Setup(w, "owner")
		facts := fx.Facts(true, nil)
		if e := job.Evaluate(facts); e != nil { // control: accepted while the node is trusted
			t.Fatalf("control: %v", e)
		}
		row := store.ReleaseRow{RunID: g4Run, Project: g4Project, Revision: g4Revision, State: store.ReleasePublished, ManifestSHA256: g4Manifest, KeyID: "ci-release-0000000000000000"}
		if err := w.St.RecordRelease(w.ctx, row); err != nil {
			t.Fatal(err)
		}
		aff, err := cirun.RevokeNode(w.ctx, w.St, &audit.Log{St: w.St}, g4Node, "operator:cli:alice")
		if err != nil || len(aff.Keys) == 0 || len(aff.Releases) != 1 {
			t.Fatalf("revoke: %+v %v", aff, err)
		}
		e := g4E715(t, facts) // the very same signed document, after the revocation
		if len(g4AuditRows(w, "release.quarantine")) != 1 {
			t.Fatalf("the promoted release was not audited as quarantined: %v", g4AuditRows(w, ""))
		}
		return Observed{Outcome: Refused, Code: e.Code, Reason: e.Reason, Audit: g4Has(w, "evidence.revoke"), Landed: 0}
	})

	register("C17-REL-06", func(t *testing.T) Observed {
		w := newWorld(t)
		fx := g4Setup(w, "owner")
		rel := newG4Release(t, g4Run, nil)
		rel.Opt.Quarantined = func(ctx2 context.Context, ev model.Evidence) (bool, error) {
			return store.AttestLedger{S: w.St}.Quarantined(ctx2, ev.RunID)
		}
		if res := rel.Verify(); !res.Verified {
			t.Fatalf("control: the honest release does not verify: %s %s", res.Reason, res.Message)
		}
		if e := job.Evaluate(fx.Facts(true, nil)); e != nil { // the node's signature on run-1's build is accepted and recorded
			t.Fatalf("signed evidence refused: %v", e)
		}
		row := store.ReleaseRow{RunID: g4Run, Project: g4Project, Revision: g4Revision, State: store.ReleasePublished, ManifestSHA256: g4Manifest, KeyID: "ci-release-0000000000000000"}
		if err := w.St.RecordRelease(w.ctx, row); err != nil {
			t.Fatal(err)
		}
		if _, err := cirun.RevokeNode(w.ctx, w.St, &audit.Log{St: w.St}, g4Node, "operator:cli:alice"); err != nil {
			t.Fatal(err)
		}
		res := rel.Verify()
		if res.Verified {
			t.Fatal("a release of a quarantined run verified")
		}
		if res.Code != verify.Code || res.Exit != 1 {
			t.Fatalf("code %s exit %d", res.Code, res.Exit)
		}
		return Observed{Outcome: Refused, Code: res.Code, Reason: res.Reason, Audit: g4Has(w, "release.quarantine"), Landed: 0}
	})
}
