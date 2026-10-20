package runnerjob

// Purpose: the finalizer and the orphan sweep (Epic D6): a run that ends deregisters the runner of each
// of its attempts and proves it absent; a Reconciler pass removes registrations nobody owns any more
// (a coordinator that died between mint and delivery, a node that vanished).
// Inputs:  run.RunInfo of a closed run; the ledger rows; the minters.
// Outputs: removed registrations, rows marked gone; Finalize returns the joined failures (a note on the
//          run, never a changed verdict); Reconcile returns the joined failures of one pass.
// Constraints: both paths are idempotent and safe to repeat. A row stays minted until the minter has
//          proved the runner absent, so a failed removal is retried on the next pass. The sweep removes a
//          ledger row's registration only after MintedTTL (no runner job lasts longer; a busy runner is
//          never deleted early), and asks the minter for idle registrations older than OrphanAge.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

// OrphanAge is how old an idle registration must be before the sweep removes it: the forge honours an
// unused one for 60 minutes, plus ten for clock drift.
const OrphanAge = 70 * time.Minute

// SweepEvery is the cadence of the orphan sweep.
const SweepEvery = 5 * time.Minute

// Finalize removes the registration of every listed attempt that has one. The composition root adapts it
// to run.Finalizer (cmd/providers_github_demand.go): this package may not import run.
func Finalize(ctx context.Context, attempts []string) error {
	led := CurrentLedger()
	if led == nil {
		return nil
	}
	var errs []error
	for _, a := range attempts {
		if a != "" {
			errs = append(errs, Teardown(ctx, led, a))
		}
	}
	return errors.Join(errs...)
}

// Teardown removes the registration of one attempt and marks it gone. An attempt with no registration, or
// one already gone, is nothing to do.
func Teardown(ctx context.Context, led *Ledger, attempt string) error {
	row, ok, err := led.Runner(ctx, attempt)
	if err != nil {
		return fail("ledger.read")
	}
	if !ok || row.State != stateMinted {
		return nil
	}
	return removeRow(ctx, led, row)
}

func removeRow(ctx context.Context, led *Ledger, row Runner) error {
	_, m := state.minterOf(row.Provider)
	if m == nil {
		return fail("minter.unavailable")
	}
	if err := m.Deregister(ctx, row.Repo, row.Handle); err != nil {
		return err
	}
	if err := led.MarkGone(ctx, row); err != nil {
		return fail("ledger.write")
	}
	return nil
}

// Sweeper is the orphan sweep, registered as a coordinator Reconciler.
type Sweeper struct {
	Now func() time.Time // default time.Now
}

// Name is the Reconciler name.
func (*Sweeper) Name() string { return "runnerjob-orphans" }

// Every is the sweep cadence.
func (*Sweeper) Every() time.Duration { return SweepEvery }

// Reconcile runs one pass over every repository the ledger knows.
func (s *Sweeper) Reconcile(ctx context.Context, _ coordinator.Enqueuer) error {
	// compat.V15(P7-HOST-19): no sweep (v1.4) -> orphaned forge-runner registrations are removed
	if !compat.V15() {
		return nil
	}
	led := CurrentLedger()
	if led == nil {
		return nil
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	repos, err := led.Repos(ctx)
	if err != nil {
		return fail("ledger.read")
	}
	at := now()
	var errs []error
	for _, k := range repos {
		if ctx.Err() != nil {
			break
		}
		errs = append(errs, s.sweepRepo(ctx, led, k, at))
	}
	return errors.Join(errs...)
}

func (s *Sweeper) sweepRepo(ctx context.Context, led *Ledger, k RepoKey, now time.Time) error {
	cutoff := now.Add(-OrphanAge)
	_, m := state.minterOf(k.Provider)
	if m == nil {
		return nil
	}
	var errs []error
	open, err := led.Open(ctx, k.Repo)
	if err != nil {
		return fail("ledger.read")
	}
	for _, row := range open {
		if now.Sub(row.At) > MintedTTL { // no runner job lasts longer: whatever is left is nobody's
			errs = append(errs, removeRow(ctx, led, row))
		}
	}
	handles, err := m.Orphans(ctx, k.Repo, cutoff)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, h := range handles {
		errs = append(errs, m.Deregister(ctx, k.Repo, h))
	}
	return errors.Join(errs...)
}
