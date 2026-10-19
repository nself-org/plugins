package background

// Shared fixtures: runners with chosen availability facts and background,
// normal or interactive jobs. Randomised tests print their seed on failure.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func ptr[T any](v T) *T { return &v }

// signals are the contention inputs of one runner; nil is an unknown fact.
type signals struct {
	Plugged     *bool
	BatteryPct  *int
	Interactive *string
	Operator    bool
	Active      bool
}

// runner builds a runner every fixture job is otherwise eligible for.
func runner(id string, s signals) sched.Runner {
	slots, loc := 2, "local"
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Availability.Slots.Value = &slots
	c.Availability.PluggedIn.Value = s.Plugged
	c.Availability.BatteryPct.Value = s.BatteryPct
	c.Availability.Interactive.Value = s.Interactive
	c.Identity.ID, c.Identity.Provider, c.Identity.Transport = id, "operator", "kind-"+id
	c.Identity.Labels = []string{"arch=amd64"}
	c.Location.Kind.Value = &loc
	owner := "operator"
	if !s.Operator {
		owner = "provider"
	}
	c.Identity.Ownership.Value = &owner
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Active: s.Active,
		Avail:      sched.Avail{Slots: slots, CPU: 8, MemMB: 8192},
		Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged",
			UIDSeparation: true, Ownership: owner, PersonalOwnNode: true, NoLANRoute: true}}
}

// job is a fixture job of the given band ("" = normal).
func job(key, band string) sched.Job {
	return sched.Job{Key: key, ID: key, Name: key, Pipeline: sched.Pipeline{ID: "pipe", Band: band, Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan",
		TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
}

// idleDesk is an operator-owned machine on mains power and known idle.
var idleDesk = signals{Plugged: ptr(true), Interactive: ptr("idle"), Operator: true}
