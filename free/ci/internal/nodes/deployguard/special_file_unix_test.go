//go:build unix

package deployguard

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// D-1269: a config or identity file that is not a regular file (a FIFO here) must be refused at once,
// never read: os.ReadFile on a FIFO blocks until a writer arrives and then reads without a bound.
func TestNonRegularFilesAreRefusedWithoutBlocking(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "config.yaml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no fifo support: %v", err)
	}
	t.Cleanup(func() { // free a loader that is stuck on open so a failing run does not leak it
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
	})
	cases := map[string]func() error{
		"never-list config": func() error { _, err := LoadNeverList(fifo); return err },
		"identity file":     func() error { _, err := readSmall(fifo); return err },
	}
	for name, load := range cases {
		done := make(chan error, 1)
		go func() { done <- load() }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: a FIFO was accepted", name)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("%s: the loader blocked on a FIFO", name)
		}
	}
}

// A file that grows past the cap after the size check must still be cut off by the read bound.
func TestReadSmallRefusesAFileOverTheCap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(p, make([]byte, 1<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSmall(p); err == nil {
		t.Fatal("over the cap accepted")
	}
}
