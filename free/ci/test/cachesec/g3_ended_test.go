//go:build cachesec

package cachesec

// Purpose: TM-13 writes after the attempt really ended. The attempt goes through the production terminal lifecycle
// (result -> finalizing, cache finalize, store.Transition to passed / failed) while a node is still uploading, and the
// node's late frames (blob, cache-ref, artifact-ref) reach the handlers over the in-process ssh and agent carriers.
// Inputs: g3rig fixtures and scripted nodes. Outputs: registered cases C13-SSH-06 and C13-AGT-05 of matrix.yaml.
// Constraints: production handlers and store only; the attempt ends BEFORE the handler sees the first late blob, so the
// refusal comes from the ended attempt and its fenced lease, never from a handler closed by Finalize. Not run on the
// real fleet: the in-process carriers are the production carriers, the container adds nothing to this refusal.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// End completes the production terminal lifecycle of the attempt: the result frame (running -> finalizing),
// the cache finalize for a passing job, then the store transition to `to` (passed or failed).
func (r *g3rig) End(to string) {
	r.t.Helper()
	ctx, st := r.w.ctx, r.w.St
	if err := r.Sink().Result(ctx, protocol.Result{LeaseID: r.leaseID, Epoch: uint64(r.epoch)}); err != nil {
		r.t.Fatalf("end %s: result: %v", r.id, err)
	}
	if to == "passed" && r.plan != nil {
		if err := r.cache.Finalize(ctx, time.Second); err != nil {
			r.t.Fatalf("end %s: finalize: %v", r.id, err)
		}
	}
	state, epoch, err := st.AttemptStateEpoch(ctx, r.id)
	if err != nil || state != "finalizing" {
		r.t.Fatalf("end %s: state %s err %v, want finalizing", r.id, state, err)
	}
	if err = st.Transition(ctx, r.id, "finalizing", model.JobState(to), store.TransitionDetail{Epoch: epoch}); err != nil {
		r.t.Fatalf("end %s: transition: %v", r.id, err)
	}
	if got, _, err := st.AttemptStateEpoch(ctx, r.id); err != nil || string(got) != to {
		r.t.Fatalf("end %s: attempt is %s (%v), want %s", r.id, got, err, to)
	}
}

// g3EndedCase: a cache attempt that ended passed and an artifact attempt that ended failed each get late frames.
func g3EndedCase(t *testing.T, carrier string) Observed {
	tgz := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC late payload"})
	g3CacheControl(t, carrier, tgz) // controls: the same uploads while the attempt lives are admitted
	g3ArtControlLocal(t, carrier)

	// cache: the attempt ended passed (Finalize ran) before the zombie's blob reaches the handler
	w := newWorld(t)
	r := newG3Rig(t, w, "late1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
	n := &g3node{steps: []g3step{g3Send("cache", tgz), g3CacheRef(g3Key, [2]any{"mod", tgz})}}
	s := r.Sink()
	s.onBlob = func() { r.End("passed") }
	g3Run(t, carrier, r, n, s)
	got := g3FirstRefusalOf(t, s)
	if w.AuditOf("cache.remote.refused") == "" {
		t.Fatal("the late cache blob left no cache.remote.refused audit row")
	}
	ref := protocol.CacheRef{LeaseID: r.leaseID, Spec: "go", Key: g3Key, Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: g3Sum(tgz), Size: int64(len(tgz))}}}
	if rr := g3Refused(t, s.CacheRef(w.ctx, ref)); rr.Code != "E714" && rr.Code != "E711" { // the late cache-ref frame
		t.Fatalf("late cache-ref: %+v", rr)
	}
	got.Audit = w.AuditOf("cache.remote.refused")
	got.Landed = r.Landed()

	// artifact: the attempt ended failed through the store before the zombie's artifact blob arrives
	aw := newWorld(t)
	ar := newG3Rig(t, aw, "late2", g3Main, nil, g3ArtJob())
	an := &g3node{steps: []g3step{g3Send("artifact", g3ArtData), g3ArtRef("out/a.txt", g3ArtData)}}
	as := ar.Sink()
	as.onBlob = func() { ar.End("failed") }
	g3Run(t, carrier, ar, an, as)
	ag := g3FirstRefusalOf(t, as)
	if ag.Code != "E714" || ag.Reason != "stale_epoch" || aw.AuditOf("artifact.remote.refused") == "" {
		t.Fatalf("late artifact blob: %+v audit %v", ag, aw.AuditActions())
	}
	if rr := g3Refused(t, as.ArtifactRef(aw.ctx, g3ArtRefOf(ar, "out/a.txt", int64(len(g3ArtData))))); rr.Code != "E714" { // the late artifact-ref frame
		t.Fatalf("late artifact-ref: %+v", rr)
	}
	got.Landed += ar.Landed(g3Sum(g3ArtData))
	return got
}

// g3FirstRefusalOf is the first refusal the handlers recorded (any fabric code).
func g3FirstRefusalOf(t *testing.T, s *g3Sink) Observed {
	t.Helper()
	errs := s.Errs()
	if len(errs) == 0 {
		t.Fatal("the handlers refused nothing")
	}
	return g3Refused(t, errs[0])
}

func init() {
	register("C13-SSH-06", func(t *testing.T) Observed { return g3EndedCase(t, "ssh") })
	register("C13-AGT-05", func(t *testing.T) Observed { return g3EndedCase(t, "agent") })
}
