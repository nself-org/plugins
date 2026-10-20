package coordinator

// Purpose: the shutdown order of Run with a drain (P7-NODE-20): intake stops first, Config.Drain runs while the
//          running dispatches are still alive, and only then are they cancelled; a failed drain fails Run.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestRunDrainKeepsDispatchesAliveAndPausesIntake(t *testing.T) {
	st := openStore(t, newClock())
	started := make(chan struct{})
	dispatchErr := make(chan error, 1)
	var c *Coordinator
	dctx := make(chan context.Context, 1)
	drained := make(chan struct{})
	c, err := New(Config{Store: st, Insecure: true, Addr: "127.0.0.1:0", Poll: 10 * time.Millisecond,
		Dispatch: func(ctx context.Context, row store.IntakeRow) (RunResult, error) {
			dctx <- ctx
			close(started)
			<-drained // the drain finished: only now may the dispatch see its context end
			select {
			case <-ctx.Done():
				dispatchErr <- nil
			case <-time.After(5 * time.Second):
				dispatchErr <- errors.New("dispatch context never ended after the drain")
			}
			return RunResult{}, ctx.Err()
		},
		Drain: func(ctx context.Context) error {
			if !c.IntakePaused() {
				return errors.New("intake was not paused before the drain")
			}
			time.Sleep(200 * time.Millisecond) // a dispatch that finishes on its own would be done by now
			defer close(drained)
			if d := <-dctx; d.Err() != nil {
				return errors.New("the dispatch context was cancelled before the drain ran")
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Enqueue(context.Background(), Delivery{Source: "github", DeliveryID: "d-1", Project: "o/r", Revision: testSHA}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("no dispatch started")
	}
	cancel() // SIGTERM
	if err = <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	if e := <-dispatchErr; e != nil {
		t.Fatal(e)
	}
}

func TestRunReturnsAFailedDrain(t *testing.T) {
	st := openStore(t, newClock())
	c, err := New(Config{Store: st, Insecure: true, Addr: "127.0.0.1:0", Poll: 10 * time.Millisecond,
		Drain: func(context.Context) error { return errors.New("hand-back failed") }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if err = <-done; err == nil {
		t.Fatal("a failed drain must fail Run")
	}
}
