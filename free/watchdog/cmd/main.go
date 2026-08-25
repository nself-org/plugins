// Purpose: entry point for the standalone nself-watchdog binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code — 0 on success, 1 on a cobra/runtime error, 2
// when watchdogStatusCmd finds an open circuit breaker (see root.go for why
// exitCode is a package var: cobra's own Execute() only distinguishes
// "errored" from "did not error", not this three-way status).
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

// exitCode is set by watchdogStatusCmd's RunE when it needs an exit status
// other than 0/1 (cobra itself only distinguishes success from error). Read
// once, after Execute returns, in main().
var exitCode int

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}
