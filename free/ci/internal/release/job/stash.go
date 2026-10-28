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
	refreshEvery = time.Duration(5e8)  // 500 ms
	maxAge       = time.Duration(1e10) // 10 s
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
	// Revoked reports whether the evidence signer's key id is revoked. It is
	// called inside the lock during ReleaseDecision so a revocation that commits
	// between Guard and Admit is seen atomically; nil revokes nothing.
	Revoked func(keyID string) bool
}

// NewStash returns an empty stash.
func NewStash() *Stash { return &Stash{m: map[string]*entry{}} }

// Put records static facts for one attempt; they never expire (tests).
func (s *Stash) Put(attemptID string, f Facts) {
	perr := Evaluate(f)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[attemptID] = &entry{f: f, perr: perr}
}

// Forget drops one attempt.
func (s *Stash) Forget(attemptID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, attemptID)
}

// ReleaseFacts returns the latest evaluated facts and gate refusal.
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
	if s.Revoked != nil && e.f.Evidence.Signature.KeyID != nil && s.Revoked(*e.f.Evidence.Signature.KeyID) {
		return Facts{}, refuse(CodeNotAdmitted, SignerRevoked, "the evidence signer was revoked"), true
	}
	return e.f, e.perr, true
}

// update replaces the facts of the live entry e if it is still the attempt's.
// at is the time the provider was asked, not answered: a slow digester does
// not make an old observation look fresh.
func (s *Stash) update(attemptID string, e *entry, f Facts, perr *Error, at time.Time) {
	if perr == nil {
		perr = Evaluate(f)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[attemptID] == e {
		e.f, e.perr, e.at = f, perr, at
	}
}

// Guard submits one release attempt. It asks p and judges the answer first (an
// unacceptable attempt fails fast with its E715 *Error and never queues), then
// keeps the answer fresh while next runs (next blocks until the attempt is
// admitted, and the admission step judges the latest answer on every try), and
// forgets it when next returns.
func (s *Stash) Guard(ctx context.Context, attemptID string, p Provider, next func() error) error {
	s.mu.Lock()
	_, busy := s.m[attemptID]
	s.mu.Unlock()
	if busy { // two submissions of one attempt would share, and one would delete, the other's facts
		return refuse(CodeNotAdmitted, Duplicate, "attempt %s is already being submitted", attemptID)
	}
	asked := now()
	f, perr := p(ctx)
	if perr != nil {
		return perr
	}
	if e := Evaluate(f); e != nil {
		return e
	}
	own := &entry{f: f, at: asked, live: true}
	s.mu.Lock()
	if _, busy = s.m[attemptID]; busy {
		s.mu.Unlock()
		return refuse(CodeNotAdmitted, Duplicate, "attempt %s is already being submitted", attemptID)
	}
	s.m[attemptID] = own
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
				asked := now()
				nf, nerr := p(rctx)
				if rctx.Err() == nil {
					s.update(attemptID, own, nf, nerr, asked)
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-done
		s.mu.Lock()
		if s.m[attemptID] == own {
			delete(s.m, attemptID)
		}
		s.mu.Unlock()
	}()
	return next()
}

// ReleaseDecision reads the gate result without verifying signatures or writing the ledger.
// Guard, refresh and Put evaluate before publishing the snapshot.
func (s *Stash) ReleaseDecision(attemptID string) (*Error, bool) {
	_, perr, ok := s.ReleaseFacts(attemptID)
	return perr, ok
}
