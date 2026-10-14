//go:build unix

package config

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO at the config path must be refused without opening it (open would block).
func TestReadRegularRefusesFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "ci.yaml")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readRegular(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO read succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readRegular blocked on a FIFO")
	}
}
