package sched

// Tests of MulDivUp and of Cost.DivMs (P7-SCHED-07): a rate per DivMs
// milliseconds keeps its precision and rounds up once; DivMs 0 is the old
// per-millisecond rate, byte for byte.

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestMulDivUpIsExactOrSaturates(t *testing.T) {
	seed := rand.Int63()
	rng := rand.New(rand.NewSource(seed))
	pick := func() int64 {
		if rng.Intn(3) == 0 {
			return math.MaxInt64 - rng.Int63n(1<<10)
		}
		return rng.Int63() >> uint(rng.Intn(63))
	}
	for i := 0; i < 20_000; i++ {
		a, b, d := pick(), pick(), pick()+1
		if d <= 0 {
			d = math.MaxInt64
		}
		got, of := MulDivUp(a, b, d)
		num := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
		exact := new(big.Int).Div(new(big.Int).Add(num, big.NewInt(d-1)), big.NewInt(d))
		switch {
		case exact.IsInt64() && (of || got != exact.Int64()):
			t.Fatalf("seed %d: %d*%d/%d = %s, got %d overflow=%v", seed, a, b, d, exact, got, of)
		case !exact.IsInt64() && (!of || got != math.MaxInt64):
			t.Fatalf("seed %d: %d*%d/%d = %s exceeds int64, got %d overflow=%v", seed, a, b, d, exact, got, of)
		}
	}
	for _, bad := range [][3]int64{{-1, 1, 1}, {1, -1, 1}, {1, 1, 0}, {1, 1, -3}} {
		if got, of := MulDivUp(bad[0], bad[1], bad[2]); !of || got != math.MaxInt64 {
			t.Errorf("%v accepted: %d %v", bad, got, of)
		}
	}
	if got, of := MulDivUp(7, 3, 2); of || got != 11 {
		t.Errorf("ceil(21/2) = %d %v", got, of)
	}
}

func TestPaidCostWithADivisor(t *testing.T) {
	plain := Cost{RateMicro: 2, BillingIncrementMs: 60000}
	if got, of := worstCost(120000, plain); of || got != 240000 {
		t.Fatalf("per-millisecond rate unchanged: %d %v", got, of)
	}
	hourly := Cost{RateMicro: 3_600_000, DivMs: 3_600_000, BillingIncrementMs: 60000, MinimumChargeMicro: 5, ExtrasMicro: 1}
	if got, of := worstCost(90_000, hourly); of || got != 120_000+5+1 {
		t.Fatalf("two one-minute increments at 3.6M micro per hour: %d %v", got, of)
	}
	if got, of := worstCost(60_000, Cost{RateMicro: math.MaxInt64, DivMs: 1, BillingIncrementMs: 60000}); !of || got != math.MaxInt64 {
		t.Fatalf("saturation: %d %v", got, of)
	}
}

// TestCostClassFromTheContributorWins: a runner that says it is `owned` on a provider the
// owner priced as metered is held to the paid checks, so the scheduler and the budget
// step (which prices by the owner's table) agree on what it costs.
func TestCostClassFromTheContributorWins(t *testing.T) {
	r := fixtureRunner("claims-owned", "lan")
	r.Class, r.Provider = "owned", "cloudco"
	j := fixtureJob("j")
	j.TimeoutMs = 120000
	w := fixtureWorld(r)
	w.Costs[r.ID] = Cost{Class: "metered", RateMicro: 2, BillingIncrementMs: 60000}
	w.Policy.PaidEnabled = true
	got := Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.BudgetUnset)) {
		t.Fatalf("a rated provider must face the paid checks whatever its runner claims: %+v", got)
	}
	w.Budgets.Day = Budget{Set: true, Remaining: 240000}
	got = Place(w, []Job{j})
	if len(got.Placements) != 1 || got.Placements[0].CostMicro != 240000 {
		t.Fatalf("priced, not free: %+v", got)
	}
	w.Apply(got.Placements[0])
	if w.Budgets.Day.Remaining != 0 {
		t.Fatalf("a placement debits the budget by the contributor's class: %d", w.Budgets.Day.Remaining)
	}
}
