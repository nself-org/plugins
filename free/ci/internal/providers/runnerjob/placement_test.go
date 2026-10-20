package runnerjob_test

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func schedRunner(id, loc string, decl trust.Declaration) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{testRepo}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS, c.Platform.Arch = known("linux"), known("amd64")
	c.Resources.CPU, c.Resources.MemMB = known(float64(8)), known(int64(8192))
	c.Tools.Docker = known(true)
	c.Availability.PluggedIn, c.Availability.Interactive = known(true), known("idle") // the background band needs a node that is idle and on mains
	c.Identity.ID, c.Identity.Labels = id, []string{"arch=amd64"}
	c.Location.Kind = known(loc)
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: 2, CPU: 8, MemMB: 8192}, Capability: c, Decl: decl}
}

func runnerSchedJob(t *testing.T) sched.Job {
	t.Helper()
	p, err := runnerjob.Synthesize(runnerSpec("41"))
	if err != nil {
		t.Fatal(err)
	}
	c := planview.Context{PipelineID: p.ID, Project: testRepo, Support: []string{"linux/amd64"}, DefaultPlatform: "linux/amd64"}
	j := planview.SchedJob(c, planview.PlannedJob{ID: "job", Name: "job", Job: p.Jobs[0]})
	runnerjob.Constrain(&j)
	return j
}

func placed(r sched.Result) bool { return len(r.Placements) == 1 }

func reasons(r sched.Result) map[string]bool {
	out := map[string]bool{}
	for _, e := range r.Explanations {
		for _, a := range e.Alternatives {
			for _, rr := range a.Reasons {
				out[rr.Code] = true
			}
		}
	}
	return out
}

var untrustedOK = []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}

// TestPlacementRules proves acceptance line 5 (Epic D6, review F6): a forge-runner job waits with reasons
// (runner.remote_only, isolation.below_minimum) when only the local runner and a plain-container node are
// available; a gVisor node whose NoLANRoute holds takes it, with the restricted scope and no secrets; a
// gVisor node without a proven no-LAN-route fact does not.
func TestPlacementRules(t *testing.T) {
	j := runnerSchedJob(t)
	if j.Trust != "untrusted" || j.Network != "restricted" || len(j.SecretClasses) != 0 || !j.RemoteOnly || j.Pipeline.Band != "background" ||
		j.Requirements.IsolationMin != "sandboxed-container" {
		t.Fatalf("job %+v", j)
	}
	local := schedRunner("local", "local", trust.Declaration{Accepts: untrustedOK, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: true, NoLANRoute: true})
	plain := schedRunner("plain", "lan", trust.Declaration{Accepts: untrustedOK, Isolation: "container", Network: "restricted", UIDSeparation: true, Ownership: "operator", NoLANRoute: true})
	world := func(rs ...sched.Runner) sched.World {
		return sched.World{Runners: rs, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	}
	got := sched.Place(world(local, plain), []sched.Job{j})
	if placed(got) {
		t.Fatalf("only the local runner and a plain-container node: the demand must wait, got %+v", got.Placements)
	}
	r := reasons(got)
	if !r["runner.remote_only"] || !r["isolation.below_minimum"] {
		t.Fatalf("want runner.remote_only and isolation.below_minimum, got %v", r)
	}
	gv := trust.Declaration{Accepts: untrustedOK, Isolation: "sandboxed-container", Network: "restricted", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}
	got = sched.Place(world(local, plain, schedRunner("gvisor", "lan", gv)), []sched.Job{j})
	if !placed(got) || got.Placements[0].RunnerID != "gvisor" {
		t.Fatalf("the gVisor node with NoLANRoute must take the demand: %+v %v", got.Placements, reasons(got))
	}
	gv.NoLANRoute = false
	got = sched.Place(world(schedRunner("gvisor-lan", "lan", gv)), []sched.Job{j})
	if placed(got) {
		t.Fatalf("a gVisor node that cannot prove no LAN route must not take an untrusted runner job: %+v", got.Placements)
	}
	withSecrets := j
	withSecrets.SecretClasses = []model.SecretClass{"ci"}
	runnerjob.Constrain(&withSecrets)
	if len(withSecrets.SecretClasses) != 0 || withSecrets.Network != "restricted" {
		t.Fatal("Constrain must drop secret classes")
	}
}
