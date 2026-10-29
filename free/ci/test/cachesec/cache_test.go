//go:build cachesec

package cachesec

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (r *rig) cacheWrite(id string) error {
	f, target := owner(), owner()
	switch id {
	case "fork-main":
		f = fork("1")
	case "pr-a-pr-b":
		f, target = fork("1"), fork("2")
	}
	k := key(r.t, f)
	r.finalizing("writer", f, k)
	epoch := int64(3)
	if id == "ended-local" {
		must(r.t, r.st.Transition(r.ctx, "writer", "finalizing", "passed", store.TransitionDetail{Epoch: 3}))
	}
	if id == "epoch-local" {
		epoch = 2
	}
	if id == "field-boundary-collision" {
		a := entries.KeyInputs{Namespace: namespace(r.t, f), SpecID: "ab", SpecVersion: "c"}
		b := a
		b.SpecID, b.SpecVersion = "a", "bc"
		ka, err := entries.Key(a)
		must(r.t, err)
		kb, err := entries.Key(b)
		must(r.t, err)
		if ka == kb {
			r.t.Fatal("field boundaries collide")
		}
		// A valid key that was not planned must not alias the planned key.
		k = kb
	}
	// Missing blobs are intentional: authorization must run before blob checks.
	m := entries.Manifest{Namespace: namespace(r.t, target).Canonical(), Key: k, Spec: "go", Blobs: []entries.Blob{{Path: "data", SHA256: strings.Repeat("a", 64), Size: 1}}}
	err := r.entries.Commit(r.ctx, "writer", epoch, namespace(r.t, target), m)
	r.noEntries()
	if len(tree(r.t, filepath.Join(r.home, "blobs"))) != 0 {
		r.t.Error("unauthorized write left blob bytes")
	}
	return err
}

func (r *rig) restore(id string) error {
	f := owner()
	if id == "secret-snapshot" {
		f.SecretsDeclared = true
	}
	k := key(r.t, f)
	r.finalizing("writer", f, k)
	b := r.archive(id)
	m := r.manifest(f, k, b)
	must(r.t, r.entries.Commit(r.ctx, "writer", 3, namespace(r.t, f), m))
	must(r.t, r.st.Transition(r.ctx, "writer", "finalizing", "passed", store.TransitionDetail{Epoch: 3}))
	if id == "corrupt-local" || id == "partial-local" {
		d := m.Blobs[0].SHA256
		p := filepath.Join(r.home, "blobs", string(cacheRealm(m.Namespace)), "sha256", d[:2], d)
		changed := append([]byte(nil), b...)
		if id == "partial-local" {
			changed = changed[:len(changed)/2]
		} else {
			changed[512] ^= 1
		}
		must(r.t, os.WriteFile(p, changed, 0o600))
	}
	dest := r.t.TempDir()
	base, err := os.OpenRoot(dest)
	must(r.t, err)
	defer base.Close()
	access := authz.ModeRWOwn
	if id == "secret-snapshot" {
		access = authz.ModeROSnapshot
	}
	hit, n, err := r.engine.Restore(r.ctx, base, namespace(r.t, f), k, access, specs.Spec{ID: "go"}, specs.Dirs{"data": "restored"}, specs.UnpackOptions{})
	if hit || n != 0 {
		r.t.Error("crafted cache was restored")
	}
	if len(tree(r.t, dest)) != 0 {
		r.t.Error("refused restore left bytes")
	}
	if err == nil {
		r.note = "restore returned a miss (hit=false, 0 bytes restored), not a refusal"
	}
	return err
}

func (r *rig) archive(id string) []byte {
	h := &tar.Header{Name: "payload", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}
	content := []byte("x")
	switch id {
	case "restore-traversal":
		h.Name = "../escape"
	case "restore-absolute":
		h.Name = "/escape"
	case "restore-symlink":
		h.Typeflag, h.Size, h.Linkname = tar.TypeSymlink, 0, "../../escape"
		content = nil
	case "restore-hardlink":
		h.Typeflag, h.Size, h.Linkname = tar.TypeLink, 0, "../../escape"
		content = nil
	case "restore-fifo":
		h.Typeflag, h.Size = tar.TypeFifo, 0
		content = nil
	case "restore-device":
		h.Typeflag, h.Size = tar.TypeChar, 0
		content = nil
	case "restore-ratio":
		content = bytes.Repeat([]byte("x"), 16<<20)
		h.Size = int64(len(content))
	}
	var out bytes.Buffer
	w := tar.NewWriter(&out)
	must(r.t, w.WriteHeader(h))
	_, err := w.Write(content)
	must(r.t, err)
	must(r.t, w.Close())
	if id != "restore-ratio" {
		return out.Bytes()
	}
	var compressed bytes.Buffer
	g := gzip.NewWriter(&compressed)
	_, err = g.Write(out.Bytes())
	must(r.t, err)
	must(r.t, g.Close())
	return compressed.Bytes()
}
