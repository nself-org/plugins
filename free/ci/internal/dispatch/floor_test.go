package dispatch

// Regression for finding A01 (P7-TRUST-18): a delivered job runs at max(declared level, the floor
// placement required), never lower; an unknown or empty declared level ranks lowest.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func floorQueued(tr model.TrustClass, min model.Isolation) Queued {
	return Queued{Job: Job{Sched: sched.Job{Trust: tr, Requirements: model.PlacementRequirements{IsolationMin: min}}}}
}

func TestDeliveredIsolationIsRaisedToTheFloor(t *testing.T) {
	cases := []struct {
		name     string
		trust    model.TrustClass
		min      model.Isolation
		declared model.Isolation
		hosted   bool
		want     model.Isolation
	}{
		{"untrusted declared process runs sandboxed", "untrusted", "", "process", false, "sandboxed-container"},
		{"untrusted with no declared level", "untrusted", "", "", false, "sandboxed-container"},
		{"untrusted with an unknown declared level", "untrusted", "", "bogus", false, "sandboxed-container"},
		{"untrusted declared container is raised", "untrusted", "", "container", false, "sandboxed-container"},
		{"a declared level above the floor is kept", "untrusted", "", "vm", false, "vm"},
		{"collaborator declared process runs in a container", "collaborator", "", "process", false, "container"},
		{"requirement minimum raises a collaborator job", "collaborator", "vm", "container", false, "vm"},
		{"hosted delivery is left as written", "untrusted", "", "process", true, "process"},
		{"hosted-disposable is never written into a job", "untrusted", "hosted-disposable", "container", false, "container"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := floorQueued(c.trust, c.min)
			job := model.Job{Isolation: c.declared}
			got := raiseIsolation(job, requiredIsolation(q, sched.Runner{}, c.hosted))
			if got.Isolation != c.want {
				t.Fatalf("isolation = %q, want %q", got.Isolation, c.want)
			}
		})
	}
}

func TestRaiseIsolationNeverLowers(t *testing.T) {
	for _, level := range []model.Isolation{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"} {
		for _, floor := range []model.Isolation{"", "process", "container", "sandboxed-container"} {
			got := raiseIsolation(model.Job{Isolation: level}, floor).Isolation
			if got != level && !(floor != "" && got == floor) {
				t.Fatalf("level %q floor %q gave %q", level, floor, got)
			}
			if rank := map[model.Isolation]int{"process": 0, "container": 1, "sandboxed-container": 2, "vm": 3, "ephemeral-vm": 4}; rank[got] < rank[level] {
				t.Fatalf("level %q lowered to %q by floor %q", level, got, floor)
			}
		}
	}
}
