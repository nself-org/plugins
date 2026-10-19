package background

// The in-process simulation (acceptance 2): a real dispatcher, a real store and
// a one-slot runner. A background flood holds the slot; an interactive job
// arrives behind it; when the slot frees, the first admission is the interactive
// job, within one dispatcher tick. No sleeps: the test waits on channels, and
// the injected clock moves by a step on every read so waiters re-tick.

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// stepClock advances 1 ms per read.
type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Millisecond)
	return c.t
}

// watchOrderer wraps the dispatcher's own orderer and reports the queue depth of every tick.
type watchOrderer struct {
	inner dispatch.Orderer
	depth chan int
}

func (w watchOrderer) Order(q []dispatch.Queued) []dispatch.Queued {
	select {
	case w.depth <- len(q):
	default:
	}
	return w.inner.Order(q)
}

// admissions records admitted jobs in order.
type admissions struct {
	mu   sync.Mutex
	keys []string
}

func (a *admissions) Record(_ context.Context, d dispatch.Decision) {
	if d.Outcome != dispatch.OutcomeAdmitted {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keys = append(a.keys, d.Job.Key)
}

func passAttempt(t testing.TB, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	for _, step := range [][2]model.JobState{{"leased", "running"}, {"running", "finalizing"}, {"finalizing", "passed"}} {
		_, epoch, err := st.AttemptStateEpoch(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err = st.Transition(ctx, id, step[0], step[1], store.TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestInteractiveFirstDispatcher: see the file comment.
func TestInteractiveFirstDispatcher(t *testing.T) {
	st := openStore(t)
	const flood = 6
	desk := runner("desk", idleDesk)
	one := 1
	desk.Capability.Availability.Slots.Value = &one
	var builds, freeSeen atomic.Int64
	world := func(ctx context.Context, _ sched.Pipeline) (sched.World, string, error) {
		builds.Add(1)
		snap, err := st.WorldSnapshot(ctx)
		if err != nil {
			return sched.World{}, "", err
		}
		r := desk
		rs := snap.Reservations[r.ID]
		r.Avail.Slots = max(0, one-int(rs.Slots))
		if r.Avail.Slots > 0 {
			freeSeen.Add(1) // a world that offers the slot: the tick that builds it must give it to the interactive job
		}
		r.Avail.CPU -= rs.CPU
		r.Avail.MemMB -= rs.MemMB
		return sched.World{Policy: sched.Policy{Preset: "balanced"}, Runners: []sched.Runner{r}, Costs: map[string]sched.Cost{}, Previous: map[string]string{},
			Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}, InteractiveQueued: map[string]bool{}}, "digest-" + strconv.FormatInt(builds.Load(), 10), nil
	}
	clock := &stepClock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	sink := &admissions{}
	orderer := watchOrderer{inner: dispatch.NewFair(nil, clock.Now), depth: make(chan int, 1024)}
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", World: world, Clock: clock.Now, Sink: sink, Orderer: orderer, Poll: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	interactiveQueued := atomic.Bool{}
	withProbe(t, func(id string) bool { return id == "desk" && interactiveQueued.Load() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	attempt := func(name, band string) (string, dispatch.Job) {
		id := queueAttempt(t, st, name)
		j := job(id, band)
		j.Pipeline.Band = band
		return id, dispatch.Job{AttemptID: id, Sched: j, Foreground: band == "interactive"}
	}
	submit := func(j dispatch.Job) chan error {
		done := make(chan error, 1)
		go func() { _, e := d.Submit(ctx, j); done <- e }()
		return done
	}

	// the first background job takes the only slot
	firstID, first := attempt("bg0", BandName)
	if err = <-submit(first); err != nil {
		t.Fatalf("first background job: %v", err)
	}
	// the rest of the flood queues behind it, then the interactive job
	var waiting []chan error
	for i := 1; i <= flood; i++ {
		_, j := attempt("bg"+strconv.Itoa(i), BandName)
		waiting = append(waiting, submit(j))
	}
	fgID, fg := attempt("fg", "interactive")
	interactiveQueued.Store(true)
	fgDone := submit(fg)

	timeout := time.After(30 * time.Second)
	for seen := false; !seen; { // every job is in the queue once a tick sees them all
		select {
		case depth := <-orderer.depth:
			seen = depth == flood+1
		case <-timeout:
			t.Fatal("no tick saw the whole queue")
		}
	}
	sink.mu.Lock()
	before := len(sink.keys)
	sink.mu.Unlock()
	if before != 1 {
		t.Fatalf("only the first job holds the slot, got %d admissions", before)
	}

	freeSeen.Store(0) // the slot is still held, so no world has offered it since the first admission
	passAttempt(t, st, firstID)
	select {
	case err = <-fgDone:
		if err != nil {
			t.Fatalf("interactive job: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the interactive job was not admitted after the slot freed")
	}
	interactiveQueued.Store(false)
	sink.mu.Lock()
	keys := append([]string(nil), sink.keys...)
	sink.mu.Unlock()
	if len(keys) != 2 || keys[1] != fgID {
		t.Fatalf("the freed slot must go to the interactive job, admissions: %v", keys)
	}
	if n := freeSeen.Load(); n != 1 { // exactly one tick saw the free slot, and it admitted the interactive job
		t.Fatalf("%d ticks saw the free slot before the interactive job started, want 1", n)
	}
	for i, w := range waiting {
		select {
		case e := <-w:
			t.Fatalf("background job %d was admitted (%v) while the slot is held", i+1, e)
		default:
		}
	}
	cancel() // release the flood's waiters
}
