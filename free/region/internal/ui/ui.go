// Package ui prints the message styles this plugin uses.
//
// Purpose: match what the command printed before it moved out of the CLI.
// Icon, colour and the stream each line goes to all have to stay the same,
// because a user who installs the plugin should not be able to tell that
// anything moved.
//
// Constraints: a deliberate copy of the handful of functions this command
// calls, not an import — a plugin is a separate module and must not depend on
// github.com/nself-org/cli/internal/*. The CLI package is 1,105 lines; copy
// another function here if a later change needs one, never the package.
package ui

import (
	"fmt"
	"os"
)

const (
	green = "\033[0;32m"
	reset = "\033[0m"
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

// Success prints a green success and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, "✔"), msg)
}

// stdoutIsTerminal reports whether stdout is connected to a terminal.
//
// The CLI uses golang.org/x/term for this. Here it is the stdlib character-
// device check, to keep the plugin's dependencies to cobra alone. The two
// agree on the cases that matter — a real terminal, a pipe, a file, CI — and
// differ only on exotic character devices that are not terminals, where this
// errs toward emitting colour.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
