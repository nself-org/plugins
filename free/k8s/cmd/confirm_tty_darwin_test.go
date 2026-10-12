// Purpose: open the terminal side of a macOS pty for the isTerminal positive
// control (grantpt, unlockpt and ptsname by ioctl, stdlib only).
package main

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// openPTYSlave returns the terminal side of the pty whose master is m.
func openPTYSlave(m *os.File) (*os.File, error) {
	for _, req := range []uintptr{syscall.TIOCPTYGRANT, syscall.TIOCPTYUNLK} {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), req, 0); e != 0 {
			return nil, e
		}
	}
	var name [128]byte
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); e != 0 {
		return nil, e
	}
	path := string(name[:bytes.IndexByte(name[:], 0)])
	return os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
}
