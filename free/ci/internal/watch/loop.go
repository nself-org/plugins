package watch

// Purpose: the watch loop (P7-AUTO-11): passes every Interval, offset per host and jittered so the hosts
//          of a fleet do not poll in step, held back until a recorded quota reset.
// Inputs:  a context that SIGTERM cancels; once.
// Outputs: one PassReport per pass to the caller's sink.
// Constraints: no sleep outside the injected After; the first pass of a long-running loop waits the
//          host's phase offset (a stable hash of the host name, under one Interval); each later wait is
//          Interval x [0.9, 1.1); a cancelled context ends the loop after the pass in flight has drained.

import (
	"context"
	"hash/fnv"
	"time"
)

// Offset is this host's phase inside one Interval: stable for a host name, different between hosts.
func (w *Watcher) Offset() time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(w.C.Host))
	if w.C.Interval <= 0 {
		return 0
	}
	return time.Duration(h.Sum64() % uint64(w.C.Interval))
}

// Next is the wait before the pass after one that ended at now.
func (w *Watcher) Next(now time.Time) time.Duration {
	j := 0.5
	if w.P.Jitter != nil {
		j = w.P.Jitter()
	}
	d := time.Duration(float64(w.C.Interval) * (0.9 + 0.2*j))
	if q := w.QuotaUntil(); q.After(now) && q.Sub(now) > d {
		d = q.Sub(now)
	}
	return d
}

// Run passes until ctx is cancelled, or once. sink receives every report (nil: discarded).
func (w *Watcher) Run(ctx context.Context, once bool, sink func(PassReport)) {
	if !once {
		select {
		case <-ctx.Done():
			return
		case <-w.after(w.Offset()):
		}
	}
	for {
		rep := w.Pass(ctx)
		if sink != nil {
			sink(rep)
		}
		if once || ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-w.after(w.Next(rep.At)):
		}
	}
}
