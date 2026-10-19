package jit

// Purpose: Deregister and Orphans, the teardown half (Epic D6).
// Inputs:  a RunnerRef from MintRunner; an orphan cutoff.
// Outputs: nil when the runner is gone and proven gone; the orphan list of the configured scope.
// Constraints: DELETE answering 404 means done and needs no second look. Any other success is
//          followed by a read of the runner list, and a runner still in it is E707
//          deregister_unverified, never a silent pass. Orphans are runners this package named
//          (nself-jit-<ulid>), not busy, born before the cutoff; a name that is not ours or whose
//          ULID does not parse is never returned, because deleting a persistent runner is worse than
//          leaving an orphan.

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Deregister deletes the runner and verifies it is absent from the runner list.
func (r *Registrar) Deregister(ctx context.Context, ref providers.RunnerRef) error {
	t, err := r.resolve(ref.Scope)
	if err != nil {
		return err
	}
	if ref.ID == "" {
		return providers.NewError(providers.Config, "E702", "runner.id", 0, time.Time{})
	}
	tmpl, params := r.runnersPath(t)
	params["id"] = ref.ID
	resp, err := r.c.Do(ctx, "DELETE", tmpl+"/{id}", params, nil, nil)
	if resp.Status == 404 {
		return nil
	}
	if err != nil {
		return runnerErr(err, "deregister")
	}
	all, err := r.listRunners(ctx, t)
	if err != nil {
		var pe *providers.Error
		if errors.As(err, &pe) && pe.Class != providers.Config {
			return err
		}
		return providers.NewError(providers.Config, "E707", "deregister_unverified", 0, time.Time{})
	}
	for _, x := range all {
		if strconv.FormatInt(x.ID, 10) == ref.ID {
			return providers.NewError(providers.Config, "E707", "deregister_unverified", 0, time.Time{})
		}
	}
	return nil
}

// Orphans lists the runners of scope that are older than before and idle. A zero before means
// OrphanAge ago.
func (r *Registrar) Orphans(ctx context.Context, scope providers.RunnerScope, before time.Time) ([]providers.RunnerRef, error) {
	t, err := r.resolve(scope)
	if err != nil {
		return nil, err
	}
	if before.IsZero() {
		before = r.now().Add(-OrphanAge)
	}
	all, err := r.listRunners(ctx, t)
	if err != nil {
		return nil, err
	}
	var out []providers.RunnerRef
	for _, x := range all {
		born, ok := runnerBirth(x.Name)
		if ok && !x.Busy && born.Before(before) {
			out = append(out, providers.RunnerRef{ID: strconv.FormatInt(x.ID, 10), Scope: scope})
		}
	}
	return out, nil
}
