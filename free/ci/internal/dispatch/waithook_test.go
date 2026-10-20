package dispatch

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestOnWaiting: a hook sees a job the tick could not place (with how long it queued and the project
// queue ahead of it) and does not see a job that was admitted (D-1329).
func TestOnWaiting(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	local := testRunner("local", "local", 1)
	var mu sync.Mutex
	var got []Waiting
	restore := setWaitHooks(func(_ context.Context, w Waiting) {
		mu.Lock()
		got = append(got, w)
		mu.Unlock()
	})
	defer restore()
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, local), Clock: clock.Now, Sink: &recSink{}})
	a, b := queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b")
	enqueueAt(t, d, clock, a, b)
	clock.Advance(4 * time.Second)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Job.AttemptID != b || got[0].Ahead != 0 || got[0].Demand != 1 || got[0].Waited < 4*time.Second || got[0].ExpectedWaitMs <= 0 {
		t.Fatalf("waiting hook calls = %+v, want one for the second job (none ahead, demand 1, waited >= 4s, a positive expected wait)", got)
	}
}
