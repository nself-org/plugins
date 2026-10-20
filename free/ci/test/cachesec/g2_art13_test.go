//go:build cachesec

package cachesec

// Purpose: TM-13 cases on the local artifact store (writes by a stale, ended or failed attempt).
// Inputs: a real store, cas and the records package through g2World. Outputs: registered cases
// C13-ART-01..06 of matrix.yaml.
// Constraints: production packages only; each case runs the honest operation first on a control
// attempt of the same world (so a refusal cannot come from a broken fixture); Landed is the new
// reachable bytes since Mark() (new committed record bytes plus new blobs). A zombie's blob staged
// before it was ended is no record's blob (staged rows are dropped at the end of an attempt, checked
// by g2StagedDropped, and cas.Sweep removes the blob), so it is not reachable and is not counted.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// g2RefusedAudit returns the audit action of the "*.refused" row recorded for target; the case fails without one.
func g2RefusedAudit(t *testing.T, w *world, target, reason string) string {
	t.Helper()
	for _, r := range w.Audit("") {
		if r.Target == target && strings.HasPrefix(r.Action, "artifact.") && strings.HasSuffix(r.Action, ".refused") && strings.Contains(r.Detail, "reason="+reason) {
			return r.Action
		}
	}
	t.Fatalf("no artifact refusal audit row for %s (reason %s); rows: %v", target, reason, w.AuditActions())
	return ""
}

// g2Control runs the honest lifecycle on attempt "ctl" and checks it landed bytes.
func g2Control(t *testing.T, g *g2World) {
	t.Helper()
	g.Write("ctl.txt", "honest control bytes")
	recs := g.Pass("ctl", "ctl.txt")
	if len(recs) != 1 || g.CommittedBytes("ctl") != int64(len("honest control bytes")) {
		t.Fatalf("control did not commit: %v", recs)
	}
	if len(g.Audit("artifact.commit")) != 1 {
		t.Fatal("control commit left no audit row")
	}
}

// g2Zombie returns an attempt that staged one file and is finalizing at epoch 3.
func g2Zombie(g *g2World, id string) {
	g.Write(id+".txt", "zombie bytes of "+id)
	f, ds := g.Attempt(id, id+".txt")
	g.Stage(id, f, ds)
	g.Finalizing(id)
}

// g2Held is what a zombie keeps in memory: the staged records it verified before it was ended.
func g2Held(t *testing.T, g *g2World, id string) []records.Record {
	t.Helper()
	held, err := g.St.ArtifactStaged(g.ctx, id)
	if err != nil || len(held) != 1 {
		t.Fatalf("zombie %s: staged %v err %v", id, held, err)
	}
	return held
}

// g2StagedDropped checks the store dropped the staged rows when the attempt ended (a late commit has nothing to flip).
func g2StagedDropped(t *testing.T, g *g2World, id string) {
	t.Helper()
	if left, _ := g.St.ArtifactStaged(g.ctx, id); len(left) != 0 {
		t.Fatalf("the ended attempt %s still holds %d staged record(s)", id, len(left))
	}
}

func g2Refuse(t *testing.T, g *g2World, err error, attempt, wantReason string) Observed {
	t.Helper()
	got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
	if got.Reason != wantReason {
		t.Fatalf("reason %s, want %s (%v)", got.Reason, wantReason, err)
	}
	got.Audit = g2RefusedAudit(t, g.world, attempt, wantReason)
	got.Landed = g.LandedSince() // Mark() was taken just before the attack
	return got
}

func init() {
	register("C13-ART-01", func(t *testing.T) Observed { // a cancelled attempt keeps running and commits
		g := newG2World(t)
		g2Control(t, g)
		g2Zombie(g, "z1")
		held := g2Held(t, g, "z1")
		if err := g.St.Transition(g.ctx, "z1", "finalizing", "cancelled", store.TransitionDetail{Epoch: 3}); err != nil {
			t.Fatal(err)
		}
		g.Mark()
		_, err := g.St.ArtifactCommit(g.ctx, "z1", 4, held, g.Pol)
		g2StagedDropped(t, g, "z1")
		return g2Refuse(t, g, err, "z1", records.ReasonAttempt)
	})
	register("C13-ART-02", func(t *testing.T) Observed { // a finalizing attempt commits with the epoch before its lease epoch
		g := newG2World(t)
		g2Control(t, g)
		g2Zombie(g, "z2")
		g.Mark()
		_, err := records.CommitAndVerify(g.ctx, g.St, g.Blobs, "z2", 2, g.Pol)
		return g2Refuse(t, g, err, "z2", records.ReasonEpoch)
	})
	register("C13-ART-03", func(t *testing.T) Observed { // a passed attempt reserves store bytes again
		g := newG2World(t)
		g2Control(t, g)
		g.Attempt("c3")
		if err := g.St.ArtifactReserve(g.ctx, "c3", 2, 100, g.Pol); err != nil {
			t.Fatalf("a running attempt could not reserve (control): %v", err)
		}
		g.Pass("z3")
		g.Mark()
		err := g.St.ArtifactReserve(g.ctx, "z3", 4, 100, g.Pol)
		return g2Refuse(t, g, err, "z3", records.ReasonAttempt)
	})
	register("C13-ART-04", func(t *testing.T) Observed { // a running attempt stages records with a stale epoch
		g := newG2World(t)
		g2Control(t, g)
		g.Write("z4.txt", "stale epoch bytes")
		f, ds := g.Attempt("z4", "z4.txt")
		st, err := g.Collector().Collect(g.ctx, f.Project, g.Ws, ds)
		if err != nil {
			t.Fatal(err)
		}
		g.Mark()
		err = g.St.ArtifactStage(g.ctx, "z4", 1, records.ToRecords("z4", f, 1, st, g.clockTime()))
		got := g2Refuse(t, g, err, "z4", records.ReasonEpoch)
		if staged, _ := g.St.ArtifactStaged(g.ctx, "z4"); len(staged) != 0 {
			t.Fatalf("a refused stage left %d staged record(s)", len(staged))
		}
		return got
	})
	register("C13-ART-05", func(t *testing.T) Observed { // a failed attempt commits what it staged
		g := newG2World(t)
		g2Control(t, g)
		g2Zombie(g, "z5")
		held := g2Held(t, g, "z5")
		g.End("z5", model.JobState("failed"))
		g.Mark()
		_, err := g.St.ArtifactCommit(g.ctx, "z5", 4, held, g.Pol)
		g2StagedDropped(t, g, "z5")
		return g2Refuse(t, g, err, "z5", records.ReasonAttempt)
	})
	register("C13-ART-06", func(t *testing.T) Observed { // a failed attempt reserves store bytes
		g := newG2World(t)
		g2Control(t, g)
		g.Attempt("z6", "z6.txt")
		if err := g.St.Transition(g.ctx, "z6", "running", "failed", store.TransitionDetail{Epoch: 2}); err != nil {
			t.Fatal(err)
		}
		g.Mark()
		err := g.St.ArtifactReserve(g.ctx, "z6", 3, 4096, g.Pol)
		return g2Refuse(t, g, err, "z6", records.ReasonAttempt)
	})
}
