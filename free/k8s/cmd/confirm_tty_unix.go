//go:build linux || darwin

// Purpose: the terminal test behind stdinIsTTY on Linux and macOS.
//
// Inputs: a file descriptor.
//
// Outputs: true only when the descriptor answers the read-termios ioctl,
// which a terminal does and /dev/null, /dev/zero, files and pipes do not.
//
// Constraints: stdlib syscall only (the module vendors no terminal package);
// the request number is per OS (confirm_tty_linux.go, confirm_tty_darwin.go).
package main

import (
	"syscall"
	"unsafe"
)

// isTerminal reports whether fd is a terminal.
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlReadTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
