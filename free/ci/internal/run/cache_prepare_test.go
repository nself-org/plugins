package run

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestPrepareFailureClosesCache(t *testing.T) {
	cache := &fakeCache{}
	r := passRig(t)
	r.C.Opts.Services = []string{"db"}
	r.C.Hooks.Cache = cache
	r.C.Hooks.Services = failingServices{}
	res := r.run(t)
	if res.Exit != 2 || cache.after.Load() != 1 {
		t.Fatalf("exit %d cache After %d", res.Exit, cache.after.Load())
	}
}

func TestResolvedCachePolicyWins(t *testing.T) {
	root := t.TempDir()
	if !topLevelDisabled(root, &model.CacheConfig{Disable: true}) {
		t.Fatal("resolved external config disable ignored")
	}
	if topLevelDisabled(root, &model.CacheConfig{}) {
		t.Fatal("empty resolved policy disables cache")
	}
	r := passRig(t)
	r.C.Plan.Cache = &model.CacheConfig{Disable: true}
	r.C.Hooks.AttemptStarts = []AttemptStartHook{startFn(func(a AttemptInfo) error {
		if a.PipelineCache == nil || !a.PipelineCache.Disable {
			t.Error("resolved cache policy not passed to attempt")
		}
		return nil
	})}
	r.run(t)
}

type failingServices struct{}

func (failingServices) Up(context.Context, ServicesRequest) (ServicesSetup, error) {
	return ServicesSetup{}, errors.New("service unavailable")
}
func (failingServices) Down(context.Context, ServicesRequest) error { return nil }
