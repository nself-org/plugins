package economics

// Tests of the pessimistic bound, cost classes and the scheduler's view of a
// cost (Epic D11): billing increments, minimum charge and extras, GitHub as
// `included` only, and the arithmetic at the edge of int64.

import (
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestEconomicsOneMinuteJobHourlyBilled(t *testing.T) {
	rate := schedpolicy.Rate{Class: ClassMetered, PerHourMicro: 7_200_000, BillingIncrementMs: msPerHour, MinimumChargeMicro: 1000, ExtrasMicro: 500}
	q := QuoteJob(rate, 60_000, 0)
	if q.Reason != "" || q.BilledMs != msPerHour || q.Micro != 7_200_000+1000+500 {
		t.Fatalf("one minute on an hourly-billed provider is one increment plus minimum and extras: %+v", q)
	}
	rate = schedpolicy.Rate{PerMinuteMicro: 100, BillingIncrementMs: 60_000}
	if q = QuoteJob(rate, 61_000, 0); q.BilledMs != 120_000 || q.Micro != 200 {
		t.Fatalf("61 s rounds up to two one-minute increments: %+v", q)
	}
	// 3.6 million micro per hour is 1 micro per millisecond: 90 s in one-second steps is 90 000 micro.
	rate = schedpolicy.Rate{PerHourMicro: 3_600_000, BillingIncrementMs: 1000}
	if q = QuoteJob(rate, 90_000, 0); q.Micro != 90_000 {
		t.Fatalf("per-hour rate must not round per millisecond: %+v", q)
	}
	// An hourly rate that is not a whole number of micro per millisecond keeps its precision and rounds up once.
	rate = schedpolicy.Rate{PerHourMicro: 7300, BillingIncrementMs: 60_000}
	if q = QuoteJob(rate, 60_000, 0); q.Micro != 122 { // 7300/60 = 121.67
		t.Fatalf("a Hetzner-sized rate: %+v", q)
	}
}

func TestEconomicsQuoteReasons(t *testing.T) {
	good := schedpolicy.Rate{PerMinuteMicro: 10}
	for _, tc := range []struct {
		name    string
		rate    schedpolicy.Rate
		timeout int64
		capMs   int64
		want    model.PlacementReason
	}{
		{"no rate", schedpolicy.Rate{}, 60_000, 0, model.CostUnknownRate},
		{"no timeout", good, 0, 0, model.CostUnbounded},
		{"above the default cap", good, schedpolicy.DefaultMaxPaidTimeoutMs + 1, 0, model.CostUnbounded},
		{"above a lower cap", good, 61_000, 60_000, model.CostUnbounded},
		{"at the cap", good, 60_000, 60_000, ""},
		{"saturated", schedpolicy.Rate{PerMinuteMicro: math.MaxInt64, BillingIncrementMs: 60_000, ExtrasMicro: math.MaxInt64}, 3_600_000, 0, model.BudgetExceeded},
	} {
		if q := QuoteJob(tc.rate, tc.timeout, tc.capMs); q.Reason != tc.want {
			t.Errorf("%s: reason %q, want %q (%+v)", tc.name, q.Reason, tc.want, q)
		}
	}
}

// TestEconomicsChargeNeverWraps: at the edge of int64 a figure is exact or it
// saturates and says so (property, 20 000 cases, seed printed on failure).
func TestEconomicsChargeNeverWraps(t *testing.T) {
	seed := rand.Int63()
	rng := rand.New(rand.NewSource(seed))
	edge := []int64{0, 1, 2, 59_999, 60_000, 3_600_000, math.MaxInt32, math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64}
	pick := func() int64 {
		switch rng.Intn(4) {
		case 0:
			return edge[rng.Intn(len(edge))]
		case 1:
			return rng.Int63()
		case 2:
			return rng.Int63n(1 << 20)
		}
		return math.MaxInt64 - rng.Int63n(1<<16)
	}
	for i := 0; i < 20_000; i++ {
		rs := store.RateSnap{RateMicro: pick(), RateUnitMs: 1 + rng.Int63n(msPerHour), IncMs: pick(), MinMicro: pick(), ExtrasMicro: pick()}
		ms := pick()
		micro, billed, overflow := Charge(rs, ms)
		if micro < 0 || billed < 0 {
			t.Fatalf("seed %d case %d: negative figure for %+v ms=%d: %d %d", seed, i, rs, ms, micro, billed)
		}
		if ms <= 0 {
			if micro != 0 || overflow {
				t.Fatalf("seed %d: zero duration bills nothing: %d %v", seed, micro, overflow)
			}
			continue
		}
		inc := rs.IncMs
		if inc <= 0 {
			inc = defaultIncrement
		}
		units := new(big.Int).Div(new(big.Int).Add(big.NewInt(ms), big.NewInt(inc-1)), big.NewInt(inc))
		exactBilled := new(big.Int).Mul(units, big.NewInt(inc))
		num := new(big.Int).Mul(exactBilled, big.NewInt(rs.RateMicro))
		den := big.NewInt(rs.RateUnitMs)
		exact := new(big.Int).Div(new(big.Int).Add(num, new(big.Int).Sub(den, big.NewInt(1))), den)
		exact.Add(exact, big.NewInt(rs.MinMicro)).Add(exact, big.NewInt(rs.ExtrasMicro))
		if exact.IsInt64() && !exactBilled.IsInt64() {
			continue // billed saturated first; the figure is flagged below
		}
		if exact.IsInt64() {
			if overflow || micro != exact.Int64() {
				t.Fatalf("seed %d case %d: %+v ms=%d: got %d overflow=%v, exact %s", seed, i, rs, ms, micro, overflow, exact)
			}
		} else if !overflow || micro != math.MaxInt64 {
			t.Fatalf("seed %d case %d: %+v ms=%d: exact %s exceeds int64 but got %d overflow=%v (a wrapped figure)", seed, i, rs, ms, exact, micro, overflow)
		}
	}
}

func TestEconomicsGitHubIsIncludedOnly(t *testing.T) {
	p := spendPolicy(t, paidKV(map[string]string{"economics.rates.github-hosted": `{"class":"included","rate_per_minute_micro":8}`}))
	for _, id := range []string{"github-hosted", "github-jit", "gh", "hosted", "actions", "git-hub"} {
		for _, runnerClass := range []string{"", ClassMetered, ClassEphemeral, ClassOwned} {
			if got := classOf(p, runnerClass, id); got != ClassIncluded {
				t.Errorf("provider %s with runner class %q priced as %s, want included", id, runnerClass, got)
			}
			if c := CostFor(p, runnerClass, id); c.Class != ClassIncluded {
				t.Errorf("CostFor(%s,%q) class %s", id, runnerClass, c.Class)
			}
		}
		if schedpolicy.PaidAllowed(p, id) {
			t.Errorf("paid allowed for %s", id)
		}
	}
	if got := classOf(p, "", "cloudco"); got != ClassMetered {
		t.Errorf("a declared metered rate: class %s", got)
	}
	if got := classOf(p, "", "nobody"); got != ClassOwned {
		t.Errorf("no rate, no class: %s", got)
	}
}

func TestEconomicsCostForMatchesTheScheduler(t *testing.T) {
	p := spendPolicy(t, paidKV(nil))
	c := CostFor(p, "", "cloudco")
	if c.Class != ClassMetered || c.RateMicro != 3_600_000 || c.DivMs != msPerHour || c.BillingIncrementMs != 60_000 || c.Confidence != ConfEstimated {
		t.Fatalf("cost: %+v", c)
	}
	q := QuoteJob(p.Rates["cloudco"], 125_000, 0)
	// The scheduler and the step must price the same job the same way.
	if q.Micro != 180_000 { // 3 minutes at 60 000 micro per minute
		t.Fatalf("quote %+v", q)
	}
	if got, of := sched.MulDivUp(q.BilledMs, c.RateMicro, c.DivMs); of || got != q.Micro {
		t.Fatalf("scheduler formula %d (%v) vs step %d", got, of, q.Micro)
	}
	if c := CostFor(p, "", "nobody"); c.Class != ClassOwned || c.RateMicro != 0 || c.Confidence != ConfComputed {
		t.Fatalf("owned cost: %+v", c)
	}
	p2 := spendPolicy(t, paidKV(map[string]string{"economics.providers.cloudco": `{"owner_coordinator":"coord-1"}`}))
	if c := CostFor(p2, "", "cloudco"); c.OwnerCoordinator != "coord-1" {
		t.Fatalf("owner coordinator not carried: %+v", c)
	}
}
