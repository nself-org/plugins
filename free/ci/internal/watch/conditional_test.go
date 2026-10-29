package watch

// Purpose: P7-AUTO-11 conditional requests on the watcher side: a scan whose
// conditional reads all answered 304 keeps the previous heads (no fresh listing
// is needed to act on them), a 304 before any listing acts on nothing, and the
// passes of two hosts are offset and jittered apart.
// Inputs: the shared rig (fake ledger, scripted runner) and a scripted scanner.
// Outputs: pass or fail. Constraints: no network, no sleeps.

import (
	"context"
	"testing"
	"time"
)

// notModScan answers the first scan with heads and every later one with 304.
type notModScan struct {
	heads []Head
	calls int
}

func (s *notModScan) Scan(context.Context, string) (Scan, error) {
	s.calls++
	if s.calls == 1 {
		return Scan{Heads: append([]Head(nil), s.heads...)}, nil
	}
	return Scan{NotModified: true}, nil
}

func TestConditionalRequests(t *testing.T) {
	ctx := context.Background()

	// A 304 keeps the heads of the last full scan: the pass still sees them (here: already final).
	r := newRig(t, ownerHead("o/r", 1))
	scan := &notModScan{heads: r.scan.heads}
	r.w.P.Scanner = scan
	if rep := r.w.Pass(ctx); rep.Ran != 1 {
		t.Fatalf("the first pass must run the head: %+v", rep)
	}
	rep := r.w.Pass(ctx)
	if scan.calls != 2 || len(rep.Decisions) != 1 || rep.Decisions[0].SHA != sha(1) || rep.Ran != 0 {
		t.Fatalf("a 304 scan must keep the previous heads and run nothing new: calls=%d %+v", scan.calls, rep)
	}
	if r.runner.count() != 1 {
		t.Fatalf("a 304 scan must not rerun a head: %d runs", r.runner.count())
	}

	// A 304 before any full scan has nothing to act on.
	fresh := newRig(t)
	fresh.w.P.Scanner = &notModScan{calls: 1}
	if rep := fresh.w.Pass(ctx); len(rep.Decisions) != 0 || rep.Ran != 0 {
		t.Fatalf("a 304 without a previous scan must act on nothing: %+v", rep)
	}

	// Two hosts poll out of step: distinct phase offsets inside one interval, and a
	// jittered wait within [0.9, 1.1) of the interval.
	c := Defaults()
	c.Interval = time.Minute
	mk := func(host string, jitter float64) *Watcher {
		c.Host = host
		w, err := New(c, Ports{Scanner: scan, Ledger: r.gh.on(host), Reporter: r.gh.on(host), Runner: r.runner, Jitter: func() float64 { return jitter }})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	a, b := mk("mini1", 0), mk("mini2", 0.999)
	if a.Offset() == b.Offset() {
		t.Fatalf("two hosts must not share a phase offset: %v", a.Offset())
	}
	for _, w := range []*Watcher{a, b} {
		if o := w.Offset(); o < 0 || o >= c.Interval {
			t.Fatalf("offset %v is outside one interval", o)
		}
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if lo, hi := a.Next(now), b.Next(now); lo != 54*time.Second || hi < lo || hi >= 66*time.Second {
		t.Fatalf("jittered waits %v and %v must lie in [0.9, 1.1) of the interval", lo, hi)
	}
}
