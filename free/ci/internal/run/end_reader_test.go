package run

// D-1008: the coordinator reads the end revision while an evidence builder that
// implements EndReader reads the worktree; Complete receives that reading.

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

type endReadBuilder struct {
	reads    atomic.Int32
	gotEnd   any
	endToken string
}

func (b *endReadBuilder) Begin(context.Context, string, Revision, Capture) any { return nil }
func (b *endReadBuilder) ReadEnd(context.Context, string, Capture) any {
	b.reads.Add(1)
	return b.endToken
}
func (b *endReadBuilder) Complete(_ context.Context, _ any, f EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	b.gotEnd = f.End
	return ev, nil
}

func TestEndReaderResultReachesComplete(t *testing.T) {
	yes := true
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{
		"unit": {Kind: "test", Run: []string{"true"}, Required: &yes}}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	b := &endReadBuilder{endToken: "worktree-at-end"}
	c := &Coordinator{Opts: Options{Root: t.TempDir(), Home: t.TempDir()}, Plan: pl, ConfigDigest: "cfg", Out: io.Discard, Poll: 10 * time.Millisecond,
		Hooks: Hooks{Evidence: b},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	if _, err = c.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.reads.Load() != 1 || b.gotEnd != "worktree-at-end" {
		t.Fatalf("ReadEnd ran %d times, Complete saw %v", b.reads.Load(), b.gotEnd)
	}
}
