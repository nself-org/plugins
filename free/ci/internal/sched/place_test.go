package sched

import (
	"encoding/json"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func fixtureCapability(project string) model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{project}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions = []int{1}
	c.Protocol.Min = 1
	return c
}
func fixtureRunner(id, loc string) Runner {
	c := fixtureCapability("project")
	c.Platform.OS.Value = ptr("linux")
	c.Platform.Arch.Value = ptr("amd64")
	c.Resources.CPU.Value = ptr(float64(8))
	c.Resources.MemMB.Value = ptr(int64(8192))
	c.Identity.Labels = []string{"arch=amd64"}
	c.Location.Kind.Value = &loc
	return Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: Avail{Slots: 1, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: loc == "local", NoLANRoute: true}}
}
func fixtureJob(key string) Job {
	return Job{Key: key, ID: key, Project: "project", Pipeline: Pipeline{ID: "pipe", Support: []string{"linux/amd64"}}, Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}
func fixtureWorld(rs ...Runner) World {
	return World{Runners: rs, Policy: Policy{Preset: "balanced"}, Costs: map[string]Cost{}, Previous: map[string]string{}}
}
func containsCode(rs []model.PlacementReasonDetail, code string) bool {
	for _, r := range rs {
		if r.Code == code {
			return true
		}
	}
	return false
}

func TestOrderKept(t *testing.T) {
	w := fixtureWorld(fixtureRunner("a", "local"))
	jobs := []Job{fixtureJob("first"), fixtureJob("second")}
	got := Place(w, jobs)
	if len(got.Placements) != 1 || got.Placements[0].JobKey != "first" || got.Explanations[1].Status != "waiting" {
		t.Fatalf("order/capacity: %+v", got)
	}
	if w.Runners[0].Avail.Slots != 1 {
		t.Fatal("mutated caller world")
	}
}
func TestDeterminism(t *testing.T) {
	base := []Runner{fixtureRunner("z", "lan"), fixtureRunner("a", "lan"), fixtureRunner("m", "lan")}
	want, _ := json.Marshal(Place(fixtureWorld(base...), []Job{fixtureJob("job")}))
	rnd := rand.New(rand.NewSource(73))
	for i := 0; i < 1000; i++ {
		rs := append([]Runner(nil), base...)
		rnd.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
		got, _ := json.Marshal(Place(fixtureWorld(rs...), []Job{fixtureJob("job")}))
		if string(got) != string(want) {
			t.Fatalf("permutation %d differs", i)
		}
	}
}
func TestSaturation(t *testing.T) {
	if v, o := Add(math.MaxInt64, 1); !o || v != math.MaxInt64 {
		t.Fatal("add")
	}
	if v, o := Mul(math.MaxInt64, 2); !o || v != math.MaxInt64 {
		t.Fatal("mul")
	}
	r := fixtureRunner("p", "lan")
	r.Class = "metered"
	w := fixtureWorld(r)
	w.Policy.PaidEnabled = true
	w.Budgets.Day = Budget{Remaining: math.MaxInt64, Set: true}
	w.Costs = map[string]Cost{"p": {Class: "metered", RateMicro: math.MaxInt64, BillingIncrementMs: 1}}
	j := fixtureJob("job")
	j.TimeoutMs = math.MaxInt64
	got := Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.BudgetExceeded)) {
		t.Fatalf("overflow admitted: %+v", got)
	}
}
func TestEligibilityReasons(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Capability.Platform.OS.Value = ptr("darwin")
	r.Decl.Accepts = nil
	r.Decl.UIDSeparation = false
	got := Place(fixtureWorld(r), []Job{fixtureJob("job")})
	rs := got.Explanations[0].Alternatives[0].Reasons
	if !containsCode(rs, string(model.PlatformMismatch)) || !containsCode(rs, "trust.not_accepted") || !containsCode(rs, "isolation.uid_separation") {
		t.Fatalf("missing simultaneous reasons: %+v", rs)
	}
	for _, v := range rs {
		if !model.PlacementReason(v.Code).Valid() && !trustReason(v.Code) {
			t.Fatalf("unknown reason %s", v.Code)
		}
	}
}
func trustReason(s string) bool {
	for r := range trust.ReasonMeaning {
		if string(r) == s {
			return true
		}
	}
	return false
}
func TestAntiAffinity(t *testing.T) {
	a, b := fixtureRunner("a", "lan"), fixtureRunner("b", "lan")
	j := fixtureJob("job")
	j.Attempt = 2
	w := fixtureWorld(a, b)
	w.Previous[j.Key] = "a"
	got := Place(w, []Job{j})
	if len(got.Placements) != 1 || got.Placements[0].RunnerID != "b" || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.RetryAntiAffinity)) {
		t.Fatalf("anti-affinity: %+v", got)
	}
	got = Place(fixtureWorld(a), []Job{j})
	if len(got.Placements) != 1 {
		t.Fatalf("single runner fallback: %+v", got)
	}
}
func TestSupportGuard(t *testing.T) {
	j := fixtureJob("j")
	j.Requirements.Platform = "darwin/arm64"
	got := Place(fixtureWorld(fixtureRunner("a", "local")), []Job{j})
	if got.ErrorCode != "E690" || len(got.Placements) != 0 {
		t.Fatalf("guard: %+v", got)
	}
}
func TestFastPath(t *testing.T) {
	j := fixtureJob("j")
	r := fixtureRunner("local", "local")
	got := Place(fixtureWorld(r), []Job{j})
	if !got.FastPath.Taken || got.FastPath.Reason != "no_remote" || got.Placements[0].RunnerID != "local" {
		t.Fatalf("fast path: %+v", got)
	}
}
func TestRequirements(t *testing.T) {
	a := fixtureRunner("a", "lan")
	a.Capability.Identity.Labels = []string{"xcode", "arch=arm64"}
	a.Capability.Tools.Toolchains = map[string]model.Fact[string]{"xcode": {Value: ptr("16")}}
	a.Capability.Resources.Accelerators.Value = ptr([]string{"gpu"})
	j := fixtureJob("j")
	j.Requirements.Labels = []string{"xcode", "arch=arm64"}
	j.Requirements.Tools = []string{"xcode"}
	j.Accelerators = []string{"gpu"}
	if got := Place(fixtureWorld(a), []Job{j}); len(got.Placements) != 1 {
		t.Fatalf("requirements: %+v", got)
	}
	a.Capability.Identity.Labels = []string{"arch=amd64"}
	a.Capability.Tools.Toolchains = nil
	a.Capability.Resources.Accelerators.Value = nil
	rs := Place(fixtureWorld(a), []Job{j}).Explanations[0].Alternatives[0].Reasons
	for _, code := range []model.PlacementReason{model.LabelsMismatch, model.ToolsMissing, model.ResourcesInsufficient} {
		if !containsCode(rs, string(code)) {
			t.Fatalf("missing %s: %+v", code, rs)
		}
	}
}
func TestStatus(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Avail.Slots = 0
	j := fixtureJob("j")
	got := Place(fixtureWorld(r), []Job{j})
	if got.Explanations[0].Status != "waiting" || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.CapacityBusy)) {
		t.Fatalf("busy: %+v", got)
	}
	r.Capability.Platform.OS.Value = ptr("darwin")
	got = Place(fixtureWorld(r), []Job{j})
	if got.Explanations[0].Status != "unplaceable" {
		t.Fatalf("closed: %+v", got)
	}
	w := fixtureWorld(r)
	w.OpenRegistry = true
	got = Place(w, []Job{j})
	if got.Explanations[0].Status != "waiting" || !containsCode(got.Explanations[0].Reasons, string(model.PlatformNoneAvailable)) {
		t.Fatalf("open: %+v", got)
	}
}
func TestWorldApply(t *testing.T) {
	w := fixtureWorld(fixtureRunner("r", "lan"))
	w.Apply(Placement{RunnerID: "r", CPU: 1, MemMB: 128})
	if !reflect.DeepEqual(w.Runners[0].Avail, Avail{Slots: 0, CPU: 7, MemMB: 8064}) {
		t.Fatal(w.Runners[0].Avail)
	}
}
