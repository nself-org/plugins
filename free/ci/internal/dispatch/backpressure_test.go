package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func limitsOf(max int64) func(context.Context) (QueueLimits, error) {
	return func(context.Context) (QueueLimits, error) {
		return QueueLimits{StarvationAfterMs: DefaultStarvationAfterMs, MaxDepth: max}, nil
	}
}

func jobIn(project, attempt string) Job {
	j := testQueued(attempt)
	j.Sched.Project = project
	return j
}

// TestBackpressure: a project over queue.max_depth gets E694 with the expected
// wait; another project is unaffected; the refusal queues nothing.
func TestBackpressure(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: limitsOf(3), Slots: func() int { return 2 }})
	for _, id := range []string{"a1", "a2", "a3"} {
		if _, err := d.enqueue(jobIn("alpha", id)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := d.enqueue(jobIn("alpha", "a4"))
	var bp *BackpressureError
	var de *Error
	if !errors.As(err, &bp) || !errors.As(err, &de) || de.Code != fairq.CodeBackpressure || fairq.CodeBackpressure != "E694" {
		t.Fatalf("a full queue must refuse with E694, got %v", err)
	}
	// 3 jobs ahead x 60 s stand-in runtime / 2 slots
	if bp.ExpectedWaitMs != 90000 || bp.WaitConfidence != "unknown" || bp.Depth != 3 || bp.Max != 3 || bp.Project != "alpha" {
		t.Fatalf("backpressure details: %+v", bp)
	}
	if !strings.Contains(err.Error(), "expected_wait_ms=90000") {
		t.Fatalf("the message must carry expected_wait_ms: %v", err)
	}
	if n := len(d.pending(time.Now())); n != 3 {
		t.Fatalf("a refused job must not be queued: %d queued", n)
	}
	if _, err = d.enqueue(jobIn("beta", "b1")); err != nil {
		t.Fatalf("another project is not affected: %v", err)
	}
}

// TestBackpressureSubmitDoesNotBlock: Submit returns E694 at once, with the context still live.
func TestBackpressureSubmitDoesNotBlock(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: limitsOf(1)})
	if _, err := d.enqueue(jobIn("alpha", "a1")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := d.Submit(ctx, jobIn("alpha", "a2"))
	var de *Error
	if !errors.As(err, &de) || de.Code != "E694" || ctx.Err() != nil {
		t.Fatalf("Submit over the cap: %v (ctx %v)", err, ctx.Err())
	}
}

// TestBackpressureWaitFromEstimators: the wait uses the settled runtime estimators.
func TestBackpressureWaitFromEstimators(t *testing.T) {
	st := openStore(t)
	for _, e := range []struct {
		project string
		ms      int64
	}{{"alpha", 100000}, {"beta", 400000}} {
		err := st.TelRecordOutcome(context.Background(), store.TelOutcome{AttemptID: "o-" + e.project, Project: e.project, JobKey: "build", RunnerClass: "local", Result: "passed", RuntimeMs: e.ms, TS: 1},
			func(store.TelEstimator, bool) store.TelEstimator {
				return store.TelEstimator{Project: e.project, JobKey: "build", RunnerClass: "local", RuntimeMs: e.ms, N: minSamples, Updated: 1}
			})
		if err != nil {
			t.Fatal(err)
		}
	}
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: limitsOf(2)})
	for _, id := range []string{"a1", "a2"} {
		if _, err := d.enqueue(jobIn("alpha", id)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := d.enqueue(jobIn("alpha", "a3"))
	var bp *BackpressureError
	if !errors.As(err, &bp) || bp.ExpectedWaitMs != 200000 || bp.WaitConfidence != "estimated" {
		t.Fatalf("2 ahead x the project's 100 s runtime on 1 slot = 200 s: %+v (%v)", bp, err)
	}
	// a project with no estimator of its own uses the mean of all settled ones
	if w, conf := d.expectedWait(context.Background(), "gamma", 1); w != 250000 || conf != "estimated" {
		t.Fatalf("fallback mean: %d %s", w, conf)
	}
}

// TestBackpressureLimits: no cap at 0, a policy error falls back to the default
// cap (never to unlimited), and a drained queue accepts again.
func TestBackpressureLimits(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: limitsOf(0)})
	for _, id := range []string{"a1", "a2", "a3", "a4"} {
		if _, err := d.enqueue(jobIn("alpha", id)); err != nil {
			t.Fatalf("max_depth 0 means no cap: %v", err)
		}
	}
	if got := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil)}).queueLimits(context.Background()); got != DefaultQueueLimits() || got.MaxDepth != 500 || got.StarvationAfterMs != 30*60*1000 {
		t.Fatalf("defaults: %+v", got)
	}
	broken := func(context.Context) (QueueLimits, error) { return QueueLimits{}, errors.New("policy unreadable") }
	if got := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: broken}).queueLimits(context.Background()); got != DefaultQueueLimits() {
		t.Fatalf("a policy error must fall back to the defaults: %+v", got)
	}
	d2 := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Queue: limitsOf(1)})
	e, err := d2.enqueue(jobIn("alpha", "x1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d2.enqueue(jobIn("alpha", "x2")); err == nil {
		t.Fatal("over the cap")
	}
	_, _ = d2.drop(e, errors.New("gone"))
	if _, err = d2.enqueue(jobIn("alpha", "x2")); err != nil {
		t.Fatalf("a drained queue accepts again: %v", err)
	}
}
