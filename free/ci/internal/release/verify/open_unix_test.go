//go:build unix

package verify

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The open refuses a leaf that is a symbolic link when it gets there, not only when an earlier Lstat
// looked (D-1198): this is the state a swap between the check and the open leaves behind.
func TestOpenCheckedRefusesASwappedLeaf(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symlinks here")
	}
	if f, err := openChecked(link, "link", false); err == nil {
		_ = f.Close()
		t.Fatal("a symbolic link leaf was opened")
	}
	f, err := openChecked(link, "link", true) // an operator trust file may be a link to a regular file
	if err != nil {
		t.Fatalf("a followed link to a regular file: %v", err)
	}
	_ = f.Close()
}

// A FIFO that is there at open time is refused after a non-blocking open, with or without following.
func TestOpenCheckedRefusesAFifoAtOpenTime(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skip("no fifo here")
	}
	for _, follow := range []bool{false, true} {
		done := make(chan error, 1)
		go func() {
			f, err := openChecked(p, "pipe", follow)
			if err == nil {
				_ = f.Close()
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("follow=%v: a fifo was opened as a regular file", follow)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("follow=%v: the open blocked on a fifo", follow)
		}
	}
}
