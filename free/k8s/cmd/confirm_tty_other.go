//go:build !linux && !darwin

// Purpose: the terminal test behind stdinIsTTY where no termios ioctl is
// wired up (Windows and the rest).
//
// Outputs: always false, so uninstall there needs --yes. Refusing is the safe
// side: a prompt that cannot tell a person from /dev/zero must not run.
package main

// isTerminal reports false: no terminal check is implemented on this OS.
func isTerminal(uintptr) bool { return false }
