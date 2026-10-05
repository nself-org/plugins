// Package clui prints the core `nself` console framing (section headers and
// status lines) so the ported ci commands match the core output byte for byte.
//
// Purpose: temporary copy of the parts of cli internal/ui that `nself ci`
// uses (Section, Info, Success, Warn, Error). Ports must not import cli
// internals (CANON D11), so the few lines are duplicated here and deleted
// with the core code at P7-SHIP-09.
// Inputs:  message strings.
// Outputs: stdout (Section, Info, Success) and stderr (Warn, Error).
// Constraints: colour only when NO_COLOR is unset and stdout is a terminal,
// exactly as core decides; no dependencies beyond the standard library.
package clui

import (
	"fmt"
	"os"
)

const (
	reset = "\033[0m"
	bold  = "\033[1m"
	red   = "\033[0;31m"
	green = "\033[0;32m"
	blue  = "\033[0;34m"
	// yellow is the warning colour.
	yellow = "\033[0;33m"

	iconSuccess = "✓"
	iconFailure = "✗"
	iconWarning = "⚠"
	iconInfo    = "ℹ"
	iconArrow   = "→"
)

// colorsEnabled mirrors core: NO_COLOR unset and stdout is a terminal. It is
// a variable so tests can drive both branches.
var colorsEnabled = os.Getenv("NO_COLOR") == "" && stdoutIsTerminal()

// stdoutIsTerminal reports whether stdout is a character device.
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// C wraps text in the colour code when colours are enabled.
func C(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Section prints a newline, a blue arrow and a bold title to stdout.
func Section(title string) {
	fmt.Printf("\n%s %s\n", C(blue, iconArrow), C(bold, title))
}

// Info prints a blue info icon and message to stdout.
func Info(msg string) {
	fmt.Printf("%s %s\n", C(blue, iconInfo), msg)
}

// Success prints a green check and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", C(green, iconSuccess), msg)
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", C(yellow, iconWarning), msg)
}

// Error prints a red cross and message to stderr.
func Error(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", C(red, iconFailure), msg)
}
