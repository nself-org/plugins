package run

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// BeforeJob creates the workspace cache dirs of a container job that missed
// (a hit is mounted read-only; a process job uses host mounts).
func (h *cacheHook) BeforeJob(ctx context.Context, a AttemptInfo, workspace *os.Root) error {
	if e, ok := h.sessions.Load(a.AttemptID); ok {
		return e.(cacheEntry).s.BeforeJob(ctx, workspace)
	}
	return nil
}

// AfterJob packs the workspace cache dirs while the workspace still exists;
// the commit waits for AttemptFinalize. A cache failure never fails the job.
func (h *cacheHook) AfterJob(ctx context.Context, a AttemptInfo, workspace *os.Root) error {
	if e, ok := h.sessions.Load(a.AttemptID); ok {
		return e.(cacheEntry).s.AfterJob(ctx, workspace)
	}
	return nil
}

// AttemptFinalize saves while the attempt is finalizing at epoch.
func (h *cacheHook) AttemptFinalize(ctx context.Context, a AttemptInfo, epoch int64) error {
	if e, ok := h.sessions.Load(a.AttemptID); ok {
		e.(cacheEntry).s.Finalize(ctx, epoch)
	}
	return nil
}

// Wants lets the run skip the finalize read when nothing can be saved.
func (h *cacheHook) Wants(a AttemptInfo) bool {
	e, ok := h.sessions.Load(a.AttemptID)
	return ok && e.(cacheEntry).s.Wants()
}

// After removes the attempt's cache directories; it saves nothing (it also
// drops the entry of an attempt that did not settle passed). The retraction
// reads the store now; the removal (a restored build cache is tens of MB of
// small files, P7-CI-46 measured 60 to 95 ms) runs in the background, and the
// run waits for it before Coordinator.Run returns (waitCacheCleanups).
func (h *cacheHook) After(ctx context.Context, a AttemptInfo, _ Outcome) error {
	if e, ok := h.sessions.LoadAndDelete(a.AttemptID); ok {
		ce := e.(cacheEntry)
		retractUnpassed(ctx, a.Store, ce)
		wg := cleanupGroup(a.RunID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			ce.s.Cleanup()
		}()
	}
	return nil
}

// cacheCleanups holds, per run id, the directory removals After started.
var cacheCleanups sync.Map // run id -> *sync.WaitGroup

func cleanupGroup(runID string) *sync.WaitGroup {
	wg, _ := cacheCleanups.LoadOrStore(runID, &sync.WaitGroup{})
	return wg.(*sync.WaitGroup)
}

// waitCacheCleanups waits for the removals the run's attempts started. Every
// After of the run happened before its jobs finished, so before this call.
func waitCacheCleanups(runID string) {
	if wg, ok := cacheCleanups.LoadAndDelete(runID); ok {
		wg.(*sync.WaitGroup).Wait()
	}
}

// closeSession retracts the entries of an attempt that did not settle passed
// and removes the attempt's cache directories (eviction ends a session this way).
func closeSession(ctx context.Context, st *store.Store, e cacheEntry) {
	retractUnpassed(ctx, st, e)
	e.s.Cleanup()
}

// retractUnpassed retracts the entries of an attempt that did not settle passed
// (a cancel won the finalizing -> passed move after the commit).
func retractUnpassed(ctx context.Context, st *store.Store, e cacheEntry) {
	if e.s.Saved() {
		if state, ok := settledState(ctx, e, st); ok && state != "passed" { // only a confirmed non-pass retracts
			e.s.Retract(ctx)
		}
	}
}

// settledState reads the attempt's state from the store it ran in, falling back
// to the caller's store (the run's handle may be closed by then).
func settledState(ctx context.Context, e cacheEntry, fallback *store.Store) (model.JobState, bool) {
	for _, st := range []*store.Store{e.st, fallback} {
		if st == nil {
			continue
		}
		if state, _, err := st.AttemptStateEpoch(ctx, e.id); err == nil {
			return state, true
		}
	}
	return "", false
}

func liveState(state model.JobState) bool {
	return state == "queued" || state == "leased" || state == "running" || state == "finalizing"
}

// live reports whether an attempt may still use its cache directories. An
// attempt the store cannot tell about is not live.
func live(ctx context.Context, st *store.Store, id string) bool {
	if st == nil {
		return false
	}
	state, _, err := st.AttemptStateEpoch(ctx, id)
	return err == nil && liveState(state)
}

// evict cleans up sessions whose attempt never reached After (prepare failed
// after Before). It acts on an attempt the store says has ended; when the store
// cannot say (or there is none) only on a session older than staleAfter. A
// live attempt is never touched.
func (h *cacheHook) evict(ctx context.Context, st *store.Store, now time.Time) {
	h.sessions.Range(func(k, v any) bool {
		e := v.(cacheEntry)
		dead := now.Sub(e.at) > staleAfter
		if state, ok := settledState(ctx, e, st); ok {
			dead = !liveState(state)
		}
		if dead && h.sessions.CompareAndDelete(k, v) { // After may have claimed it
			closeSession(ctx, st, e)
		}
		return true
	})
}
