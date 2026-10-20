//go:build cachesec

package cachesec

// Purpose: TM-14 and TM-15 matrix cases of the local artifact store that production audits or
// refuses with a typed error: a size bomb stopped by the store caps (reserve and commit), the
// substitution of a committed artifact by an ENDED attempt (stage and declare, C15-ART-01/02), and by
// a LIVE attempt B that targets the record attempt A already committed (C15-ART-16..18). Per-attempt
// versions are the designed behaviour: B may write its own version of the same name (a second
// record); what it must not do is replace or claim A's record, and A's record metadata, blob bytes
// and `artifacts get` output stay unchanged. The other unaudited refusals (E683, digest, secret)
// are in g2_c14_test.go, g2_c15_test.go and g2_c16_test.go with `audit: none`.
// Inputs: g2World. Outputs: registered cases C14-ART-01..02, C15-ART-01..02, C15-ART-16..18.
// Constraints: production packages only; honest control first; Landed is the new reachable bytes
// since Mark() (new committed record bytes plus new blobs), never a hand-written length.

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
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
		g.Mark()
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
		g.Mark()
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
		g.Mark()
		evil := first[0]
		evil.SHA256, evil.Size, evil.State = second[0].SHA256, second[0].Size, records.StateStaged // swap v1's digest for v2's
		err = g.St.ArtifactStage(g.ctx, "v1", 2, []records.Record{evil})
		got := g2Refuse(t, g, err, "v1", records.ReasonAttempt)
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
		g.Mark()
		err = g.St.ArtifactDeclare(g.ctx, "w1", 2, ds)
		return g2Refuse(t, g, err, "w1", records.ReasonAttempt)
	})
	register("C15-ART-16", func(t *testing.T) Observed { // live B stages a record that carries A's committed record id
		g, v := g2Victim(t)
		f, ds, st := g2LiveB(t, g)
		recs := records.ToRecords("vb", f, 2, st, g.clockTime())
		if err := g.St.ArtifactStage(g.ctx, "vb", 2, recs[:1]); err != nil { // control: B's own, differently named record
			t.Fatalf("control stage: %v", err)
		}
		forged := recs[1] // name out.txt, B's digest, A's record id
		forged.ID = v.rec.ID
		g.Mark()
		err := g.St.ArtifactStage(g.ctx, "vb", 2, []records.Record{forged})
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		v.same(t)
		_ = ds
		return got
	})
	register("C15-ART-17", func(t *testing.T) Observed { // live B commits a list that holds A's committed record
		g, v := g2Victim(t)
		f, _, st := g2LiveB(t, g)
		recs := records.ToRecords("vb", f, 2, st, g.clockTime())
		if err := g.St.ArtifactStage(g.ctx, "vb", 2, recs); err != nil { // control: B stages both of its own records
			t.Fatalf("control stage: %v", err)
		}
		g.Finalizing("vb")
		staged := g2Held2(t, g, "vb")
		g.Mark()
		_, err := g.St.ArtifactCommit(g.ctx, "vb", 3, append(staged[:1:1], v.rec), g.Pol)
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		v.same(t)
		g2HonestCommit(t, g, v) // the control after the attack: B's own versions commit, A's stays
		return got
	})
	register("C15-ART-18", func(t *testing.T) Observed { // live B commits its record for the name with the digest of A's blob
		g, v := g2Victim(t)
		f, _, st := g2LiveB(t, g)
		if err := g.St.ArtifactStage(g.ctx, "vb", 2, records.ToRecords("vb", f, 2, st, g.clockTime())); err != nil {
			t.Fatalf("control stage: %v", err)
		}
		g.Finalizing("vb")
		staged := g2Held2(t, g, "vb")
		claim := staged[1]
		claim.SHA256, claim.Size = v.rec.SHA256, v.rec.Size
		g.Mark()
		_, err := g.St.ArtifactCommit(g.ctx, "vb", 3, []records.Record{staged[0], claim}, g.Pol)
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		v.same(t)
		g2HonestCommit(t, g, v)
		return got
	})
}

// g2VictimRec is attempt A's committed record and the check that it is untouched.
type g2VictimRec struct {
	g   *g2World
	rec records.Record
}

// g2Victim runs attempt A ("va"): it commits out.txt ("version one") and passes.
func g2Victim(t *testing.T) (*g2World, g2VictimRec) {
	t.Helper()
	g := newG2World(t)
	g.Write("out.txt", "version one")
	return g, g2VictimRec{g: g, rec: g.Pass("va", "out.txt")[0]}
}

// same fails unless A's record row, its blob bytes and the `artifacts get` output are what A committed.
func (v g2VictimRec) same(t *testing.T) {
	t.Helper()
	g := v.g
	now, ok, err := g.St.ArtifactGet(g.ctx, v.rec.ID)
	if err != nil || !ok || now.State != records.StateCommitted || now.Attempt != "va" || now.Name != "out.txt" || now.SHA256 != v.rec.SHA256 || now.Size != v.rec.Size {
		t.Fatalf("A's record changed: %+v ok=%v err=%v", now, ok, err)
	}
	if b, err := os.ReadFile(g.BlobPath(v.rec.SHA256)); err != nil || string(b) != "version one" {
		t.Fatalf("A's blob bytes changed: %q %v", b, err)
	}
	rd := records.Reader{Ledger: g.St, Blobs: g.Blobs}
	p, _, err := rd.Get(g.ctx, authz.Viewer{Project: g2Project, Role: authz.RoleMember}, v.rec.ID, "chk", t.TempDir())
	if b, rerr := os.ReadFile(p); err != nil || rerr != nil || string(b) != "version one" {
		t.Fatalf("get of A's record: %q %v %v", b, err, rerr)
	}
}

// g2LiveB starts live attempt "vb" (running, epoch 2) declaring own.txt and out.txt (the name A committed)
// and collects both: its version of out.txt has other bytes and another length than A's.
func g2LiveB(t *testing.T, g *g2World) (records.Facts, []records.Decl, []records.Staged) {
	t.Helper()
	g.Write("own.txt", "B own record")
	g.Write("out.txt", "version two by attempt B")
	f, ds := g.Attempt("vb", "own.txt", "out.txt")
	st, err := g.Collector().Collect(g.ctx, f.Project, g.Ws, ds)
	if err != nil || len(st) != 2 || st[0].Name != "own.txt" || st[1].Name != "out.txt" {
		t.Fatalf("collect B: %v %v", st, err)
	}
	return f, ds, st
}

// g2Held2 is B's two staged records in declaration order.
func g2Held2(t *testing.T, g *g2World, id string) []records.Record {
	t.Helper()
	held, err := g.St.ArtifactStaged(g.ctx, id)
	if err != nil || len(held) != 2 {
		t.Fatalf("staged of %s: %v %v", id, held, err)
	}
	return held
}

// g2HonestCommit commits B's own versions after the attack: both names are listed, A's record is unchanged.
func g2HonestCommit(t *testing.T, g *g2World, v g2VictimRec) {
	t.Helper()
	if _, err := g.St.ArtifactCommit(g.ctx, "vb", 3, g2Held2(t, g, "vb"), g.Pol); err != nil {
		t.Fatalf("B's honest commit after the refused attack: %v", err)
	}
	list, err := g.St.ArtifactList(g.ctx, records.Filter{Name: "out.txt"})
	if err != nil || len(list) != 2 || list[0].ID == list[1].ID {
		t.Fatalf("per-attempt versions of out.txt: %v %v", list, err)
	}
	v.same(t)
}
