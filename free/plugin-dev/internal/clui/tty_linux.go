//go:build linux

package clui

import (
	"syscall"
	"unsafe"
)

// isTerminalFd reports whether fd is a terminal (TCGETS ioctl, as x/term does).
func isTerminalFd(fd uintptr) bool {
	var t syscall.Termios
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t)))
	return e == 0
}
