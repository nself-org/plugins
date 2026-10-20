//go:build cachesec

package cachesec

// Purpose: TM-15 cases C15-ART-03..15 on integrity: a blob corrupted, truncated, swapped or removed
// on disk after commit (records get, CommitAndVerify), a corrupt blob or wrong partial in the local
// cas (E710) and a corrupted, short, oversize or mis-announced stream into the blob transfer
// receiver (E714). Production refuses each and publishes nothing, but appends no audit row: records
// rows say `gap: D-1450`, cas and transfer rows `gap: D-1451`; each case asserts no audit row appeared.
// Inputs: g2World. Outputs: registered cases of matrix.d/g2.yaml.
// Constraints: production packages only; an honest control of the same operation runs first; Landed
// is new reachable blob or record bytes, plus bytes written under the get destination.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func g2Sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// g2GetTampered commits two files, gets one honestly, tampers with the other blob and gets that.
func g2GetTampered(mutate func(p string)) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		g.Write("ok.txt", "honest bytes")
		g.Write("evil.txt", "bytes the attacker will change")
		recs := g.Pass("bl", "ok.txt", "evil.txt")
		byName := map[string]records.Record{}
		for _, r := range recs {
			byName[r.Name] = r
		}
		rd := records.Reader{Ledger: g.St, Blobs: g.Blobs}
		v := authz.Viewer{Project: g2Project, Role: authz.RoleMember}
		cwd := t.TempDir()
		if p, _, err := rd.Get(g.ctx, v, byName["ok.txt"].ID, "ok", cwd); err != nil {
			t.Fatalf("control: %v", err)
		} else if b, _ := os.ReadFile(p); string(b) != "honest bytes" {
			t.Fatalf("control bytes %q", b)
		}
		mutate(g.BlobPath(byName["evil.txt"].SHA256))
		before := g2TreeBytes(cwd)
		g.Mark()
		_, _, err := rd.Get(g.ctx, v, byName["evil.txt"].ID, "evil", cwd)
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		if _, serr := os.Stat(filepath.Join(cwd, "evil")); serr == nil {
			t.Fatal("a failed get created its destination")
		}
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()+max(0, g2TreeBytes(cwd)-before)
		return got
	}
}

func g2Flip(p string) { b, _ := os.ReadFile(p); b[3] ^= 0x01; _ = os.WriteFile(p, b, 0o600) }

// g2CommitTampered stages a file, tampers with its blob and runs CommitAndVerify.
func g2CommitTampered(mutate func(p string)) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		g.Write("ctl.txt", "control")
		g.Pass("c1", "ctl.txt")
		g.Write("sub.txt", "staged bytes")
		f, ds := g.Attempt("s1", "sub.txt")
		st := g.Stage("s1", f, ds)
		mutate(g.BlobPath(st[0].SHA256))
		g.Finalizing("s1")
		g.Mark()
		_, err := records.CommitAndVerify(g.ctx, g.St, g.Blobs, "s1", 3, g.Pol)
		got := refusal(t, err, func(err error) string { return g2RecordsReason(t, err) })
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

func g2CasReason(err error) string {
	var e *cas.Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return "unknown"
}

// g2TransferCase opens a receiver, runs the honest transfer, then the attack through attack().
func g2TransferCase(attack func(t *testing.T, g *g2World, r *transfer.Receiver) error) func(*testing.T) Observed {
	return func(t *testing.T) Observed {
		g := newG2World(t)
		realm := cas.ArtifactRealm(g2Project)
		r := &transfer.Receiver{Store: g.Blobs, Realm: realm, Lease: "lease-g2", Limits: transfer.Limits{MaxFile: 4096}, Live: func(context.Context) error { return nil }}
		honest := bytes.Repeat([]byte("0123456789"), 100)
		w, err := r.Open(g.ctx, transport.BlobHeader{Stream: transfer.StreamArtifact, SHA256: g2Sum(honest), Size: int64(len(honest))})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(honest); err != nil || w.Close() != nil {
			t.Fatalf("control transfer: %v", err)
		}
		if _, err = g.Blobs.Stat(realm, g2Sum(honest)); err != nil {
			t.Fatalf("control blob not published: %v", err)
		}
		g.Mark()
		err = attack(t, g, r)
		got := refusal(t, err, func(err error) string {
			var e *transfer.Error
			if !errors.As(err, &e) {
				t.Fatalf("want E714, got %v", err)
			}
			return e.Reason
		})
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	}
}

func g2Announce(r *transfer.Receiver, g *g2World, announce []byte) (*transfer.Writer, error) {
	return r.Open(g.ctx, transport.BlobHeader{Stream: transfer.StreamArtifact, SHA256: g2Sum(announce), Size: int64(len(announce))})
}

func init() {
	register("C15-ART-03", g2GetTampered(g2Flip))
	register("C15-ART-04", g2GetTampered(func(p string) { b, _ := os.ReadFile(p); _ = os.WriteFile(p, b[:len(b)/2], 0o600) }))
	register("C15-ART-05", g2GetTampered(func(p string) { _ = os.WriteFile(p, []byte("a different, attacker chosen blob!"), 0o600) }))
	register("C15-ART-06", g2GetTampered(func(p string) { _ = os.Remove(p) }))
	register("C15-ART-07", g2CommitTampered(func(p string) { b, _ := os.ReadFile(p); b[0] ^= 0xff; _ = os.WriteFile(p, b, 0o600) }))
	register("C15-ART-08", g2CommitTampered(func(p string) { _ = os.Remove(p) }))
	register("C15-ART-09", func(t *testing.T) Observed { // a bit flipped in a cas blob at rest
		g := newG2World(t)
		realm := cas.ArtifactRealm(g2Project)
		sha, _, err := g.Blobs.Put(g.ctx, realm, bytes.NewReader([]byte("honest cas bytes")))
		if err != nil {
			t.Fatal(err)
		}
		drain := func(r io.Reader) error { _, e := io.Copy(io.Discard, r); return e }
		if err = g.Blobs.ReadVerified(realm, sha, drain); err != nil {
			t.Fatalf("control: %v", err)
		}
		g2Flip(g.BlobPath(sha))
		g.Mark()
		got := refusal(t, g.Blobs.ReadVerified(realm, sha, drain), g2CasReason)
		if _, serr := g.Blobs.Stat(realm, sha); serr == nil {
			t.Fatal("a corrupt blob stayed reachable after the mismatch")
		}
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	})
	register("C15-ART-10", func(t *testing.T) Observed { // a partial that holds other bytes than its announced digest
		g := newG2World(t)
		realm := cas.ArtifactRealm(g2Project)
		put := func(writer string, announced, sent []byte) error {
			p, err := g.Blobs.Partial(realm, g2Sum(announced), writer)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Write(sent); err != nil {
				t.Fatal(err)
			}
			return p.Finalize()
		}
		good := []byte("the announced bytes")
		if err := put("w1", good, good); err != nil {
			t.Fatalf("control: %v", err)
		}
		g.Mark()
		bad := []byte("another announce!!!")
		err := put("w2", bad, []byte("NOT the announced!!"))
		got := refusal(t, err, g2CasReason)
		if _, serr := g.Blobs.Stat(realm, g2Sum(bad)); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("a wrong partial was published: %v", serr)
		}
		got.Audit, got.Landed = g.AuditSince(), g.LandedSince()
		return got
	})
	register("C15-ART-11", g2TransferCase(func(t *testing.T, g *g2World, r *transfer.Receiver) error { // one flipped byte
		bad := bytes.Repeat([]byte("abcdefghij"), 100)
		flipped := append([]byte(nil), bad...)
		flipped[500] ^= 0x01
		w, err := g2Announce(r, g, bad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(flipped); err != nil {
			t.Fatal(err)
		}
		return w.Close()
	}))
	register("C15-ART-12", g2TransferCase(func(t *testing.T, g *g2World, r *transfer.Receiver) error { // the stream ends early
		short := bytes.Repeat([]byte("klmnopqrst"), 100)
		w, err := g2Announce(r, g, short)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(short[:600]); err != nil {
			t.Fatal(err)
		}
		return w.Close()
	}))
	register("C15-ART-13", g2TransferCase(func(t *testing.T, g *g2World, r *transfer.Receiver) error { // more bytes than announced
		long := bytes.Repeat([]byte("uvwxyzabcd"), 100)
		w, err := g2Announce(r, g, long)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(append(long, 'x'))
		return err
	}))
	register("C15-ART-14", g2TransferCase(func(t *testing.T, g *g2World, r *transfer.Receiver) error { // announce a held digest at another size
		held := bytes.Repeat([]byte("0123456789"), 100)
		_, err := r.Open(g.ctx, transport.BlobHeader{Stream: transfer.StreamArtifact, SHA256: g2Sum(held), Size: int64(len(held)) - 1})
		return err
	}))
	register("C15-ART-15", g2TransferCase(func(t *testing.T, g *g2World, r *transfer.Receiver) error { // announce above the per-file cap
		_, err := r.Open(g.ctx, transport.BlobHeader{Stream: transfer.StreamArtifact, SHA256: g2Sum([]byte("huge")), Size: 1 << 20})
		return err
	}))
}
