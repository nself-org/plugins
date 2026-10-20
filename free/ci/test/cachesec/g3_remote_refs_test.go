//go:build cachesec

package cachesec

// Purpose: artifact-ref refusals on the coordinator handler that production does not audit yet (debt D-1451):
// a crafted path (TM-14), another project's digest (TM-12) and a size that disagrees with the stored blob (TM-15).
// Each case baselines the bytes reachable after its authorized upload and reports only what the refused ref added. Each case asserts the refusal code and reason AND that no refusal audit row was appended (Observed.Audit = "none"),
// so it fails the day production adds the row. The handler is driven directly through the sink (carrier coordinator).
// Inputs: g3rig fixtures. Outputs: registered cases of the g3 rows of matrix.yaml.
// Constraints: production handlers only; every case first admits the honest ref on a fresh world (positive control).

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// g3RefWorld uploads data as the attempt's blob and returns the rig and sink with the lease running.
func g3RefWorld(t *testing.T, w *world, id string, f entries.Facts) (*g3rig, *g3Sink) {
	t.Helper()
	r := newG3Rig(t, w, id, f, nil, g3ArtJob())
	s := r.Sink()
	if err := s.Log(w.ctx, protocol.Log{Text: "started"}); err != nil {
		t.Fatal(err)
	}
	return r, s
}

// g3Added is what the refused ref made reachable beyond base (the bytes reachable before it, incl. the authorized
// upload): blobs by digest in the attacker's realm and any staged record of the attempt. It must be 0.
func g3Added(t *testing.T, r *g3rig, base int64, digest string) int64 {
	t.Helper()
	if staged, err := r.w.St.ArtifactStaged(r.w.ctx, r.id); err != nil || len(staged) != 0 {
		t.Fatalf("the refused ref left %d staged records (err %v)", len(staged), err)
	}
	return r.Landed(digest) - base
}

func g3ArtRefOf(r *g3rig, path string, size int64) protocol.ArtifactRef {
	return protocol.ArtifactRef{LeaseID: r.leaseID, Path: path, SHA256: g3Sum(g3ArtData), Size: size}
}

func init() {
	// C14-REM-01: the node names a traversal path in its artifact-ref (an `artifacts get` into .git/hooks starts here)
	register("C14-REM-01", func(t *testing.T) Observed {
		cw := newWorld(t)
		cr, cs := g3RefWorld(t, cw, "rctl1", g3Main)
		if err := g3Upload(cw.ctx, cs, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		if err := cs.ArtifactRef(cw.ctx, g3ArtRefOf(cr, "out/a.txt", int64(len(g3ArtData)))); err != nil {
			t.Fatalf("control: honest ref not admitted: %v", err)
		}
		w := newWorld(t)
		r, s := g3RefWorld(t, w, "rtrav1", g3Main)
		if err := g3Upload(w.ctx, s, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		base := r.Landed(g3Sum(g3ArtData)) // the authorized upload: the blob is reachable by digest, no record names it
		got := g3Refused(t, s.ArtifactRef(w.ctx, g3ArtRefOf(r, "../../.git/hooks/pre-commit", int64(len(g3ArtData)))))
		got.Audit = g3NoAudit(t, w)
		got.Landed = g3Added(t, r, base, g3Sum(g3ArtData))
		return got
	})

	// C12-REM-01: the ref names a digest that only another project's artifact realm holds
	register("C12-REM-01", func(t *testing.T) Observed {
		cw := newWorld(t)
		cr, cs := g3RefWorld(t, cw, "rctl2", g3Main)
		if err := g3Upload(cw.ctx, cs, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		if err := cs.ArtifactRef(cw.ctx, g3ArtRefOf(cr, "out/a.txt", int64(len(g3ArtData)))); err != nil {
			t.Fatalf("control: honest ref not admitted: %v", err)
		}
		w := newWorld(t)
		victim := entries.Facts{Project: "other", Trust: "owner", Protection: "protected"}
		vr, vs := g3RefWorld(t, w, "rvict1", victim)
		if err := g3Upload(w.ctx, vs, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		_ = vr
		r, s := g3RefWorld(t, w, "ratk1", g3Main)
		if base := r.Landed(g3Sum(g3ArtData)); base != 0 {
			t.Fatalf("the attacker's realm already holds %d B of the victim's digest", base)
		}
		got := g3Refused(t, s.ArtifactRef(w.ctx, g3ArtRefOf(r, "out/a.txt", int64(len(g3ArtData)))))
		got.Audit = g3NoAudit(t, w)
		got.Landed = g3Added(t, r, 0, g3Sum(g3ArtData)) // the victim's digest must not become reachable in the attacker's project
		return got
	})

	// C15-REM-01: the ref states a size other than the stored blob's (a substitution attempt)
	register("C15-REM-01", func(t *testing.T) Observed {
		cw := newWorld(t)
		cr, cs := g3RefWorld(t, cw, "rctl3", g3Main)
		if err := g3Upload(cw.ctx, cs, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		if err := cs.ArtifactRef(cw.ctx, g3ArtRefOf(cr, "out/a.txt", int64(len(g3ArtData)))); err != nil {
			t.Fatalf("control: honest ref not admitted: %v", err)
		}
		w := newWorld(t)
		r, s := g3RefWorld(t, w, "rsize1", g3Main)
		if err := g3Upload(w.ctx, s, "artifact", g3ArtData); err != nil {
			t.Fatal(err)
		}
		base := r.Landed(g3Sum(g3ArtData))
		got := g3Refused(t, s.ArtifactRef(w.ctx, g3ArtRefOf(r, "out/a.txt", int64(len(g3ArtData))+5)))
		got.Audit = g3NoAudit(t, w)
		got.Landed = g3Added(t, r, base, g3Sum(g3ArtData))
		return got
	})
}
