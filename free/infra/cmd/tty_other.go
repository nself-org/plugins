//go:build !darwin && !linux

// Purpose: isatty fallback for platforms the plugin does not ship on.
//
// Inputs: a file descriptor.
//
// Outputs: always false, so the spend gate refuses (fail closed).
//
// Constraints: none.
package main

func isatty(fd uintptr) bool { return false }
