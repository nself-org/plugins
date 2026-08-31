// Purpose: entry point for the standalone nself-mail binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code — 0 on success, the code carried by an
// *exitCodeError when RunE returns one (2 for "no license configured",
// matching mailExitNoLicense in core), or 1 for any other error.
//
// Constraints: this file is package main in its own Go module, so — unlike
// the core CLI, where os.Exit is confined to cmd/nself/main.go — os.Exit here
// is the only sane way to report the exit code to the parent nself process
// that exec'd this binary (internal/plugin.ProxyCommand in the core CLI).
// Mirrors cmd/nself/main.go's silent-error handling: requireLicense already
// wrote its own message to cmd.ErrOrStderr(), so main() must not print
// "Error: ..." again for that case.
package main

import (
	"fmt"
	"os"
)

// exitCoder is satisfied by *exitCodeError. Named locally (not imported)
// because the core CLI's internal/errs.ExitCoder is unreachable across the
// module boundary — this plugin defines its own minimal equivalent.
type exitCoder interface {
	ExitCode() int
}

// exitCodeError is the plugin's local equivalent of the core CLI's
// internal/plugin.ExitCodeError: requireLicense (mail.go) returns one so
// main() exits with code 2 without printing a duplicate message.
type exitCodeError struct {
	Code int
}

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }
func (e *exitCodeError) ExitCode() int { return e.Code }

func main() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}

	code := 1
	if coder, ok := err.(exitCoder); ok {
		code = coder.ExitCode()
		// A *exitCodeError means requireLicense already printed its own
		// message; printing "Error: ..." again would duplicate it.
		if _, silent := err.(*exitCodeError); silent {
			os.Exit(code)
		}
	}

	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(code)
}
