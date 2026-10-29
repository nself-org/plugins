package readiness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// source is a fixed readiness answer that counts how often it was asked.
type source struct {
	report Report
	err    error
	calls  int
}

func (s *source) Evaluate(context.Context) (Report, error) { s.calls++; return s.report, s.err }

func notReady(why ...string) Report {
	return Report{Schema: SchemaID, Ready: false, Why: why, YieldedTo: []string{}}
}

func ptr[T any](v T) *T { return &v }

// runner builds a placeable runner; the one called "local" carries world.LocalLabel.
func runner(id string) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Identity.Labels = []string{"arch=amd64"}
	loc := "lan"
	if id == "local" {
		loc = "local"
		c.Identity.Labels = append(c.Identity.Labels, world.LocalLabel)
	}
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: id == "local"}}
}

func job() sched.Job {
	return sched.Job{Key: "job", ID: "job", Project: "project", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}

func worldOf(rs ...sched.Runner) sched.World {
	proof := map[string]sched.NetworkFact{}
	for _, r := range rs {
		proof[r.ID] = sched.NetworkFact{Enforced: []model.NetworkScope{"none", "restricted", "internet"}}
	}
	return sched.World{Network: proof, Runners: rs, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
}

func codes(ds []model.PlacementReasonDetail) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Code)
	}
	return strings.Join(out, ",")
}

func TestNotReadyExclusion(t *testing.T) {
	ctx := context.Background()
	w := worldOf(runner("local"))
	if got := sched.Place(w, []sched.Job{job()}); got.Placements[0].RunnerID != "local" {
		t.Fatalf("baseline: a ready local runner takes the job: %+v", got)
	}

	src := &source{report: notReady("free memory 100 MB below the 1024 MB floor", "yielding to zoom.us")}
	ex, err := ExclusionProvider(src, nil)(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex) != 1 || codes(ex["local"]) != "node.not_ready" || !strings.Contains(ex["local"][0].Detail, "free memory 100 MB") || !strings.Contains(ex["local"][0].Detail, "yielding to zoom.us") {
		t.Fatalf("exclusions %+v", ex)
	}
	w.Exclusions = ex
	got := sched.Place(w, []sched.Job{job()})
	if len(got.Placements) != 0 || got.Explanations[0].Status != "waiting" {
		t.Fatalf("a not-ready local runner must be skipped and waited for: %+v", got)
	}
	shown := false
	for _, a := range got.Explanations[0].Alternatives {
		shown = shown || a.RunnerID == "local" && !a.Eligible && strings.Contains(codes(a.Reasons), "node.not_ready")
	}
	if !shown {
		t.Fatalf("`ci plan` must list node.not_ready for the local runner: %+v", got.Explanations[0])
	}

	// It clears by itself: the next answer is ready, so no exclusion remains.
	src.report = Report{Schema: SchemaID, Ready: true, Why: []string{}, YieldedTo: []string{}}
	if ex, _ = ExclusionProvider(src, nil)(ctx, w); len(ex) != 0 {
		t.Fatalf("a ready host adds no exclusion: %+v", ex)
	}

	// Only the local runner is judged: a remote node keeps its own agent-side facts.
	src.report = notReady("busy")
	two := worldOf(runner("local"), runner("lan1"))
	ex, _ = ExclusionProvider(src, nil)(ctx, two)
	if _, remote := ex["lan1"]; remote || len(ex["local"]) != 1 {
		t.Fatalf("exclusions %+v", ex)
	}
	two.Exclusions = ex
	if p := sched.Place(two, []sched.Job{job()}).Placements; len(p) != 1 || p[0].RunnerID != "lan1" {
		t.Fatalf("the job must go to the ready remote node: %+v", p)
	}

	// No local runner in the world: nothing to judge and nothing sampled.
	src.calls = 0
	if ex, _ = ExclusionProvider(src, nil)(ctx, worldOf(runner("lan1"))); len(ex) != 0 || src.calls != 0 {
		t.Fatalf("a world without a local runner must not sample: %+v calls=%d", ex, src.calls)
	}

	// A failing evaluation fails the world build (fail closed).
	boom := errors.New("policy unreadable")
	if _, err = ExclusionProvider(&source{err: boom}, nil)(ctx, w); !errors.Is(err, boom) {
		t.Fatalf("error %v", err)
	}
}

func TestForegroundFloorKept(t *testing.T) {
	src := &source{report: notReady("busy")}
	w := worldOf(runner("local"))
	foreground := true
	provider := ExclusionProvider(src, func() bool { return foreground })
	ex, err := provider(context.Background(), w)
	if err != nil || len(ex) != 0 || src.calls != 0 {
		t.Fatalf("the operator's own foreground run keeps its local slot: ex=%+v err=%v calls=%d", ex, err, src.calls)
	}
	if got := sched.Place(w, []sched.Job{job()}); len(got.Placements) != 1 || got.Placements[0].RunnerID != "local" {
		t.Fatalf("a foreground run is placed locally: %+v", got)
	}
	foreground = false
	if ex, _ = provider(context.Background(), w); len(ex["local"]) != 1 {
		t.Fatalf("a background run is held back: %+v", ex)
	}
}

func TestRegisterAddsTheProvider(t *testing.T) {
	err := Register(&source{report: notReady("x")}, nil)
	if err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatal(err)
	}
	found := false
	for _, name := range autoworld.Providers() {
		found = found || name == ProviderName
	}
	if !found {
		t.Fatalf("providers %v", autoworld.Providers())
	}
	if err := Register(&source{}, nil); err == nil {
		t.Fatal("a second registration under the same name must fail")
	}
}

func TestProviderKeepsEveryReasonOfALongList(t *testing.T) {
	why := []string{"free memory 100 MB below the 1024 MB floor", "free disk 10 MB below the 2048 MB floor", "CPU idle 3% below the 20% floor",
		"yielding to obs, zoom.us", strings.Repeat("the operator's own words ", 20)}
	ex, err := ExclusionProvider(&source{report: notReady(why...)}, nil)(context.Background(), worldOf(runner("local")))
	if err != nil {
		t.Fatal(err)
	}
	d := ex["local"][0].Detail
	if !strings.Contains(d, "yielding to obs, zoom.us") || !strings.Contains(d, "CPU idle 3%") || len([]rune(d)) > maxDetailLen || strings.Contains(d, "\n") {
		t.Fatalf("detail %q", d)
	}
}

// A registered node can carry the local label when contributors run (the world builder
// strips it afterwards); only the injected runner "local" may be judged.
func TestProviderIgnoresARemoteNodeWithAForgedLocalLabel(t *testing.T) {
	remote := runner("lan1")
	remote.Capability.Identity.Labels = append(remote.Capability.Identity.Labels, world.LocalLabel)
	ex, err := ExclusionProvider(&source{report: notReady("busy")}, nil)(context.Background(), worldOf(runner("local"), remote))
	if err != nil {
		t.Fatal(err)
	}
	if _, judged := ex["lan1"]; judged || len(ex["local"]) != 1 {
		t.Fatalf("exclusions %+v", ex)
	}
}
