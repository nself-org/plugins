//go:build windows

package exec

// GateArg is unused on Windows, where jobs are not supported.
const GateArg = "--nself-ci-start-gate"

// GateRequested is always false on Windows.
func GateRequested([]string) bool { return false }

// RunGate is never called on Windows.
func RunGate([]string) int { return 2 }

func gatedCommand(s JobSpec, _ string, _ []string) ([]string, bool, error) {
	return s.Command, false, nil
}
