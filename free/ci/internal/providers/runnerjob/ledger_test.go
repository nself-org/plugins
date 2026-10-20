package runnerjob_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// TestLedgerReserveIsAtomicUnderConcurrency: thirty demands of one repository race for three slots and
// exactly three win.
func TestLedgerReserveIsAtomicUnderConcurrency(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := led.Reserve(context.Background(), "fakeforge", testRepo, strconv.Itoa(1000+i), 3); err == nil && ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 3 {
		t.Fatalf("%d demands took a slot, the cap is 3", won.Load())
	}
}

// TestLedgerDoesNotGrowWithEveryJobEverRun: ended demands and runners leave the repository document an hour
// after they end, so the document holds what is live (or just ended) and nothing older.
func TestLedgerDoesNotGrowWithEveryJobEverRun(t *testing.T) {
	st := use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		d := strconv.Itoa(2000 + i)
		if ok, err := led.Reserve(ctx, "fakeforge", testRepo, d, 5); err != nil || !ok {
			t.Fatal(err)
		}
		r := runnerjob.Runner{Attempt: "att-" + d, Provider: "fakeforge", Repo: testRepo, Demand: d, Handle: "repo:" + d}
		if err := led.RecordRunner(ctx, r); err != nil {
			t.Fatal(err)
		}
		got, _, _ := led.Runner(ctx, "att-"+d)
		if err := led.MarkGone(ctx, got); err != nil {
			t.Fatal(err)
		}
		advance(10 * time.Minute)
	}
	f, ok, err := st.ProviderFactGet(ctx, "runnerjob", testRepo, "state")
	if err != nil || !ok {
		t.Fatalf("no state document: %v", err)
	}
	if n := strings.Count(string(f.Value), `"attempt"`); n > 7 {
		t.Fatalf("the document keeps %d runner rows after 200 minutes; ended ones should leave after an hour: %s", n, f.Value)
	}
	if got, ok, _ := led.Runner(ctx, "att-2019"); !ok || got.State != "gone" {
		t.Fatalf("a recently ended row is still readable: %+v %v", got, ok)
	}
	if _, ok, _ := led.Runner(ctx, "att-2000"); ok {
		t.Fatal("an old ended row should be gone")
	}
}
