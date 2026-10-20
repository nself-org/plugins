package burst

// Tests of the pure decisions: Decide (provision or wait) and SweepDecide (keep or
// delete a listed machine). Both are checked against an independent statement of
// the rule over seeded random inputs (the seed prints on failure) plus a table that
// names each reason.

import (
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

const propertyCases = 20000

func seedOf(t *testing.T) *rand.Rand {
	t.Helper()
	seed := int64(20261019)
	if v := os.Getenv("BURST_SEED"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seed = n
	}
	t.Logf("seed %d (set BURST_SEED to replay)", seed)
	return rand.New(rand.NewSource(seed))
}

func pick(r *rand.Rand, vals ...int64) int64 { return vals[r.Intn(len(vals))] }

// randInput is half fully random and half a plausible allowing input with one or two fields disturbed,
// so the allow path and each single-rule refusal are reached often.
func randInput(r *rand.Rand) Input {
	if r.Intn(2) == 0 {
		in := baseInput()
		for n := 1 + r.Intn(2); n > 0; n-- {
			disturb(r, &in)
		}
		return in
	}
	b := func() bool { return r.Intn(4) != 0 } // mostly true, so the allow path is exercised
	return Input{Provider: "p", Cloud: r.Intn(2) == 0, Team: b(), Enabled: b(), Paid: r.Intn(2) == 0, PaidAllowed: b(), ReservationFits: b(),
		ExpectedWaitMs: pick(r, 0, 1, 59_000, 120_000, 180_000, 180_001, 600_000, math.MaxInt64, r.Int63n(1_000_000)),
		LatencyMs:      pick(r, -5, 0, 1, 30_000, 120_000, math.MaxInt64, r.Int63n(300_000)),
		MarginMs:       pick(r, -10, 0, 60_000, math.MaxInt64, r.Int63n(120_000)),
		QueuedDemand:   r.Intn(5), InFlight: r.Intn(5), JobWindowCount: r.Intn(3), Open: r.Intn(4), MaxConcurrent: r.Intn(4) - 1,
		BreakerOpen: r.Intn(5) == 0, TeardownFailed: r.Intn(4) / 3}
}

// allows is the rule of Epic D12 written out again, without Decide's switch.
func allows(in Input) bool {
	lat := in.LatencyMs
	if lat <= 0 {
		lat = 120_000
	}
	margin := max(in.MarginMs, 0)
	need := new(bigSum).add(lat).add(margin)
	return (!in.Cloud || in.Team) && in.Enabled && (!in.Paid || in.PaidAllowed) && in.TeardownFailed == 0 && !in.BreakerOpen && in.ReservationFits &&
		need.less(in.ExpectedWaitMs) && in.QueuedDemand > in.InFlight && in.JobWindowCount == 0 && in.Open < max(in.MaxConcurrent, 0)
}

// bigSum adds without wrapping, to compare with a saturating implementation.
type bigSum struct{ hi, lo uint64 }

func (b *bigSum) add(v int64) *bigSum {
	lo := b.lo + uint64(v)
	if lo < b.lo {
		b.hi++
	}
	b.lo = lo
	return b
}

// less reports whether the sum is strictly below v.
func (b *bigSum) less(v int64) bool { return b.hi == 0 && b.lo < uint64(v) }

func TestBurstDecideProperty(t *testing.T) {
	r := seedOf(t)
	allowed := 0
	for i := 0; i < propertyCases; i++ {
		in := randInput(r)
		d := Decide(in)
		if d.Allow != allows(in) {
			t.Fatalf("case %d: Decide(%+v) = %v (%s), independent rule says %v", i, in, d.Allow, d.Reason, allows(in))
		}
		if d.Input != in {
			t.Fatalf("case %d: the record does not hold every input: %+v vs %+v", i, d.Input, in)
		}
		if d.Allow != (d.Reason == ReasonAllowed) {
			t.Fatalf("case %d: allow=%v with reason %q", i, d.Allow, d.Reason)
		}
		if d.Latency != EffectiveLatency(in.LatencyMs) || d.Latency <= 0 {
			t.Fatalf("case %d: latency %d for %d", i, d.Latency, in.LatencyMs)
		}
		if d.Allow {
			allowed++
			// One queued job, one provision per window: a job that already started one is refused.
			again := in
			again.JobWindowCount = 1
			if Decide(again).Allow {
				t.Fatalf("case %d: a second provision for the same job in the window was allowed", i)
			}
			// In-flight provisions count as capacity: enough of them end the need.
			covered := in
			covered.InFlight = in.QueuedDemand
			if Decide(covered).Allow {
				t.Fatalf("case %d: provisions already on their way did not count as capacity", i)
			}
		}
	}
	if allowed < propertyCases/50 {
		t.Fatalf("only %d of %d random cases allowed: the generator does not reach the allow path", allowed, propertyCases)
	}
}

func disturb(r *rand.Rand, in *Input) {
	switch r.Intn(14) {
	case 0:
		in.Team = !in.Team
	case 1:
		in.Enabled = !in.Enabled
	case 2:
		in.PaidAllowed = !in.PaidAllowed
	case 3:
		in.ReservationFits = !in.ReservationFits
	case 4:
		in.BreakerOpen = !in.BreakerOpen
	case 5:
		in.TeardownFailed = r.Intn(2)
	case 6:
		in.ExpectedWaitMs = pick(r, 0, 150_000, 150_001, 149_999, math.MaxInt64)
	case 7:
		in.LatencyMs = pick(r, -1, 0, 1, math.MaxInt64, r.Int63n(300_000))
	case 8:
		in.MarginMs = pick(r, -1, 0, math.MaxInt64, r.Int63n(300_000))
	case 9:
		in.QueuedDemand = r.Intn(4)
	case 10:
		in.InFlight = r.Intn(4)
	case 11:
		in.JobWindowCount = r.Intn(3)
	case 12:
		in.Open = r.Intn(4)
	case 13:
		in.MaxConcurrent = r.Intn(4) - 1
	}
}

func baseInput() Input {
	return Input{Provider: "p", Cloud: true, Team: true, Enabled: true, Paid: true, PaidAllowed: true, ReservationFits: true, ExpectedWaitMs: 300_000, LatencyMs: 90_000,
		MarginMs: 60_000, QueuedDemand: 1, MaxConcurrent: 2}
}

func TestBurstDecideReasons(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(*Input)
		reason string
	}{
		{"allowed", func(*Input) {}, ReasonAllowed},
		{"personal mode", func(i *Input) { i.Team = false }, ReasonTeamOnly},
		{"local provider in personal mode", func(i *Input) { i.Team, i.Cloud, i.Paid = false, false, false }, ReasonAllowed},
		{"disabled", func(i *Input) { i.Enabled = false }, ReasonDisabled},
		{"paid not allowed", func(i *Input) { i.PaidAllowed = false }, ReasonPaidDisabled},
		{"teardown failed blocks", func(i *Input) { i.TeardownFailed = 1 }, ReasonTeardownFailed},
		{"breaker open", func(i *Input) { i.BreakerOpen = true }, ReasonBreakerOpen},
		{"reservation does not fit", func(i *Input) { i.ReservationFits = false }, ReasonNoFit},
		{"wait equals latency plus margin", func(i *Input) { i.ExpectedWaitMs = 150_000 }, ReasonWaitShort},
		{"wait just over", func(i *Input) { i.ExpectedWaitMs = 150_001 }, ReasonAllowed},
		{"unknown latency is 120 s", func(i *Input) { i.LatencyMs, i.ExpectedWaitMs = 0, 180_000 }, ReasonWaitShort},
		{"unknown latency, long wait", func(i *Input) { i.LatencyMs, i.ExpectedWaitMs = 0, 180_001 }, ReasonAllowed},
		{"in flight covers", func(i *Input) { i.InFlight = 1 }, ReasonInFlightCovers},
		{"job already provisioned in window", func(i *Input) { i.JobWindowCount = 1 }, ReasonJobWindowCap},
		{"provider cap", func(i *Input) { i.Open = 2 }, ReasonProviderCap},
		{"zero cap", func(i *Input) { i.MaxConcurrent = 0 }, ReasonProviderCap},
	}
	for _, c := range cases {
		in := baseInput()
		c.mut(&in)
		if d := Decide(in); d.Reason != c.reason || d.Allow != (c.reason == ReasonAllowed) {
			t.Errorf("%s: got %v %q, want %q", c.name, d.Allow, d.Reason, c.reason)
		}
	}
	if WindowMs(0) != 240_000 || WindowMs(10_000) != 20_000 {
		t.Errorf("window is 2 x latency: %d %d", WindowMs(0), WindowMs(10_000))
	}
}

// TestDecideIsPure: decide.go and decide_sweep.go import no os, os/exec, net or database/sql.
func TestDecideIsPure(t *testing.T) {
	for _, file := range []string{"decide.go", "decide_sweep.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == "os" || p == "os/exec" || p == "net" || strings.HasPrefix(p, "net/") || p == "database/sql" || strings.Contains(p, "/store") {
				t.Errorf("%s imports %s", file, p)
			}
		}
	}
}

func randFacts(r *rand.Rand) SweepFacts {
	f := SweepFacts{Mine: r.Intn(4) != 0, Excluded: r.Intn(5) == 0, HasIntent: r.Intn(3) != 0, MaxLifetimeMs: pick(r, 3_600_000, 1, r.Int63n(7_200_000)), GraceMs: pick(r, 0, 600_000, r.Int63n(1_800_000))}
	f.IntentState = []string{"live", "tearing-down", "terminal"}[r.Intn(3)]
	f.AgeMs = pick(r, 0, f.MaxLifetimeMs-1, f.MaxLifetimeMs, f.MaxLifetimeMs+f.GraceMs, f.MaxLifetimeMs+f.GraceMs+1, math.MaxInt64, r.Int63n(10_000_000))
	return f
}

func TestSweepDecideProperty(t *testing.T) {
	r := seedOf(t)
	var del, keep int
	for i := 0; i < propertyCases; i++ {
		f := randFacts(r)
		v := SweepDecide(f)
		own := f.Mine && !f.Excluded
		old := new(bigSum).add(f.MaxLifetimeMs).add(f.GraceMs).less(f.AgeMs)
		mustDelete := own && (f.HasIntent && f.IntentState != "live" || old)
		if v.Delete != mustDelete {
			t.Fatalf("case %d: SweepDecide(%+v) = %+v, rule says delete=%v", i, f, v, mustDelete)
		}
		if !own && v.Delete {
			t.Fatalf("case %d: deleted a machine that is not ours or is excluded: %+v", i, f)
		}
		if own && f.HasIntent && f.IntentState == "live" && f.AgeMs < f.MaxLifetimeMs && v.Delete {
			t.Fatalf("case %d: deleted a live intent's machine younger than max_lifetime: %+v", i, f)
		}
		if v.Delete {
			del++
		} else {
			keep++
		}
	}
	if del < propertyCases/10 || keep < propertyCases/10 {
		t.Fatalf("generator is lopsided: %d deletes, %d keeps", del, keep)
	}
}

func TestSweepDecideReasons(t *testing.T) {
	base := SweepFacts{Mine: true, HasIntent: true, IntentState: "live", AgeMs: 1000, MaxLifetimeMs: 3_600_000, GraceMs: 600_000}
	for _, c := range []struct {
		name   string
		mut    func(*SweepFacts)
		reason string
		del    bool
	}{
		{"live and young", func(*SweepFacts) {}, SweepLive, false},
		{"foreign", func(f *SweepFacts) { f.Mine = false }, SweepForeign, false},
		{"excluded", func(f *SweepFacts) { f.Excluded, f.IntentState = true, "terminal" }, SweepExcluded, false},
		{"terminal intent", func(f *SweepFacts) { f.IntentState = "terminal" }, SweepIntentTerminal, true},
		{"tearing down", func(f *SweepFacts) { f.IntentState = "tearing-down" }, SweepIntentTearing, true},
		{"live in grace", func(f *SweepFacts) { f.AgeMs = 3_600_000 + 600_000 }, SweepLive, false},
		{"live past grace", func(f *SweepFacts) { f.AgeMs = 3_600_000 + 600_001 }, SweepAgeExpired, true},
		{"orphan young", func(f *SweepFacts) { f.HasIntent = false }, SweepOrphan, false},
		{"orphan expired", func(f *SweepFacts) { f.HasIntent, f.AgeMs = false, 4_200_001 }, SweepOrphanExpired, true},
	} {
		f := base
		c.mut(&f)
		if v := SweepDecide(f); v.Reason != c.reason || v.Delete != c.del {
			t.Errorf("%s: got %+v, want %q delete=%v", c.name, v, c.reason, c.del)
		}
	}
}
