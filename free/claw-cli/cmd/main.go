// Purpose: entry point for the standalone nself-claw binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code — 0 on success, the code carried by an
// *exitError when RunE returns one (see exit.go — mirrors cli/internal/errs'
// ExitError contract for the one caller that needs a specific code,
// claw_keys_ops.go's bootstrap validation), 1 on any other error.
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		if ec, ok := err.(exitCoder); ok {
			if !isSilent(err) {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
			os.Exit(ec.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
