package run

// Shared fixtures: a plan of shell jobs, a fake git/gh capture, a fake
// reporter and a Coordinator wired to a private state directory.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

// sh is a required job that runs a shell script.
func sh(script string, deps ...string) model.Job {
	return model.Job{Kind: "test", Run: []string{"sh", "-c", script}, Deps: deps, Required: yes()}
}

func buildPlan(t *testing.T, jobs map[string]model.Job) *plan.Plan {
	t.Helper()
	p, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return p
}

// gitFake answers git and gh calls: dirty and remote are switchable.
type gitFake struct {
	dirty, noRemote, noAuth bool
}

func (g gitFake) capture(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return testSHA + "\n", 0, nil
	case strings.Contains(j, "status --porcelain"):
		if g.dirty {
			return " M file.go\n", 0, nil
		}
		return "", 0, nil
	case strings.Contains(j, "remote get-url"):
		if g.noRemote {
			return "", 1, nil
		}
		return "git@github.com:acme/widgets.git\n", 0, nil
	case strings.Contains(j, "gh auth status"):
		if g.noAuth {
			return "", 1, nil
		}
		return "ok\n", 0, nil
	}
	return "", 0, nil
}

type fakeReporter struct {
	mu     sync.Mutex
	posted []report.Status
}

func (f *fakeReporter) Post(_ context.Context, s report.Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted = append(f.posted, s)
	return nil
}

func (f *fakeReporter) all() []report.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]report.Status(nil), f.posted...)
}

func roomy() ciexec.Capacity {
	return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
}

type rig struct {
	C    *Coordinator
	Out  *bytes.Buffer
	Rep  *fakeReporter
	Home string
}

func newRig(t *testing.T, jobs map[string]model.Job, g gitFake) *rig {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &rig{Out: &bytes.Buffer{}, Rep: &fakeReporter{}, Home: t.TempDir()}
	t.Setenv("NSELF_CI_HOME", r.Home) // the executor sweep reads it; never the user's real home
	r.C = &Coordinator{Opts: Options{Root: root, Home: r.Home}, Plan: buildPlan(t, jobs), ConfigDigest: "cfg", Out: r.Out,
		Capture: g.capture, Reporter: r.Rep, Capacity: roomy, Poll: 10 * time.Millisecond}
	return r
}

func (r *rig) run(t *testing.T) Result {
	t.Helper()
	res, err := r.C.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, r.Out)
	}
	return res
}

func (r *rig) store(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(r.Home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func outcome(res Result, id string) JobOutcome {
	for _, j := range res.Summary.Jobs {
		if j.ID == id {
			return j
		}
	}
	return JobOutcome{}
}
