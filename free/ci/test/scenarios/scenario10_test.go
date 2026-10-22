//go:build scenarios

package scenarios

// Purpose: Epic acceptance 4, scenario 10 (capacity saturation). Three times more jobs than slots
// from two projects meet a real dispatcher and store: no runner ever holds more than its slots or
// CPU, served work stays fair across projects, a full queue refuses with E694 and its expected
// wait, and an interactive job overtakes a background flood.
// Inputs:  local plus two LAN runners with 2 slots each (6 slots), 18 jobs of projects alpha and beta.
// Outputs: assertions only.
// Constraints: no sleeps; the test frees capacity (pass) and ticks the dispatcher itself. The
//          fairness bound comes from internal/fairq: start-time fair queuing across projects with
//          equal weights and equal cost (every job costs 1) emits alternately, so while both
//          projects still have queued jobs their served counts differ by at most 1.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func s10Runners() []sched.Runner {
	return []sched.Runner{s10Runner("local", "local", 2), s10Runner("lan1", "lan", 2), s10Runner("lan2", "lan", 2)}
}

// s10JobRuntimeMs is the equal runtime used by the no-telemetry fallback case.
// Unequal runtime estimates are covered by s10UnequalRuntimeFairness.
const s10JobRuntimeMs = int64(90_000)

// s10CheckFair returns a violation for every admission prefix where two projects that both still
// had queued jobs differ in served count by more than 1, or in served runtime by more than
// max job runtime x slots.
func s10CheckFair(log []s10Event, total map[string]int, slots int) []string {
	served := map[string]int{}
	var out []string
	bound := s10JobRuntimeMs * int64(slots)
	for i, e := range log {
		served[e.Project]++
		if served["alpha"] < total["alpha"] && served["beta"] < total["beta"] {
			runtimeGap := int64(abs(served["alpha"]-served["beta"])) * s10JobRuntimeMs
			if abs(served["alpha"]-served["beta"]) > 1 || runtimeGap > bound {
				out = append(out, fmt.Sprintf("after admission %d: alpha %d, beta %d (served runtime gap %d ms, bound %d ms)", i+1, served["alpha"], served["beta"], runtimeGap, bound))
			}
		}
	}
	return out
}

func abs(n int) int { return max(n, -n) }

// s10CheckLeasesAdmitted returns a violation for every attempt that is leased, running or
// finalizing without a sched_admitted row: a lease must come from dispatch.Admit.
func s10CheckLeasesAdmitted(ctx context.Context, st *store.Store, attempts []string) []string {
	var out []string
	for _, id := range attempts {
		state, _, err := st.AttemptStateEpoch(ctx, id)
		if err != nil {
			out = append(out, id+": "+err.Error())
			continue
		}
		if state != "leased" && state != "running" && state != "finalizing" {
			continue
		}
		if _, _, _, _, _, ok, err := st.AdmittedIdentity(ctx, id); err != nil || !ok {
			out = append(out, fmt.Sprintf("attempt %s is %s with no admission record (%v)", id, state, err))
		}
	}
	return out
}

func TestScenario10SaturationStaysWithinCapacityAndFair(t *testing.T) {
	t.Run("unequal runtime", s10UnequalRuntimeFairness)
	seed := invariants.Seed()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("seed %d (NSELF_CI_INVARIANT_SEED)", seed)
		}
	})
	rig := newS10Rig(t, s10Runners(), nil, nil)
	// One project submits its whole flood before the other starts, so arrival order (FIFO) would serve
	// 6 jobs of the first project before any of the second: only fair queuing keeps the gap at 1. The seed
	// picks which project floods first, so every seed discriminates between the orderers.
	first, second := "alpha", "beta"
	if rand.New(rand.NewSource(seed)).Intn(2) == 1 {
		first, second = second, first
	}
	projects := []string{}
	for i := 0; i < 9; i++ {
		projects = append(projects, first)
	}
	for i := 0; i < 9; i++ {
		projects = append(projects, second)
	}
	for _, p := range projects {
		rig.submit(p, "")
	}
	rig.waitQueued(18) // all 18 queued before any placement
	rig.release()
	rig.waitAdmitted(6)
	rig.tick()
	if n := len(rig.sink.events(dispatch.OutcomeAdmitted)); n != 6 {
		t.Fatalf("%d admitted with 6 slots", n)
	}
	total := map[string]int{"alpha": 9, "beta": 9}
	next := 0
	for step := 1; step <= 18; step++ {
		log := rig.sink.events(dispatch.OutcomeAdmitted)
		if v := append(append(s10CheckCapacity(rig.ctx, rig.st, rig.trueSlots, 8), s10CheckFair(log, total, 6)...), s10CheckLeasesAdmitted(rig.ctx, rig.st, rig.attempts)...); len(v) > 0 {
			t.Fatalf("step %d: %v\nadmission order: %+v", step, v, log)
		}
		if want := min(18, 6+next); len(log) != want {
			t.Fatalf("step %d: %d admitted, want %d (slots free exactly as work ends)", step, len(log), want)
		}
		if next == 18 {
			break
		}
		rig.finish(log[next].Key)
		next++
	}
	if v := rig.sink.violations(); len(v) > 0 {
		t.Fatalf("breach seen at an admission: %v", v)
	}
	rig.cancel()
	rig.wg.Wait()
	close(rig.errs)
	for err := range rig.errs {
		t.Fatalf("a submit failed: %v", err)
	}
}

func TestScenario10BackpressureCarriesExpectedWait(t *testing.T) {
	const depth = 5
	rig := newS10Rig(t, s10Runners(), nil, func(context.Context) (dispatch.QueueLimits, error) {
		return dispatch.QueueLimits{MaxDepth: depth}, nil
	})
	for i := 0; i < depth; i++ {
		rig.submit("alpha", "")
	}
	rig.submit("beta", "") // another project's queue is not full
	rig.waitQueued(depth + 1)
	_, extra := s10Job(t, rig.st, "alpha", "")
	_, err := rig.d.Submit(rig.ctx, extra)
	var be *dispatch.BackpressureError
	if !errors.As(err, &be) || be.Err.Code != "E694" {
		t.Fatalf("want E694, got %v", err)
	}
	est, _ := dispatch.RuntimeEstimate(rig.ctx, rig.st, "alpha")
	if want := dispatch.WaitFor(depth, est, 6); be.ExpectedWaitMs <= 0 || be.ExpectedWaitMs != want || be.Depth != depth || be.Max != depth || be.Project != "alpha" {
		t.Fatalf("expected wait %d ms (want %d from %d queued, %d ms each, 6 slots): %+v", be.ExpectedWaitMs, want, depth, est, be)
	}
	var de *dispatch.Error
	if !errors.As(err, &de) || de.Code != "E694" {
		t.Fatalf("the E694 must unwrap to a dispatch error: %v", err)
	}
	// the refused job never entered the queue: after a full tick no tick has ordered more than the 6 accepted jobs
	rig.release()
	if got := rig.ord.mostLen(); got != depth+1 {
		t.Fatalf("the longest queue a tick ordered was %d jobs, want %d (the refused job entered the queue)", got, depth+1)
	}
}

func TestScenario10InteractiveStartsBeforeQueuedBackground(t *testing.T) {
	seed := invariants.Seed()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("seed %d (NSELF_CI_INVARIANT_SEED)", seed)
		}
	})
	rig := newS10Rig(t, s10Runners(), nil, nil)
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i < 12; i++ {
		rig.submit([]string{"alpha", "beta"}[rng.Intn(2)], "background")
	}
	rig.waitQueued(12)
	rig.release()
	rig.waitAdmitted(6)
	vip := rig.submit("beta", "interactive") // arrives while 6 run and 6 background jobs wait
	rig.waitQueued(7)
	before := rig.sink.events(dispatch.OutcomeAdmitted)
	rig.finish(before[0].Key)
	log := rig.sink.events(dispatch.OutcomeAdmitted)
	if len(log) != 7 || log[6].Key != vip || log[6].Band != "interactive" {
		t.Fatalf("the next start after the flood began was %+v, want the interactive job %s", log[len(log)-1], vip)
	}
	for _, e := range before {
		if e.Band != "background" {
			t.Fatalf("flood admission %+v is not background", e)
		}
	}
	if v := rig.sink.violations(); len(v) > 0 {
		t.Fatalf("breach: %v", v)
	}
}

// s10Placer is the placement seam: sched.Place, or a defective stand-in.
type s10Placer func(sched.World, []sched.Job) sched.Result

// s10CheckPriority returns a violation for every placement onto a runner that the honest
// scheduler lists as ineligible for that job: priority and band only reorder, they never admit.
func s10CheckPriority(place s10Placer, w sched.World, jobs []sched.Job) []string {
	var out []string
	byKey := map[string]sched.Job{}
	for _, j := range jobs {
		byKey[j.Key] = j
	}
	for _, p := range place(w, jobs).Placements {
		for _, a := range sched.Place(w, []sched.Job{byKey[p.JobKey]}).Explanations[0].Alternatives {
			if a.RunnerID == p.RunnerID && !a.Eligible {
				out = append(out, fmt.Sprintf("job %s (priority %q, band %q) placed on ineligible runner %s: %v", p.JobKey, byKey[p.JobKey].Priority, byKey[p.JobKey].Pipeline.Band, p.RunnerID, codes(a.Reasons)))
			}
		}
	}
	return out
}

// s10PriorityWorld is two linux runners and an interactive high-priority job that needs darwin/arm64.
func s10PriorityWorld() (sched.World, []sched.Job) {
	w := sched.World{Runners: s10Runners()[:2], Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	vip := job("vip", "darwin/arm64")
	vip.Priority, vip.Pipeline.Band, vip.Pipeline.Support = "high", "interactive", []string{"darwin/arm64", "linux/amd64"}
	ok := job("ok", "linux/amd64")
	ok.Pipeline.Support = vip.Pipeline.Support
	return w, []sched.Job{vip, ok}
}

func TestScenario10PriorityNeverAdmitsIneligible(t *testing.T) {
	w, jobs := s10PriorityWorld()
	if v := s10CheckPriority(sched.Place, w, jobs); len(v) > 0 {
		t.Fatalf("priority admitted an ineligible job: %v", v)
	}
	// the dispatcher half: the high-priority job has no matching runner, so it waits unplaced
	rig := newS10Rig(t, s10Runners()[:2], nil, nil)
	attempt, j := s10Job(t, rig.st, "alpha", "interactive")
	j.Sched.Priority, j.Sched.Requirements.Platform, j.Sched.Pipeline.Support = "high", "darwin/arm64", []string{"darwin/arm64", "linux/amd64"}
	rig.wg.Add(1)
	go func() { defer rig.wg.Done(); _, _ = rig.d.Submit(rig.ctx, j) }()
	rig.waitQueued(1)
	rig.release()
	for len(rig.sink.events(dispatch.OutcomeUnplaceable)) == 0 {
		select {
		case <-rig.sink.ch:
		case <-rig.ctx.Done():
			t.Fatal("the high-priority job was never refused")
		}
	}
	if state, _, err := rig.st.AttemptStateEpoch(rig.ctx, attempt); err != nil || state != model.JobState("queued") {
		t.Fatalf("a job no runner can take is %s (%v)", state, err)
	}
	if v := s10CheckLeasesAdmitted(rig.ctx, rig.st, []string{attempt}); len(v) > 0 || len(rig.sink.events(dispatch.OutcomeAdmitted)) != 0 {
		t.Fatalf("admitted: %v", v)
	}
}

func TestScenario10EveryLeaseComesFromAdmit(t *testing.T) {
	rig := newS10Rig(t, s10Runners(), nil, nil)
	for i := 0; i < 4; i++ {
		rig.submit("alpha", "")
	}
	rig.waitQueued(4)
	rig.release()
	rig.waitAdmitted(4)
	if v := s10CheckLeasesAdmitted(rig.ctx, rig.st, rig.attempts); len(v) > 0 {
		t.Fatalf("leases outside Admit: %v", v)
	}
}
