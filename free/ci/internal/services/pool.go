package services

// Purpose: keyed lifecycle of provisions for the orchestrator hook (P7-CI-42).
// The key is the attempt id, so every attempt has its own provision and its own
// Down, and jobs of one run that need the same services never share or wait for
// each other. Each provision is tied to a safety net: a job context that ends
// before Down tears the project down.
// Inputs:  a Provisioner, a key, the specs. Outputs: the job environment.
// Constraints: the safety net keeps the entry until the owning attempt's Down;
// teardown runs once, whoever asks first, and Down returns only after it
// finished. A second Up for a key that is still live is refused. Down belongs after Up has returned: a Down that
// arrives while Up is still running finds nothing to tear down.

import (
	"context"
	"sync"
	"time"
)

// Pool tracks the live provisions by key.
type Pool struct {
	mu   sync.Mutex
	live map[string]*liveProvision
	busy map[string]bool // keys whose Up is running
}

// NewPool returns an empty Pool.
func NewPool() *Pool {
	return &Pool{live: map[string]*liveProvision{}, busy: map[string]bool{}}
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

// Up provisions the key and returns the job environment. A key that is live (or being provisioned) is refused: the
// owner's Down must come first.
func (pl *Pool) Up(ctx context.Context, prov *Provisioner, key string, specs []Spec) (map[string]string, error) {
	pl.mu.Lock()
	if pl.live[key] != nil || pl.busy[key] {
		pl.mu.Unlock()
		return nil, fail(CodeProvision, "services for %s are already up", key)
	}
	pl.busy[key] = true
	pl.mu.Unlock()
	defer func() { // also on a panic in the provisioner
		pl.mu.Lock()
		delete(pl.busy, key)
		pl.mu.Unlock()
	}()
	h, err := prov.Up(ctx, specs)
	if err != nil {
		return nil, err
	}
	l := &liveProvision{prov: prov, handle: h, done: make(chan struct{})}
	l.stop = context.AfterFunc(ctx, func() { pl.safetyNet(l) })
	pl.mu.Lock()
	pl.live[key] = l
	pl.mu.Unlock()
	return h.Env, nil
}

// Down tears the key's provision down. It returns only when the teardown has
// finished, including one the safety net started.
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
