//go:build security

package redteam

// Purpose: red team attacks on admission and placement (TM-01, TM-05, TM-07, TM-08, TM-11, TM-45).
//   - A17: an untrusted job against every node declaration that should refuse it (weak isolation, a LAN
//     route, no UID separation, wide network scope, secrets in any class spelling, trust spelling tricks);
//   - A18: co-tenancy in both directions, with the Running facts set;
//   - A19: a collaborator head (TM-45) never lands on a node below container isolation; unknown values
//     decode to the strictest meaning;
//   - A20: the retry floor holds when World.PreviousIsolation is set, and a deploy host is never chosen;
// Inputs:  trust.Admissible, trust.RetryIsolation, sched.Place over hand-built declarations and runners.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: pure functions; the policy comes from the real registry (internal/policy keys).

import (
	"slices"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func aPolicy(t *testing.T) trust.Effective {
	t.Helper()
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.DefaultPolicy("public"))
	if err != nil {
		t.Fatal(err)
	}
	return eff
}

var aAllTrust = []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}

// aSandboxNode is a node that is allowed to host untrusted work.
func aSandboxNode() trust.Declaration {
	return trust.Declaration{Accepts: aAllTrust, Isolation: "sandboxed-container", Network: "restricted", UIDSeparation: true, NoLANRoute: true,
		Ownership: "operator", PrivacyZone: "private-infrastructure", SecretClasses: []model.SecretClass{"project"}}
}

func aHas(rs []trust.Reason, want trust.Reason) bool { return slices.Contains(rs, want) }

func TestA17_UntrustedAgainstWeakNodes(t *testing.T) {
	eff := aPolicy(t)
	job := trust.JobFacts{Trust: "untrusted", Network: "restricted"}
	if rs := trust.Admissible(job, aSandboxNode(), eff); len(rs) != 0 {
		t.Fatalf("control: a proper sandbox node must admit the job, got %v", rs)
	}
	weak := map[string]func(d *trust.Declaration){
		"container":    func(d *trust.Declaration) { d.Isolation = "container" },
		"process":      func(d *trust.Declaration) { d.Isolation = "process" },
		"empty":        func(d *trust.Declaration) { d.Isolation = "" },
		"unknown":      func(d *trust.Declaration) { d.Isolation = "sandboxed-container " },
		"upper":        func(d *trust.Declaration) { d.Isolation = "Sandboxed-Container" },
		"lan route":    func(d *trust.Declaration) { d.NoLANRoute = false },
		"no uid sep":   func(d *trust.Declaration) { d.UIDSeparation = false },
		"not accepted": func(d *trust.Declaration) { d.Accepts = []model.TrustClass{"owner", "internal", "collaborator"} },
		"unknown acc":  func(d *trust.Declaration) { d.Accepts = []model.TrustClass{"Untrusted"} },
		"owner process": func(d *trust.Declaration) {
			d.Isolation, d.Ownership, d.PersonalOwnNode, d.CoordinatorMode = "process", "operator", true, "personal"
		},
	}
	for name, f := range weak {
		d := aSandboxNode()
		f(&d)
		if rs := trust.Admissible(job, d, eff); len(rs) == 0 {
			t.Errorf("untrusted job admitted on a %s node", name)
		}
	}
	for _, net := range []model.NetworkScope{"lan", "privileged", "LAN", "", "internet", "lan,none", "restricted "} {
		j := job
		j.Network = net
		d := aSandboxNode()
		d.Network = "privileged"
		if rs := trust.Admissible(j, d, eff); len(rs) == 0 {
			t.Errorf("untrusted job admitted with network scope %q (default policy caps it at restricted)", net)
		}
	}
	for _, class := range []model.SecretClass{"project", "environment", "team", "release", "deploy", "", "None", "NONE", "none ", "x"} {
		j := job
		j.SecretClasses = []model.SecretClass{class}
		d := aSandboxNode()
		d.SecretClasses = []model.SecretClass{class, "project"}
		if rs := trust.Admissible(j, d, eff); !aHas(rs, trust.SecretsUntrusted) {
			t.Errorf("untrusted job with secret class %q not refused as untrusted: %v", class, rs)
		}
	}
	for _, spelled := range []model.TrustClass{"", "Owner", "OWNER", "owner ", "​owner", "trusted", "admin", "internal\x00", "collaborator,owner", "ownеr"} {
		j := trust.JobFacts{Trust: spelled, Network: "restricted"}
		d := aSandboxNode()
		d.Accepts = []model.TrustClass{"owner", "internal", "collaborator"}
		if rs := trust.Admissible(j, d, eff); !aHas(rs, trust.TrustNotAccepted) {
			t.Errorf("trust spelled %q was read as a class the node accepts: %v", spelled, rs)
		}
	}
}

// A18: co-tenancy, both directions, with the node reporting what runs on it.
func TestA18_CotenancyBothWays(t *testing.T) {
	eff := aPolicy(t)
	d := aSandboxNode()
	d.SecretClasses = []model.SecretClass{"project"}
	d.Running = trust.Running{SecretBearing: true}
	if rs := trust.Admissible(trust.JobFacts{Trust: "untrusted", Network: "restricted"}, d, eff); !aHas(rs, trust.IsolationCotenancy) {
		t.Errorf("untrusted job beside a secret-bearing attempt: %v", rs)
	}
	d.Running = trust.Running{Untrusted: true}
	for _, class := range []model.SecretClass{"project", "release", "", "x"} {
		j := trust.JobFacts{Trust: "owner", Network: "restricted", SecretClasses: []model.SecretClass{class}}
		d.SecretClasses = []model.SecretClass{class}
		if rs := trust.Admissible(j, d, eff); !aHas(rs, trust.IsolationCotenancy) {
			t.Errorf("secret class %q beside an untrusted attempt: %v", class, rs)
		}
	}
}

// A19: a collaborator head (TM-45) and the trust minimums; the pushed-branch trust never lowers the floor.
func TestA19_CollaboratorAndOwnerFloors(t *testing.T) {
	eff := aPolicy(t)
	for _, iso := range []model.Isolation{"process", "", "bogus"} {
		d := aSandboxNode()
		d.Isolation = iso
		for _, class := range []model.TrustClass{"collaborator", "owner", "internal", "bogus"} {
			j := trust.JobFacts{Trust: class, Network: "none"}
			rs := trust.Admissible(j, d, eff)
			if class != "owner" && class != "internal" && !aHas(rs, trust.IsolationBelowMinimum) {
				t.Errorf("%s job admitted on a %q node: %v", class, iso, rs)
			}
		}
	}
	// the owner exception needs ALL of: owner trust, operator node, personal own node, personal coordinator, own run
	d := aSandboxNode()
	d.Isolation = "process"
	d.UIDSeparation = false
	for name, j := range map[string]trust.JobFacts{
		"collaborator with own-run flag": {Trust: "collaborator", PersonalOwnRun: true, Network: "none"},
		"internal without own run":       {Trust: "internal", Network: "none"},
		"unknown with own-run flag":      {Trust: "Owner", PersonalOwnRun: true, Network: "none"},
	} {
		d.Ownership, d.PersonalOwnNode, d.CoordinatorMode = "operator", true, "personal"
		if rs := trust.Admissible(j, d, eff); len(rs) == 0 {
			t.Errorf("%s: admitted on a process node without UID separation", name)
		}
	}
	for _, v := range []model.Isolation{"", "bogus", "PROCESS"} {
		if got := trust.RankIsolation(trust.RetryIsolation(v, "container")); got < trust.RankIsolation("ephemeral-vm") {
			t.Errorf("RetryIsolation(%q, container) rank %d: an unknown previous level must not lower the floor", v, got)
		}
	}
}

func aFact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

// aRunner builds a placeable runner the way the world builder derives one from a capability.
func aRunner(id string, iso model.Isolation, deploy bool) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	os, arch, slots := "linux", "amd64", 4
	c.Platform.OS.Value, c.Platform.Arch.Value, c.Availability.Slots.Value = &os, &arch, &slots
	cpu, mem := float64(8), int64(8192)
	c.Resources.CPU.Value, c.Resources.MemMB.Value = &cpu, &mem
	c.Identity.ID, c.Identity.Provider = id, "operator"
	loc := "lan"
	c.Location.Kind.Value = &loc
	c.DeployHost.Matched = deploy
	c.Trust.Accepts = aFact(aAllTrust)
	c.Trust.Isolation = aFact(iso)
	c.Trust.Network = aFact([]model.NetworkScope{"none", "restricted"})
	c.Trust.SecretClasses = aFact([]model.SecretClass{})
	c.Trust.PrivacyZone = aFact(model.PrivacyZone("private-infrastructure"))
	c.Identity.Ownership = aFact("operator")
	c.Separation.UIDSeparation = aFact(true)
	r := sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: slots, CPU: 8, MemMB: 8192}, Capability: c}
	r.Decl = sched.DeclarationFromCapability(c, false, "")
	return r
}

func aJob(key string, attempt int) sched.Job {
	return sched.Job{Key: key, ID: key, Name: key, Attempt: attempt, Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "none", TimeoutMs: 60000, Project: "project"}
}

func aWorld(prev map[string]model.Isolation, rs ...sched.Runner) sched.World {
	return sched.World{Runners: rs, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, PreviousIsolation: prev}
}

func aPlaced(w sched.World, j sched.Job) string {
	res := sched.Place(w, []sched.Job{j})
	if len(res.Placements) == 0 {
		return ""
	}
	return res.Placements[0].RunnerID
}

// A20: the floor holds when the input is there; a deploy host is never chosen.
func TestA20_RetryFloorAndDeployHost(t *testing.T) {
	low, high := aRunner("a-container", "container", false), aRunner("b-vm", "ephemeral-vm", false)
	if got := aPlaced(aWorld(nil, low, high), aJob("j", 1)); got == "" {
		t.Fatal("control: a first attempt must place")
	}
	for _, prev := range []model.Isolation{"ephemeral-vm", "bogus", "EPHEMERAL-VM"} {
		if got := aPlaced(aWorld(map[string]model.Isolation{"j": prev}, low, high), aJob("j", 2)); got != "b-vm" {
			t.Errorf("previous %q: the retry was placed on %q, want the ephemeral-vm node", prev, got)
		}
	}
	dep := aRunner("deploy", "ephemeral-vm", true)
	if got := aPlaced(aWorld(nil, dep), aJob("k", 1)); got != "" {
		t.Errorf("a deploy host was chosen: %q", got)
	}
}
