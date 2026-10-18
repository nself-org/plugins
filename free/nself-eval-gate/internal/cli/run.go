package cli

// Purpose: the argv dispatcher of the nself-eval binary. The nself host mounts
//   the binary as `nself eval`, so argv is what followed `nself ci eval` in core:
//   flags for the run itself, or `gate [flags]` for the tier gate.
// Inputs:  argv without the program name; stdout and stderr writers.
// Outputs: the process exit code (0/1/2/3, see eval.go).
// Constraints: subcommand detection follows cobra: the first non-flag token,
//   where a non-boolean flag swallows the next token, selects `gate`; any other
//   positional is ignored.
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001

import "io"

// Run executes the eval command surface and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	s := streams{out: stdout, err: stderr}
	if i := firstPositional(evalFlags, args); i >= 0 && args[i] == "gate" {
		rest := append(append([]string{}, args[:i]...), args[i+1:]...)
		return runGate(rest, s)
	}
	return runEval(args, s)
}
