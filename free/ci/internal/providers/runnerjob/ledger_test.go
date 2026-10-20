package runnerjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// TestLedgerCapIsPerRepoAndIdempotent: Reserve takes a slot per demand, accepts the same demand again
// without a second slot, refuses past the cap, keeps repositories apart and frees a slot on MarkGone.
func TestLedgerCapIsPerRepoAndIdempotent(t *testing.T) {
	st := use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	reserve := func(repo, demand string) bool {
		t.Helper()
		ok, err := led.Reserve(ctx, "fakeforge", repo, demand, 2)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !reserve(testRepo, "1") || !reserve(testRepo, "2") || !reserve(testRepo, "1") {
		t.Fatal("two demands and a repeat of the first must be accepted")
	}
	if reserve(testRepo, "3") {
		t.Fatal("the third live demand must be refused")
	}
	if !reserve("acme/other", "3") {
		t.Fatal("the cap is per repository: another repository has its own slots")
	}
	if reserve("ACME/Widgets", "4") {
		t.Fatal("repository names compare case-insensitively")
	}
	row := runnerjob.Runner{Attempt: "a1", Provider: "fakeforge", Repo: testRepo, Demand: "1", Handle: "repo:101"}
	if err := led.RecordRunner(ctx, row); err != nil {
		t.Fatal(err)
	}
	got, ok, err := led.Runner(ctx, "a1")
	if err != nil || !ok || got.Handle != "repo:101" || got.State != "minted" {
		t.Fatalf("runner row %+v ok=%v err=%v", got, ok, err)
	}
	if err = led.MarkGone(ctx, got); err != nil {
		t.Fatal(err)
	}
	if !reserve(testRepo, "3") {
		t.Fatal("a finished demand must free its slot")
	}
	open, _ := led.Open(ctx, testRepo)
	if len(open) != 0 {
		t.Fatalf("a gone runner is not open: %v", open)
	}
	repos, _ := led.Repos(ctx)
	if len(repos) != 2 {
		t.Fatalf("repos %v", repos)
	}
	_ = st
}

// TestLedgerPendingDemandExpires: a demand that was never admitted frees its slot after PendingTTL, a
// minted one only after MintedTTL (a runner job lasts hours).
func TestLedgerPendingDemandExpires(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "1", 1); !ok {
		t.Fatal("first")
	}
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "2", 1); ok {
		t.Fatal("capped")
	}
	advance(runnerjob.PendingTTL + time.Second)
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "2", 1); !ok {
		t.Fatal("an unadmitted demand must expire")
	}
	if err := led.RecordRunner(ctx, runnerjob.Runner{Attempt: "a2", Provider: "fakeforge", Repo: testRepo, Demand: "2", Handle: "repo:102"}); err != nil {
		t.Fatal(err)
	}
	advance(2 * time.Hour)
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "3", 1); ok {
		t.Fatal("a minted runner job may run for hours and still holds its slot")
	}
	advance(runnerjob.MintedTTL)
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "3", 1); !ok {
		t.Fatal("past MintedTTL nothing can still be running")
	}
}
