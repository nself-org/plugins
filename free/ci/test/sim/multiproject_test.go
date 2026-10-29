//go:build integration

package sim

// Purpose: P7-NODE-30 acceptance, scenario "several projects on one fleet" (Epic NODE D19). Projects A and B
// are two git repositories; the fleet is mac1 (label xcode), vps-a (label project=a, authorized for A only)
// and vps-shared (authorized for A and B). Labels and authorizations are set through the same operations
// `nself ci nodes label` and `nodes projects` run, A's project-scope policy `sched.overrides.labels:
// [project=a]` is written through the policy writer `ci policy set --scope project` uses, and the world
// and the placement are the ones `ci plan` builds. A's jobs land only on vps-a, B's never on vps-a, and an A
// job that also needs xcode waits with labels.mismatch whose explanation names the override.
// Inputs:  git on PATH, no network, no processes beyond git.
// Outputs: pass or fail.
// Constraints: INTEGRATION=1; the world label source is process-wide, so this is the only test of the
//          package that registers it; tests do not run in parallel.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// projectRepo makes a git repository with one commit and returns its root and repo id.
func projectRepo(t *testing.T, run policy.Runner, name string) (root, id string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte(name), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "f.txt")
	gitIn(t, root, "commit", "-q", "-m", "init "+name)
	id = policy.ProjectIDGit(context.Background(), run, root)
	if id == "" {
		t.Fatalf("no repo id for %s", root)
	}
	return root, id
}

// registerFleetNode registers one linux node the owner's jobs may use, online, with the trust a dedicated
// operator node has; the vps fixture supplies a valid capability document.
func registerFleetNode(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	raw, err := os.ReadFile("../../internal/model/testdata/capability/valid/vps.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	lan, owned, uid, slots := "lan", "owned", true, 4
	c.Identity.ID, c.Identity.Name, c.Identity.Labels = id, id, nil
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	c.Availability.Slots.Value = &slots
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{}} // enrolled by the operator, not merely discovered
	reg := registry.New(st)
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	// the way `nodes add` stores an enrolled node: not merely discovered, no project yet
	if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	trust := model.CapabilityTrust{
		Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     assigned(model.Isolation("container")),
		Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: assigned([]model.SecretClass{"none"}),
		PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, id, trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
}

func fleetJob(key, project string, requires ...string) sched.Job {
	return sched.Job{Key: key, ID: key, Project: project, Pipeline: sched.Pipeline{ID: "pipe-" + project, Support: []string{"linux/arm64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/arm64", CPU: 1, MemMB: 128, Labels: requires},
		Trust:        "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}

// explain renders every alternative's reasons for a failure message.
func explain(r sched.Result) string {
	out := ""
	for _, x := range r.Explanations {
		out += x.JobKey + " " + x.Status + ":"
		for _, a := range x.Alternatives {
			out += " [" + a.RunnerID
			for _, q := range a.Reasons {
				out += " " + q.Code + "(" + q.Detail + ")"
			}
			out += "]"
		}
	}
	return out
}

func TestSeveralProjects(t *testing.T) {
	integration(t)
	ctx := context.Background()
	run := policy.Runner(report.Capture)
	rootA, idA := projectRepo(t, run, "project-a")
	rootB, idB := projectRepo(t, run, "project-b")
	if idA == idB {
		t.Fatal("two repositories share a repo id")
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, n := range []string{"mac1", "vps-a", "vps-shared"} {
		registerFleetNode(t, st, n)
	}
	resolve := func(c context.Context, dir string) (string, bool) {
		id := policy.ProjectIDGit(c, run, dir)
		return id, id != ""
	}
	env := labels.Env{Store: st, Registry: registry.New(st), Actor: "cli:test", ResolveDir: resolve}
	if err = labels.RegisterSource(env.Store); err != nil {
		t.Fatal(err)
	}
	for node, set := range map[string][]string{"mac1": {"xcode"}, "vps-a": {"project=a"}} {
		if _, err = env.Edit(ctx, node, set, nil); err != nil {
			t.Fatal(err)
		}
	}
	// A directory argument resolves to the repo id evidence uses; a bare id is taken as given.
	for node, args := range map[string][]string{"mac1": {rootA, rootB}, "vps-a": {rootA}, "vps-shared": {rootA, idB}} {
		added, e := env.AddProjects(ctx, node, args, false)
		if e != nil || len(added) != len(args) {
			t.Fatalf("authorize %s: %v %v", node, added, e)
		}
	}
	if got, _ := env.Projects(ctx, "vps-a"); len(got) != 1 || got[0] != idA {
		t.Fatalf("vps-a is authorized for %v, want only A", got)
	}
	// A's project-scope policy, written the way `ci policy set --scope project` writes it.
	svc := roles.New(st, &audit.Log{St: st})
	w := &policy.Writer{Store: st, Log: svc.Log, Run: run}
	if _, err = w.Apply(ctx, svc.Operator("cli:test"), policy.Change{Op: policy.OpSet, Scope: "project", Root: rootA, Project: idA,
		Key: schedpolicy.KeyLabels, Value: json.RawMessage(`["project=a"]`)}); err != nil {
		t.Fatal(err)
	}
	policyOf := func(root, id string) sched.Policy {
		t.Helper()
		got, e := (&policy.Resolver{Store: st, Run: run}).Resolve(ctx, policy.Repo{Root: root, Project: id, Visibility: "unknown", Class: "owner"}, policy.Revision{})
		if e != nil {
			t.Fatal(e)
		}
		p, e := schedpolicy.Placement(&got.Effective)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	place := func(root, id string, jobs ...sched.Job) sched.Result {
		t.Helper()
		sp := policyOf(root, id)
		deps := world.Deps{Store: st, Policy: func(context.Context, string) (sched.Policy, error) { return sp, nil }, Clock: time.Now}
		wd, _, e := world.Build(ctx, deps, jobs[0].Pipeline)
		if e != nil {
			t.Fatal(e)
		}
		return sched.Place(wd, jobs)
	}

	resA := place(rootA, idA, fleetJob("a1", idA), fleetJob("a2", idA), fleetJob("a3", idA))
	if len(resA.Placements) != 3 {
		t.Fatalf("A's jobs: %d placed, explanations %s", len(resA.Placements), explain(resA))
	}
	for _, p := range resA.Placements {
		if p.RunnerID != "vps-a" {
			t.Fatalf("A's job %s landed on %s, want only vps-a", p.JobKey, p.RunnerID)
		}
	}
	resB := place(rootB, idB, fleetJob("b1", idB), fleetJob("b2", idB), fleetJob("b3", idB), fleetJob("b4", idB))
	if len(resB.Placements) != 4 {
		t.Fatalf("B's jobs: %d placed, explanations %+v", len(resB.Placements), resB.Explanations)
	}
	for _, p := range resB.Placements {
		if p.RunnerID == "vps-a" {
			t.Fatalf("B's job %s landed on vps-a, which is not authorized for B", p.JobKey)
		}
	}
	// A job of A that also needs xcode: mac1 has xcode but not project=a, vps-a the reverse; it waits.
	resX := place(rootA, idA, fleetJob("a-xcode", idA, "xcode"))
	if len(resX.Placements) != 0 || len(resX.Explanations) != 1 {
		t.Fatalf("an xcode job of A was placed: %+v", resX)
	}
	named := map[string]bool{}
	for _, alt := range resX.Explanations[0].Alternatives {
		for _, r := range alt.Reasons {
			if r.Code == string(model.LabelsMismatch) {
				named[alt.RunnerID+":"+r.Detail] = true
			}
		}
	}
	// project=a is not in the job's own requires.labels ([xcode]): the explanation names it only because
	// A's project-scope policy sched.overrides.labels carries it, so the override is what the text points at.
	if got := policyOf(rootA, idA).Overrides.Labels; len(got) != 1 || got[0] != "project=a" {
		t.Fatalf("A's effective %s = %v", schedpolicy.KeyLabels, got)
	}
	for _, r := range resX.Explanations[0].Alternatives[0].Reasons {
		if r.Detail == "project=a" && r.Code != string(model.LabelsMismatch) {
			t.Fatalf("the override is reported as %s, want %s", r.Code, model.LabelsMismatch)
		}
	}
	for _, want := range []string{"mac1:project=a", "vps-a:xcode", "vps-shared:xcode", "vps-shared:project=a"} {
		if !named[want] {
			t.Fatalf("labels.mismatch %q missing from the explanation: %v", want, named)
		}
	}
	// B's xcode job has no override and runs on mac1, the only node with the label that B may use.
	resBX := place(rootB, idB, fleetJob("b-xcode", idB, "xcode"))
	if len(resBX.Placements) != 1 || resBX.Placements[0].RunnerID != "mac1" {
		t.Fatalf("B's xcode job: %+v", resBX)
	}
}
