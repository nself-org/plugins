//go:build !windows

package main

// Purpose: isTerminal for the unix family: the terminal-size ioctl succeeds on
// a terminal and fails with ENOTTY on a pipe, a file, /dev/null or /dev/zero.
// Inputs: an open file. Outputs: whether it is a terminal.
// Constraints: golang.org/x/sys/unix is a declared, vendored dependency.

import (
	"os"

	"golang.org/x/sys/unix"
)

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	return err == nil
}
