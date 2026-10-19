package schedtel

// Purpose: which store each run's telemetry goes to. The dispatcher builds its
// sink from the store it was given (dispatch.SinkFactory); the finalizer and
// the world contributor have no store of their own, so they find it here.
// Inputs:  bind(run, sink) from the sink on its first decision of a run.
// Outputs: storeFor(run), latest(), unbind(run).
// Constraints: in-process only; a binding lives from the first decision of a
// run to its finalizer; latest() is the store the contributor reads when its
// context names none (tests pass one with WithStore).

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	bindMu  sync.Mutex
	byRun   = map[string]*Sink{}
	lastSnk *Sink
	// now is the clock new sinks use; tests replace it through SetClock.
	now = time.Now
)

// SetClock replaces the clock of sinks created after the call and returns a
// function that restores the previous one.
func SetClock(f func() time.Time) (restore func()) {
	bindMu.Lock()
	defer bindMu.Unlock()
	old := now
	now = f
	return func() { bindMu.Lock(); now = old; bindMu.Unlock() }
}

func clock() func() time.Time {
	bindMu.Lock()
	defer bindMu.Unlock()
	return now
}

func bind(run string, s *Sink) {
	bindMu.Lock()
	byRun[run], lastSnk = s, s
	bindMu.Unlock()
}

// bindLatest makes s the fallback sink, from the moment its dispatcher is
// built: the first world of a run is built before the first decision.
func bindLatest(s *Sink) {
	bindMu.Lock()
	lastSnk = s
	bindMu.Unlock()
}

func sinkFor(run string) *Sink {
	bindMu.Lock()
	defer bindMu.Unlock()
	return byRun[run]
}

func unbind(run string) {
	bindMu.Lock()
	defer bindMu.Unlock()
	if byRun[run] == lastSnk {
		lastSnk = nil
	}
	delete(byRun, run)
}

type ctxKey struct{}

// WithStore names the store the world contributor reads for ctx.
func WithStore(ctx context.Context, st *store.Store) context.Context {
	return context.WithValue(ctx, ctxKey{}, st)
}

// storeIn is the store the contributor reads: the context's, else the one the
// most recent run is writing to, else nil (no estimates).
func storeIn(ctx context.Context) *store.Store {
	if st, ok := ctx.Value(ctxKey{}).(*store.Store); ok && st != nil {
		return st
	}
	bindMu.Lock()
	defer bindMu.Unlock()
	if lastSnk != nil {
		return lastSnk.st
	}
	return nil
}

// noteFailure hands a contributor failure to the sink of the latest run, whose
// finalizer reports it.
func noteFailure(err error) {
	bindMu.Lock()
	s := lastSnk
	bindMu.Unlock()
	if s != nil {
		s.fail(err)
	}
}
