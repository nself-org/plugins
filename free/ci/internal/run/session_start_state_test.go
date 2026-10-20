package run

import (
	"context"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// probeDispatch wraps the registered Dispatch hook (not parallel-safe: callers do not run in parallel)
// and records the Session each attempt is submitted under.
func probeDispatch(t *testing.T) func() *Session {
	t.Helper()
	var mu sync.Mutex
	var got *Session
	prev := Dispatch
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) {
		mu.Lock()
		got = SessionFrom(ctx)
		mu.Unlock()
		return prev(ctx, s)
	}
	t.Cleanup(func() { Dispatch = prev })
	return func() *Session { mu.Lock(); defer mu.Unlock(); return got }
}

// TestSessionRecordsTheStateTheRunStartedIn (D-1075): the release gate reads
// these three facts from the Session, so Coordinator.Run must set them.
func TestSessionRecordsTheStateTheRunStartedIn(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		dirty, outside, subset       bool
		wantDirty, wantOut, wantPart bool
	}{
		{"clean full run", false, false, false, false, false, false},
		{"dirty tree", true, false, false, true, false, false},
		{"config outside the commit", false, true, false, true, true, false},
		{"--job subset", false, false, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true")}, gitFake{dirty: tc.dirty})
			seen := probeDispatch(t)
			r.C.ConfigOutside = tc.outside
			if tc.subset {
				r.C.Opts.Job = "a"
			}
			r.run(t)
			s := seen()
			if s == nil {
				t.Fatal("the attempt was submitted without a session")
			}
			if s.Dirty != tc.wantDirty || s.ConfigOutside != tc.wantOut || s.Subset != tc.wantPart {
				t.Fatalf("session dirty=%v outside=%v subset=%v, want %v %v %v", s.Dirty, s.ConfigOutside, s.Subset, tc.wantDirty, tc.wantOut, tc.wantPart)
			}
		})
	}
}
