package main

// Purpose: renew hosted leases from their provider's liveness while the coordinator serves.
// Inputs: the coordinator lease engine, durable handle book and process ownership check.
// Outputs: registered hosted profiles and a cancellable polling worker.
// Constraints: only matching fenced handles owned by this process are polled; PollDue enforces
// each provider's interval. Shutdown joins the worker before detaching the store.

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func startHostedPolling(ctx context.Context, eng *lease.Engine, book runtime.Book, owned func(context.Context, string) bool, logf func(string, ...any)) (func(), error) {
	if err := runtime.RegisterProfiles(eng, book); err != nil {
		return nil, err
	}
	pctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-tick.C:
				leases, err := eng.Store.LeaseExpired(pctx, time.Unix(0, math.MaxInt64))
				if err == nil {
					err = pollHostedLeases(pctx, leases, book, owned, eng.PollDue)
				}
				if err != nil && pctx.Err() == nil && logf != nil {
					logf("hosted lease polling: %v", err)
				}
			}
		}
	}()
	return func() { cancel(); wg.Wait() }, nil
}

func pollHostedLeases(ctx context.Context, leases []store.RemoteLease, book runtime.Book, owned func(context.Context, string) bool, poll func(context.Context, string, string, string, int64) (bool, lease.LiveState, error)) error {
	var errs []error
	for _, l := range leases {
		if !owned(ctx, l.CoordinatorID) {
			continue
		}
		e, ok, err := book.Get(ctx, l.AttemptID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ok || e.Epoch != l.Epoch || e.Handle.Provider == "" {
			continue
		}
		if _, _, err := poll(ctx, e.Handle.Provider, l.ID, l.AttemptID, l.Epoch); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
