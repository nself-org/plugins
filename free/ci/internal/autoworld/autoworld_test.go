package autoworld

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func reset(t *testing.T) {
	t.Helper()
	clear := func() { providers.Lock(); providers.byName = map[string]ExclusionFn{}; providers.Unlock() }
	clear()
	t.Cleanup(clear)
}

func reason(code, detail string) model.PlacementReasonDetail {
	return model.PlacementReasonDetail{Code: code, Detail: detail}
}

// TestAutoworldOneOwner: autoworld owns World.Exclusions; a second owner is refused.
func TestAutoworldOneOwner(t *testing.T) {
	err := world.Register(world.Contributor{Name: "intruder", Field: "Exclusions", Fn: func(context.Context, *sched.World) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "Exclusions owned by autoworld and intruder") {
		t.Fatalf("a second Exclusions owner must be refused with E699, got %v", err)
	}
	if err = world.Register(world.Contributor{Name: Name, Field: "Estimates", Fn: func(context.Context, *sched.World) error { return nil }}); err == nil {
		t.Fatal("the contributor name is taken")
	}
}

func TestRegisterExclusionRules(t *testing.T) {
	reset(t)
	ok := func(context.Context, sched.World) (map[string][]model.PlacementReasonDetail, error) { return nil, nil }
	if RegisterExclusion("", ok) == nil || RegisterExclusion("x", nil) == nil {
		t.Fatal("empty name or nil function must be refused")
	}
	if err := RegisterExclusion("pools", ok); err != nil {
		t.Fatal(err)
	}
	if RegisterExclusion("pools", ok) == nil {
		t.Fatal("a duplicate provider name must be refused")
	}
	_ = RegisterExclusion("a-first", ok)
	if got := Providers(); !reflect.DeepEqual(got, []string{"a-first", "pools"}) {
		t.Fatalf("providers run in name order: %v", got)
	}
}

func TestContributeMergesInNameOrder(t *testing.T) {
	reset(t)
	w := &sched.World{}
	if err := contribute(context.Background(), w); err != nil || w.Exclusions != nil {
		t.Fatalf("no provider: Exclusions must stay nil, got %v %v", w.Exclusions, err)
	}
	_ = RegisterExclusion("readiness", func(context.Context, sched.World) (map[string][]model.PlacementReasonDetail, error) {
		return map[string][]model.PlacementReasonDetail{"r1": {reason("node.not_ready", "warming")}}, nil
	})
	_ = RegisterExclusion("pools", func(_ context.Context, in sched.World) (map[string][]model.PlacementReasonDetail, error) {
		if len(in.Runners) != 1 {
			t.Error("a provider sees the world")
		}
		return map[string][]model.PlacementReasonDetail{"r1": {reason("pool.excluded", "not_enrolled")}, "r2": {reason("pool.excluded", "dev_not_allowed")}}, nil
	})
	w = &sched.World{Runners: []sched.Runner{{ID: "r1"}}}
	if err := contribute(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	want := map[string][]model.PlacementReasonDetail{
		"r1": {reason("pool.excluded", "not_enrolled"), reason("node.not_ready", "warming")},
		"r2": {reason("pool.excluded", "dev_not_allowed")},
	}
	if !reflect.DeepEqual(w.Exclusions, want) {
		t.Fatalf("merged = %+v", w.Exclusions)
	}
}

func TestContributeFailsClosed(t *testing.T) {
	reset(t)
	_ = RegisterExclusion("broken", func(context.Context, sched.World) (map[string][]model.PlacementReasonDetail, error) {
		return nil, errors.New("pool store unreadable")
	})
	if err := contribute(context.Background(), &sched.World{}); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a provider error must fail the build and name the provider: %v", err)
	}
	reset(t)
	_ = RegisterExclusion("typo", func(context.Context, sched.World) (map[string][]model.PlacementReasonDetail, error) {
		return map[string][]model.PlacementReasonDetail{"r": {reason("pool.exluded", "x")}}, nil
	})
	if err := contribute(context.Background(), &sched.World{}); err == nil || !strings.Contains(err.Error(), "unknown reason code") {
		t.Fatalf("a code outside the closed set must be refused: %v", err)
	}
}

func TestContributeSurvivesProviderPanic(t *testing.T) {
	reset(t)
	_ = RegisterExclusion("boom", func(context.Context, sched.World) (map[string][]model.PlacementReasonDetail, error) {
		panic("nil pool table")
	})
	if err := contribute(context.Background(), &sched.World{}); err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("a provider panic must become an error naming the provider: %v", err)
	}
}
