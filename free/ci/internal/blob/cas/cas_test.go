package cas

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func digestOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestThousandBlobsAndCorruption(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("namespace")
	count := 1000
	if os.Getenv("CAS_MUTATION") == "1" {
		count = 25
	}
	for i := 0; i < count; i++ {
		b := make([]byte, 128)
		if _, e := rand.Read(b); e != nil {
			t.Fatal(e)
		}
		d, n, e := s.Put(context.Background(), r, bytes.NewReader(b))
		if e != nil || n != int64(len(b)) {
			t.Fatalf("put: %v %d", e, n)
		}
		f, e := s.Open(r, d)
		if e != nil {
			t.Fatal(e)
		}
		got, e := io.ReadAll(f)
		if closeErr := f.Close(); e == nil {
			e = closeErr
		}
		if e != nil || !bytes.Equal(b, got) {
			t.Fatalf("roundtrip: %v", e)
		}
		if i == 0 {
			p, _ := s.blobPath(r, d)
			if e := os.WriteFile(p, bytes.Repeat([]byte{'x'}, len(b)), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Open(r, d); !errors.Is(e, ErrDigestMismatch) {
				t.Fatalf("mismatch: %v", e)
			}
			if _, e := s.Stat(r, d); !errors.Is(e, os.ErrNotExist) {
				t.Fatalf("corrupt still visible: %v", e)
			}
		}
	}
}

func TestRealmIsolationAndUsage(t *testing.T) {
	s := testStore(t)
	a, b := CacheRealm("a"), ArtifactRealm("b")
	d, _, e := s.Put(context.Background(), a, strings.NewReader("same"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Stat(b, d); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("cross realm: %v", e)
	}
	if _, _, e = s.Put(context.Background(), b, strings.NewReader("same")); e != nil {
		t.Fatal(e)
	}
	for _, r := range []Realm{a, b} {
		n, e := s.Usage(r)
		if e != nil || n != 4 {
			t.Fatalf("usage %s %d %v", r, n, e)
		}
	}
}

func TestPartialResumeConflict(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("partial")
	b := []byte("correct bytes")
	d := digestOf(b)
	p, e := s.Partial(r, d, "writer")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Write(b[:4]); e != nil {
		t.Fatal(e)
	}
	p, e = s.Partial(r, d, "writer")
	if e != nil || p.Offset() != 4 {
		t.Fatalf("resume: %v %d", e, p.Offset())
	}
	if e = p.Resume(3); !errors.Is(e, ErrPartialConflict) {
		t.Fatal(e)
	}
	if _, e = p.Write(b[4:]); e != nil {
		t.Fatal(e)
	}
	if e = p.Finalize(); e != nil {
		t.Fatal(e)
	}
	f, e := s.Open(r, d)
	if e != nil {
		t.Fatal(e)
	}
	got, e := io.ReadAll(f)
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, b) {
		t.Fatal("wrong bytes")
	}
	bad, e := s.Partial(r, digestOf([]byte("other")), "bad")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = bad.Write(b); e != nil {
		t.Fatal(e)
	}
	if e = bad.Finalize(); !errors.Is(e, ErrPartialConflict) {
		t.Fatalf("bad digest: %v", e)
	}
	if _, e = s.Stat(r, digestOf([]byte("other"))); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}

func TestSweepGraceAndKind(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	cache, art := CacheRealm("cache"), ArtifactRealm("art")
	old := []byte("old")
	fresh := []byte("fresh")
	artifact := []byte("artifact")
	for _, v := range []struct {
		r Realm
		b []byte
	}{{cache, old}, {cache, fresh}, {art, artifact}} {
		if _, _, e := s.Put(context.Background(), v.r, bytes.NewReader(v.b)); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []struct {
		r Realm
		b []byte
	}{{cache, old}, {art, artifact}} {
		p, _ := s.blobPath(v.r, digestOf(v.b))
		if e := os.Chtimes(p, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	freshPath, _ := s.blobPath(cache, digestOf(fresh))
	if e := os.Chtimes(freshPath, now.Add(-time.Minute), now.Add(-time.Minute)); e != nil {
		t.Fatal(e)
	}
	artPath, _ := s.blobPath(art, digestOf(artifact))
	before, _ := os.ReadFile(artPath)
	if e := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Stat(cache, digestOf(old)); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("old cache survived: %v", e)
	}
	if _, e := s.Stat(cache, digestOf(fresh)); e != nil {
		t.Fatalf("fresh cache deleted: %v", e)
	}
	after, e := os.ReadFile(artPath)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("artifact changed")
	}
}

func TestSweepExpiryAndIdempotentPut(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	r := CacheRealm("expiry")
	b := []byte("old but reused")
	d, _, e := s.Put(context.Background(), r, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	path, _ := s.blobPath(r, d)
	old := now.Add(-2 * time.Hour)
	if e = os.Chtimes(path, old, old); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Put(context.Background(), r, bytes.NewReader(b)); e != nil {
		t.Fatal(e)
	}
	if e = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Stat(r, d); e != nil {
		t.Fatalf("reused blob swept: %v", e)
	}
	p, e := s.Partial(r, digestOf([]byte("never finished")), "expired")
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{p.data, p.offsetPath} {
		if e = os.Chtimes(name, now.Add(-25*time.Hour), now.Add(-25*time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	corruptDir := filepath.Join(s.blobs(), "corrupt")
	if e = s.secureDir(corruptDir); e != nil {
		t.Fatal(e)
	}
	corrupt := filepath.Join(corruptDir, "expired")
	if e = os.WriteFile(corrupt, []byte("bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Chtimes(corrupt, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, now); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{p.data, p.offsetPath, corrupt} {
		if _, e = os.Stat(name); !errors.Is(e, os.ErrNotExist) {
			t.Fatalf("expired file remains %s: %v", name, e)
		}
	}
}

func TestInvalidInputsDoNotTouchFilesystem(t *testing.T) {
	home := filepath.Join(t.TempDir(), "absent")
	s, _ := New(home)
	originalLstat := lstat
	calls := 0
	lstat = func(path string) (os.FileInfo, error) {
		calls++
		return originalLstat(path)
	}
	t.Cleanup(func() { lstat = originalLstat })
	valid := strings.Repeat("a", 64)
	for _, r := range []Realm{"../x", "/abs", Realm("cache-" + strings.ToUpper(valid)), Realm("cache-" + valid[:63]), ""} {
		if _, e := s.Stat(r, valid); !errors.Is(e, ErrInvalid) {
			t.Fatalf("realm %q: %v", r, e)
		}
	}
	for _, d := range []string{"../x", "/abs", strings.ToUpper(valid), valid[:63], ""} {
		if _, e := s.Stat(CacheRealm("x"), d); !errors.Is(e, ErrInvalid) {
			t.Fatalf("digest %q: %v", d, e)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input caused %d filesystem calls", calls)
	}
	if _, e := s.Stat(CacheRealm("probe"), valid); !errors.Is(e, os.ErrNotExist) || calls == 0 {
		t.Fatalf("filesystem spy did not observe a valid lookup: %v, calls=%d", e, calls)
	}
	if _, e := os.Stat(home); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("filesystem touched: %v", e)
	}
}

func TestSymlinkRefused(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("links")
	d := digestOf([]byte("data"))
	p, _ := s.blobPath(r, d)
	if e := s.secureDir(filepath.Dir(p)); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("/etc/hosts", p); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Open(r, d); !errors.Is(e, ErrInvalid) {
		t.Fatalf("followed symlink: %v", e)
	}
}

func FuzzOpen(f *testing.F) {
	f.Add([]byte("abc"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 4096 {
			return
		}
		s := testStore(t)
		r := CacheRealm("fuzz")
		d, _, e := s.Put(context.Background(), r, bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		p, _ := s.blobPath(r, d)
		if len(b) > 0 {
			b[0] ^= 1
			if e = os.WriteFile(p, b, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Open(r, d); !errors.Is(e, ErrDigestMismatch) {
				t.Fatalf("accepted corruption: %v", e)
			}
		}
	})
}

func FuzzPartial(f *testing.F) {
	f.Add([]byte("abc"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 4096 {
			return
		}
		s := testStore(t)
		r := CacheRealm("fuzz")
		d := digestOf(b)
		p, e := s.Partial(r, d, "fuzzer")
		if e != nil {
			t.Fatal(e)
		}
		split := len(b) / 2
		if _, e = p.Write(b[:split]); e != nil {
			t.Fatal(e)
		}
		p, e = s.Partial(r, d, "fuzzer")
		if e != nil || p.Offset() != int64(split) {
			t.Fatal(e)
		}
		if _, e = p.Write(b[split:]); e != nil {
			t.Fatal(e)
		}
		if e = p.Finalize(); e != nil {
			t.Fatal(e)
		}
		out, e := s.Open(r, d)
		if e != nil {
			t.Fatal(e)
		}
		got, e := io.ReadAll(out)
		if closeErr := out.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(got, b) {
			t.Fatal("wrong data")
		}
		wrongDigest := digestOf(append(append([]byte(nil), b...), 0))
		bad, e := s.Partial(r, wrongDigest, "mismatch")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = bad.Write(b); e != nil {
			t.Fatal(e)
		}
		if e = bad.Finalize(); !errors.Is(e, ErrPartialConflict) {
			t.Fatalf("accepted mismatched bytes: %v", e)
		}
		if _, e = s.Stat(r, wrongDigest); !errors.Is(e, os.ErrNotExist) {
			t.Fatalf("published mismatched bytes: %v", e)
		}
	})
}
