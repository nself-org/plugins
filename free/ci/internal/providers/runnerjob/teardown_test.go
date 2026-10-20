package runnerjob_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// finalize is what the run finalizer (cmd/providers_github_demand.go) calls for a closed run.
func finalize(ctx context.Context, attempts ...string) error {
	return runnerjob.Finalize(ctx, attempts)
}

// TestFinalizerDeregistersAndFreesTheSlot: when a run closes, the runner of each of its attempts is
// removed and proven absent, the ledger row is gone and the demand's slot is free again; a second run of
// the finalizer is a no-op.
func TestFinalizerDeregistersAndFreesTheSlot(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "21", 1); !ok {
		t.Fatal("reserve")
	}
	if _, err := deliver(goodNode(), "attempt-z", runnerJob(t, "21")); err != nil {
		t.Fatal(err)
	}
	if forge.alive() != 1 {
		t.Fatal("registration should exist before the run closes")
	}
	if err := finalize(ctx, "attempt-z", "attempt-without-runner", ""); err != nil {
		t.Fatal(err)
	}
	if forge.alive() != 0 {
		t.Fatal("registration must be gone")
	}
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "22", 1); !ok {
		t.Fatal("the slot of a finished demand must be free")
	}
	if err := finalize(ctx, "attempt-z"); err != nil || len(forge.deregCalls) != 1 {
		t.Fatalf("second finalize: %v, deregister calls %v", err, forge.deregCalls)
	}
}

// TestFinalizerKeepsTheRowWhenAbsenceIsNotProven: a failed removal is returned (a note on the run) and the
// row stays minted, so the next pass retries.
func TestFinalizerKeepsTheRowWhenAbsenceIsNotProven(t *testing.T) {
	use(t)
	if _, err := deliver(goodNode(), "attempt-y", runnerJob(t, "23")); err != nil {
		t.Fatal(err)
	}
	forge.deregErr = providers.NewError(providers.Config, "E707", "deregister_unverified", 0, epoch)
	err := finalize(context.Background(), "attempt-y")
	if err == nil || !strings.Contains(err.Error(), "E707") {
		t.Fatalf("want E707, got %v", err)
	}
	row, _, _ := ledgerRunner("attempt-y")
	if row.State != "minted" {
		t.Fatalf("row state %q", row.State)
	}
	forge.deregErr = nil
	if err = finalize(context.Background(), "attempt-y"); err != nil || forge.alive() != 0 {
		t.Fatalf("retry: %v alive %d", err, forge.alive())
	}
}

func sweeper() *runnerjob.Sweeper { return &runnerjob.Sweeper{Now: clock} }

// TestSweepRemovesOrphansOlderThanSeventyMinutes: the minter lists idle registrations older than the
// cutoff the sweep passes (OrphanAge, 70 minutes) and the sweep removes them; it passes the right cutoff.
func TestSweepRemovesOrphansOlderThanSeventyMinutes(t *testing.T) {
	use(t)
	if _, err := deliver(goodNode(), "attempt-s", runnerJob(t, "24")); err != nil { // makes the repo known to the ledger
		t.Fatal(err)
	}
	forge.mu.Lock()
	forge.live["repo:900"] = epoch.Add(-3 * time.Hour)
	forge.orphanAfter["repo:900"] = true
	forge.mu.Unlock()
	if err := sweeper().Reconcile(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, still := forge.live["repo:900"]; still {
		t.Fatal("the orphan must be deleted")
	}
	if _, kept := forge.live["repo:101"]; !kept {
		t.Fatal("a registration younger than MintedTTL that the minter does not list as orphan must stay")
	}
	if want := clock().Add(-70 * time.Minute); !forge.lastCutoff.Equal(want) {
		t.Fatalf("the sweep asked for orphans before %v, want %v", forge.lastCutoff, want)
	}
}

// TestSweepRemovesLedgerRowsPastMintedTTL: a row whose job cannot still be running is removed and closed
// even though the minter lists no orphan (a node that vanished), and an error of one repo does not stop
// the pass.
func TestSweepRemovesLedgerRowsPastMintedTTL(t *testing.T) {
	use(t)
	if _, err := deliver(goodNode(), "attempt-old", runnerJob(t, "25")); err != nil {
		t.Fatal(err)
	}
	advance(2 * time.Hour)
	if err := sweeper().Reconcile(context.Background(), nil); err != nil || forge.alive() != 1 {
		t.Fatalf("a row younger than MintedTTL is left to its job: err %v alive %d", err, forge.alive())
	}
	advance(runnerjob.MintedTTL)
	if err := sweeper().Reconcile(context.Background(), nil); err != nil || forge.alive() != 0 {
		t.Fatalf("a row past MintedTTL must go: err %v alive %d", err, forge.alive())
	}
	forge.orphansErr = providers.NewError(providers.Transient, "E704", "down", 0, epoch)
	if err := sweeper().Reconcile(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "E704") {
		t.Fatalf("a minter failure is reported: %v", err)
	}
}

// TestSweepIsDormantInV14AndUnbound.
func TestSweepIsDormantInV14AndUnbound(t *testing.T) {
	use(t)
	if _, err := deliver(goodNode(), "attempt-d", runnerJob(t, "26")); err != nil {
		t.Fatal(err)
	}
	advance(8 * time.Hour)
	t.Setenv("NSELF_V15", "0")
	if err := sweeper().Reconcile(context.Background(), nil); err != nil || forge.alive() != 1 {
		t.Fatalf("v1.4 must not sweep: %v alive %d", err, forge.alive())
	}
	t.Setenv("NSELF_V15", "1")
	runnerjob.Use(nil)
	if err := sweeper().Reconcile(context.Background(), nil); err != nil || forge.alive() != 1 {
		t.Fatalf("unbound must not sweep: %v alive %d", err, forge.alive())
	}
	if sweeper().Name() != "runnerjob-orphans" || sweeper().Every() != runnerjob.SweepEvery {
		t.Fatal("name or cadence")
	}
}
