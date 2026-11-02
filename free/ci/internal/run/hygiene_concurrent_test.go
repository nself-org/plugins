package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// gatedRuntime is a container runtime whose first container listing stops until the test lets it go, so the
// test can start work in the run while the sweep is still listing.
type gatedRuntime struct {
	mu      sync.Mutex
	items   map[string]string // "kind/id" -> attempt label
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedRuntime) add(kind ciexec.ResourceKind, id, attempt string) {
	g.mu.Lock()
	g.items[string(kind)+"/"+id] = attempt
	g.mu.Unlock()
}

func (g *gatedRuntime) has(kind ciexec.ResourceKind, id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.items[string(kind)+"/"+id]
	return ok
}

func (g *gatedRuntime) List(_ context.Context, kind ciexec.ResourceKind, _ string) ([]ciexec.Labelled, error) {
	if kind == ciexec.KindContainer {
		g.once.Do(func() { close(g.entered); <-g.release })
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []ciexec.Labelled
	for k, a := range g.items {
		if id, ok := strings.CutPrefix(k, string(kind)+"/"); ok {
			out = append(out, ciexec.Labelled{ID: id, Attempt: a})
		}
	}
	return out, nil
}

func (g *gatedRuntime) Remove(_ context.Context, kind ciexec.ResourceKind, id, _ string) error {
	g.mu.Lock()
	delete(g.items, string(kind)+"/"+id)
	g.mu.Unlock()
	return nil
}

// The start sweep runs beside the run. Work the run starts while the sweep is still listing, with a root and a
// container the sweep would otherwise find and take for an orphan, is never removed; the orphan is.
func TestConcurrentStartSweepNeverTouchesTheRunsOwnAttempts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	work := filepath.Join(home, "work")
	old := time.Now().Add(-3 * orphanGrace) // old enough that only liveness, not the grace period, protects a root
	rt := &gatedRuntime{items: map[string]string{}, entered: make(chan struct{}), release: make(chan struct{})}
	for _, id := range []string{"own1.a1", "own2.a1", "orphan.a1"} {
		root, err := hygiene.JobRoot(work, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(root, old, old); err != nil {
			t.Fatal(err)
		}
		rt.add(ciexec.KindContainer, "c-"+id, id)
		rt.add(ciexec.KindNetwork, "n-"+id, id)
	}
	h := newHygieneHook()
	h.rt = rt
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own1.a1"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-rt.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the sweep never started listing: it must start at once")
	}
	// the run goes on while the sweep lists: its second attempt starts and its container is created
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own2.a1"}); err != nil {
		t.Fatal(err)
	}
	rt.add(ciexec.KindContainer, "c-live-new", "own2.a1")
	close(rt.release)
	h.joinSweep(context.Background())
	for _, id := range []string{"own1.a1", "own2.a1"} {
		if _, err := os.Lstat(filepath.Join(work, id)); err != nil {
			t.Errorf("the sweep removed the root of the run's own attempt %s", id)
		}
		if !rt.has(ciexec.KindContainer, "c-"+id) || !rt.has(ciexec.KindNetwork, "n-"+id) {
			t.Errorf("the sweep removed a resource of the run's own attempt %s", id)
		}
	}
	if !rt.has(ciexec.KindContainer, "c-live-new") {
		t.Error("the sweep removed the container the run created while it listed")
	}
	if _, err := os.Lstat(filepath.Join(work, "orphan.a1")); err == nil || rt.has(ciexec.KindContainer, "c-orphan.a1") {
		t.Error("the sweep must still remove a real orphan")
	}
	if err := h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "own1.a1"}, {AttemptID: "own2.a1"}}}); err != nil {
		t.Fatal(err)
	}
}

// The run does not wait for the sweep to start, and its result is not written before the sweep ends.
func TestStartSweepDoesNotBlockTheRunAndFinalizeJoinsIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	rt := &gatedRuntime{items: map[string]string{}, entered: make(chan struct{}), release: make(chan struct{})}
	h := newHygieneHook()
	h.rt = rt
	started := make(chan struct{})
	go func() {
		_ = h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "a.a1"})
		close(started)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("AttemptStart waited for the sweep")
	}
	<-rt.entered
	finished := make(chan error, 1)
	go func() {
		finished <- h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "a.a1"}}})
	}()
	select {
	case <-finished:
		t.Fatal("Finalize returned while the start sweep was still running")
	case <-time.After(200 * time.Millisecond):
	}
	close(rt.release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Finalize never returned after the sweep ended")
	}
}

func TestMayAskRuntime(t *testing.T) {
	job := func(iso string) plan.PlannedJob {
		return plan.PlannedJob{ID: "j", Job: model.Job{Isolation: model.Isolation(iso)}}
	}
	for name, tc := range map[string]struct {
		opts Options
		jobs []plan.PlannedJob
		want bool
	}{
		"process only":      {Options{}, []plan.PlannedJob{job(""), job("process")}, false},
		"container job":     {Options{}, []plan.PlannedJob{job("process"), job("container")}, true},
		"sandboxed job":     {Options{}, []plan.PlannedJob{job("sandboxed-container")}, true},
		"--container":       {Options{Container: true}, []plan.PlannedJob{job("process")}, true},
		"untrusted source":  {Options{Trust: "untrusted"}, []plan.PlannedJob{job("process")}, true},
		"owner process run": {Options{Trust: "owner"}, []plan.PlannedJob{job("process")}, false},
	} {
		c := &Coordinator{Opts: tc.opts, Plan: &plan.Plan{Jobs: tc.jobs}}
		if got := c.mayAskRuntime(); got != tc.want {
			t.Errorf("%s: mayAskRuntime=%v want %v", name, got, tc.want)
		}
	}
}
