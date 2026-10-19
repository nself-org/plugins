package job

// Purpose: the hand-over between the coordinator and the admission step. The
// coordinator judges a release attempt's facts before it queues, then keeps
// refreshing them outside the store's writer transaction while the attempt
// waits for capacity; the step reads the latest answer (a worktree probe or a
// digester must never run under the writer lock).
// Inputs:  a Provider per release attempt (Guard) or static Facts (Put, tests).
// Outputs: Source.ReleaseFacts: the latest Facts, or an E715 when the last
// refresh failed or is older than maxAge (the attempt then stays queued).
// Constraints: safe for concurrent use; the refresher stops when the submission
// returns; an entry is forgotten with it. maxAge is several refresh intervals, so
// a change made while the attempt waits is seen within about refreshEvery.

import (
	"context"
	"sync"
	"time"
)

// Timing of the refresher (vars so tests can shrink them).
var (
	refreshEvery = 500 * time.Millisecond
	maxAge       = 10 * time.Second
	now          = time.Now
)

type entry struct {
	f    Facts
	perr *Error
	at   time.Time
	live bool // refreshed by Guard: subject to maxAge
}

// Stash holds the latest Facts of the release attempts being submitted.
type Stash struct {
	mu sync.Mutex
	m  map[string]*entry
}

// NewStash returns an empty stash.
func NewStash() *Stash { return &Stash{m: map[string]*entry{}} }

// Put records static facts for one attempt; they never expire (tests).
func (s *Stash) Put(attemptID string, f Facts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[attemptID] = &entry{f: f}
}

// Forget drops one attempt.
func (s *Stash) Forget(attemptID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, attemptID)
}

// ReleaseFacts implements Source.
func (s *Stash) ReleaseFacts(attemptID string) (Facts, *Error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[attemptID]
	if !ok {
		return Facts{}, nil, false
	}
	if e.live && now().Sub(e.at) > maxAge {
		return Facts{}, refuse(CodeNotAdmitted, FactsExpired, "the facts of attempt %s were last refreshed %s ago", attemptID, now().Sub(e.at).Round(time.Second)), true
	}
	return e.f, e.perr, true
}

// update replaces the facts of a live entry that still exists.
func (s *Stash) update(attemptID string, f Facts, perr *Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[attemptID]; ok && e.live {
		e.f, e.perr, e.at = f, perr, now()
	}
}

// Guard submits one release attempt. It asks p and judges the answer first (an
// unacceptable attempt fails fast with its E715 *Error and never queues), then
// keeps the answer fresh while next runs (next blocks until the attempt is
// admitted, and the admission step judges the latest answer on every try), and
// forgets it when next returns.
func (s *Stash) Guard(ctx context.Context, attemptID string, p Provider, next func() error) error {
	f, perr := p(ctx)
	if perr != nil {
		return perr
	}
	if e := Evaluate(f); e != nil {
		return e
	}
	s.mu.Lock()
	s.m[attemptID] = &entry{f: f, at: now(), live: true}
	s.mu.Unlock()
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(refreshEvery)
		defer t.Stop()
		for {
			select {
			case <-rctx.Done():
				return
			case <-t.C:
				nf, nerr := p(rctx)
				if rctx.Err() == nil {
					s.update(attemptID, nf, nerr)
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-done
		s.Forget(attemptID)
	}()
	return next()
}
