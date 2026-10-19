package run

// Purpose: cache session close, settle state and eviction helpers (split from cache_hook.go, D-1252).

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// closeSession retracts the entries of an attempt that did not settle passed
// (a cancel won the finalizing -> passed move after the commit) and removes the
// attempt's cache directories. After and eviction both end a session this way.
func closeSession(ctx context.Context, st *store.Store, e cacheEntry) {
	if e.s.Saved() {
		if state, ok := settledState(ctx, e, st); ok && state != "passed" { // only a confirmed non-pass retracts
			e.s.Retract(ctx)
		}
	}
	e.s.Cleanup()
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

// imageDigest is the pinned digest of the job's container image, "" if unknown.
func (h *cacheHook) imageDigest(ctx context.Context, a AttemptInfo) string {
	img := ""
	if a.Job.Container != nil {
		img = a.Job.Container.Image
	}
	if img == "" && Registered.Image != nil {
		img, _ = Registered.Image.Image(ctx, a)
	}
	if _, d, ok := strings.Cut(img, "@"); ok && strings.HasPrefix(d, "sha256:") {
		return d
	}
	return ""
}
