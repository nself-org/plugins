package main

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/reproduce"
)

// TestMachineRunsKeepForegroundCap (D-1089): `ci runs reproduce` and
// `ci evidence verify --reproduce` are commands a person types, so they take
// the same interactiveRun check as a bare run and keep the dispatcher's 10
// minute foreground queue cap; a script (no terminal, NSELF_INVOKED_BY) does not.
func TestMachineRunsKeepForegroundCap(t *testing.T) {
	defer func(f func() bool) { stdinTerminal = f }(stdinTerminal)
	t.Setenv("NSELF_INVOKED_BY", "")
	for _, tc := range []struct {
		name string
		tty  bool
		by   string
		want bool
	}{
		{"terminal", true, "", true},
		{"script without a terminal", false, "", false},
		{"terminal under a wrapper", true, "cam", false},
	} {
		stdinTerminal = func() bool { return tc.tty }
		t.Setenv("NSELF_INVOKED_BY", tc.by)
		if got := reproduceOptions("h", reproduce.RunSpec{Tree: "t", Job: "unit"}).Interactive; got != tc.want {
			t.Errorf("reproduce, %s: Interactive %v, want %v", tc.name, got, tc.want)
		}
		if got := verifyOptions("t", "h").Interactive; got != tc.want {
			t.Errorf("verify, %s: Interactive %v, want %v", tc.name, got, tc.want)
		}
	}
}
