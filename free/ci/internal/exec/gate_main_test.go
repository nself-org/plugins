package exec

import (
	"os"
	"strings"
	"testing"
)

// TestMain lets this test binary stand in for two programs: the agent, as a gated job's start-gate helper
// (the executor re-executes its own binary with GateArg), and a payload that prints its exact environment.
func TestMain(m *testing.M) {
	if GateRequested(os.Args) {
		os.Exit(RunGate(os.Args))
	}
	if len(os.Args) == 2 && os.Args[1] == "--dump-env-nul" {
		_, _ = os.Stdout.WriteString(strings.Join(os.Environ(), "\x00"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}
