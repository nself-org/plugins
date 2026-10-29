package main

// Purpose: the read side of `nself ci runs wait` (P7-CI-63, Epic D30): runSource reads one run's
// state per poll, from a RemoteResolver when the run lives on another machine, else from the local
// store (split from runs_wait.go to keep each file within the 300-line budget).
// Inputs:  a run reference, the registered RemoteResolvers, NSELF_CI_HOME.
// Outputs: detach.View per poll, plus the RunView detail of the latest poll.
// Constraints: every call is bounded by the caller's context; a held SQLite lock never outlives it.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// runSource reads one run's state: from a RemoteResolver when the run lives on
// another machine, else from the local store.
type runSource struct {
	resolver  cirun.RemoteResolver
	ref       string
	remote    bool
	st        *store.Store
	row       store.PipelineRow
	found     bool
	read      bool            // a local read has finished
	firstRead context.Context // when set, bounds the first local read instead of the caller's context
	last      time.Time
	// Detail is the RunView of the latest View call.
	Detail cirun.RunView
}

// recoverEvery bounds how often a poll runs dead-coordinator recovery.
const recoverEvery = 2 * time.Second

func openRunSource(ctx context.Context, ref, coordinator string) (*runSource, error) {
	return openRunSourceGrace(ctx, ctx, ref, coordinator)
}

// openRunSourceGrace is openRunSource with a separate context for opening the local store, which
// bounds remote lookups by ctx and the store open by storeCtx.
func openRunSourceGrace(ctx, storeCtx context.Context, ref, coordinator string) (*runSource, error) {
	if coordinator != "" {
		if err := os.Setenv(detach.EnvCoordinator, coordinator); err != nil {
			return nil, err
		}
	}
	s := &runSource{ref: ref}
	for _, r := range cirun.Registered.Remote {
		v, ok, err := r.Resolve(ctx, ref)
		if err != nil && cirun.RemoteFatal(r) { // an active coordinator target never falls back to the local store (P7-NODE-19)
			return nil, err
		}
		if err == nil && ok {
			s.remote, s.Detail, s.resolver = true, v, r
			return s, nil
		}
	}
	// recovery after dead coordinators runs in View, under the caller's deadline
	st, err := openStoreCtx(storeCtx, runsStorePath())
	if err != nil {
		return nil, err
	}
	s.st = st
	return s, nil
}

// Close releases the store.
func (s *runSource) Close() {
	if s.st != nil {
		_ = s.st.Close()
	}
}

// View returns the run's current state.
func (s *runSource) View(ctx context.Context) (detach.View, error) {
	if s.remote {
		for _, r := range cirun.Registered.Remote {
			v, ok, err := r.Resolve(ctx, s.ref)
			if err != nil {
				return detach.View{}, err
			}
			if ok {
				s.Detail = v
				return toView(v), nil
			}
		}
		return detach.View{}, fmt.Errorf("run %q is no longer known to its coordinator", s.ref)
	}
	if !s.read && s.firstRead != nil {
		ctx = s.firstRead
	}
	if time.Since(s.last) >= recoverEvery {
		s.last = time.Now()
		_ = cirun.Recover(ctx, s.st, "", os.Getenv("NSELF_CI_HOME"))
	}
	if !s.found {
		row, err := cirun.ResolveRun(ctx, s.st, s.ref)
		if err != nil {
			return detach.View{}, err
		}
		s.row, s.found = row, true
	} else if row, ok, err := s.st.PipelineByID(ctx, s.row.ID); err != nil {
		return detach.View{}, err
	} else if ok {
		s.row = row
	}
	v, err := cirun.ViewRun(ctx, s.st, s.row)
	if err != nil {
		return detach.View{}, err
	}
	s.Detail = v
	s.read = true
	return toView(v), nil
}

func toView(v cirun.RunView) detach.View {
	out := detach.View{RunID: v.ID, State: v.Status}
	for _, j := range v.Jobs {
		out.Jobs = append(out.Jobs, detach.JobState{Name: j.Name, State: j.State, FailureClass: j.FailureClass})
	}
	return out
}

// openStoreCtx opens the run store but gives up when ctx ends (a held SQLite
// write lock can make the open wait for seconds); the open finishes in the
// background and closes its store.
func openStoreCtx(ctx context.Context, path string) (*store.Store, error) {
	type result struct {
		st  *store.Store
		err error
	}
	ch := make(chan result, 1)
	go func() {
		st, err := cirun.OpenStore(path)
		ch <- result{st, err}
	}()
	select {
	case r := <-ch:
		return r.st, r.err
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.st != nil {
				_ = r.st.Close()
			}
		}()
		return nil, ctx.Err()
	}
}
