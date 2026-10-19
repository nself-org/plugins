package isolation

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type fakeProfile struct {
	level model.Isolation
	av    Availability
}

func (f fakeProfile) Level() model.Isolation                    { return f.level }
func (f fakeProfile) Probe(context.Context) Availability        { return f.av }
func (f fakeProfile) Prepare(context.Context, Attempt) error    { return nil }
func (f fakeProfile) Wrap(s exec.JobSpec) (exec.JobSpec, error) { return s, nil }
func (f fakeProfile) Teardown(context.Context, Attempt) error   { return nil }

func TestEffectiveOnlyLowers(t *testing.T) {
	for _, tc := range []struct{ declared, probed, want model.Isolation }{
		{"container", "container", "container"},
		{"sandboxed-container", "container", "container"},
		{"container", "sandboxed-container", "container"}, // a higher probe never raises
		{"container", "vm", "container"},
		{"container", "process", "process"},
		{"container", "", ""},
		{"container", "bogus", ""},
		{"bogus", "container", ""},
	} {
		if got := Effective(tc.declared, tc.probed); got != tc.want {
			t.Errorf("Effective(%q,%q)=%q want %q", tc.declared, tc.probed, got, tc.want)
		}
	}
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code + "/" + e.Reason
	}
	return ""
}

func TestSelectProcessIsBaseline(t *testing.T) {
	r := &Registry{}
	for _, level := range []model.Isolation{"", "process"} {
		p, av, err := r.Select(context.Background(), level)
		if p != nil || err != nil || av.Level != "process" {
			t.Fatalf("%q: %v %v %v", level, p, av, err)
		}
	}
}

func TestSelectNeverFallsBack(t *testing.T) {
	ctx := context.Background()
	r := &Registry{}
	if _, _, err := r.Select(ctx, "container"); code(err) != "E678/"+ReasonNoProfile {
		t.Fatalf("empty registry: %v", err)
	}
	r.Register(fakeProfile{"container", Availability{Reason: ReasonRuntimeMissing}})
	p, _, err := r.Select(ctx, "container")
	if p != nil || code(err) != "E678/"+ReasonRuntimeMissing {
		t.Fatalf("unavailable runtime: %v %v", p, err)
	}
	if _, _, err := r.Select(ctx, "sandboxed-container"); code(err) == "" {
		t.Fatal("a container profile satisfied a sandboxed request")
	}
	if _, _, err := r.Select(ctx, "not-a-level"); code(err) != "E678/"+ReasonUnknownLevel {
		t.Fatalf("unknown level: %v", err)
	}
}

func TestSelectPicksLowestSufficientAndNeverRaisesProbe(t *testing.T) {
	ctx := context.Background()
	r := &Registry{}
	r.Register(fakeProfile{"sandboxed-container", Availability{Level: "sandboxed-container"}})
	r.Register(fakeProfile{"container", Availability{Level: "vm"}}) // probe higher than declared
	p, av, err := r.Select(ctx, "container")
	if err != nil || p.Level() != "container" || av.Level != "container" {
		t.Fatalf("got %v %v %v", p, av, err)
	}
	p, _, err = r.Select(ctx, "sandboxed-container")
	if err != nil || p.Level() != "sandboxed-container" {
		t.Fatalf("sandboxed: %v %v", p, err)
	}
	if _, _, err := r.Select(ctx, "vm"); code(err) == "" {
		t.Fatal("a probe raised a level")
	}
}

func TestSelectProbeLowersDeclared(t *testing.T) {
	r := &Registry{}
	r.Register(fakeProfile{"sandboxed-container", Availability{Level: "container"}})
	if _, _, err := r.Select(context.Background(), "sandboxed-container"); code(err) == "" {
		t.Fatal("lowered profile still satisfied the declared level")
	}
	p, _, err := r.Select(context.Background(), "container")
	if err != nil || p == nil {
		t.Fatalf("lowered profile should still serve container: %v", err)
	}
}

func TestRegisterReplacesSameLevel(t *testing.T) {
	r := &Registry{}
	r.Register(fakeProfile{"container", Availability{Reason: "old"}})
	r.Register(fakeProfile{"container", Availability{Level: "container"}})
	if n := len(r.Profiles()); n != 1 {
		t.Fatalf("%d profiles", n)
	}
	if _, _, err := r.Select(context.Background(), "container"); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultNetworkPlanner(t *testing.T) {
	pl := DefaultNetworkPlanner()
	for scope, want := range map[model.NetworkScope]string{"": "none", "none": "none", "lan": "bridge"} {
		got, err := pl.Plan(scope)
		if err != nil || got.Network != want {
			t.Errorf("%q: %v %v", scope, got, err)
		}
	}
	for _, scope := range []model.NetworkScope{"restricted", "internet", "privileged", "bogus"} {
		if _, err := pl.Plan(scope); code(err) != "E679/"+ReasonScope {
			t.Errorf("%q: %v", scope, err)
		}
	}
}

func TestCodesRegistered(t *testing.T) {
	if p := model.CodeProblems(); len(p) != 0 {
		t.Fatal(p)
	}
}

type markerPlanner struct{}

func (markerPlanner) Plan(model.NetworkScope) (NetworkPlan, error) {
	return NetworkPlan{Network: "marker"}, nil
}

// InstallNetworkPlanner replaces the process-wide planner, also for planners built before the install.
func TestInstallNetworkPlanner(t *testing.T) {
	early := DefaultNetworkPlanner()
	InstallNetworkPlanner(markerPlanner{})
	t.Cleanup(func() { InstallNetworkPlanner(nil) })
	if got, err := early.Plan("restricted"); err != nil || got.Network != "marker" {
		t.Fatalf("%+v %v", got, err)
	}
	ap, ok := early.(AttemptPlanner)
	if !ok {
		t.Fatal("the default planner is not an AttemptPlanner")
	}
	if got, err := ap.PlanAttempt(context.Background(), "docker", Attempt{ID: "a1"}, "restricted"); err != nil || got.Network != "marker" {
		t.Fatalf("stateless fallback: %+v %v", got, err)
	}
	InstallNetworkPlanner(nil)
	if _, err := early.Plan("restricted"); code(err) != "E679/"+ReasonScope {
		t.Errorf("after restoring the default: %v", err)
	}
}
