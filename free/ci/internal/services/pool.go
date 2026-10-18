package services

// Purpose: keyed lifecycle of provisions for the orchestrator hook (P7-CI-42).
// The key is the attempt id (D-0975), so jobs of one run that need the same
// services each get their own project and run side by side. Each provision is
// tied to a safety net: a job context that ends before Down tears the project
// down.
// Inputs:  a Provisioner, a key, the specs. Outputs: the job environment.
// Constraints: a key is live from Up until Down; a second Up for a live key
// fails at once instead of waiting. The safety net keeps the entry until the
// owning job's Down, so that Down cannot hit a later job with the same key;
// teardown runs once, whoever asks first, and Down returns only after it
// finished.

import (
	"context"
	"sync"
	"time"
)

// Pool tracks the live provisions by key.
type Pool struct {
	mu      sync.Mutex
	pending map[string]bool // keys whose Up is still provisioning
	live    map[string]*liveProvision
}

// NewPool returns an empty Pool.
func NewPool() *Pool {
	return &Pool{pending: map[string]bool{}, live: map[string]*liveProvision{}}
}

// liveProvision is one running provision.
type liveProvision struct {
	prov   *Provisioner
	handle *Handle
	stop   func() bool // cancels the safety net
	once   sync.Once
	done   chan struct{}
	err    error
}

func (l *liveProvision) teardown(ctx context.Context) error {
	l.once.Do(func() {
		l.err = l.prov.Down(ctx, l.handle)
		close(l.done)
	})
	<-l.done
	return l.err
}

// Up provisions the specs for key and returns the job environment. A key that
// is already live or provisioning is an error: keys are attempt ids, so a
// repeat is a caller bug, not a request to wait for.
func (pl *Pool) Up(ctx context.Context, prov *Provisioner, key string, specs []Spec) (map[string]string, error) {
	pl.mu.Lock()
	if pl.pending[key] || pl.live[key] != nil {
		pl.mu.Unlock()
		return nil, &Error{Code: CodeProvision, Message: "services for attempt " + key + " are already up"}
	}
	pl.pending[key] = true
	pl.mu.Unlock()
	h, err := prov.Up(ctx, specs)
	if err != nil {
		pl.mu.Lock()
		delete(pl.pending, key)
		pl.mu.Unlock()
		return nil, err
	}
	l := &liveProvision{prov: prov, handle: h, done: make(chan struct{})}
	l.stop = context.AfterFunc(ctx, func() { pl.safetyNet(l) })
	pl.mu.Lock()
	delete(pl.pending, key)
	pl.live[key] = l
	pl.mu.Unlock()
	return h.Env, nil
}

// Down tears the key's provision down. It returns only when
// the teardown has finished, including one the safety net started.
func (pl *Pool) Down(ctx context.Context, key string) error {
	pl.mu.Lock()
	l := pl.live[key]
	delete(pl.live, key)
	pl.mu.Unlock()
	if l == nil {
		return nil
	}
	l.stop()
	// The orchestrator's cleanup context is short (15 s); a teardown that is cut
	// off leaves containers behind, so it gets its own budget.
	bg, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer done()
	return l.teardown(bg)
}

// safetyNet tears the provision down when the job context ended before Down.
func (pl *Pool) safetyNet(l *liveProvision) {
	bg, done := context.WithTimeout(context.Background(), 2*time.Minute)
	defer done()
	_ = l.teardown(bg)
}
