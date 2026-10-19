package specs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func TestPackAndRemoveStayInsideThePinnedRoot(t *testing.T) {
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	putFile(t, outside+"/sub/secret.txt", "host file")
	if err := os.Chmod(outside+"/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(parent, "work")
	putFile(t, cacheRoot+"/go/mod/ok.txt", "x")
	base, err := os.OpenRoot(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	// the job replaces the spec dir with a symlink to a host directory
	if err := os.RemoveAll(cacheRoot + "/go/mod"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, cacheRoot+"/go/mod"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := Pack(context.Background(), base, "go/mod", &buf, cacheLimits()); err == nil || strings.Contains(buf.String(), "host file") {
		t.Fatalf("Pack followed a symlink out of the cache root (err %v)", err)
	}
	if _, err := Unpack(bytes.NewReader(tarGz(t, tent{name: "f", body: "x"})), base, "go/mod", cacheLimits(), UnpackOptions{}); err == nil {
		t.Fatal("Unpack followed a symlink out of the cache root")
	}
	_ = removeIn(base, "go/mod")
	if info, err := os.Stat(outside + "/sub"); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the outside directory was touched: %v %v", info, err)
	}
	if getFile(t, outside+"/sub/secret.txt") != "host file" {
		t.Fatal("the outside file changed")
	}
}

func TestPackDoesNotHangOnAFifoAndStopsWithItsContext(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(dir+"/swapped", 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	done := make(chan error, 1)
	go func() { done <- copyFile(context.Background(), root, "swapped", io.Discard, 1, nil) }() // the file was a regular file when it was listed
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("err %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copyFile blocked on a FIFO")
	}
	src := t.TempDir()
	putFile(t, src+"/a", "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base, _ := os.OpenRoot(filepath.Dir(src))
	defer func() { _ = base.Close() }()
	if _, err := Pack(ctx, base, filepath.Base(src), io.Discard, cacheLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("a canceled context must stop the walk: %v", err)
	}
}

func TestUnpackOtherReadMakesTheTreeReadableByAContainerUID(t *testing.T) {
	data := tarGz(t, tent{name: "d/", typ: 0x35, mode: 0o700}, tent{name: "d/secret-mode.txt", body: "x", mode: 0o600}, tent{name: "d/run.sh", body: "#!/bin/sh", mode: 0o700})
	dir := filepath.Join(t.TempDir(), "mount")
	if _, err := unpackTo(t, bytes.NewReader(data), dir, cacheLimits(), UnpackOptions{OtherRead: true}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o755, dir + "/d": 0o755, dir + "/d/secret-mode.txt": 0o644, dir + "/d/run.sh": 0o755} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v (%v), want %v", path, info, err, want)
		}
	}
	dir2 := filepath.Join(t.TempDir(), "private")
	if _, err := unpackTo(t, bytes.NewReader(data), dir2, cacheLimits(), UnpackOptions{}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir2 + "/d/secret-mode.txt"); info.Mode().Perm() != 0o600 {
		t.Errorf("without OtherRead the file keeps its mode, got %v", info.Mode().Perm())
	}
}

func TestScanWriterFindsTheLongestValueInItsWidestFormAcrossAWindowEdge(t *testing.T) {
	r := redact.New()
	value := bytes.Repeat([]byte("v"), redact.MaxValueLen) // no newline: one match must see it whole
	if err := r.Register(value); err != nil {
		t.Fatal(err)
	}
	escaped := bytes.Repeat([]byte(`\u0076`), redact.MaxValueLen) // every byte as a \uXXXX escape: 6 times the value
	for name, form := range map[string][]byte{"raw": value, "escaped": escaped} {
		for _, off := range []int{scanWindow - 100<<10, scanWindow - len(form)/2, scanWindow - len(form) + 10} {
			data := append(append(bytes.Repeat([]byte("."), off), form...), bytes.Repeat([]byte("."), 2*scanWindow)...)
			w := newScanWriter(io.Discard, r)
			_, err := w.Write(data)
			if err == nil {
				err = w.Close()
			}
			if !errors.Is(err, ErrSecretMatch) {
				t.Errorf("%s form at %d: err %v", name, off, err)
			}
		}
	}
}

var _ = authz.ModeNone

func TestRemoveNeverWalksThroughASymlink(t *testing.T) {
	parent := t.TempDir()
	live := filepath.Join(parent, "live-attempt")
	putFile(t, live+"/sub/f.txt", "x")
	if err := os.Chmod(live+"/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(live, filepath.Join(parent, "old-link")); err != nil { // a link inside the root to a live sibling
		t.Fatal(err)
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	if err := RemoveIn(base, "old-link"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(live + "/sub"); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the link target was chmodded: %v %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "old-link")); !os.IsNotExist(err) {
		t.Fatal("the link was not removed")
	}
	link := filepath.Join(t.TempDir(), "l")
	if err := os.Symlink(live, link); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTree(link); err != nil || getFile(t, live+"/sub/f.txt") != "x" {
		t.Fatalf("RemoveTree followed a top-level symlink: %v", err)
	}
}
