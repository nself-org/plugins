package lease

import (
	"context"
	"testing"
	"time"
)

// A sweeper started on an engine whose lease deadline passed fails the lease without anyone calling Sweep.
func TestSweeperFailsAnExpiredLease(t *testing.T) {
	e, now := fixture(t, 1, false)
	(*now)[0] = (*now)[0].Add(2 * e.Config.TTL) // the deadline passed
	stop := e.StartSweeper(5*time.Millisecond, t.Logf)
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, _, err := e.Store.AttemptStateEpoch(context.Background(), "a0")
		if err != nil {
			t.Fatal(err)
		}
		if state == "retryable" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the expired lease was not failed by the sweeper")
}

// Stop returns, and no sweep runs afterwards.
func TestSweeperStops(t *testing.T) {
	e, _ := fixture(t, 0, false)
	stop := e.StartSweeper(time.Millisecond, nil)
	stop()
	stop2 := e.StartSweeper(0, nil) // the default period
	stop2()
}
