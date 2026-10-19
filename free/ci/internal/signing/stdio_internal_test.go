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
