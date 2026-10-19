package run

// D-1008: the cache clean-up of a finished attempt (After) removes a build-cache
// tree and nothing after it reads that tree, so it runs beside the end of the run
// instead of ahead of it; Run still joins it before it returns.

import (
	"context"
	"io"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

type slowAfterCache struct{ afterDone atomic.Bool }

func (*slowAfterCache) Before(context.Context, AttemptInfo) (CacheSetup, error) {
	return CacheSetup{}, nil
}
func (h *slowAfterCache) After(context.Context, AttemptInfo, Outcome) error {
	time.Sleep(300 * time.Millisecond)
	h.afterDone.Store(true)
	return nil
}

type completeProbe struct {
	afterDoneAtComplete *atomic.Bool
	sawDone             bool
}

func (completeProbe) Begin(context.Context, string, Revision, Capture) any { return nil }
func (p *completeProbe) Complete(_ context.Context, _ any, _ EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	p.sawDone = p.afterDoneAtComplete.Load()
	return ev, nil
}

func TestCacheAfterRunsBesideTheEndOfTheRun(t *testing.T) {
	yes := true
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{
		"unit": {Kind: "test", Run: []string{"true"}, Required: &yes}}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	cache := &slowAfterCache{}
	probe := &completeProbe{afterDoneAtComplete: &cache.afterDone}
	c := &Coordinator{Opts: Options{Root: t.TempDir(), Home: t.TempDir()}, Plan: pl, ConfigDigest: "cfg", Out: io.Discard, Poll: 10 * time.Millisecond,
		Hooks: Hooks{Cache: cache, Evidence: probe},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	if _, err = c.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if probe.sawDone {
		t.Fatal("the run's evidence waited for the cache clean-up of the last attempt")
	}
	if !cache.afterDone.Load() {
		t.Fatal("Run returned before the cache clean-up finished")
	}
}
