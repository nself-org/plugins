package drain

// Purpose: TestDrain (P7-NODE-20 acceptance): SIGTERM with running jobs stops intake; each job finishes or
//          is handed back only after a cancel ack or lease expiry; old and new attempts never overlap; a
//          release or idempotent:false job is never handed back; exit 0 and no goroutine leak.
// Inputs:  the fake clock and world of rig_test.go. Outputs: pass/fail.
// Constraints: deterministic (fake clock), one real SIGTERM delivered to this process in one subtest.

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

type drainRun struct {
	d    *Drainer
	done chan struct{}
	rep  Report
	err  error
}

func start(t *testing.T, c *fakeClock, w *fakeWorld, mod func(*Config)) *drainRun {
	t.Helper()
	cfg := Config{Intake: w, Leases: w, Canceller: w, Clock: c, Deadline: 10 * time.Minute, Poll: time.Second, AckTimeout: 2 * time.Minute}
	if mod != nil {
		mod(&cfg)
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := &drainRun{d: d, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.rep, r.err = d.Run(context.Background())
	}()
	return r
}

func (w *fakeWorld) ack(d *Drainer, id string) {
	w.mu.Lock()
	w.log("ack %s", id)
	w.mu.Unlock()
	d.Ack(id)
}

func outcome(t *testing.T, rep Report, id string) Outcome {
	t.Helper()
	for _, r := range rep.Results {
		if r.LeaseID == id {
			return r.Outcome
		}
	}
	t.Fatalf("no result for %s in %+v", id, rep.Results)
	return ""
}

func TestDrain(t *testing.T) {
	t.Run("one job finishes, the other is cancelled and left alone after its ack until it ends", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 12*time.Minute)
		w.add("b", "test", true, 12*time.Minute)
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		if w.stops() != 1 {
			t.Fatalf("intake stopped %d times before any lease settled", w.stops())
		}
		c.Advance(5 * time.Minute)
		c.settleOn(t, 1, r.done)
		w.finish("a")
		c.Advance(5 * time.Minute) // the deadline
		c.settleOn(t, 1, r.done)   // b is cancelled and waits
		if _, ok := w.find("handback b expired=false"); ok {
			t.Fatal("handed back before the ack")
		}
		c.Advance(5 * time.Second)
		c.settleOn(t, 1, r.done)
		w.ack(r.d, "b")
		c.settleOn(t, 1, r.done)
		w.never(t, "handback b expired=false") // the ack means "cancel received", not "stopped"
		w.finish("b")                          // its job and after-job hooks ended: the result was committed
		c.Advance(5 * time.Second)
		<-r.done
		if r.err != nil || r.rep.ExitCode() != 0 {
			t.Fatalf("err %v exit %d", r.err, r.rep.ExitCode())
		}
		if outcome(t, r.rep, "a") != OutcomeFinished || outcome(t, r.rep, "b") != OutcomeAcked {
			t.Fatalf("results %+v", r.rep.Results)
		}
		w.never(t, "cancel a node=node-a")
		stop, cancel, ack, fin := w.indexOf(t, "intake.stop"), w.indexOf(t, "cancel b node=node-b"), w.indexOf(t, "ack b"), w.indexOf(t, "finish b")
		if stop >= cancel || cancel >= ack || ack >= fin {
			t.Fatalf("order stop=%d cancel=%d ack=%d finish=%d: %v", stop, cancel, ack, fin, w.names())
		}
		w.never(t, "handback b expired=false")
	})

	t.Run("an acked job that keeps running is handed back only when its lease expires", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("b", "test", true, 10*time.Minute+30*time.Second)
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute) // the deadline: b is cancelled
		c.settleOn(t, 1, r.done)
		w.ack(r.d, "b")
		c.settleOn(t, 1, r.done)
		w.never(t, "handback b expired=false")
		w.never(t, "handback b expired=true") // still within its lease
		c.run(t, 1, time.Second, r.done, nil)
		<-r.done
		if outcome(t, r.rep, "b") != OutcomeExpired {
			t.Fatalf("results %+v", r.rep.Results)
		}
		w.never(t, "handback b expired=false")
	})

	t.Run("a result that lands before the cancel ack is not undone by a hand-back", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("b", "test", true, 12*time.Minute)
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute) // the deadline: b is cancelled and waits for its ack
		c.settleOn(t, 1, r.done)
		w.finish("b") // the agent delivered its result just before it saw the cancel
		w.ack(r.d, "b")
		<-r.done
		if got := outcome(t, r.rep, "b"); got != OutcomeAcked && got != OutcomeFinished {
			t.Fatalf("results %+v", r.rep.Results)
		}
		w.never(t, "handback b expired=false")
	})

	t.Run("two running jobs, no ack: handed back only after the lease expires", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 10*time.Minute+20*time.Second)
		w.add("b", "test", true, 10*time.Minute+20*time.Second)
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute)
		expires := c.Now().Add(20 * time.Second)
		c.run(t, 1, time.Second, r.done, func() {
			if c.Now().Before(expires) {
				for _, n := range w.names() {
					if n == "handback a expired=true" || n == "handback b expired=true" {
						t.Fatalf("handed back at %v, before the lease deadline %v", c.Now(), expires)
					}
				}
			}
		})
		<-r.done
		if outcome(t, r.rep, "a") != OutcomeExpired || outcome(t, r.rep, "b") != OutcomeExpired || r.rep.ExitCode() != 0 {
			t.Fatalf("results %+v", r.rep.Results)
		}
		for _, id := range []string{"a", "b"} {
			if w.indexOf(t, "cancel "+id+" node=node-"+id) > w.indexOf(t, "handback "+id+" expired=true") {
				t.Fatalf("lease %s handed back before its cancel", id)
			}
		}
	})

	t.Run("a release job and an idempotent:false job are never handed back", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("rel", "release", true, 12*time.Minute)
		w.add("once", "test", false, 12*time.Minute)
		w.add("plain", "test", true, 12*time.Minute)
		r := start(t, c, w, nil)
		w.setOnCancel(func(id string) { w.ack(r.d, id); w.finish(id) }) // the agent acks at once and its job stops
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute)
		<-r.done
		if outcome(t, r.rep, "rel") != OutcomeNeedsOperator || outcome(t, r.rep, "once") != OutcomeNeedsOperator || (outcome(t, r.rep, "plain") != OutcomeAcked && outcome(t, r.rep, "plain") != OutcomeFinished) {
			t.Fatalf("results %+v", r.rep.Results)
		}
		for _, id := range []string{"rel", "once"} {
			w.never(t, "cancel "+id+" node=node-"+id)
			w.never(t, "handback "+id+" expired=false")
			w.never(t, "handback "+id+" expired=true")
		}
		if r.rep.ExitCode() != 0 {
			t.Fatalf("exit %d", r.rep.ExitCode())
		}
	})

	t.Run("a cancel that is neither acked nor expired is never handed back", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 12*time.Minute)
		w.renew = true // the agent keeps renewing and ignores the cancel
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute)
		c.run(t, 1, time.Second, r.done, nil)
		<-r.done
		if outcome(t, r.rep, "a") != OutcomeLeftToGrace {
			t.Fatalf("results %+v", r.rep.Results)
		}
		w.never(t, "handback a expired=false")
		w.never(t, "handback a expired=true")
	})

	t.Run("an ack that arrives before the cancel is ignored", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 10*time.Minute+10*time.Second)
		r := start(t, c, w, nil)
		c.settleOn(t, 1, r.done)
		r.d.Ack("a") // not cancelled yet
		c.Advance(10 * time.Minute)
		c.run(t, 1, time.Second, r.done, nil)
		<-r.done
		if outcome(t, r.rep, "a") != OutcomeExpired {
			t.Fatalf("an early ack handed the lease back: %+v", r.rep.Results)
		}
	})

	t.Run("a cancel error waits for expiry instead of handing back", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 10*time.Minute+5*time.Second)
		r := start(t, c, w, func(cfg *Config) { cfg.Canceller = failingCanceller{w} })
		c.settleOn(t, 1, r.done)
		c.Advance(10 * time.Minute)
		c.run(t, 1, time.Second, r.done, nil)
		<-r.done
		if outcome(t, r.rep, "a") != OutcomeExpired {
			t.Fatalf("results %+v", r.rep.Results)
		}
	})

	t.Run("a hard stop abandons the wait and returns", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 12*time.Minute)
		ctx, cancel := context.WithCancel(context.Background())
		d, err := New(Config{Intake: w, Leases: w, Canceller: w, Clock: c})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var rep Report
		go func() { defer close(done); rep, _ = d.Run(ctx) }()
		c.settleOn(t, 1, done)
		cancel()
		<-done
		if outcome(t, rep, "a") != OutcomeAbandoned {
			t.Fatalf("results %+v", rep.Results)
		}
	})

	t.Run("SIGTERM starts the drain and the process exits 0", func(t *testing.T) {
		noLeak(t)
		c := newClock()
		w := newWorld(c)
		w.add("a", "test", true, 12*time.Minute)
		d, err := New(Config{Intake: w, Leases: w, Canceller: w, Clock: c})
		if err != nil {
			t.Fatal(err)
		}
		w.setOnCancel(func(id string) { w.ack(d, id); w.finish(id) })
		term, stopTerm := NotifyTerm()
		defer stopTerm()
		done := make(chan struct{})
		var rep Report
		var serr error
		go func() { defer close(done); rep, serr = d.Serve(context.Background(), term) }()
		if w.stops() != 0 {
			t.Fatal("intake stopped before SIGTERM")
		}
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		c.settleOn(t, 1, done)
		if w.stops() != 1 {
			t.Fatalf("SIGTERM did not stop intake (%d)", w.stops())
		}
		c.Advance(10 * time.Minute)
		<-done
		if serr != nil || rep.ExitCode() != 0 || (outcome(t, rep, "a") != OutcomeAcked && outcome(t, rep, "a") != OutcomeFinished) {
			t.Fatalf("err %v exit %d results %+v", serr, rep.ExitCode(), rep.Results)
		}
	})

	t.Run("a context that ends before SIGTERM drains nothing", func(t *testing.T) {
		noLeak(t)
		w := newWorld(newClock())
		d, _ := New(Config{Intake: w, Leases: w})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if rep, err := d.Serve(ctx, make(chan struct{})); err != nil || len(rep.Results) != 0 || w.stops() != 0 {
			t.Fatalf("rep %+v err %v stopped %d", rep, err, w.stops())
		}
	})

	t.Run("New refuses a missing intake or lease source", func(t *testing.T) {
		if _, err := New(Config{}); err == nil {
			t.Fatal("New accepted an empty config")
		}
	})
}

type failingCanceller struct{ w *fakeWorld }

func (f failingCanceller) Cancel(nodeID, leaseID, _ string) error {
	f.w.mu.Lock()
	f.w.log("cancel %s node=%s", leaseID, nodeID)
	f.w.mu.Unlock()
	return errors.New("agent unreachable")
}
