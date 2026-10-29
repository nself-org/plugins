package dispatch

// Purpose: place fills each job's QueuedMs from its EnqueuedAt at the tick's
// clock (P7-AUTO-05), so the `auto` tier wait is bounded by real queue age; a
// job never enqueued, or a clock that went back, reads as no wait at all.
// Inputs: a two-runner `auto` world whose better tier is busy; a fixed now.
// Outputs: pass or fail. Constraints: no store, no clock reads.

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

var msLeftRe = regexp.MustCompile(`: (\d+) ms left of step_sec`)

// autoBusyWorld has a busy runner in the better tier and a free one a tier lower.
func autoBusyWorld() sched.World {
	better, worse := testRunner("better", "lan", 0), testRunner("worse", "lan", 1)
	net := map[string]sched.NetworkFact{}
	for _, id := range []string{"better", "worse"} {
		net[id] = sched.NetworkFact{Enforced: []model.NetworkScope{"none", "restricted", "internet"}}
	}
	return sched.World{Network: net, Runners: []sched.Runner{better, worse}, Policy: sched.Policy{Preset: sched.AutoPreset},
		Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}},
		Bias: map[string]int64{"better": 0, "worse": sched.TierStepMs}}
}

func TestDispatchFillsQueuedMs(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	step := sched.DefaultAutoStepSec * 1000
	cases := []struct {
		name     string
		enqueued time.Time
		placed   bool  // past step_sec: runs on the lower tier
		left     int64 // otherwise: the wait left that the reason reports
	}{
		{"never enqueued reads as no wait", time.Time{}, false, step},
		{"waited five seconds", now.Add(-5 * time.Second), false, step - 5000},
		{"a clock that went back never adds wait", now.Add(10 * time.Second), false, step},
		{"waited past step_sec", now.Add(-time.Duration(step+1000) * time.Millisecond), true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := Queued{Job: testQueued("a1"), EnqueuedAt: c.enqueued}
			res := place(autoBusyWorld(), []Queued{q}, now)
			if c.placed {
				if len(res.Placements) != 1 || res.Placements[0].RunnerID != "worse" {
					t.Fatalf("a job past step_sec must run on the lower tier: %+v", res)
				}
				return
			}
			if len(res.Placements) != 0 || len(res.Explanations) != 1 || res.Explanations[0].Status != "waiting" {
				t.Fatalf("the job must wait for the better tier: %+v", res)
			}
			var detail string
			for _, r := range res.Explanations[0].Reasons {
				if r.Code == string(model.TierWaiting) {
					detail = r.Detail
				}
			}
			m := msLeftRe.FindStringSubmatch(detail)
			if m == nil {
				t.Fatalf("no tier.waiting detail with the wait left: %+v", res.Explanations[0].Reasons)
			}
			if got, _ := strconv.ParseInt(m[1], 10, 64); got != c.left {
				t.Fatalf("wait left = %d ms, want %d (QueuedMs from EnqueuedAt at now): %s", got, c.left, detail)
			}
		})
	}
}

// Plan uses the dispatcher's clock, the same path as a tick.
func TestDispatchPlanFillsQueuedMsFromItsClock(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d := &Dispatcher{cfg: Config{Orderer: FIFO{}, Clock: func() time.Time { return now }}}
	stale := Queued{Job: testQueued("a1"), EnqueuedAt: now.Add(-time.Hour)}
	if _, res := d.Plan(autoBusyWorld(), []Queued{stale}); len(res.Placements) != 1 || res.Placements[0].RunnerID != "worse" {
		t.Fatalf("an hour-old job must not wait for the busy tier: %+v", res)
	}
}
