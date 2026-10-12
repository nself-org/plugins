// Purpose: open the terminal side of a Linux pty for the isTerminal positive
// control (unlockpt and ptsname by ioctl, stdlib only).
package main

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// openPTYSlave returns the terminal side of the pty whose master is m.
func openPTYSlave(m *os.File) (*os.File, error) {
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		return nil, e
	}
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		return nil, e
	}
	return os.OpenFile("/dev/pts/"+strconv.FormatUint(uint64(n), 10), os.O_RDWR|syscall.O_NOCTTY, 0)
}
