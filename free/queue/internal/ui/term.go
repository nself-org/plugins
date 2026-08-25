package ui

import "os"

// stdoutIsTerminal reports whether stdout is connected to a terminal.
//
// The CLI uses golang.org/x/term for this. Here it is the stdlib character-
// device check, so the plugin's only dependency stays cobra. The two agree on
// the cases that matter — a real terminal, a pipe, a file, CI — and differ only
// on exotic character devices that are not terminals, where this errs toward
// emitting colour.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
