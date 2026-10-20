package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func TestNodeReadThroughRehashAndRealm(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("synthetic blob")
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	fetches := 0
	fetch := func(_ context.Context, w io.Writer) error { fetches++; _, err := w.Write(data); return err }
	open := func(realm string) {
		t.Helper()
		f, err := c.Open(context.Background(), realm, digest, int64(len(data)), fetch)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		got, err := io.ReadAll(f)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("got=%q err=%v", got, err)
		}
	}
	open("realm-a")
	open("realm-a")
	if fetches != 1 {
		t.Fatalf("warm copy fetched: %d", fetches)
	}
	name, _ := c.name("realm-a", digest)
	if err := os.WriteFile(name, bytes.Repeat([]byte("x"), len(data)), 0600); err != nil {
		t.Fatal(err)
	}
	open("realm-a")
	if fetches != 2 {
		t.Fatal("corrupted copy was served")
	}
	open("realm-b")
	if fetches != 3 {
		t.Fatal("copy crossed realms")
	}
	bad := fmt.Sprintf("%x", sha256.Sum256([]byte("different")))
	if _, err := c.Open(context.Background(), "realm-a", bad, int64(len(data)), fetch); err == nil {
		t.Fatal("corrupt transit accepted")
	}
	name, _ = c.name("realm-a", bad)
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("corrupt transit published", err)
	}
}

func TestNodeReadThroughLRU(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, data := range []string{"a", "b", "c"} {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
		f, err := c.Open(context.Background(), "realm", digest, 1, func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, data); return err })
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		name, _ := c.name("realm", digest)
		at := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(name, at, at); err != nil {
			t.Fatal(err)
		}
	}
	// Lease-start pruning must also handle a cap reduced since the last lease.
	c.max = 2
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	a := fmt.Sprintf("%x", sha256.Sum256([]byte("a")))
	if _, err := c.Open(context.Background(), "realm", a, 1, nil); !os.IsNotExist(err) {
		t.Fatal("oldest copy survived", err)
	}
	for _, data := range []string{"b", "c"} {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
		f, err := c.Open(context.Background(), "realm", digest, 1, nil)
		if err != nil {
			t.Fatal("newer copy evicted", err)
		}
		_ = f.Close()
	}
}

func nodeSaveLease() protocol.Lease {
	ns := authz.Namespace{Project: "project", Trust: "owner", Protection: authz.Protected}
	return protocol.Lease{LeaseID: "lease-node", AttemptID: "attempt-node", Epoch: 3, Cache: &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}}}
}

func TestNodeStageSecretSendsNothing(t *testing.T) {
	l := nodeSaveLease()
	s, err := NewNodeStage(filepath.Join(t.TempDir(), "stage"), l)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	dir := t.TempDir()
	for _, d := range []string{"go/mod", "go/build"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go/mod", "clean"), []byte("ordinary synthetic data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go/build", "planted"), []byte("synthetic-registered-value"), 0600); err != nil {
		t.Fatal(err)
	}
	base, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	r := redact.New()
	if err := r.Register([]byte("synthetic-registered-value")); err != nil {
		t.Fatal(err)
	}
	if err := s.Collect(context.Background(), base, r); err == nil || !strings.Contains(err.Error(), "secret_match") {
		t.Fatal("secret not refused", err)
	}
	u := &nodeUploader{}
	if err := s.Ship(context.Background(), nil, u); err != nil {
		t.Fatal(err)
	}
	if u.frames != 0 {
		t.Fatalf("secret refusal sent %d frames", u.frames)
	}
}

type nodeUploader struct {
	frames  int
	offsets chan protocol.Offset
}

func (n *nodeUploader) Digest(d protocol.Digest) error {
	n.frames++
	if n.offsets != nil {
		n.offsets <- protocol.Offset{Stream: d.Stream, SHA256: d.SHA256}
	}
	return nil
}
func (n *nodeUploader) Chunk(protocol.Chunk) error       { n.frames++; return nil }
func (n *nodeUploader) CacheRef(protocol.CacheRef) error { n.frames++; return nil }

func TestNodeStageReadonlyRefused(t *testing.T) {
	l := nodeSaveLease()
	l.Cache.Access = "ro"
	if _, err := NewNodeStage(filepath.Join(t.TempDir(), "stage"), l); err == nil {
		t.Fatal("read-only save stage accepted")
	}
}

func TestNodeStagePublishedCopySurvivesStageClose(t *testing.T) {
	l := nodeSaveLease()
	s, err := NewNodeStage(filepath.Join(t.TempDir(), "stage"), l)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	s.Copies, err = NewReadThrough(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, d := range []string{"go/mod", "go/build"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "payload"), []byte("synthetic cache data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	base, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	if err := s.Collect(context.Background(), base, redact.New()); err != nil {
		t.Fatal(err)
	}
	u := &nodeUploader{offsets: make(chan protocol.Offset, 1)}
	if err := s.Ship(context.Background(), u.offsets, u); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, ref := range s.refs {
		for _, b := range ref.Blobs {
			f, err := s.Copies.Open(context.Background(), l.Cache.Namespace, b.SHA256, b.Size, nil)
			if err != nil {
				t.Fatal("advertised local copy missing after stage cleanup", err)
			}
			out, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			_, err = specs.Unpack(f, out, "restored", authz.DefaultLimits(), specs.UnpackOptions{})
			_ = f.Close()
			_ = out.Close()
			if err != nil {
				t.Fatal("published compression cannot restore", err)
			}
		}
	}
}

func TestNodeStageCombinedCopiesExceedCapStillShips(t *testing.T) {
	l := nodeSaveLease()
	s, err := NewNodeStage(filepath.Join(t.TempDir(), "stage"), l)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	s.Copies, err = NewReadThrough(t.TempDir(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, data := range []string{"aa", "bb"} {
		digest, size, err := s.blobs.Put(context.Background(), s.realm, strings.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		s.refs = append(s.refs, protocol.CacheRef{LeaseID: l.LeaseID, Spec: fmt.Sprintf("spec-%d", i), Key: strings.Repeat(fmt.Sprint(i), 64), Blobs: []protocol.CacheBlob{{Path: "data", SHA256: digest, Size: size}}})
	}
	u := &nodeUploader{offsets: make(chan protocol.Offset, 1)}
	if err := s.Ship(context.Background(), u.offsets, u); err != nil {
		t.Fatal("local copy cap blocked shared upload", err)
	}
	if u.frames != 6 {
		t.Fatalf("shared upload sent %d frames, want two digests, chunks, refs", u.frames)
	}
	for _, ref := range s.refs {
		for _, b := range ref.Blobs {
			name, err := s.Copies.name(l.Cache.Namespace, b.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(name); !os.IsNotExist(err) {
				t.Fatal("copy published before aggregate cap check", err)
			}
		}
	}
}
