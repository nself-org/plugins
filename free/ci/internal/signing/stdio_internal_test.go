//go:build unix

package signing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCaptureKeepsAtMostMaxOutput(t *testing.T) {
	var c capture
	c.write(make([]byte, maxOutput-1))
	c.write([]byte("ab")) // one byte of room: only "a" is kept
	if len(c.buf) != maxOutput || c.buf[maxOutput-1] != 'a' {
		t.Fatalf("%d bytes", len(c.buf))
	}
	c.write([]byte("c")) // exactly full: nothing more
	if len(c.buf) != maxOutput {
		t.Fatalf("%d bytes after a full buffer", len(c.buf))
	}
	var d capture
	d.write([]byte("hello"))
	d.write([]byte(" world"))
	if string(d.buf) != "hello world" {
		t.Fatalf("%q", d.buf)
	}
}

func TestReleaseOpensTheReadEndOnce(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "f")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if !release(fifo) {
		t.Fatal("a FIFO that exists was not opened")
	}
	if release(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("a missing FIFO was reported opened")
	}
}

func TestMkfifoFailure(t *testing.T) {
	old := mkfifo
	mkfifo = func(string, uint32) error { return errors.New("no fifo") }
	defer func() { mkfifo = old }()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	_, code, err := (&FunnelRunner{Home: t.TempDir()}).RunStdio(context.Background(), StdioCall{Argv: []string{"cat"}, Stdin: []byte("x"), Timeout: 10 * time.Second})
	if !IsReason(err, ReasonSignerFailed) || code != -1 || !strings.Contains(err.Error(), "pipe") {
		t.Fatalf("%d %v", code, err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("the private directory was left behind: %v", left)
	}
}

// A writer that reaches its open late (after the first release) is still freed.
func TestFreeWriterFreesALateWriter(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "f")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(150 * time.Millisecond) // later than the first 10 ms release
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_, _ = w.Write([]byte("x"))
			_ = w.Close()
		}
	}()
	finished := make(chan struct{})
	go func() { freeWriter(fifo, done); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("freeWriter did not return")
	}
	select {
	case <-done:
	default:
		t.Fatal("freeWriter returned before the writer finished")
	}
}

// A writer that never finishes does not hold RunStdio longer than the bound.
func TestFreeWriterGivesUp(t *testing.T) {
	start := time.Now()
	freeWriter(filepath.Join(t.TempDir(), "missing"), make(chan struct{}))
	if d := time.Since(start); d < 4*time.Second || d > 20*time.Second {
		t.Fatalf("gave up after %v", d)
	}
}

func TestExecFailsBeforeTheWriterOpens(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = (&FunnelRunner{Home: "/nonexistent/home"}).RunStdio(context.Background(), StdioCall{Argv: []string{"cat"}, Stdin: []byte("x"), Timeout: time.Second})
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("RunStdio hung when the funnel failed before the child opened its stdin")
	}
}

func TestCLIStoreLockSerializes(t *testing.T) {
	dir := t.TempDir()
	a, b := &CLIStore{LockDir: dir}, &CLIStore{LockDir: dir}
	unlock, err := a.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := b.Lock(ctx); !IsReason(err, ReasonSignerFailed) {
		t.Fatalf("a second holder got the lock: %v", err)
	}
	got := make(chan struct{})
	go func() {
		u, err := b.Lock(context.Background())
		if err == nil {
			u()
		}
		close(got)
	}()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-got:
		t.Fatal("the second holder did not wait")
	default:
	}
	unlock()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the second holder never got the lock after release")
	}
	if fi, err := os.Stat(filepath.Join(dir, "release-key.lock")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("lock file: %v %v", fi, err)
	}
	// no LockDir: no lock, nothing created
	u, err := (&CLIStore{}).Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u()
	if _, err := (&CLIStore{LockDir: filepath.Join(dir, "f", "x")}).Lock(context.Background()); err != nil {
		t.Fatalf("a lock directory that can be created: %v", err)
	}
	file := filepath.Join(dir, "plainfile")
	_ = os.WriteFile(file, nil, 0o600)
	if _, err := (&CLIStore{LockDir: filepath.Join(file, "sub")}).Lock(context.Background()); !IsReason(err, ReasonSignerFailed) {
		t.Fatalf("an impossible lock directory: %v", err)
	}
}
