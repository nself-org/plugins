//go:build windows

package main

// Purpose: isTerminal for Windows: a character device (the console) counts.
// Inputs: an open file. Outputs: whether it is a terminal.

import "os"

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
