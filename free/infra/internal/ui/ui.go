// Package ui prints the two message styles this plugin uses.
//
// Purpose: match the output `nself infra` produced before this command moved
// out of the CLI. A user who installs the plugin should not be able to tell
// that anything moved, which means the icon, the colour and the stream each
// line goes to all have to stay the same.
//
// Inputs: a message string.
//
// Outputs: Info to stdout, Warn to stderr.
//
// Constraints: this is a deliberate copy of the two functions the CLI's own
// internal/ui exports, not an import of it — a plugin is a separate module and
// must not depend on github.com/nself-org/cli/internal/*. It copies only Info
// and Warn; the CLI package is 1,105 lines and this command calls two of them.
// If a later change needs a third, copy that one too rather than the package.
package ui

import (
	"fmt"
	"os"
)

const (
	yellow = "\033[0;33m"
	blue   = "\033[0;34m"
	reset  = "\033[0m"

	iconWarning = "⚠" // ⚠
	iconInfo    = "ℹ" // ℹ
)

// colorsEnabled mirrors the CLI: colour is off when NO_COLOR is set or stdout
// is not a terminal, so piped output stays clean.
var colorsEnabled = os.Getenv("NO_COLOR") == "" && stdoutIsTerminal()

func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg)
}

// Info prints a blue info icon and message to stdout.
func Info(msg string) {
	fmt.Printf("%s %s\n", c(blue, iconInfo), msg)
}

// stdoutIsTerminal reports whether stdout is connected to a terminal.
//
// The CLI uses golang.org/x/term for this. Here it is the stdlib character-
// device check, to keep the plugin's dependencies to cobra alone. The two agree
// on the cases that matter — a real terminal, a pipe, a file, CI — and differ
// only on exotic character devices that are not terminals, where this errs
// toward emitting colour.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
