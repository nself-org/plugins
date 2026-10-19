//go:build unix

package records_test

import "syscall"

const fifoSupported = true

func mkfifo(path string) error { return syscall.Mkfifo(path, 0o644) }
