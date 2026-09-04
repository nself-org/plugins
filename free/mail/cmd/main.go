// Purpose: entry point for the standalone nself-mail binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code — 0 on success, 1 on any RunE error.
//
// Constraints: this file is package main in its own Go module, so — unlike
// the core CLI, where os.Exit is confined to cmd/nself/main.go — os.Exit here
// is the only sane way to report the exit code to the parent nself process
// that exec'd this binary (internal/plugin.ProxyCommand in the core CLI).
//
// AMENDMENT 2026-09-03 (P6-E3-W2-S1-T5 FIX-PLUGINS): the local exitCodeError/
// exitCoder machinery and its "no license configured" exit-2 path were
// removed along with mail.go's requireLicense — mail is a free plugin
// (plugin.json: requires_license=false) and no longer hard-blocks without a
// license key. Any remaining ping_api-side auth error now surfaces as a
// normal error via mapMailError, same as every other backend failure.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
