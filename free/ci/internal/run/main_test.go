package run

import (
	"os"
	"testing"
)

// TestMain points NSELF_CI_HOME at a throwaway directory for the whole run:
// the executor's orphan sweep reads it, and no test may ever sweep the
// user's real ~/.nself/ci. Tests give each run its own Home; Recover skips the
// sweep for a Home that differs from this one.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ci37-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("NSELF_CI_HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
