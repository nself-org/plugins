//go:build unix

package verify

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO where a listed file should be must be refused, not opened (an open
// would block the verifier for ever).
func TestOpenRefusesAFifoWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644); err != nil {
		t.Skip("no fifo here")
	}
	done := make(chan error, 1)
	go func() { _, err := digest(dir, "pipe"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a fifo was hashed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("open blocked on a fifo")
	}
}
