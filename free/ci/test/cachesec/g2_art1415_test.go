//go:build cachesec

package cachesec

// Purpose: TM-14 and TM-15 matrix cases of the local artifact store that production audits: a size
// bomb stopped by the store caps (reserve and commit) and the substitution of a committed artifact by
// an ended attempt (stage and declare). The refusals production does NOT audit (E683 collection and
// get destination rules, digest mismatch, secret match) are in g2_plain_test.go and FINDING-g2-1.md.
// Inputs: g2World. Outputs: registered cases C14-ART-01..02, C15-ART-01..02.
// Constraints: production packages only; honest control first; Landed counts committed record bytes
// of the attacking attempt, and the blob files on disk where the refusal precedes any write.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func init() {
	register("C14-ART-01", func(t *testing.T) Observed { // a second attempt declares outputs the store has no room for next to a running claim
		g := newG2World(t)
		g.Pol.MaxFile, g.Pol.MaxTotal = 4096, 4096
		g.Write("big1.bin", strings.Repeat("a", 3000))
		g.Write("big2.bin", strings.Repeat("b", 3000))
		fa, da := g.Attempt("b1", "big1.bin")
		c := g.Collector()
		c.Reserve = func(total int64) error { return g.St.ArtifactReserve(g.ctx, "b1", 2, total, g.Pol) }
		if _, err := c.Collect(g.ctx, fa.Project, g.Ws, da); err != nil {
			t.Fatalf("the first 3000 B claim was refused (control): %v", err)
		}
		before := g.BlobFiles()
		fb, db := g.Attempt("b2", "big2.bin")
		c.Reserve = func(total int64) error { return g.St.ArtifactReserve(g.ctx, "b2", 2, total, g.Pol) }
		_, err := c.Collect(g.ctx, fb.Project, g.Ws, db)
		got := g2Refuse(t, g, err, "b2", records.ReasonSizeCap)
		if g.BlobFiles() != before {
			t.Fatalf("the refused claim wrote blobs: %d -> %d", before, g.BlobFiles())
		}
		if got.Code != "E712" {
			t.Fatalf("size_cap on reserve must be E712, got %s", got.Code)
		}
		return got
	})
	register("C14-ART-02", func(t *testing.T) Observed { // the store cap shrinks after staging: the commit of an oversize set is refused
		g := newG2World(t)
		g.Write("fits.bin", strings.Repeat("c", 3000))
		g.Pol.MaxFile, g.Pol.MaxTotal = 4096, 4096
		g.Pass("k1", "fits.bin") // control: 3000 B commit under a 4096 B cap
		g.Write("more.bin", strings.Repeat("d", 3000))
		f, ds := g.Attempt("k2", "more.bin")
		g.Pol.MaxTotal = 8192 // staged while room existed
		g.Stage("k2", f, ds)
		g.Finalizing("k2")
		g.Pol.MaxTotal = 4096 // the operator lowered the cap (or other attempts filled the store)
		_, err := records.CommitAndVerify(g.ctx, g.St, g.Blobs, "k2", 3, g.Pol)
		return g2Refuse(t, g, err, "k2", records.ReasonSizeCap)
	})
	register("C15-ART-01", func(t *testing.T) Observed { // an ended attempt stages a different blob under the name it already committed
		g := newG2World(t)
		g.Write("out.txt", "version one")
		first := g.Pass("v1", "out.txt")
		g.Write("out.txt", "version two") // attempt 2 legitimately writes the same name: a second record, not an overwrite
		second := g.Pass("v2", "out.txt")
		if first[0].ID == second[0].ID || first[0].SHA256 == second[0].SHA256 {
			t.Fatalf("per-attempt versions collapsed: %v %v", first[0], second[0])
		}
		list, err := g.St.ArtifactList(g.ctx, records.Filter{Name: "out.txt"})
		if err != nil || len(list) != 2 {
			t.Fatalf("both versions must be listed: %v %v", list, err)
		}
		evil := first[0]
		evil.SHA256, evil.Size, evil.State = second[0].SHA256, second[0].Size, records.StateStaged // swap v1's digest for v2's
		err = g.St.ArtifactStage(g.ctx, "v1", 2, []records.Record{evil})
		got := g2Refuse(t, g, err, "v1", records.ReasonAttempt)
		got.Landed -= int64(len("version one")) // the committed v1 bytes are the control, not the attack
		again, _, _ := g.St.ArtifactGet(g.ctx, first[0].ID)
		if again.SHA256 != first[0].SHA256 {
			t.Fatal("the committed record of attempt 1 changed")
		}
		return got
	})
	register("C15-ART-02", func(t *testing.T) Observed { // an ended attempt re-declares its outputs to collect a different file
		g := newG2World(t)
		g.Write("out.txt", "version one")
		g.Pass("w1", "out.txt")
		ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "evil.txt"}}}, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = g.St.ArtifactDeclare(g.ctx, "w1", 2, ds)
		got := g2Refuse(t, g, err, "w1", records.ReasonAttempt)
		if got.Landed != int64(len("version one")) {
			t.Fatalf("the committed bytes of attempt 1 changed: %d", got.Landed)
		}
		got.Landed = 0 // the committed v1 bytes are the control, not the attack
		return got
	})
}
