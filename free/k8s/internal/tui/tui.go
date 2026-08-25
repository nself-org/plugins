// Package tui provides the handful of terminal-output helpers the k8s
// plugin needs to reproduce the exact look of `nself k8s` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package (Go's internal/ visibility rule forbids it across
// module boundaries), so this file reimplements only the subset of
// internal/ui that cmd/install.go, cmd/upgrade.go, and cmd/status.go
// actually call: Info, Success, and Warn. Colors and icons are copied
// byte-for-byte from internal/ui/{colors,icons,messages}.go so output is
// unchanged for a user upgrading from the in-core command to the plugin.
//
// Inputs: plain strings from the caller.
//
// Outputs: writes to stdout (Info/Success) or stderr (Warn), matching the
// split internal/ui used.
//
// Constraints: no dependency beyond the standard library — this package
// must stay buildable offline, like the rest of the plugin. Unlike
// internal/ui/term.go, this skips golang.org/x/term TTY detection and only
// honors NO_COLOR, which is enough for a plugin this small (same tradeoff
// the dogfood plugin's internal/tui made).
package tui

import (
	"fmt"
	"os"
)

const (
	reset  = "\033[0m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
	blue   = "\033[0;34m"
)

const (
	iconSuccess = "✓" // check mark
	iconWarning = "⚠" // warning triangle
	iconInfo    = "ℹ" // info
)

// colorsEnabled mirrors internal/ui: off when NO_COLOR is set.
var colorsEnabled = os.Getenv("NO_COLOR") == ""

func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Success prints a green checkmark and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, iconSuccess), msg)
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg)
}

// Info prints a blue info icon and message to stdout.
func Info(msg string) {
	fmt.Printf("%s %s\n", c(blue, iconInfo), msg)
}
