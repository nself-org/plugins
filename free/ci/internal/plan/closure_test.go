package plan

// Regression: a dependency excluded by its own `when` must not silently drop
// a selected required job (Epic D28), on any branch.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestRequiredJobKeepsDependencyExcludedByWhen(t *testing.T) {
	build := job()
	build.When = &model.WhenConfig{Branches: []string{"main"}}
	build.Required = bptr(false)
	gate := job("build")
	gate.Required = bptr(true)
	gate.Path = "deep"
	cfg := cfgOf(map[string]model.Job{"build": build, "gate": gate})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	if find(p, "gate") == nil {
		t.Fatalf("required deep job dropped (skipped as %q)", skippedReason(p, "gate"))
	}
	if find(p, "build") == nil {
		t.Errorf("dependency of a required job missing from the plan (skipped as %q)", skippedReason(p, "build"))
	}
}

func TestOptionalJobStillDroppedWithExcludedDependency(t *testing.T) {
	build := job()
	build.When = &model.WhenConfig{Branches: []string{"main"}}
	build.Required = bptr(false)
	child := job("build")
	child.Required = bptr(false)
	cfg := cfgOf(map[string]model.Job{"build": build, "child": child})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	if got := skippedReason(p, "child"); got != "dep.failed" {
		t.Errorf("optional child skipped with %q, want dep.failed", got)
	}
}

// A dependency restored for a required job must not outlive that job: when
// selection drops the required job, the dependency stays excluded by its own
// `when` and cannot be picked on its own.
func TestRestoredDependencyDroppedWithUnselectedRequiredJob(t *testing.T) {
	build := job()
	build.When = &model.WhenConfig{Branches: []string{"main"}}
	build.Required = bptr(false)
	gate := job("build")
	gate.Required = bptr(true)
	gate.Path = "deep"
	cfg := cfgOf(map[string]model.Job{"build": build, "gate": gate})
	t.Cleanup(func() { SetSelector(nil) })
	SetSelector(func(in SelectionInput) (model.Selection, []string) {
		return model.Selection{Mode: "affected", Reason: "selection.affected"}, []string{"build"}
	})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	if find(p, "build") != nil {
		t.Errorf("dependency ran on its own although its `when` excludes it and the required job was not selected")
	}
	if got := skippedReason(p, "gate"); got != "selection.affected" {
		t.Errorf("unselected required job skipped with %q, want selection.affected", got)
	}
}

// The selector gets copies: it cannot mutate the caller's revision.
func TestSelectorCannotMutateRevision(t *testing.T) {
	t.Cleanup(func() { SetSelector(nil) })
	SetSelector(func(in SelectionInput) (model.Selection, []string) {
		for i := range in.Changed {
			in.Changed[i] = "mutated"
		}
		return model.Selection{Mode: "full", Reason: "selection.full_required"}, nil
	})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	cfg := cfgOf(map[string]model.Job{"build": job()})
	mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	if rev.Changed[0] != "a.go" {
		t.Errorf("selector mutated Revision.Changed: %v", rev.Changed)
	}
}
