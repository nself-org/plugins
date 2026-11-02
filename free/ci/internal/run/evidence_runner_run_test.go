package run

// Regression tests for the runner evidence of a real run: the run loop (not the test) admits every
// attempt, an infra retry and a confirmation rerun included, through a fake dispatcher that places
// each attempt on a node of its own. Removing the admission note in runAttemptN fails them.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// nodeDispatch admits locally, then reports attempt N of a run as admitted on "node-<run>-<N>" with the
// details a dispatcher would have noted. It remembers the attempts it placed.
type nodeDispatch struct {
	mu  sync.Mutex
	ids []string
}

func (d *nodeDispatch) hook(ctx context.Context, s AttemptSpec) (Admitted, error) {
	adm, err := LocalAdmit(ctx, s)
	if err != nil {
		return adm, err
	}
	adm.RunnerID = fmt.Sprintf("node-%s-%d", SessionFrom(ctx).RunID, s.N)
	NoteRunner(adm.AttemptID, remoteRunner(adm.RunnerID))
	d.mu.Lock()
	d.ids = append(d.ids, adm.AttemptID)
	d.mu.Unlock()
	return adm, nil
}

// requireAttemptNodes asserts the evidence lists attempts 1..n of job, each on the node of that attempt
// of this run, and, because they ran on different nodes, names the coordinator as the run-level runner.
func requireAttemptNodes(t *testing.T, res Result, job string, n int) {
	t.Helper()
	ev, run := res.Summary.Doc, res.Summary.RunID
	if len(ev.Attempts) != n {
		t.Fatalf("run %s: evidence lists %d attempts, want %d: %+v", run, len(ev.Attempts), n, ev.Attempts)
	}
	for i, a := range ev.Attempts {
		if want := fmt.Sprintf("node-%s-%d", run, i+1); a.JobID != job || a.Attempt != i+1 || a.Runner.Node != want || a.Runner.Platform != "linux/arm64" {
			t.Errorf("attempt %d: %+v, want job %s on %s", i+1, a, job, want)
		}
	}
	if ev.Runner != CoordinatorRunner() {
		t.Errorf("run %s: attempts ran on %d nodes, the run-level runner must be the coordinator, got %+v", run, n, ev.Runner)
	}
}

// requireCleared asserts the run left no admissions and no runner notes behind.
func requireCleared(t *testing.T, d *nodeDispatch, runs ...string) {
	t.Helper()
	for _, run := range runs {
		if left := admissionsOf(run); len(left) != 0 {
			t.Errorf("run %s left %d admissions behind", run, len(left))
		}
	}
	for _, id := range d.ids {
		if _, ok := RunnerNote(id); ok {
			t.Errorf("runner note of %s outlived its run", id)
		}
	}
}

func TestEvidenceListsEveryAttemptOfARealRun(t *testing.T) {
	t.Run("an infra retry on another node", func(t *testing.T) {
		t.Cleanup(func() { Dispatch = nil })
		d := &nodeDispatch{}
		Dispatch = d.hook
		m := newMarker(t)
		r := newRig(t, map[string]model.Job{"disk": counting(m, `if [ "$n" -eq 0 ]; then echo "write: no space left on device"; exit 1; fi; exit 0`)}, gitFake{})
		res := r.run(t)
		if o := outcome(res, "disk"); o.State != StatePassed || o.N != 2 {
			t.Fatalf("the job must pass on its retry: %+v\n%s", o, r.Out)
		}
		requireAttemptNodes(t, res, "disk", 2)
		requireCleared(t, d, res.Summary.RunID)
	})
	t.Run("a confirmation rerun", func(t *testing.T) {
		t.Cleanup(func() { Dispatch = nil })
		d := &nodeDispatch{}
		Dispatch = d.hook
		m := newMarker(t)
		r := newRig(t, map[string]model.Job{"unit": counting(m, `if [ "$n" -lt 2 ]; then echo "--- FAIL: TestRace"; exit 1; fi; exit 0`)}, gitFake{})
		r.C.Opts.ConfirmFlaky = 3
		res := r.run(t)
		if o := outcome(res, "unit"); o.State != StateFailed || o.Flaky == nil {
			t.Fatalf("the original failure must stand with a flaky note: %+v\n%s", o, r.Out)
		}
		requireAttemptNodes(t, res, "unit", 3) // the failed first attempt and two confirmation reruns, none of them a job outcome but the last
		requireCleared(t, d, res.Summary.RunID)
	})
}

// TestConcurrentRunsKeepTheirRunnersApart: two runs at once, each retrying onto a second node, record only
// their own attempts, and both leave nothing behind.
func TestConcurrentRunsKeepTheirRunnersApart(t *testing.T) {
	t.Cleanup(func() { Dispatch = nil })
	d := &nodeDispatch{}
	Dispatch = d.hook
	const runs = 2
	results := make([]Result, runs)
	outs := make([]string, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		m := newMarker(t)
		r := newRig(t, map[string]model.Job{"disk": counting(m, `if [ "$n" -eq 0 ]; then echo "write: no space left on device"; exit 1; fi; exit 0`)}, gitFake{})
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := r.C.Run(context.Background())
			if err != nil {
				outs[i] = "Run: " + err.Error() + "\n" + r.Out.String()
				return
			}
			results[i] = res
		}(i)
	}
	wg.Wait()
	ids := []string{}
	for i, res := range results {
		if outs[i] != "" {
			t.Fatal(outs[i])
		}
		requireAttemptNodes(t, res, "disk", 2)
		for _, a := range res.Summary.Doc.Attempts {
			if !strings.HasPrefix(a.Runner.Node, "node-"+res.Summary.RunID+"-") {
				t.Errorf("run %s records a node of another run: %s", res.Summary.RunID, a.Runner.Node)
			}
		}
		ids = append(ids, res.Summary.RunID)
	}
	if ids[0] == ids[1] {
		t.Fatalf("the two runs share the id %s", ids[0])
	}
	requireCleared(t, d, ids...)
	if len(d.ids) != 2*runs {
		t.Errorf("the dispatcher placed %d attempts, want %d", len(d.ids), 2*runs)
	}
}
