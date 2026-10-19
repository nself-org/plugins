//go:build windows

package main

// Purpose: isTerminal for Windows: the console-mode query succeeds on a console
// and fails on a pipe, a file and the NUL device (which is a character device).
// Inputs: an open file. Outputs: whether it is a terminal. A console-less shell
// (mintty, Git Bash) hands the process a pipe, so it counts as scripted.
// Constraints: golang.org/x/sys/windows is a declared, vendored dependency.

import (
	"os"

	"golang.org/x/sys/windows"
)

func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
