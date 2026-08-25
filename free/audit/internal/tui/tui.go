// Package tui provides the handful of terminal-output helpers the audit
// plugin needs to reproduce the exact look of `nself audit` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package (Go's internal/ visibility rule forbids it across
// module boundaries), so this file reimplements only the subset of internal/ui
// that cmd/docs.go actually calls: Success and Info. Colors and icons are
// copied byte-for-byte from internal/ui/{colors,icons}.go so output is
// unchanged for a user upgrading from the in-core command to the plugin.
//
// Inputs: plain strings from the caller.
//
// Outputs: writes to stdout, matching internal/ui.Success/Info (both are
// stdout-only in the core CLI).
//
// Constraints: no dependency beyond the standard library — this package must
// stay buildable offline, like the rest of the plugin. Unlike internal/ui,
// which also checks whether stdout is a terminal, this only checks NO_COLOR;
// that simplification was already accepted for the dogfood plugin's
// internal/tui (CLI-R11 first slice) and is not worth a TTY-detection
// dependency for two call sites.
package tui

import (
	"fmt"
	"os"
)

// ANSI color codes, copied from the CLI's internal/ui/colors.go so a user
// sees identical output whether the command runs in-core or as a plugin.
const (
	reset = "\033[0m"
	green = "\033[0;32m"
	blue  = "\033[0;34m"
)

const (
	iconSuccess = "✓" // ✓
	iconInfo    = "ℹ" // ℹ
)

// colorsEnabled mirrors internal/ui minus the TTY check (see package doc).
var colorsEnabled = os.Getenv("NO_COLOR") == ""

func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Success prints a green checkmark and message to stdout, matching
// internal/ui.Success.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, iconSuccess), msg)
}

// Info prints a blue info icon and message to stdout, matching
// internal/ui.Info.
func Info(msg string) {
	fmt.Printf("%s %s\n", c(blue, iconInfo), msg)
}
