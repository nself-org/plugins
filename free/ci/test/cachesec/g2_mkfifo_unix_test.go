//go:build cachesec && unix

package cachesec

// Purpose: makes a FIFO for the crafted-tree test of group g2 (unix only).

import "syscall"

func g2Mkfifo(path string) error { return syscall.Mkfifo(path, 0o644) }
