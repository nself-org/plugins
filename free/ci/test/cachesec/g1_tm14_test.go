//go:build cachesec

package cachesec

// Purpose: TM-14 cases on the local carrier: restore of a crafted cache entry through
// specs.Engine.Restore. The crafted blob is committed through a normal passing attempt (a commit
// checks the manifest, not the archive: the tampered-disk or hostile-agent case) and then restored
// into a pinned job root. Production must refuse with E683 (authz.PathError), write an audit row
// (cache.restore.refused), drop the entry and leave the filesystem untouched.
// Inputs: a real store, cas and specs.Engine. Outputs: registered cases C14-CAC-01..09.
// Constraints: production packages only. Every case first restores an honest tar of the same
// fixture into a separate job root (positive control), then the crafted tar. Landed is read from
// the filesystem: bytes of every regular file (and one per other node) under the scratch parent
// except the control root and the sentinel file, plus the entry bytes still reachable.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const g1Sentinel = "SYNTHETIC_SENTINEL_outside_the_job_root"

type g1Ent struct {
	name, link string
	typ        byte
	mode       int64
	body       string
}

// g1Tar builds a plain tar of the entries.
func g1Tar(t *testing.T, ents ...g1Ent) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.typ, Mode: e.mode}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// g1Gzip compresses b at the best ratio.
func g1Gzip(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := gz.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// g1Restore commits blobs (honest, crafted) as two entries of one passing PR attempt, settles it
// passed and restores the honest one into parent/control, then the crafted one into parent/job.
func g1Restore(t *testing.T, crafted []byte) Observed {
	t.Helper()
	w := newWorld(t)
	parent := t.TempDir()
	for _, d := range []string{"outside", "control", "job"} {
		if err := os.MkdirAll(filepath.Join(parent, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	victim := filepath.Join(parent, "outside", "victim.txt")
	if err := os.WriteFile(victim, []byte(g1Sentinel), 0o600); err != nil {
		t.Fatal(err)
	}

	f, ns := g1PR1, w.NS(g1PR1)
	kOK, kBad := w.Key(f, "honest"), w.Key(f, "crafted")
	fin := w.Attempt("t1", f, []string{kOK, kBad}, true)
	put := func(key string, blob []byte) {
		digest, size, err := w.Blobs.Put(w.ctx, cas.CacheRealm(string(ns.Canonical())), bytes.NewReader(blob))
		if err != nil {
			t.Fatal(err)
		}
		m := entries.Manifest{Namespace: ns.Canonical(), Key: key, Spec: "go", Blobs: []entries.Blob{{Path: "mod", SHA256: digest, Size: size}}}
		if err = w.Es.Commit(w.ctx, "t1", fin, ns, m); err != nil {
			t.Fatalf("commit of %s: %v", key[:8], err)
		}
	}
	put(kOK, g1Tar(t, g1Ent{name: "pkg", typ: tar.TypeDir, mode: 0o755}, g1Ent{name: "pkg/ok.txt", body: "honest cache bytes"}))
	put(kBad, crafted)
	w.g1Transition("t1", "finalizing", "passed", fin)

	eng := &specs.Engine{Entries: w.Es, Blobs: w.Blobs, Scanner: redact.New(), Backend: w.St}
	spec, dirs := specs.Spec{ID: "go"}, specs.Dirs{"mod": "mod"}
	restore := func(root, key string) (bool, error) {
		base, err := os.OpenRoot(filepath.Join(parent, root))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = base.Close() }()
		hit, _, err := eng.Restore(w.ctx, base, ns, key, authz.ModeRWOwn, spec, dirs, specs.UnpackOptions{})
		return hit, err
	}
	if hit, err := restore("control", kOK); err != nil || !hit { // positive control: the honest tar restores
		t.Fatalf("control: honest entry hit=%v err=%v", hit, err)
	}
	if b, err := os.ReadFile(filepath.Join(parent, "control", "mod", "pkg", "ok.txt")); err != nil || string(b) != "honest cache bytes" {
		t.Fatalf("control: honest file not restored: %q %v", b, err)
	}

	hit, err := restore("job", kBad)
	if hit {
		t.Fatal("the crafted entry restored")
	}
	got := g1PathRefused(t, err) // fails the case when err is nil
	got.Audit = w.AuditOf("cache.restore.refused")
	rows := w.Audit("cache.restore.refused")
	if len(rows) != 1 || !strings.Contains(rows[0].Detail, "reason="+string(got.Reason)) && !strings.Contains(rows[0].Detail, "reason="+strings.Replace(got.Reason, "_", ".", 1)) {
		t.Fatalf("restore audit rows %+v do not name the sanitiser reason %s", rows, got.Reason)
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != g1Sentinel {
		t.Fatalf("the sentinel outside the job root changed: %q %v", b, err)
	}
	if h, _, _ := w.Es.Lookup(w.ctx, ns, kBad, authz.ModeRWOwn); len(h.Blobs) != 0 {
		t.Fatal("the refused entry is still served")
	}
	got.Landed = g1Walk(t, parent)
	return got
}

// g1Walk counts what the crafted restore left under parent, ignoring the control root and the sentinel.
func g1Walk(t *testing.T, parent string) (n int64) {
	t.Helper()
	err := filepath.WalkDir(parent, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(parent, p)
		switch {
		case rel == "control" || strings.HasPrefix(rel, "control"+string(filepath.Separator)):
			return filepath.SkipDir
		case rel == filepath.Join("outside", "victim.txt") || d.IsDir():
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if info.Mode().IsRegular() {
			n += info.Size()
		} else {
			n++
		}
		t.Logf("left behind: %s (%s)", rel, info.Mode())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func init() {
	reg := func(id string, mk func(t *testing.T) []byte) {
		register(id, func(t *testing.T) Observed { return g1Restore(t, mk(t)) })
	}
	reg("C14-CAC-01", func(t *testing.T) []byte { // traversal
		return g1Tar(t, g1Ent{name: "../../outside/evil.txt", body: "SYNTHETIC_EVIL"})
	})
	reg("C14-CAC-02", func(t *testing.T) []byte { // absolute path
		return g1Tar(t, g1Ent{name: "/outside/evil.txt", body: "SYNTHETIC_EVIL"})
	})
	reg("C14-CAC-03", func(t *testing.T) []byte { // symlink out of the tree
		return g1Tar(t, g1Ent{name: "ln", typ: tar.TypeSymlink, link: "../../outside"})
	})
	reg("C14-CAC-04", func(t *testing.T) []byte { // write through an earlier in-tree symlink
		return g1Tar(t, g1Ent{name: "sub", typ: tar.TypeDir, mode: 0o755}, g1Ent{name: "ln", typ: tar.TypeSymlink, link: "sub"}, g1Ent{name: "ln/evil.txt", body: "SYNTHETIC_EVIL"})
	})
	reg("C14-CAC-05", func(t *testing.T) []byte { // hardlink to a file outside
		return g1Tar(t, g1Ent{name: "h", typ: tar.TypeLink, link: "../../outside/victim.txt"})
	})
	reg("C14-CAC-06", func(t *testing.T) []byte { return g1Tar(t, g1Ent{name: "pipe", typ: tar.TypeFifo}) })
	reg("C14-CAC-07", func(t *testing.T) []byte { return g1Tar(t, g1Ent{name: "null", typ: tar.TypeChar}) })
	reg("C14-CAC-08", func(t *testing.T) []byte { // ratio bomb: 4 MiB of zeros in a few KiB of gzip
		return g1Gzip(t, g1Tar(t, g1Ent{name: "bomb.bin", body: string(make([]byte, 4<<20))}))
	})
	reg("C14-CAC-09", func(t *testing.T) []byte { // set-id bit
		return g1Tar(t, g1Ent{name: "su", mode: 0o4755, body: "x"})
	})
}
