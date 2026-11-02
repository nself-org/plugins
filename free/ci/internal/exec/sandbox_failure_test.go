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

func TestSandboxFailureOnlyForASilentFailure(t *testing.T) {
	died := deathPlanner{line: "createStub: failed to get clone"}
	cases := []struct {
		name string
		plan ContainerPlanner
		code int
		tail string
		want string
	}{
		{"silent failure, runtime cites a death", died, 2, "", died.line},
		{"blank output counts as silent", died, 2, " \n", died.line},
		{"the job printed something", died, 2, "boom", ""},
		{"exit 0", died, 0, "", ""},
		{"runtime has no record of a death (exit 2 script)", deathPlanner{}, 2, "", ""},
		{"planner cannot explain", nil, 2, "", ""},
	}
	for _, c := range cases {
		if got := sandboxFailure(c.plan, ContainerRun{}, time.Time{}, c.code, c.tail); got != c.want {
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

func TestSilentReasonUsesTheKernelCounterThenTheRuntimeRecord(t *testing.T) {
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
		got := (&Executor{}).silentReason(s, ContainerRun{}, time.Time{}, 2, "", c.pids)
		if got != c.want || !strings.Contains(string(out), c.contains) {
			t.Errorf("%s: got %q out %q", c.name, got, out)
		}
		if c.want == "" && !strings.Contains(string(out), "boot logs") {
			t.Errorf("%s: the runtime note is missing: %q", c.name, out)
		}
	}
}
