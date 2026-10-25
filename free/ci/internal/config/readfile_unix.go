//go:build unix

package config

import (
	"os"
	"syscall"
)

// openNonBlocking opens path read-only without blocking on a FIFO and without
// taking a terminal as the controlling one.
func openNonBlocking(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
}
