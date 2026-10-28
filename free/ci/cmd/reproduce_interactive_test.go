package main

import (
	"github.com/nself-org/plugins/free/ci/internal/reproduce"
	"testing"
)

func TestReproductionsKeepForegroundQueueCap(t *testing.T) {
	old := stdinTerminal
	t.Cleanup(func() { stdinTerminal = old })
	for _, tc := range []struct {
		name     string
		terminal bool
		invoked  string
		want     bool
	}{
		{"person", true, "", true}, {"pipe", false, "", false}, {"wrapper", true, "scheduler", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdinTerminal = func() bool { return tc.terminal }
			t.Setenv("NSELF_INVOKED_BY", tc.invoked)
			// Recorded triggers describe the original run, not who requests reproduction.
			a := reproduceOptions("home", reproduce.RunSpec{Tree: "tree", Trigger: "pull_request"})
			b := verifyOptions("tree", "home")
			if a.Interactive != tc.want || b.Interactive != tc.want {
				t.Fatalf("reproduce=%v verify=%v want=%v", a.Interactive, b.Interactive, tc.want)
			}
			if !a.NoStatus || !b.NoStatus || !b.Full {
				t.Fatal("reproduction options lost")
			}
		})
	}
}
