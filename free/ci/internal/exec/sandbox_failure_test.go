package exec

import (
	"strings"
	"testing"
	"time"
)

type deathPlanner struct {
	ContainerPlanner
	line string
}

func (d deathPlanner) SandboxFailure(ContainerRun, time.Time) string { return d.line }

func TestSandboxFailureUsesRuntimeEvidenceForEveryFailedExit(t *testing.T) {
	died := deathPlanner{line: "createStub: failed to get clone"}
	cases := []struct {
		name string
		plan ContainerPlanner
		code int
		want string
	}{
		{"runtime cites a death", died, 2, died.line},
		{"exit 0", died, 0, ""},
		{"runtime has no record of a death (exit 2 script)", deathPlanner{}, 2, ""},
		{"planner cannot explain", nil, 2, ""},
	}
	for _, c := range cases {
		if got := sandboxFailure(c.plan, ContainerRun{}, time.Time{}, c.code); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

type notePlanner struct {
	deathPlanner
}

func (notePlanner) SandboxNote(ContainerRun, time.Time) string {
	return "runsc log: 2 boot logs, none of this attempt"
}

func TestSandboxReasonUsesTheKernelCounterThenTheRuntimeRecord(t *testing.T) {
	cases := []struct {
		name     string
		plan     ContainerPlanner
		pids     pidsReading
		want     string
		contains string // of the line written to the job output
	}{
		{"kernel counter alone", notePlanner{}, pidsReading{Max: 5, Path: "/c", Limit: "100", Reads: 9}, "resource_limit", "max=5"},
		{"runtime record alone", notePlanner{deathPlanner{line: "createStub: failed to get clone"}}, pidsReading{Path: "/c", Limit: "100"}, "resource_limit", "failed to get clone"},
		{"neither: plain exit 2 stays code and says what it read", notePlanner{}, pidsReading{Path: "/c", Limit: "100", Reads: 4}, "", "pids.events: max=0 after 4 reads"},
		{"neither, no cgroup reachable", notePlanner{}, pidsReading{Problem: "container not seen"}, "", "container not seen"},
	}
	for _, c := range cases {
		var out []byte
		s := JobSpec{Plan: c.plan, Output: func(b []byte) { out = append(out, b...) }}
		got := (&Executor{}).sandboxReason(s, ContainerRun{}, time.Time{}, 2, c.pids, sandboxOutput{})
		if got != c.want || !strings.Contains(string(out), c.contains) {
			t.Errorf("%s: got %q out %q", c.name, got, out)
		}
		if c.want == "" && !strings.Contains(string(out), "boot logs") {
			t.Errorf("%s: the runtime note is missing: %q", c.name, out)
		}
	}
}

func TestSandboxReasonRequiresOnlyRuntimeOutputEvenWhenPidsLimitWasHit(t *testing.T) {
	for _, tc := range []struct {
		name, output, reason string
		max                  uint64
	}{
		{"limit hit with FAIL", "FAIL\n", "", 1},
		{"limit hit with only runtime diagnostics", "sandbox wait failed\nsandbox wait failed\n", "resource_limit", 1},
		{"no limit hit", "sandbox wait failed\n", "", 0},
		{"silent limit hit", "", "resource_limit", 1},
		{"silent without limit hit", "", "", 0},
		{"whitespace only limit hit", " \t\n", "resource_limit", 1},
		{"runtime line without final newline", "sandbox wait failed", "resource_limit", 1},
		{"FAIL mixed with diagnostic", "sandbox wait failed\nFAIL\n", "", 1},
		{"FAIL appended to diagnostic", "sandbox wait failed: FAIL\n", "", 1},
		{"FAIL before tail", "FAIL\n" + strings.Repeat("sandbox wait failed\n", 300), "", 1},
		{"incomplete diagnostic", "sandbox wait", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Try whole writes and split lines, as runtime output arrives in arbitrary chunks.
			for _, chunkSize := range []int{1, 7, len(tc.output) + 1} {
				var output sandboxOutput
				for rest := tc.output; len(rest) > 0; {
					n := min(chunkSize, len(rest))
					output.write([]byte(rest[:n]))
					rest = rest[n:]
				}
				got := (&Executor{}).sandboxReason(JobSpec{}, ContainerRun{}, time.Time{}, 2, pidsReading{Max: tc.max}, output)
				if got != tc.reason {
					t.Fatalf("chunk size %d: reason %q want %q", chunkSize, got, tc.reason)
				}
			}
		})
	}
}
