//go:build linux

// Purpose: isatty for linux.
//
// Inputs: a file descriptor.
//
// Outputs: true when the descriptor is a terminal.
//
// Constraints: stdlib only. TCGETS succeeds only on a terminal; /dev/null,
// /dev/zero, /dev/urandom, pipes and regular files all fail it with ENOTTY.
package main

import (
	"syscall"
	"unsafe"
)

func isatty(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
