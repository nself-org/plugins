// Purpose: tests for the D-0447 envelope edges: wantsJSON takes the last
// --json spelling as cobra does, and a JSON usage error whose envelope cannot
// be written exits with the infra status (report in main.go).
//
// Inputs: argument lists; a writer that always fails.
//
// Outputs: test results.
//
// Constraints: no network, no cluster, no process exec.
package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

func TestWantsJSONLastWins(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"status", "--json=false", "--json"}, true},
		{[]string{"status", "--json", "--json=false"}, false},
		{[]string{"status", "--json=0", "--json=1"}, true},
		{[]string{"status", "--json", "--json=maybe"}, true},
		{[]string{"status", "--json=false", "--", "--json"}, false},
		{[]string{"status", "--json", "--", "--json=false"}, true},
	}
	for _, c := range cases {
		if got := wantsJSON(c.args); got != c.want {
			t.Errorf("wantsJSON(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

// failWriter refuses every write, like a closed or full stdout.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

func TestReportUsageEnvelopeWriteFailureIsInfra(t *testing.T) {
	saved := ran
	ran = false
	defer func() { ran = saved }()
	var stderr strings.Builder
	code := report(errors.New(`unknown flag: --bogus`), true, []string{"status", "--bogus", "--json"}, failWriter{}, &stderr)
	if code != output.ExitInfra {
		t.Errorf("exit = %d, want %d (infra)", code, output.ExitInfra)
	}
	if !strings.Contains(stderr.String(), "unknown flag: --bogus") || !strings.Contains(stderr.String(), "writing the JSON envelope") {
		t.Errorf("stderr must name the usage error and the write failure:\n%s", stderr.String())
	}
	// Control: the same error with a working stdout keeps the usage status.
	var stdout strings.Builder
	if code := report(errors.New(`unknown flag: --bogus`), true, []string{"status", "--bogus", "--json"}, &stdout, &stderr); code != output.ExitUser {
		t.Errorf("writable stdout: exit = %d, want %d (user)", code, output.ExitUser)
	}
}
