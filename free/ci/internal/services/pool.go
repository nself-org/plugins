package services

// Purpose: keyed lifecycle of provisions for the orchestrator hook (P7-CI-42).
// The hook request carries no attempt id, so two jobs asking for the same key
// would be indistinguishable at Down. The Pool therefore serialises Up per key
// (the second Up waits for the first Down) and ties each provision to a
// safety net: a job context that ends before Down tears the project down.
// Inputs:  a Provisioner, a key, the specs. Outputs: the job environment.
// Constraints: the safety net keeps the entry and the slot until the owning
// job's Down, so that Down cannot hit a later job with the same key; teardown
// runs once, whoever asks first, and Down returns only after it finished.

import (
	"context"
	"sync"
	"time"
)

// Pool tracks the live provisions by key.
type Pool struct {
	mu    sync.Mutex
	slots map[string]chan struct{}
	live  map[string]*liveProvision
}

// NewPool returns an empty Pool.
func NewPool() *Pool {
	return &Pool{slots: map[string]chan struct{}{}, live: map[string]*liveProvision{}}
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

// Up waits for the key's slot, provisions, and returns the job environment.
func (pl *Pool) Up(ctx context.Context, prov *Provisioner, key string, specs []Spec) (map[string]string, error) {
	if err := pl.acquire(ctx, key); err != nil {
		return nil, err
	}
	h, err := prov.Up(ctx, specs)
	if err != nil {
		pl.release(key)
		return nil, err
	}
	l := &liveProvision{prov: prov, handle: h, done: make(chan struct{})}
	l.stop = context.AfterFunc(ctx, func() { pl.safetyNet(l) })
	pl.mu.Lock()
	pl.live[key] = l
	pl.mu.Unlock()
	return h.Env, nil
}

// Down tears the key's provision down and frees the slot. It returns only when
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
	defer pl.release(key)
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

// acquire waits for the slot of key, or for ctx.
func (pl *Pool) acquire(ctx context.Context, key string) error {
	pl.mu.Lock()
	ch, ok := pl.slots[key]
	if !ok {
		ch = make(chan struct{}, 1)
		pl.slots[key] = ch
	}
	pl.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (pl *Pool) release(key string) {
	pl.mu.Lock()
	ch := pl.slots[key]
	pl.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
		}
	}
}
