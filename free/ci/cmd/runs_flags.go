package main

// Purpose: the flag sets of the small `runs <sub>` handlers, declared once so the handler and the machine
//          surface check (cmd/surface_test.go, runsSubFlags in surface_class.go) read the same flags.
// Inputs:  argv after `runs <sub>`.
// Outputs: standard flag sets (ContinueOnError, output discarded; the handlers print their own usage).
// Constraints: a flag a handler accepts is declared here or in the handler's own constructor, never parsed by hand.

import (
	"flag"
	"io"
)

// jsonOnlyFlagSet is the flag set of a `runs` subcommand whose only flag is --json (asJSON nil: a throwaway).
func jsonOnlyFlagSet(name string, asJSON *bool) *flag.FlagSet {
	if asJSON == nil {
		asJSON = new(bool)
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(asJSON, "json", false, "print one v1 JSON envelope")
	return fs
}

// cancelFlagSet is the flag set of `runs cancel <run>`: it takes none.
func cancelFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("runs cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// pruneFlagSet is every flag `runs prune` accepts: the retention flags (runs_history.go) and the two that
// switch to the tidy report (runs_tidy.go).
func pruneFlagSet() *flag.FlagSet {
	fs := newHistoryFlagSet(&historyFlags{}, true)
	fs.SetOutput(io.Discard)
	fs.Bool("tidy", false, "report the disk the CI uses and what a prune frees")
	fs.Bool("apply", false, "with --tidy: remove what the report lists (default: report only)")
	return fs
}
