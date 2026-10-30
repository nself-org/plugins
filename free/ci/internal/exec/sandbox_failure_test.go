package exec

import (
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
