package dispatch

// Purpose: TestInvariant_I14, "no runner exceeds its reservation or
// concurrency" (ADR 0025 section 12). Two dispatchers share one store file and
// plan from the store's view; many generated submissions race for three
// runners, and an observer reads the reservations the whole time. Every
// submission is delivered through a fake transport driver that walks its
// attempt to passed, which is what frees the capacity.
// Constraints: no sleeps, a seeded generator that prints its seed on failure.

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type capacityLimit struct{ slots, cpu int }

func TestInvariant_I14(t *testing.T) {
	seed := invariants.Seed()
	t.Logf("seed=%d (NSELF_CI_INVARIANT_SEED)", seed)
	rng := rand.New(rand.NewSource(seed))
	const submissions = 1000
	limits := map[string]capacityLimit{"r1": {2, 8}, "r2": {3, 3}, "r3": {1, 8}}
	runners := []sched.Runner{testRunner("r1", "lan", 2), testRunner("r2", "lan", 3), testRunner("r3", "lan", 1)}
	for i := range runners {
		cpu := float64(limits[runners[i].ID].cpu)
		runners[i].Capability.Resources.CPU.Value = &cpu
		runners[i].Avail.CPU = int64(cpu)
	}
	path := filepath.Join(t.TempDir(), "state.db")
	s1, s2 := openStoreAt(t, path), openStoreAt(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type plan struct {
		id  string
		cpu int64
	}
	plans := make([]plan, submissions)
	for i := range plans {
		pipe := fmt.Sprintf("pipe%d", rng.Intn(5))
		plans[i] = plan{id: queueAttempt(t, s1, pipe, "proj", fmt.Sprintf("job%d", i)), cpu: int64(1 + rng.Intn(2))}
	}

	var passed atomic.Int64
	var failures sync.Map
	finish := func(st *store.Store) func(Delivery) error {
		return func(d Delivery) error {
			defer passed.Add(1)
			pass(t, st, d.AttemptID)
			return nil
		}
	}
	mk := func(st *store.Store) *Dispatcher {
		return newDispatcher(t, Config{Store: st, World: liveWorld(st, func(p *sched.Policy) { p.Limits.Global = 5 }, runners...), Poll: time.Millisecond,
			Transport: fakeTransport{run: finish(st)}})
	}
	dispatchers := []*Dispatcher{mk(s1), mk(s2)}

	// The observer: at every look, no runner holds more leases than slots or more CPU than it has.
	stop := make(chan struct{})
	var looks atomic.Int64
	var obs sync.WaitGroup
	obs.Add(1)
	go func() {
		defer obs.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			snap, err := s1.WorldSnapshot(ctx)
			if err != nil {
				failures.Store("snapshot", err)
				return
			}
			looks.Add(1)
			active := 0
			for id, lim := range limits {
				r := snap.Reservations[id]
				active += int(r.Slots)
				slots := int(*findRunner(runners, id).Capability.Availability.Slots.Value)
				if int(r.Slots) > slots || int(r.CPU) > lim.cpu {
					failures.Store("capacity "+id, fmt.Sprintf("%d leases (slots %d), %d cpu (capacity %d)", r.Slots, slots, r.CPU, lim.cpu))
				}
			}
			if active > 5 {
				failures.Store("global", fmt.Sprintf("%d active, global limit 5", active))
			}
		}
	}()

	var wg sync.WaitGroup
	for i, p := range plans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j := testQueued(p.id)
			j.Sched.Requirements.CPU = p.cpu
			if _, err := dispatchers[i%2].Submit(ctx, j); err != nil {
				failures.Store(p.id, err)
			}
		}()
	}
	wg.Wait()
	for _, d := range dispatchers {
		d.Close() // waits for the deliveries
	}
	close(stop)
	obs.Wait()
	if looks.Load() == 0 {
		t.Fatal("the observer never looked")
	}
	failures.Range(func(k, v any) bool { t.Errorf("seed %d: %v: %v", seed, k, v); return true })
	if got := passed.Load(); got != submissions {
		t.Fatalf("seed %d: %d of %d submissions ran", seed, got, submissions)
	}
	snap, err := s1.WorldSnapshot(ctx)
	if err != nil || len(snap.Active) != 0 || len(snap.Reservations) != 0 {
		t.Fatalf("seed %d: capacity not released: %+v %v", seed, snap.Reservations, err)
	}
	t.Logf("%d submissions, %d observations, no runner over its slots or CPU", submissions, looks.Load())
}

func findRunner(rs []sched.Runner, id string) sched.Runner {
	for _, r := range rs {
		if r.ID == id {
			return r
		}
	}
	return sched.Runner{}
}
