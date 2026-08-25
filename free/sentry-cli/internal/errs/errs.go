// Package errs carries an exit status out of a command without calling
// os.Exit from inside it.
//
// Purpose: `nself sentry status` reports the state it found through its exit
// code — a healthy stack and an unhealthy one both "succeed" as commands. The
// CLI expresses that by returning an error that carries a code, which main
// reads. That behaviour has to survive the move to a plugin, because scripts
// depend on it.
//
// Inputs: an exit code.
//
// Outputs: an error carrying that code, and nothing to print.
//
// Constraints: a deliberate copy of the two pieces of the CLI's internal/errs
// this command uses. The CLI package is 1,103 lines and provides sentinel
// errors this plugin does not reference. Copy another piece here if a later
// change needs one, never the package.
package errs

import "fmt"

// ExitError carries a process exit code. Its message is never printed: the
// command has already told the user what it found.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// ExitCode reports the status main should exit with.
func (e *ExitError) ExitCode() int { return e.Code }

// Silent reports that the message must not be printed.
func (e *ExitError) Silent() bool { return true }

// Exit returns an error requesting the given exit code with no output.
func Exit(code int) error { return &ExitError{Code: code} }
