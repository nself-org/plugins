// Package tui provides the handful of terminal-output helpers the gdpr
// plugin needs to reproduce the exact look of `nself gdpr` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package across the module boundary, so this file reimplements
// only the subset of internal/ui that gdpr_export.go, gdpr_delete.go, and
// gdpr_query.go actually call: Info, Success, and Warn. Colors and icons are
// copied byte-for-byte from internal/ui/{colors,icons}.go so output is
// unchanged for a user upgrading from the in-core command to the plugin.
//
// Inputs: plain strings from the caller.
//
// Outputs: writes to stdout (Info, Success) or stderr (Warn), matching the
// split internal/ui uses.
//
// Constraints: no dependency beyond the standard library. Unlike internal/ui,
// which also checks whether stdout is a terminal, this only checks NO_COLOR
// — the same simplification already accepted for the dogfood plugin's
// internal/tui (CLI-R11 first slice).
package tui

import (
	"fmt"
	"os"
)

// ANSI color codes, copied from the CLI's internal/ui/colors.go so a user
// sees identical output whether the command runs in-core or as a plugin.
const (
	reset  = "\033[0m"
	blue   = "\033[0;34m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
)

const (
	iconSuccess = "✓" // ✓
	iconWarning = "⚠" // ⚠
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

// Info prints a blue info icon and message to stdout, matching internal/ui.Info.
func Info(msg string) {
	fmt.Printf("%s %s\n", c(blue, iconInfo), msg)
}

// Success prints a green checkmark and message to stdout, matching
// internal/ui.Success.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, iconSuccess), msg)
}

// Warn prints a yellow warning icon and message to stderr, matching
// internal/ui.Warn.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg)
}
