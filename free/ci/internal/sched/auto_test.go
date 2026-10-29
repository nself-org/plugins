package sched

// Purpose: the `auto` preset (P7-AUTO-05): tier bias against scoring and
// eligibility, the bounded tier wait, and the reasons that are never waited for.

import (
	"encoding/json"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// autoWorld returns an `auto` world whose Bias puts each runner in the tier
// index given (the tier rank is 0), the way internal/tiers writes it.
func autoWorld(tiers map[string]int64, rs ...Runner) World {
	w := fixtureWorld(rs...)
	w.Policy.Preset = AutoPreset
	w.Bias = map[string]int64{}
	for id, idx := range tiers {
		w.Bias[id] = idx * TierStepMs
	}
	return w
}

// withStep sets sched.auto.step_sec through a merged policy, as the resolver would.
func withStep(t *testing.T, w World, sec int64) World {
	t.Helper()
	dec := func(raw json.RawMessage) (any, error) {
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, err
		}
		return n.Int64()
	}
	reg, err := trust.NewRegistry(trust.KeySpec{Key: KeyAutoStepSec, Owner: "sched", Kind: trust.TightenLower, Default: DefaultAutoStepSec, Decode: dec})
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{KeyAutoStepSec: json.RawMessage(strconv.FormatInt(sec, 10))}})
	if err != nil {
		t.Fatal(err)
	}
	w.Policy.Trust = eff
	return w
}

func busy(r Runner) Runner {
	r.Avail.Slots = 0
	return r
}

var leftRe = regexp.MustCompile(`: (\d+) ms left of step_sec`)

// heldLeft returns the ms left in a tier.waiting reason, or -1.
func heldLeft(x Explanation) int64 {
	for _, r := range x.Reasons {
		if r.Code == string(model.TierWaiting) {
			m := leftRe.FindStringSubmatch(r.Detail)
			if m == nil {
				return -1
			}
			n, _ := strconv.ParseInt(m[1], 10, 64)
			return n
		}
	}
	return -1
}

func TestAutoPresetProperty(t *testing.T) {
	seed := int64(20261028)
	rnd := rand.New(rand.NewSource(seed))
	locs := []string{"remote", "lan", "local"}
	ineligible := []func(*Runner){
		func(r *Runner) { r.Capability.Availability.State.Value = ptr("offline") },
		func(r *Runner) { r.Capability.Availability.State.Value = ptr("draining") },
		func(r *Runner) { r.ProviderDown = true },
		func(r *Runner) { r.Capability.Lifecycle.AuthorizedProjects = nil },
		func(r *Runner) { *r = busy(*r) },
	}
	for n := 0; n < 10000; n++ {
		bt := rnd.Intn(2)
		lt := bt + 1 + rnd.Intn(2-bt)
		better, lower := fixtureRunner("r1", locs[bt]), fixtureRunner("r2", locs[lt])
		w := autoWorld(map[string]int64{"r1": int64(bt + 1), "r2": int64(lt + 1)}, better, lower)
		w.Bias["r1"] += int64(rnd.Intn(50))
		w.Bias["r2"] += int64(rnd.Intn(50))
		w.Estimates = map[string]map[string]Sample{}
		for _, id := range []string{"r1", "r2"} {
			w.Estimates[id] = map[string]Sample{"j": {WaitMs: ptr(rnd.Int63n(4 * TierStepMs)), RuntimeMs: ptr(rnd.Int63n(TierStepMs)), Confidence: "known"}}
		}
		betterOK := rnd.Intn(3) != 0
		if !betterOK {
			ineligible[rnd.Intn(len(ineligible))](&w.Runners[0])
		}
		j := fixtureJob("j")
		j.QueuedMs = []int64{math.MaxInt64, 30000, 1 << 40}[rnd.Intn(3)] // past the step: this property is about order, not the wait
		res := Place(w, []Job{j})
		if len(res.Placements) != 1 {
			t.Fatalf("seed %d world %d: not placed: %+v", seed, n, res.Explanations)
		}
		got := res.Placements[0].RunnerID
		if !betterOK {
			if got != "r2" {
				t.Fatalf("seed %d world %d: an ineligible better tier must lose to an eligible lower tier, got %s", seed, n, got)
			}
			continue
		}
		plain := w
		plain.Bias = nil
		sb, _ := score(plain, j, w.Runners[0])
		sl, _ := score(plain, j, w.Runners[1])
		want := "r1"
		if sl+w.Bias["r2"] < sb+w.Bias["r1"] {
			want = "r2"
		}
		if got != want {
			t.Fatalf("seed %d world %d: got %s want %s (base %d vs %d, bias %v)", seed, n, got, want, sb, sl, w.Bias)
		}
		// rank adds under 50 ms here, so the lower tier wins only by scoring more than the tier steps lower
		if got == "r2" && sb-sl <= int64(lt-bt)*TierStepMs-50 {
			t.Fatalf("seed %d world %d: the lower tier won without beating the tier step (base %d vs %d)", seed, n, sb, sl)
		}
	}
}

func TestAutoStepWaitBounded(t *testing.T) {
	w := autoWorld(map[string]int64{"vps": 1, "lan": 2}, busy(fixtureRunner("vps", "remote")), fixtureRunner("lan", "lan"))
	step := AutoStepMs(w)
	if step != DefaultAutoStepSec*1000 {
		t.Fatalf("default step %d", step)
	}
	for _, c := range []struct {
		queued int64
		wait   bool
		left   int64
	}{{0, true, 30000}, {-5, true, 30000}, {math.MinInt64, true, 30000}, {12000, true, 18000}, {29999, true, 1}, {30000, false, 0}, {math.MaxInt64, false, 0}} {
		j := fixtureJob("j")
		j.QueuedMs = c.queued
		res := Place(w, []Job{j})
		x := res.Explanations[0]
		if c.wait {
			if x.Status != "waiting" || len(res.Placements) != 0 || heldLeft(x) != c.left || !strings.Contains(x.Reasons[0].Detail, "held for vps (tier vps") {
				t.Fatalf("queued %d: want a tier.waiting hold with %d ms left: %+v", c.queued, c.left, x)
			}
			continue
		}
		if len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan" {
			t.Fatalf("queued %d: after step_sec the job runs on the next tier: %+v", c.queued, res)
		}
	}
	// node.not_ready waits like capacity.busy.
	nr := autoWorld(map[string]int64{"vps": 1, "lan": 2}, fixtureRunner("vps", "remote"), fixtureRunner("lan", "lan"))
	nr.Exclusions = map[string][]model.PlacementReasonDetail{"vps": {{Code: string(model.NodeNotReady), Detail: "warming"}}}
	if x := Place(nr, []Job{fixtureJob("j")}).Explanations[0]; x.Status != "waiting" || heldLeft(x) != 30000 {
		t.Fatalf("node.not_ready must be waited for: %+v", x)
	}
	// step_sec 0 never waits; a lower step bounds the wait.
	if res := Place(withStep(t, w, 0), []Job{fixtureJob("j")}); len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan" {
		t.Fatalf("step_sec 0 must never wait: %+v", res)
	}
	if x := Place(withStep(t, w, 5), []Job{fixtureJob("j")}).Explanations[0]; heldLeft(x) != 5000 {
		t.Fatalf("step_sec 5: %+v", x)
	}
	// clock skew: any QueuedMs never panics and never adds more than one step.
	rnd := rand.New(rand.NewSource(5))
	for i := 0; i < 2000; i++ {
		j := fixtureJob("j")
		j.QueuedMs = rnd.Int63() - rnd.Int63()
		x := Place(w, []Job{j}).Explanations[0]
		if left := heldLeft(x); left > step || x.Status == "waiting" && left <= 0 {
			t.Fatalf("queued %d: %d ms left exceeds one step: %+v", j.QueuedMs, left, x)
		}
	}
	// a better tier that is free wins at once; a same-tier busy runner is not waited for.
	same := autoWorld(map[string]int64{"a": 2, "b": 2}, busy(fixtureRunner("a", "lan")), fixtureRunner("b", "lan"))
	if res := Place(same, []Job{fixtureJob("j")}); len(res.Placements) != 1 || res.Placements[0].RunnerID != "b" {
		t.Fatalf("a busy runner of the same tier is not waited for: %+v", res)
	}
	// other presets never wait.
	other := w
	other.Policy.Preset = "balanced"
	if res := Place(other, []Job{fixtureJob("j")}); len(res.Placements) != 1 {
		t.Fatalf("balanced must not wait: %+v", res)
	}
}

func TestAutoStaticNeverWaits(t *testing.T) {
	cases := map[string]func(*World){
		"revoked":      func(w *World) { now := time.Unix(1, 0); w.Runners[0].Capability.Lifecycle.RevokedAt = &now },
		"unauthorized": func(w *World) { w.Runners[0].Capability.Lifecycle.AuthorizedProjects = nil },
		"offline":      func(w *World) { w.Runners[0].Capability.Availability.State.Value = ptr("offline") },
		"draining":     func(w *World) { w.Runners[0].Capability.Availability.State.Value = ptr("draining") },
		"provider.down": func(w *World) {
			w.Runners[0].ProviderDown = true
		},
		"pool.excluded": func(w *World) {
			w.Exclusions = map[string][]model.PlacementReasonDetail{"vps": {{Code: string(model.PoolExcluded), Detail: "pool"}}}
		},
		"busy and offline": func(w *World) {
			w.Runners[0] = busy(w.Runners[0])
			w.Runners[0].Capability.Availability.State.Value = ptr("offline")
		},
		"limit": func(w *World) { w.Policy.Limits.Runner = 1; w.Counts.Runners = map[string]int{"vps": 1} },
	}
	base := autoWorld(map[string]int64{"vps": 1, "lan": 2}, fixtureRunner("vps", "remote"), fixtureRunner("lan", "lan"))
	if res := Place(base, []Job{fixtureJob("j")}); len(res.Placements) != 1 || res.Placements[0].RunnerID != "vps" {
		t.Fatalf("an eligible better tier must win: %+v", res)
	}
	for name, mutate := range cases {
		w := autoWorld(map[string]int64{"vps": 1, "lan": 2}, fixtureRunner("vps", "remote"), fixtureRunner("lan", "lan"))
		mutate(&w)
		res := Place(w, []Job{fixtureJob("j")})
		if len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan" || heldLeft(res.Explanations[0]) >= 0 {
			t.Errorf("%s: the better tier must be skipped at once: %+v", name, res)
		}
	}
}
