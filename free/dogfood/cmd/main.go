// Purpose: entry point for the standalone nself-dogfood binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code — 0 on success, 1 on a cobra/runtime error or a
// failed audit check, 2 when the audit found warnings only. exitCode is set
// by auditCmd's RunE (see audit.go) because cobra's own Execute() only
// distinguishes "errored" from "did not error", not the three-way dogfood
// verdict.
//
// Constraints: this file is package main in its own Go module, so — unlike
// the core CLI, where os.Exit is confined to cmd/nself/main.go — os.Exit here
// is the only sane way to report the exit code to the parent nself process
// that exec'd this binary (internal/plugin.ProxyCommand in the core CLI).
package main

import (
	"fmt"
	"os"
)

// exitCode is set by a subcommand's RunE when it needs an exit status other
// than 0/1 (cobra itself only distinguishes success from error). Read once,
// after Execute returns, in main().
var exitCode int

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}
