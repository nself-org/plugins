package economics

// Tests of the ephemeral resource ledger (Epic D12, review F7): a burst VM is
// reserved for its whole life, released only on a confirmed teardown, and
// shares the budgets with ordinary attempts.

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func reserveVM(ctx context.Context, st *store.Store, intent string, lifetime time.Duration) error {
	return st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
		return ReserveResource(ctx, tx, Resource{Intent: intent, Provider: "cloudco", Pipeline: "pipe", LifetimeMs: lifetime.Milliseconds()})
	})
}

func releaseVM(ctx context.Context, st *store.Store, intent string, at time.Time) error {
	return st.LedgerWrite(ctx, func(tx store.LedgerTx) error { return ReleaseResource(ctx, tx, intent, at) })
}

func TestBudgetResourceHoldsItsLifetimeUntilConfirmedTeardown(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	if err := reserveVM(c, st, "vm-1", 10*time.Minute); err != nil { // 600 000 of the 1 000 000 day budget
		t.Fatal(err)
	}
	var se *SpendError
	err := reserveVM(c, st, "vm-2", 10*time.Minute)
	if !asSpend(err, &se) || se.Reason != model.BudgetExceeded {
		t.Fatalf("a second lifetime does not fit: %v", err)
	}
	if _, ok, _ := st.Resource(ctx, "vm-2"); ok {
		t.Fatal("a refused reservation left a resource row")
	}
	// The VM outlives the attempts that used it: the attempt settles, the VM's money stays held.
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err = place(c, st, request(a, "", "cloudco", 300_000, 0)); err != nil { // 300 000 fits beside 600 000
		t.Fatal(err)
	}
	finalize(t, c, a, time.Minute) // settles at 60 000
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 600_000+60_000 {
		t.Fatalf("day held %d", held)
	}
	b := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(b, "", "cloudco", 360_000, 0))); ref.Reason != model.BudgetExceeded { // 660 000 + 360 000 > 1 000 000
		t.Fatalf("the VM's reservation must still count after its attempt ended: %+v", ref)
	}
	// Teardown is confirmed 4 minutes in: the actual (240 000) replaces the 600 000 reservation.
	clk.set(clk.now().Add(4 * time.Minute))
	if err = releaseVM(c, st, "vm-1", clk.now()); err != nil {
		t.Fatal(err)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 240_000+60_000 {
		t.Fatalf("after teardown the day holds %d", held)
	}
	if err = releaseVM(c, st, "vm-1", clk.now().Add(time.Hour)); err != nil { // a second release is a no-op
		t.Fatal(err)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 240_000+60_000 {
		t.Fatalf("a repeated release moved money: %d", held)
	}
	row, ok, _ := st.Resource(ctx, "vm-1")
	if !ok || row.ConfirmedTeardownAt == nil || row.ActualMicro == nil || *row.ActualMicro != 240_000 {
		t.Fatalf("resource row: %+v", row)
	}
	if err = releaseVM(c, st, "unknown-vm", clk.now()); err == nil {
		t.Fatal("released a resource that was never reserved")
	}
}

func asSpend(err error, target **SpendError) bool {
	for err != nil {
		if s, ok := err.(*SpendError); ok {
			*target = s
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestBudgetResourceNeedsPaidSpendAndAnOwnedAccount(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	off := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyPaidEnabled: `false`}))
	var se *SpendError
	if err := reserveVM(WithEnv(ctx, envFor(st, off, clk, "me")), st, "vm", time.Minute); !asSpend(err, &se) || se.Reason != model.PaidDisabled {
		t.Fatalf("paid off: %v", err)
	}
	other := spendPolicy(t, paidKV(map[string]string{"economics.providers.cloudco": `{"owner_coordinator":"coord-b"}`}))
	if err := reserveVM(WithEnv(ctx, envFor(st, other, clk, "coord-a")), st, "vm", time.Minute); !asSpend(err, &se) || se.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("owned elsewhere: %v", err)
	}
	if err := reserveVM(ctx, st, "vm", time.Minute); !asSpend(err, &se) {
		t.Fatalf("an unbound process must not reserve: %v", err)
	}
	if err := reserveVM(WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me")), st, "vm", 200*time.Hour); !asSpend(err, &se) || se.Reason != model.CostUnbounded {
		t.Fatalf("a lifetime past the hard cap: %v", err)
	}
}

// TestBudgetConcurrentReservationsNeverExceedAScope: 12 000 concurrent
// reservations (attempts and burst VMs, with releases) against five budgets;
// after every commit the money held in each scope is within its limit. Seeds
// are printed on failure.
func TestBudgetConcurrentReservationsNeverExceedAScope(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	const day, month, provider, pipeline = 400_000_000, 800_000_000, 600_000_000, 30_000_000
	p := spendPolicy(t, paidKV(map[string]string{
		schedpolicy.KeyBudgetDay: "400000000", schedpolicy.KeyBudgetMonth: "800000000", "economics.budgets.provider.cloudco": "600000000",
		schedpolicy.KeyBudgetPipeline: "30000000", schedpolicy.KeyBudgetJob: "400000",
	}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	check := func(tx store.LedgerTx, pipe string) {
		for _, s := range []struct {
			name, key, win string
			limit          int64
		}{{"day", "", "2026-10-19", day}, {"month", "", "2026-10", month}, {"provider", "cloudco", "2026-10", provider}, {"pipeline", pipe, "", pipeline}} {
			held, err := tx.Reserved(s.name, s.key, s.win)
			if err != nil {
				t.Error(err)
			} else if held > s.limit {
				t.Errorf("%s %s holds %d, over its limit %d", s.name, s.key, held, s.limit)
			}
		}
	}
	var accepted, refused atomic.Int64
	seed := rand.Int63()
	var wg sync.WaitGroup
	const workers, ops = 8, 2600
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed + int64(w)))
			var vms, attempts []string
			for i := 0; i < ops; i++ {
				pipe := "p" + strconv.Itoa(rng.Intn(4))
				owner := "w" + strconv.Itoa(w) + "-" + strconv.Itoa(i)
				switch k := rng.Intn(10); {
				case k < 4: // an attempt of 1 to 5 minutes at 60 000 micro per minute
					cost := int64(1+rng.Intn(5)) * 60_000
					err := st.LedgerWrite(c, func(tx store.LedgerTx) error {
						d, m, err := windows(tx, clk.now())
						if err != nil {
							return err
						}
						if _, err = reserve(tx, p, spend{Owner: owner, Pipeline: pipe, Provider: "cloudco", Micro: cost}, d, m); err != nil {
							return err
						}
						check(tx, pipe)
						return tx.PutLedger(store.LedgerRow{Attempt: owner, Pipeline: pipe, Provider: "cloudco", Class: ClassMetered, EstMicro: cost, Confidence: ConfEstimated, WindowDay: d})
					})
					if err == nil {
						attempts = append(attempts, owner)
					}
					count(err, &accepted, &refused, t)
				case k < 6: // a run closes: the attempt settles at a third of its reservation
					if len(attempts) > 0 {
						j := rng.Intn(len(attempts))
						row, _, _ := st.LedgerAttempt(c, attempts[j])
						if _, err := st.LedgerSettle(c, attempts[j], 1, row.EstMicro/3, ConfComputed); err != nil {
							t.Errorf("seed %d: settle: %v", seed, err)
						}
						attempts = append(attempts[:j], attempts[j+1:]...)
					}
				case k < 8: // a VM for 1 to 6 minutes of life
					err := st.LedgerWrite(c, func(tx store.LedgerTx) error {
						if err := ReserveResource(c, tx, Resource{Intent: owner, Provider: "cloudco", Pipeline: pipe, LifetimeMs: int64(1+rng.Intn(6)) * 60_000}); err != nil {
							return err
						}
						check(tx, pipe)
						return nil
					})
					if err == nil {
						vms = append(vms, owner)
					}
					count(err, &accepted, &refused, t)
				default: // a teardown, sometimes long after the attempt
					if len(vms) > 0 {
						j := rng.Intn(len(vms))
						if err := releaseVM(c, st, vms[j], clk.now().Add(time.Duration(rng.Intn(300))*time.Second)); err != nil {
							t.Errorf("seed %d: release: %v", seed, err)
						}
						vms = append(vms[:j], vms[j+1:]...)
					}
				}
			}
		}(w)
	}
	wg.Wait()
	if t.Failed() {
		t.Fatalf("seed %d", seed)
	}
	if accepted.Load() < 500 || refused.Load() < 500 || accepted.Load()+refused.Load() < 10_000 {
		t.Fatalf("seed %d: the generator must both fit and overflow the budgets: accepted %d refused %d", seed, accepted.Load(), refused.Load())
	}
	t.Logf("seed %d: %d reservations accepted, %d refused", seed, accepted.Load(), refused.Load())
	for _, s := range []struct {
		name, key, win string
		limit          int64
	}{{"day", "", "2026-10-19", day}, {"month", "", "2026-10", month}, {"provider", "cloudco", "2026-10", provider}} {
		if held, _ := st.Reserved(ctx, s.name, s.key, s.win); held > s.limit {
			t.Fatalf("seed %d: final %s holds %d over %d", seed, s.name, held, s.limit)
		}
	}
}

func count(err error, accepted, refused *atomic.Int64, t *testing.T) {
	var se *SpendError
	switch {
	case err == nil:
		accepted.Add(1)
	case asSpend(err, &se) && se.Reason == model.BudgetExceeded:
		refused.Add(1)
	default:
		t.Errorf("unexpected error: %v", err)
	}
}
