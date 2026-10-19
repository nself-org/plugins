package gates

// Purpose: tell a check that failed because the environment lacks its tool
// from one that failed because the code is wrong (E614, never a code failure).
// Inputs:  a catalogue check ID, the exit code and the output of a finished run.
// Outputs: the missing tool and its remediation.
// Constraints: shared by Runner.Run and the run loop's outcome hook, so a
// planned job and a Runner check classify the same output the same way; a run
// that exited 0 is never reclassified.

// toolGaps lists the checks whose output can show that a tool they run is
// missing even though the check's own launcher exists.
var toolGaps = map[string]struct {
	tool, fix string
	missing   func(output string) bool
}{
	"python-test":  {"pytest", "Add pytest to the project's dependencies", func(o string) bool { return pythonToolMissing("pytest", o) }},
	"python-types": {"mypy", "Add mypy to the project's dependencies", func(o string) bool { return pythonToolMissing("mypy", o) }},
}

// Unavailable reports whether a finished check run shows that the tool it
// needs is missing, with the tool name and the remediation.
func Unavailable(checkID string, exit int, output string) (tool, fix string, ok bool) {
	g, found := toolGaps[checkID]
	if !found || exit == 0 || !g.missing(output) {
		return "", "", false
	}
	return g.tool, g.fix, true
}

// HasToolGap reports whether a check's output is classified for missing tools.
func HasToolGap(checkID string) bool { _, ok := toolGaps[checkID]; return ok }
