package run

// Purpose: D-1265. Importing the dispatch hook must register the background
// admission step and install its queue probe, fed by the run dispatchers, so a
// background job is held back while an interactive job waits.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	bgci "github.com/nself-org/plugins/free/ci/internal/background"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestBackgroundStepIsRegisteredByTheHook(t *testing.T) {
	if !dispatch.HasStep(bgci.StepName) {
		t.Fatal("the background admission step is not registered in the run binary")
	}
}

func TestBackgroundProbeSeesQueuedInteractiveWork(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Poll: 5 * time.Millisecond,
		World: func(context.Context, sched.Pipeline) (sched.World, string, error) {
			return sched.World{}, "", errors.New("no world: the job stays queued")
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	runDispatchers.Store("probe-run", &runDispatch{d: d})
	t.Cleanup(func() { runDispatchers.Delete("probe-run") })

	bgStep := func() error {
		job := sched.Job{Key: "bg", Pipeline: sched.Pipeline{Band: bgci.BandName}}
		return bgci.Step(t.Context(), dispatch.View{Request: dispatch.Request{Runner: sched.Runner{ID: "local"}, Job: job}})
	}
	reason := func(err error) model.PlacementReason {
		var r *dispatch.Refusal
		if errors.As(err, &r) {
			return r.Reason
		}
		return ""
	}
	if reason(bgStep()) == model.BackgroundInteractiveQueued {
		t.Fatal("nothing interactive is queued yet")
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = d.Submit(ctx, dispatch.Job{AttemptID: "fg", Sched: sched.Job{Key: "fg", ID: "fg", Pipeline: sched.Pipeline{ID: "p", Band: "interactive"}}})
	}()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(5 * time.Second)
	for reason(bgStep()) != model.BackgroundInteractiveQueued {
		if time.Now().After(deadline) {
			t.Fatalf("a queued interactive job must refuse the background step, got %v", bgStep())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
