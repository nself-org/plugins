package bench

// Purpose: the in-process scenarios: detect, plan, store open and migrate, the
// per-job executor overhead and the host capacity probe. Both the Go
// benchmarks (bench_test.go) and the cibench harness call these, so a number in
// the result JSON and a `go test -bench` number measure the same code.
// Inputs:  a prepared fixture directory.
// Outputs: one function per scenario that does a single iteration.
// Constraints: no scenario reads the operator's home or state.db; every
// iteration starts from the same inputs (the store always opens a fresh file).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Inproc holds the inputs the in-process scenarios share.
type Inproc struct {
	Root  string // fixture repository
	Home  string // scratch directory for stores and executor workspaces
	cfg   model.PipelineConfig
	facts []detect.Fact
	world plan.World
	seq   atomic.Uint64
}

// PrepareInproc loads the fixture's V15 config and detects its facts once.
func PrepareInproc(root, home string) (*Inproc, error) {
	loaded, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return nil, fmt.Errorf("fixture config: %w", err)
	}
	p := &Inproc{Root: root, Home: home, cfg: loaded.Config}
	if err := p.Detect(); err != nil {
		return nil, err
	}
	p.world = plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, Tools: []string{"go", "gofmt", "npm"},
		CPU: runtime.NumCPU(), MemMB: 8192}
	return p, nil
}

// Detect is one detection pass over the fixture (ci.detect).
func (p *Inproc) Detect() error {
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(p.Root), gates.DefaultRegistry().Has)
	if err != nil {
		return err
	}
	p.facts = facts
	return nil
}

// Plan is one planner pass over the detected facts (ci.plan). The planner is
// pure, so the revision and world are fixed values.
func (p *Inproc) Plan() error {
	_, err := plan.Build(plan.Input{Config: p.cfg, Facts: p.facts, World: p.world,
		Revision: plan.Revision{Branch: "main", DefaultBranch: true}, Pipeline: "gate"})
	return err
}

// StoreOpen opens and migrates a fresh state.db, then closes it (ci.store_open).
func (p *Inproc) StoreOpen() error {
	dir := filepath.Join(p.Home, fmt.Sprintf("store-%d", p.seq.Add(1)))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		return err
	}
	return st.Close()
}

// ExecOverhead runs one no-op job through the executor funnel and returns the
// funnel's wall time for it (ci.exec_overhead): workspace, environment, process
// start, process-tree tracking and cleanup, with a job that does no work.
func (p *Inproc) ExecOverhead(ctx context.Context) (Timing, error) {
	argv := []string{"true"}
	n := p.seq.Add(1)
	ex := &ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: ciexec.AllowlistEnv{}}
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: fmt.Sprintf("bench-%d", n), CoordinatorID: "cibench",
		Job: model.Job{Run: argv}, Command: argv, Home: filepath.Join(p.Home, "exec")})
	if res.Err != nil || res.ExitCode != 0 {
		return Timing{}, fmt.Errorf("executor no-op job: exit %d: %v", res.ExitCode, res.Err)
	}
	return Timing{Wall: res.Finished.Sub(res.Started)}, nil
}

// RawSpawn starts the same no-op program directly, with no funnel
// (ci.exec_baseline): the floor ExecOverhead is read against.
func (p *Inproc) RawSpawn(ctx context.Context) (Timing, error) {
	r, err := must(ctx, []string{"true"}, p.Root, baseEnv())
	return r, err
}

// CapacityProbe reads the host quotas the way `nself ci` does before a run
// (ci.capacity_probe).
func CapacityProbe() ciexec.Capacity { return ciexec.Probe() }
