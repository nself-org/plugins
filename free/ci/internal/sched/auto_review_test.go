package sched

// Purpose: the review fixes of the `auto` preset (P7-AUTO-05): the hold stops before
// max_queue, several held jobs in one Place call, and a tier the order leaves out.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var cappedRe = regexp.MustCompile(`: (\d+) ms left of max_queue`)

// withMaxQueue sets Overrides.MaxQueueMs, as the resolver would.
func withMaxQueue(w World, ms int64) World {
	w.Policy.Overrides.MaxQueueMs = ms
	return w
}

// TestAutoHoldStopsBeforeMaxQueue: with max_queue 10 s and step_sec 30 s the hold
// is bounded by max_queue less one poll, so the job is placed on the free tier
// before the dispatcher would fail it with E693.
func TestAutoHoldStopsBeforeMaxQueue(t *testing.T) {
	base := autoWorld(map[string]int64{"vps": 1, "lan": 2}, busy(fixtureRunner("vps", "remote")), fixtureRunner("lan", "lan"))
	w := withMaxQueue(base, 10000)
	for _, c := range []struct {
		queued int64
		held   bool
		left   int64
	}{{0, true, 9000}, {8999, true, 1}, {9000, false, 0}, {9999, false, 0}, {10000, false, 0}} {
		j := fixtureJob("j")
		j.QueuedMs = c.queued
		res := Place(w, []Job{j})
		x := res.Explanations[0]
		if c.held {
			m := cappedRe.FindStringSubmatch(heldReason(x))
			if x.Status != "waiting" || m == nil || m[1] != strconv.FormatInt(c.left, 10) {
				t.Fatalf("queued %d: want a hold with %d ms left: %+v", c.queued, c.left, x)
			}
			continue
		}
		if len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan" {
			t.Fatalf("queued %d: the job must run on lan before max_queue: %+v", c.queued, res)
		}
	}
	// a max_queue at or under one poll never holds; a longer one leaves step_sec in charge.
	for _, mq := range []int64{1, AutoPollMs} {
		if res := Place(withMaxQueue(base, mq), []Job{fixtureJob("j")}); len(res.Placements) != 1 {
			t.Fatalf("max_queue %d must not hold: %+v", mq, res)
		}
	}
	if x := Place(withMaxQueue(base, 600000), []Job{fixtureJob("j")}).Explanations[0]; heldLeft(x) != 30000 {
		t.Fatalf("a long max_queue leaves step_sec in charge: %+v", x)
	}
}

func heldReason(x Explanation) string {
	for _, r := range x.Reasons {
		if r.Code == string(model.TierWaiting) {
			return r.Detail
		}
	}
	return ""
}

// TestAutoSeveralHeldJobsInOnePlace: held jobs consume nothing, so a job past its
// step in the same call still gets the free runner, and every held job is explained.
func TestAutoSeveralHeldJobsInOnePlace(t *testing.T) {
	w := autoWorld(map[string]int64{"vps": 1, "lan": 2}, busy(fixtureRunner("vps", "remote")), fixtureRunner("lan", "lan"))
	w.Runners[1].Avail = Avail{Slots: 2, CPU: 16, MemMB: 16384}
	jobs := []Job{fixtureJob("a"), fixtureJob("b"), fixtureJob("c"), fixtureJob("d")}
	jobs[1].QueuedMs = 30000 // past its step: runs on lan
	jobs[3].QueuedMs = 12000 // still held, 18 s left
	res := Place(w, jobs)
	if len(res.Placements) != 1 || res.Placements[0].JobKey != "b" || res.Placements[0].RunnerID != "lan" {
		t.Fatalf("only the job past its step runs: %+v", res.Placements)
	}
	want := map[string]int64{"a": 30000, "c": 30000, "d": 18000}
	for _, x := range res.Explanations {
		left, held := want[x.JobKey]
		if held && (x.Status != "waiting" || heldLeft(x) != left) {
			t.Errorf("job %s: want a hold with %d ms left: %+v", x.JobKey, left, x)
		}
		if !held && x.Status == "waiting" {
			t.Errorf("job %s must not be held: %+v", x.JobKey, x)
		}
	}
	// the same call again gives the same answer (determinism).
	again := Place(w, jobs)
	a, _ := json.Marshal(res)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatalf("Place is not deterministic:\n%s\n%s", a, b)
	}
}

// withOrder sets sched.auto.order through a merged policy at project scope.
func withOrder(t *testing.T, w World, raw string) World {
	t.Helper()
	dec := func(r json.RawMessage) (any, error) {
		var xs []string
		err := json.Unmarshal(r, &xs)
		return xs, err
	}
	reg, err := trust.NewRegistry(trust.KeySpec{Key: KeyAutoOrder, Owner: "sched", Kind: trust.TightenAny, Decode: dec})
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{KeyAutoOrder: json.RawMessage(raw)}})
	if err != nil {
		t.Fatal(err)
	}
	w.Policy.Trust = eff
	return w
}

// TestAutoOrderDropsOmittedTier: a tier the order leaves out is excluded, never
// merely tried last, and the exclusion is never waited for.
func TestAutoOrderDropsOmittedTier(t *testing.T) {
	w := autoWorld(map[string]int64{"vps": 1, "lan": 2, "loc": 3}, fixtureRunner("vps", "remote"), fixtureRunner("lan", "lan"), fixtureRunner("loc", "local"))
	if res := Place(w, []Job{fixtureJob("j")}); len(res.Placements) != 1 || res.Placements[0].RunnerID != "vps" {
		t.Fatalf("no order: vps first: %+v", res.Placements)
	}
	n := withOrder(t, w, `["lan","local"]`)
	res := Place(n, []Job{fixtureJob("j")})
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan" {
		t.Fatalf("vps is out of the order: %+v", res.Placements)
	}
	var excluded bool
	for _, alt := range res.Explanations[0].Alternatives {
		if alt.RunnerID == "vps" {
			excluded = !alt.Eligible && containsCode(alt.Reasons, string(model.OverrideExcluded)) && strings.Contains(alt.Reasons[0].Detail+alt.Reasons[len(alt.Reasons)-1].Detail, KeyAutoOrder)
		}
	}
	if !excluded {
		t.Fatalf("vps must be listed as excluded by the order: %+v", res.Explanations[0])
	}
	// busy lan + a dropped tier: the job waits for lan, never for the dropped vps.
	b := withOrder(t, autoWorld(map[string]int64{"vps": 1, "lan": 2, "loc": 3}, busy(fixtureRunner("vps", "remote")), busy(fixtureRunner("lan", "lan")), fixtureRunner("loc", "local")), `["lan","local"]`)
	if x := Place(b, []Job{fixtureJob("j")}).Explanations[0]; x.Status != "waiting" || !strings.Contains(heldReason(x), "held for lan") {
		t.Fatalf("hold must name the listed tier: %+v", x)
	}
	// the other presets ignore the key.
	o := n
	o.Policy.Preset = "balanced"
	for _, r := range o.Runners {
		if !tierListed(o, r) {
			t.Fatalf("%s: only `auto` reads the order", r.ID)
		}
	}
}
