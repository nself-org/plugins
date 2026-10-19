package specs

// Purpose: a blob kept in the local store is a plain tar, so a restore is a copy
// and not a gunzip (D-1008 round 3); a gzip blob (older saves, or one a remote
// store hands back) still restores.

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

func TestLocalBlobIsAPlainTarAndRestores(t *testing.T) {
	e := newEnv(t)
	src := t.TempDir()
	putFile(t, filepath.Join(src, "d", "a.txt"), "alpha alpha alpha alpha")
	base, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	realm := cas.CacheRealm("ns")
	digest, _, _, err := e.eng.putDir(e.ctx, base, realm, "d", 1<<20, false)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := e.blobs.Open(realm, digest)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 1 && raw[0] == 0x1f && raw[1] == 0x8b {
		t.Fatal("the local blob is gzip; a restore would pay a decompression")
	}
	if _, err := tar.NewReader(bytes.NewReader(raw)).Next(); err != nil {
		t.Fatalf("the local blob is not a tar: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	if _, err := unpackTo(t, bytes.NewReader(raw), dst, cacheLimits(), UnpackOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := getFile(t, filepath.Join(dst, "a.txt")); got != "alpha alpha alpha alpha" {
		t.Fatalf("restored %q", got)
	}
}

func TestPlainTarObeysTheSanitiserLimits(t *testing.T) {
	lim := authz.DefaultLimits()
	lim.MaxTotalBytes = 1 << 20
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := bytes.Repeat([]byte("ab"), 200<<10)
	for _, n := range []string{"a", "b", "c"} {
		_ = tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
		_, _ = tw.Write(body)
	}
	_ = tw.Close()
	_, err := unpackTo(t, bytes.NewReader(buf.Bytes()), t.TempDir()+"/d", lim, UnpackOptions{})
	var pe *authz.PathError
	if !errors.As(err, &pe) || pe.Reason != authz.SizeTotal {
		t.Fatalf("err %v", err)
	}
}

// Small files of a restore are written by several goroutines at once (D-1008
// round 3); the contents, modes and times match what a sequential restore wrote.
func TestUnpackWritesSmallFilesConcurrently(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	afterFileStart = func() {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
	}
	t.Cleanup(func() { afterFileStart = nil })
	var ents []tent
	for i := range 40 {
		ents = append(ents, tent{name: fmt.Sprintf("d%d/f%d", i%5, i), body: strings.Repeat("x", i+1)})
	}
	dst := filepath.Join(t.TempDir(), "out")
	if _, err := unpackTo(t, bytes.NewReader(tarGz(t, ents...)), dst, cacheLimits(), UnpackOptions{}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOMAXPROCS(0) > 1 && peak < 2 {
		t.Fatalf("files were written one at a time (peak %d)", peak)
	}
	for i := range 40 {
		if got := getFile(t, filepath.Join(dst, fmt.Sprintf("d%d/f%d", i%5, i))); got != strings.Repeat("x", i+1) {
			t.Fatalf("f%d: %q", i, got)
		}
	}
}

// A write that fails stops the restore with that error and leaves no writer running.
func TestUnpackStopsOnAWriteError(t *testing.T) {
	var ents []tent
	for i := range 30 {
		ents = append(ents, tent{name: fmt.Sprintf("f%d", i), body: "x"})
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(dst, "f7"), "already here") // O_EXCL refuses it
	if _, err := unpackTo(t, bytes.NewReader(tarGz(t, ents...)), dst, cacheLimits(), UnpackOptions{}); err == nil {
		t.Fatal("a colliding file was overwritten")
	}
}

// A restored tree is deleted on several goroutines, read-only directories included.
func TestRemoveInFansOutAndRemovesEverything(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	afterRemoveStart = func() {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
	}
	t.Cleanup(func() { afterRemoveStart = nil })
	parent := t.TempDir()
	for i := range 32 {
		putFile(t, filepath.Join(parent, "tree", "build", fmt.Sprintf("%02x", i), "f"), "x")
	}
	if err := os.Chmod(filepath.Join(parent, "tree", "build", "05"), 0o500); err != nil { // read-only dir, like a module cache
		t.Fatal(err)
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	if err := removeIn(base, "tree"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "tree")); !os.IsNotExist(err) {
		t.Fatalf("tree not removed: %v", err)
	}
	if runtime.GOMAXPROCS(0) > 1 && peak < 2 {
		t.Fatalf("subtrees were removed one at a time (peak %d)", peak)
	}
}

// The store verifies a blob while Unpack reads it, so a blob whose bytes changed
// but still parses as a tar is refused at the end of the stream and nothing it
// held is left in the job's cache directories.
func TestRestoreRefusesABlobWhoseBytesChangedAndLeavesNothing(t *testing.T) {
	e := newEnv(t)
	saved(t, e, "t1", ownerFacts).Cleanup()
	var flipped int
	_ = filepath.WalkDir(filepath.Join(e.home, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		if i := bytes.Index(b, []byte("compiled")); i >= 0 { // a file body inside a tar: the archive still parses
			b[i] = 'X'
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
			flipped++
		}
		return nil
	})
	if flipped != 1 {
		t.Fatalf("flipped %d blobs, want 1", flipped)
	}
	e.running("t2", ownerFacts)
	s := e.begin(e.input("t2", ownerFacts))
	if got := strings.Join(resultsOf(e.uses("t2")), ","); got != "go:refused,go:miss" {
		t.Fatalf("restore of a changed blob: %v", got)
	}
	for _, d := range []string{"build", "mod"} {
		if ents, _ := os.ReadDir(s.dir(d)); len(ents) != 0 {
			t.Fatalf("%s holds %d entries after a refused restore", d, len(ents))
		}
	}
}
