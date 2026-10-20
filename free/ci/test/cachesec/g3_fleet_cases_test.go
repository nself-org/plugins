//go:build cachesec

package cachesec

// Purpose: group g3 cases that run on the real simharness fleet: an artifact write by a node whose lease
// epoch was fenced (TM-13), and (corruption in transit is FINDING-g3-1: refused but not audited, so it has no matrix row). Each case has an honest control on the same fleet.
// Inputs: the shared g3Fleet. Outputs: registered cases of matrix.d/g3.yaml.
// Constraints: the real agent builds the blobs; the coordinator handlers are production code; the only
// injected fault is a wrapper around the admitted blob writer.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const g3ArtifactBody = "SYNTHETIC_ARTIFACT_aaaa1111\n"

func g3FleetArtifactJob() model.Job {
	return model.Job{Kind: "static", Run: []string{"sh", "-c", `echo started; mkdir -p out; echo SYNTHETIC_ARTIFACT_aaaa1111 > out/a.txt`},
		Artifacts: []model.ArtifactDecl{{Path: "out/a.txt", Name: "a"}}}
}

// g3ArtifactControl delivers and commits an honest artifact over the fleet and checks the record.
func g3ArtifactControl(t *testing.T, fl *g3Fleet, id string) {
	t.Helper()
	w := newWorld(t)
	r := newG3Rig(t, w, id, g3Main, nil, g3FleetArtifactJob())
	s := r.Sink()
	err := fl.Run(t, r, s)
	recs, cerr := r.CommitArtifacts()
	if cerr != nil || len(recs) != 1 || recs[0].SHA256 != g3Sum([]byte(g3ArtifactBody)) {
		t.Fatalf("control: honest artifact over the real fleet not committed: run=%v commit=%v recs=%d errs=%v logs=%q", err, cerr, len(recs), s.Errs(), s.Logs())
	}
}

func init() {
	// C13-SSH-03: a node uploads its artifact after the lease epoch was fenced.
	g3RegisterFleet("C13-SSH-03", func(t *testing.T, fl *g3Fleet) Observed {
		g3ArtifactControl(t, fl, "fact1")
		w := newWorld(t)
		r := newG3Rig(t, w, "fazomb", g3Main, nil, g3FleetArtifactJob())
		s := r.Sink()
		s.onBlob = r.Expire
		err := fl.Run(t, r, s)
		t.Logf("fleet run: %v; sink errors: %v; audit: %v", err, s.Errs(), w.AuditActions())
		got := g3FirstRefusal(t, s, "E714")
		got.Audit = w.AuditOf("artifact.remote.refused")
		got.Landed = r.Landed(g3Sum([]byte(g3ArtifactBody)))
		return got
	})
}

// g3FleetRun runs the lease over the fleet's carrier: "ssh" or "agent".
func (fl *g3Fleet) via(t *testing.T, carrier string, r *g3rig, s *g3Sink) error {
	t.Helper()
	if carrier == "agent" {
		return fl.RunAgent(t, r, s)
	}
	return fl.Run(t, r, s)
}

func init() {
	// C13-AGT-03 / C13-AGT-04: the stale-epoch cache and artifact writes with a real container on the agent pull carrier.
	g3RegisterFleet("C13-AGT-03", func(t *testing.T, fl *g3Fleet) Observed {
		cw := newWorld(t)
		cr := newG3Rig(t, cw, "gctl1", g3Main, g3PlanFull(g3NS(t, g3Main), "rw", g3Key), g3FleetCacheJob())
		cs := cr.Sink()
		err := fl.via(t, "agent", cr, cs)
		if fe := cr.cache.Finalize(context.Background(), 5*time.Second); fe != nil || cw.EntryBytes() == 0 {
			t.Fatalf("control: honest cache save over the real agent fleet not admitted: run=%v finalize=%v errs=%v bytes=%d logs=%q", err, fe, cs.Errs(), cw.EntryBytes(), cs.Logs())
		}
		w := newWorld(t)
		r := newG3Rig(t, w, "gzomb1", g3Main, g3PlanFull(g3NS(t, g3Main), "rw", g3Key), g3FleetCacheJob())
		s := r.Sink()
		s.onBlob = r.Expire
		err = fl.via(t, "agent", r, s)
		t.Logf("fleet run: %v; sink errors: %v; audit: %v", err, s.Errs(), w.AuditActions())
		got := g3FirstRefusal(t, s, "E714")
		got.Audit = w.AuditOf("cache.remote.refused")
		got.Landed = r.Landed()
		return got
	})
	g3RegisterFleet("C13-AGT-04", func(t *testing.T, fl *g3Fleet) Observed {
		cw := newWorld(t)
		cr := newG3Rig(t, cw, "gact1", g3Main, nil, g3FleetArtifactJob())
		cs := cr.Sink()
		err := fl.via(t, "agent", cr, cs)
		if recs, cerr := cr.CommitArtifacts(); cerr != nil || len(recs) != 1 {
			t.Fatalf("control: honest artifact over the real agent fleet not committed: run=%v commit=%v errs=%v logs=%q", err, cerr, cs.Errs(), cs.Logs())
		}
		w := newWorld(t)
		r := newG3Rig(t, w, "gazomb", g3Main, nil, g3FleetArtifactJob())
		s := r.Sink()
		s.onBlob = r.Expire
		err = fl.via(t, "agent", r, s)
		t.Logf("fleet run: %v; sink errors: %v; audit: %v", err, s.Errs(), w.AuditActions())
		got := g3FirstRefusal(t, s, "E714")
		got.Audit = w.AuditOf("artifact.remote.refused")
		got.Landed = r.Landed(g3Sum([]byte(g3ArtifactBody)))
		return got
	})
}
