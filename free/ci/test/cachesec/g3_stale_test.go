//go:build cachesec

package cachesec

// Purpose: TM-13 on the remote carriers: a node writes a cache entry or an artifact after its lease epoch
// was fenced (expired lease) or after the attempt ended.
// Inputs: g3rig fixtures. Outputs: registered cases of matrix.d/g3.yaml.
// Constraints: production handlers only; each refusal has an honest control on a fresh world.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// g3Tgz is a valid gzip tar of the files, the shape of a cache blob.
func g3Tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, n := range []string{"a.txt", "b.txt"} {
		c, ok := files[n]
		if !ok {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(c))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func g3Run(t *testing.T, carrier string, r *g3rig, n *g3node, s *g3Sink) {
	t.Helper()
	var err error
	if carrier == "agent" {
		err = r.RunAgent(n, s)
	} else {
		err = r.RunSSH(n, s)
	}
	t.Logf("%s run: %v; sink errors: %v", carrier, err, s.Errs())
	t.Logf("audit actions: %v; node errs: %v", r.w.AuditActions(), n.errs)
}

// g3CacheControl commits an honest cache entry over the carrier on a fresh world (the positive control).
func g3CacheControl(t *testing.T, carrier string, tgz []byte) {
	t.Helper()
	w := newWorld(t)
	r := newG3Rig(t, w, "ctl1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
	n := &g3node{steps: []g3step{g3Send("cache", tgz), g3CacheRef(g3Key, [2]any{"mod", tgz})}}
	g3Run(t, carrier, r, n, r.Sink())
	if err := r.cache.Finalize(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("control (%s): honest cache save not admitted: %v", carrier, err)
	}
	if w.EntryBytes() == 0 {
		t.Fatalf("control (%s): honest cache save left no entry", carrier)
	}
}

// g3StaleCache: the node holds a fenced epoch when it uploads a cache blob and its ref.
func g3StaleCache(t *testing.T, carrier string) Observed {
	tgz := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC cache payload"})
	g3CacheControl(t, carrier, tgz)
	w := newWorld(t)
	r := newG3Rig(t, w, "zomb1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
	n := &g3node{steps: []g3step{g3Send("cache", tgz), g3CacheRef(g3Key, [2]any{"mod", tgz})}}
	s := r.Sink()
	s.onBlob = r.Expire // the lease lapses between the carrier routing the blob and the handler admitting it
	g3Run(t, carrier, r, n, s)
	var first error
	for _, e := range s.Errs() {
		if c, _ := g3Reasons(e); c == "E714" {
			first = e
			break
		}
	}
	got := g3Refused(t, first)
	got.Audit = w.AuditOf("cache.remote.refused")
	got.Landed = r.Landed()
	return got
}

func init() {
	register("C13-SSH-01", func(t *testing.T) Observed { return g3StaleCache(t, "ssh") })
	register("C13-AGT-01", func(t *testing.T) Observed { return g3StaleCache(t, "agent") })
}

// g3FleetCacheJob is a job that writes into both planned cache dirs of the "go" spec, then exits.
func g3FleetCacheJob() model.Job {
	return model.Job{Kind: "static", Run: []string{"sh", "-c", `echo started; echo SYNTHETIC_MOD_aaaa1111 > "$GOMODCACHE/m.txt"; echo SYNTHETIC_BUILD_bbbb2222 > "$GOCACHE/b.txt"`}}
}

func init() {
	// C13-SSH-02: the same stale cache write, but the node is a real sshd container running the real agent.
	g3RegisterFleet("C13-SSH-02", func(t *testing.T, fl *g3Fleet) Observed {
		cw := newWorld(t)
		cr := newG3Rig(t, cw, "fctl1", g3Main, g3PlanFull(g3NS(t, g3Main), "rw", g3Key), g3FleetCacheJob())
		cs := cr.Sink()
		err := fl.Run(t, cr, cs)
		if fe := cr.cache.Finalize(context.Background(), 5*time.Second); fe != nil || cw.EntryBytes() == 0 {
			t.Fatalf("control: honest cache save over the real fleet not admitted: run=%v finalize=%v errs=%v bytes=%d logs=%q", err, fe, cs.Errs(), cw.EntryBytes(), cs.Logs())
		}
		w := newWorld(t)
		r := newG3Rig(t, w, "fzomb1", g3Main, g3PlanFull(g3NS(t, g3Main), "rw", g3Key), g3FleetCacheJob())
		s := r.Sink()
		s.onBlob = r.Expire
		err = fl.Run(t, r, s)
		t.Logf("fleet run: %v; sink errors: %v; audit: %v", err, s.Errs(), w.AuditActions())
		got := g3FirstRefusal(t, s, "E714")
		got.Audit = w.AuditOf("cache.remote.refused")
		got.Landed = r.Landed()
		return got
	})
}

// g3FirstRefusal turns the first sink error with the given code into an Observed refusal.
func g3FirstRefusal(t *testing.T, s *g3Sink, code string) Observed {
	t.Helper()
	var first error
	for _, e := range s.Errs() {
		if c, _ := g3Reasons(e); c == code {
			first = e
			break
		}
	}
	return g3Refused(t, first)
}

// g3ArtJob declares one artifact; the scripted nodes deliver out/a.txt.
func g3ArtJob() model.Job {
	return model.Job{Kind: "build", Run: []string{"true"}, Artifacts: []model.ArtifactDecl{{Path: "out/a.txt", Name: "a"}}}
}

var g3ArtData = []byte(g3ArtifactBody)

// g3ArtifactControl... is on the fleet; this one is the in-process honest delivery.
func g3ArtControlLocal(t *testing.T, carrier string) {
	t.Helper()
	w := newWorld(t)
	r := newG3Rig(t, w, "actl1", g3Main, nil, g3ArtJob())
	n := &g3node{steps: []g3step{g3Send("artifact", g3ArtData), g3ArtRef("out/a.txt", g3ArtData)}}
	g3Run(t, carrier, r, n, r.Sink())
	if recs, err := r.CommitArtifacts(); err != nil || len(recs) != 1 {
		t.Fatalf("control (%s): honest artifact not committed: %v %d", carrier, err, len(recs))
	}
}

// g3StaleArtifact: the node holds a fenced epoch when it uploads an artifact blob and its ref.
func g3StaleArtifact(t *testing.T, carrier string) Observed {
	g3ArtControlLocal(t, carrier)
	w := newWorld(t)
	r := newG3Rig(t, w, "azomb1", g3Main, nil, g3ArtJob())
	n := &g3node{steps: []g3step{g3Send("artifact", g3ArtData), g3ArtRef("out/a.txt", g3ArtData)}}
	s := r.Sink()
	s.onBlob = r.Expire
	g3Run(t, carrier, r, n, s)
	got := g3FirstRefusal(t, s, "E714")
	got.Audit = w.AuditOf("artifact.remote.refused")
	got.Landed = r.Landed(g3Sum(g3ArtData))
	return got
}

func init() {
	register("C13-AGT-02", func(t *testing.T) Observed { return g3StaleArtifact(t, "agent") })
	register("C13-SSH-04", func(t *testing.T) Observed { return g3StaleArtifact(t, "ssh") })

	// C13-SSH-05: the attempt already ended (result recorded, entries finalised) when a zombie sends a late save.
	register("C13-SSH-05", func(t *testing.T) Observed {
		// control: before the end of the attempt the same upload and ref are admitted and commit
		cw := newWorld(t)
		cr := newG3Rig(t, cw, "ectl1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
		tgz := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC late payload"})
		cs := cr.Sink()
		if err := g3Upload(cw.ctx, cs, "cache", tgz); err != nil {
			t.Fatal(err)
		}
		if err := cs.CacheRef(cw.ctx, protocol.CacheRef{LeaseID: cr.leaseID, Spec: "go", Key: g3Key, Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: g3Sum(tgz), Size: int64(len(tgz))}}}); err != nil {
			t.Fatal(err)
		}
		if err := cs.Result(cw.ctx, protocol.Result{LeaseID: cr.leaseID, Epoch: uint64(cr.epoch)}); err != nil {
			t.Fatal(err)
		}
		if err := cr.cache.Finalize(cw.ctx, time.Second); err != nil || cw.EntryBytes() == 0 {
			t.Fatalf("control: honest save not committed: %v", err)
		}
		// attack: the attempt saved nothing, ended, and a zombie of the same lease now sends its save
		w := newWorld(t)
		r := newG3Rig(t, w, "eend1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
		s := r.Sink()
		if err := s.Result(w.ctx, protocol.Result{LeaseID: r.leaseID, Epoch: uint64(r.epoch)}); err != nil {
			t.Fatal(err)
		}
		if err := r.cache.Finalize(w.ctx, time.Second); err != nil {
			t.Logf("finalize: %v", err)
		}
		err := g3Upload(w.ctx, s, "cache", tgz)
		if err == nil {
			err = s.CacheRef(w.ctx, protocol.CacheRef{LeaseID: r.leaseID, Spec: "go", Key: g3Key, Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: g3Sum(tgz), Size: int64(len(tgz))}}})
		}
		got := g3Refused(t, err)
		got.Audit = w.AuditOf("cache.remote.refused")
		got.Landed = r.Landed()
		return got
	})
}
