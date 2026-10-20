package dispatch

// Regression for finding A01 (P7-TRUST-18): a delivered job runs at max(declared level, the floor
// placement required), never lower; an unknown or empty declared level ranks lowest.

import (
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func floorQueued(tr model.TrustClass, min model.Isolation) Queued {
	return Queued{Job: Job{Sched: sched.Job{Trust: tr, Requirements: model.PlacementRequirements{IsolationMin: min}}}}
}

// isolationPolicy is an effective policy whose trust.isolation_min is level.
func isolationPolicy(t *testing.T, level string) trust.Effective {
	t.Helper()
	r, err := trust.NewRegistry(trust.KeySpec{Key: "trust.isolation_min", Owner: "trust", Kind: trust.TightenHigher, Order: []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}, Default: "process"})
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.isolation_min": json.RawMessage(`"` + level + `"`)}})
	if err != nil {
		t.Fatal(err)
	}
	return e
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
		{"hosted-disposable minimum keeps the trust floor and is delivered as ephemeral-vm", "untrusted", "hosted-disposable", "container", false, "ephemeral-vm"},
		{"hosted-disposable minimum on a collaborator job is ephemeral-vm", "collaborator", "hosted-disposable", "process", false, "ephemeral-vm"},
		{"hosted delivery with a hosted-disposable minimum is left as written", "untrusted", "hosted-disposable", "container", true, "container"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := floorQueued(c.trust, c.min)
			job := model.Job{Isolation: c.declared}
			r := sched.Runner{}
			if c.hosted {
				r.Location = "hosted"
			}
			got := raiseIsolation(job, requiredIsolation(q, r, sched.Policy{}))
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

// TestHostedRunnerPredicateIsShared (S2): every form placement treats as hosted is hosted for delivery;
// an ephemeral runner that is not hosted still gets the trust floor.
func TestHostedRunnerPredicateIsShared(t *testing.T) {
	hosted := model.Capability{}
	kind := "hosted"
	hosted.Location.Kind.Value = &kind
	cases := []struct {
		name string
		r    sched.Runner
		want model.Isolation
	}{
		{"location hosted", sched.Runner{Location: "hosted"}, ""},
		{"capability location kind hosted", sched.Runner{Capability: hosted}, ""},
		{"declaration hosted", func() sched.Runner { r := sched.Runner{}; r.Decl.Hosted = true; return r }(), ""},
		{"ephemeral but not hosted gets the floor", sched.Runner{Location: "lan", Ephemeral: true}, "sandboxed-container"},
		{"plain lan node gets the floor", sched.Runner{Location: "lan"}, "sandboxed-container"},
	}
	for _, c := range cases {
		if got := requiredIsolation(floorQueued("untrusted", ""), c.r, sched.Policy{}); got != c.want {
			t.Errorf("%s: floor %q, want %q", c.name, got, c.want)
		}
	}
}

// TestPolicyMinimumsRaiseTheDeliveredLevel (S3): the override and trust.isolation_min minimums raise the
// delivered level, never lower it.
func TestPolicyMinimumsRaiseTheDeliveredLevel(t *testing.T) {
	q := floorQueued("collaborator", "")
	cases := []struct {
		name     string
		pol      sched.Policy
		declared model.Isolation
		want     model.Isolation
	}{
		{"trust.isolation_min vm raises a container job", sched.Policy{Trust: isolationPolicy(t, "vm")}, "container", "vm"},
		{"override min_isolation vm raises a container job", sched.Policy{Overrides: sched.Overrides{MinIsolation: "vm"}}, "container", "vm"},
		{"a policy minimum below the floor changes nothing", sched.Policy{Trust: isolationPolicy(t, "process")}, "process", "container"},
		{"a declared level above the policy minimum is kept", sched.Policy{Trust: isolationPolicy(t, "vm")}, "ephemeral-vm", "ephemeral-vm"},
	}
	for _, c := range cases {
		got := raiseIsolation(model.Job{Isolation: c.declared}, requiredIsolation(q, sched.Runner{}, c.pol)).Isolation
		if got != c.want {
			t.Errorf("%s: delivered at %q, want %q", c.name, got, c.want)
		}
	}
}

// TestProductionBoundaryRaisesTheDeliveredLevel (P7-TRUST-18, cross-family review): admission only checks the
// node's declared level on a coordinator host or an overridden deploy host; delivery must also run the job at
// the vm floor there (D21, D28), except for a personal coordinator's own owner and internal work.
func TestProductionBoundaryRaisesTheDeliveredLevel(t *testing.T) {
	deploy := model.Capability{}
	deploy.DeployHost.Matched, deploy.DeployHost.Override = true, true
	cases := []struct {
		name string
		tr   model.TrustClass
		r    sched.Runner
		want model.Isolation
	}{
		{"collaborator on a team coordinator host runs at vm", "collaborator", sched.Runner{CoordinatorHost: true, CoordinatorMode: trust.Team}, "vm"},
		{"collaborator on a personal coordinator host runs at vm", "collaborator", sched.Runner{CoordinatorHost: true, CoordinatorMode: trust.Personal}, "vm"},
		{"untrusted on a coordinator host runs at vm, above the sandbox floor", "untrusted", sched.Runner{CoordinatorHost: true, CoordinatorMode: trust.Personal}, "vm"},
		{"owner work on a team coordinator host runs at vm", "owner", sched.Runner{CoordinatorHost: true, CoordinatorMode: trust.Team}, "vm"},
		{"owner work on a personal coordinator host keeps its normal level", "owner", sched.Runner{CoordinatorHost: true, CoordinatorMode: trust.Personal}, "container"},
		{"owner work on an overridden deploy host runs at vm", "owner", sched.Runner{Capability: deploy}, "vm"},
		{"a node that is neither keeps the trust floor", "collaborator", sched.Runner{}, "container"},
	}
	for _, c := range cases {
		got := raiseIsolation(model.Job{Isolation: "process"}, requiredIsolation(floorQueued(c.tr, ""), c.r, sched.Policy{})).Isolation
		if got != c.want {
			t.Errorf("%s: delivered at %q, want %q", c.name, got, c.want)
		}
	}
}
