package main

import "fmt"

// Purpose: a narrow local copy of cli/internal/errs' ExitError contract —
// let a RunE choose a specific process exit code without calling os.Exit
// directly (which main.go reserves, matching go.md's "os.Exit outside
// cmd/nself/main.go" rule applied to this plugin's own main.go).
//
// Inputs: an exit code (claw_keys_ops.go's bootstrapExitCode == 2).
//
// Outputs: an error whose ExitCode() main.go reads to set the process exit
// status, and whose Silent() suppresses main.go's own "Error: ..." line when
// the command already wrote its own diagnostic to stderr (bootstrap's
// validation failures do exactly that).
//
// Constraints: cli/internal/errs is unreachable from this plugin module and
// covers far more than this one case (a general ExitWith/ExitWithf API used
// across 10 core command files) — this copies only the Exit(code) shape
// claw_keys_ops.go actually calls.
type exitError struct {
	code int
}

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// ExitCode satisfies exitCoder, read by main.go.
func (e *exitError) ExitCode() int { return e.code }

// exitCoder mirrors cli/internal/errs.ExitCoder so main.go can type-assert
// without importing that package.
type exitCoder interface {
	ExitCode() int
}

// exitWithCode returns an error that exits with code and prints nothing
// else — the command has already written its own diagnostics to stderr.
// Mirrors cli/internal/errs.Exit.
func exitWithCode(code int) error {
	return &exitError{code: code}
}

// isSilent reports whether err is an *exitError with no wrapped message,
// meaning main.go must not print anything beyond the exit code itself.
// Every exitError produced by this plugin is silent — Error() only
// synthesizes a generic "exit status N" string, never real diagnostic text
// (that was already printed to stderr by the caller before returning it) —
// so this only needs to check the type, not inspect the message.
func isSilent(err error) bool {
	_, ok := err.(*exitError)
	return ok
}
