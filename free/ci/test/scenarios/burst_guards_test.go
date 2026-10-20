//go:build scenarios

package scenarios

// Purpose: Epic acceptance 6, guard bullets: one queued job causes at most one provision per window
// (no provisioning storm); a burst VM's reservation covers its WHOLE lifetime; PERSONAL never
// provisions cloud capacity; a private-infrastructure project never reaches a burst VM by default (I03).
// Inputs:  burst.Engine over fakecloud and a real store; sched.Place over a world holding an ephemeral VM.
// Outputs: assertions; burstCheckOneProvisionPerWindow is the check function the planted regression reuses.
// Constraints: no sleeps; time moves through the injected clock only.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// burstCheckOneProvisionPerWindow drives one queued job (demand 5, so the engine would happily take more
// machines if nothing stopped it) through repeated steps inside one window, then past it with the demand
// already covered. step is the seam under test: it asks for a machine for the job. It returns the violations
// of "at most one provision per queued job per window" (empty = holds).
func burstCheckOneProvisionPerWindow(r *burstRig, step func(context.Context, burst.Request) error) []string {
	ctx := context.Background()
	var bad []string
	run := func(req burst.Request) {
		if err := step(ctx, req); err != nil && burst.CodeOf(err) != burst.CodeProvisionRefused {
			bad = append(bad, fmt.Sprintf("step failed with something other than a refusal: %v", err))
		}
	}
	window := time.Duration(burst.WindowMs(r.cloud.Latency)) * time.Millisecond
	req := r.request("job-storm")
	req.QueuedDemand = 5
	run(req)
	for i := 0; i < 12; i++ { // 12 x 10 s = 2 minutes, inside the 4 minute window
		r.clk.Advance(10 * time.Second)
		run(req)
	}
	if n := r.cloud.Calls["Provision"]; n != 1 {
		bad = append(bad, fmt.Sprintf("%d provider provisions inside one window for one queued job, want 1", n))
	}
	r.clk.Advance(window) // past the window; the one machine on its way already covers a single queued job
	req.QueuedDemand = 1
	run(req)
	if n := r.cloud.Calls["Provision"]; n != 1 {
		bad = append(bad, fmt.Sprintf("a new provision after the window although the machine on its way covers the demand (%d total)", n))
	}
	return bad
}

func TestBurstOneQueuedJobCausesAtMostOneProvisionPerWindow(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t)
	step := func(ctx context.Context, req burst.Request) error { _, err := r.eng.Provision(ctx, req); return err }
	if bad := burstCheckOneProvisionPerWindow(r, step); len(bad) != 0 {
		t.Fatalf("provisioning storm: %v", bad)
	}
	// the check ended past the window with the one machine still on its way; the refusal reasons are the documented ones
	req := r.request("job-storm")
	req.QueuedDemand = 1
	if _, err := r.eng.Provision(ctx, req); burst.ReasonOf(err) != burst.ReasonInFlightCovers {
		t.Fatalf("one job, one machine on its way: %v", err)
	}
	// after the window a new machine is allowed because the demand (5) still needs one
	req.QueuedDemand = 5
	if _, err := r.eng.Provision(ctx, req); err != nil {
		t.Fatalf("after the window, with 5 queued and 1 on its way: %v", err)
	}
	if n := r.cloud.Calls["Provision"]; n != 2 {
		t.Fatalf("provider provisions %d, want 2", n)
	}
	if _, err := r.eng.Provision(ctx, req); burst.ReasonOf(err) != burst.ReasonJobWindowCap {
		t.Fatalf("the new machine opens a new window: %v", err)
	}
}

func TestBurstReservationCoversTheWholeLifetimeNotJustOneJob(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t, func(r *burstRig) { r.pol.Budgets.Day.Micro = 10_000_000 })
	res := r.provision("job-1")
	row := r.resourceRow(res.Token)
	const perMin = int64(60_000) // 3.6e6 micro an hour in 60 s increments
	if want := r.cfg.LifetimeMs() / 60_000 * perMin; row.ReservedMicro != want {
		t.Fatalf("reserved %d, want the full lifetime bound %d (max_lifetime + grace + sweep + retry)", row.ReservedMicro, want)
	}
	if floor := int64(r.cfg.MaxLifetime/time.Minute) * perMin; row.ReservedMicro < floor {
		t.Fatalf("reserved %d is less than MaxLifetime x rate %d", row.ReservedMicro, floor)
	}
	if held := r.dayHeld(res.Token); held != row.ReservedMicro {
		t.Fatalf("the day budget holds %d, want %d from the moment of provisioning", held, row.ReservedMicro)
	}
	r.join(res.Token, "node-1")
	a := queueAttempt(t, r.st, "pipe-hold", "project", "long")
	if err := burstAdmit(ctx, r.st, a, "node-1", "owner"); err != nil {
		t.Fatal(err)
	}
	second := r.request("job-2")
	second.QueuedDemand = 2
	start := r.clk.Now()
	for _, step := range []time.Duration{20 * time.Minute, 20 * time.Minute, 15 * time.Minute} { // the job runs for 55 minutes
		r.clk.Advance(step)
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if held := r.dayHeld(res.Token); held != row.ReservedMicro {
			t.Fatalf("at +%v the budget holds %d, want the whole reservation %d until teardown", r.clk.Now().Sub(start), held, row.ReservedMicro)
		}
		if _, err := r.eng.Provision(ctx, second); burst.ReasonOf(err) != burst.ReasonNoFit {
			t.Fatalf("at +%v the budget left (%d of %d) cannot take a second VM's lifetime: %v", r.clk.Now().Sub(start), 10_000_000-row.ReservedMicro, 10_000_000, err)
		}
	}
	pass(t, r.st, a)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	done := r.resourceRow(res.Token)
	if done.ConfirmedTeardownAt == nil || done.ActualMicro == nil {
		t.Fatalf("the reservation is released at teardown: %+v", done)
	}
	if want := int64(r.clk.Now().Sub(start)/time.Minute) * perMin; *done.ActualMicro != want {
		t.Fatalf("settled at %d, want the 55 minutes actually used (%d)", *done.ActualMicro, want)
	}
	if held := r.dayHeld(res.Token); held != *done.ActualMicro {
		t.Fatalf("after teardown the budget holds the actual %d, got %d", *done.ActualMicro, held)
	}
	if _, err := r.eng.Provision(ctx, second); err != nil {
		t.Fatalf("with the money released the second VM fits: %v", err)
	}
}

func TestBurstPersonalNeverProvisionsCloudCapacity(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t, func(r *burstRig) { r.team = false })
	for i := 0; i < 5; i++ {
		req := r.request(fmt.Sprintf("job-%d", i))
		req.QueuedDemand = 3
		res, err := r.eng.Provision(ctx, req)
		if burst.CodeOf(err) != burst.CodeProvisionRefused || burst.ReasonOf(err) != burst.ReasonTeamOnly || res.Decision.Allow {
			t.Fatalf("PERSONAL provisioned or refused for another reason: %v (%s)", err, res.Decision.Reason)
		}
	}
	if burst.ReasonTeamOnly != string(model.BurstTeamOnly) {
		t.Fatalf("the refusal must be the scheduler's own reason %s, got %s", model.BurstTeamOnly, burst.ReasonTeamOnly)
	}
	if n := r.cloud.Calls["Provision"]; n != 0 || len(r.cloud.All()) != 0 {
		t.Fatalf("cloud capacity under PERSONAL: %d provisions, %d machines", n, len(r.cloud.All()))
	}
	if open, err := r.st.BurstIntents(ctx, burstCoord); err != nil || len(open) != 0 {
		t.Fatalf("intents %+v %v", open, err)
	}
	if ds, _ := r.st.BurstDecisions(ctx, "job-0"); len(ds) != 1 || ds[0].Allowed || ds[0].Reason != burst.ReasonTeamOnly {
		t.Fatalf("the refusal is logged with its reason: %+v", ds)
	}
}

// TestBurstPrivateInfrastructureNeverReachesABurstVMByDefault (I03): a burst VM is a hosted, ephemeral
// runner; with the default privacy zone (private-infrastructure) no trust class lands on it, and the
// reason is privacy.hosted_not_authorized, while the same job places on an owned LAN runner.
func TestBurstPrivateInfrastructureNeverReachesABurstVMByDefault(t *testing.T) {
	vm := runner("burst-vm-1", "hosted", "linux/amd64", 1)
	vm.Ephemeral, vm.Class, vm.Provider = true, "ephemeral", "fakecloud"
	vm.Decl.Hosted, vm.Decl.Provider = true, "fakecloud"
	owned := runner("owned-lan-1", "lan", "linux/amd64", 1)
	world := func(rs ...sched.Runner) sched.World {
		w := sched.World{Policy: sched.Policy{Preset: "balanced", PaidEnabled: true}, Costs: map[string]sched.Cost{"burst-vm-1": {Class: "ephemeral", RateMicro: 1, BillingIncrementMs: 60000}},
			Previous: map[string]string{}, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}}
		for _, r := range rs {
			if r.Location == "hosted" {
				w.Hosted = append(w.Hosted, r)
			} else {
				w.Runners = append(w.Runners, r)
			}
		}
		return w
	}
	for _, tr := range []model.TrustClass{"owner", "internal", "collaborator", "untrusted"} {
		t.Run(string(tr), func(t *testing.T) {
			j := job("private-job", "linux/amd64")
			j.Trust = tr
			if tr == "untrusted" {
				j.Network = "internet" // untrusted work never gets a LAN route (I07), so the control stays about privacy
			}
			got := sched.Place(world(vm), []sched.Job{j})
			if len(got.Placements) != 0 {
				t.Fatalf("a private-infrastructure job was placed on a burst VM: %+v", got.Placements)
			}
			found := false
			for _, x := range got.Explanations {
				for _, alt := range x.Alternatives {
					if alt.RunnerID == "burst-vm-1" {
						found = hasCode(alt.Reasons, "privacy.hosted_not_authorized")
						if alt.Eligible {
							t.Fatal("the burst VM is listed as eligible")
						}
					}
				}
			}
			if !found {
				t.Fatalf("the burst VM was not excluded for privacy.hosted_not_authorized: %+v", got.Explanations)
			}
			// positive control: the same job and world with an owned runner added goes to the owned runner
			both := sched.Place(world(vm, owned), []sched.Job{j})
			if len(both.Placements) != 1 || both.Placements[0].RunnerID != "owned-lan-1" {
				t.Fatalf("control: want the owned runner, got %+v", both.Placements)
			}
		})
	}
}
