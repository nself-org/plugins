//go:build scenarios

package scenarios

// Purpose: Epic acceptance 6, sweeper bullets: a coordinator kill at EACH step of a machine's life
// (after-intent, after-create, after-join, mid-job) leaves zero labelled resources of this coordinator
// after ONE sweep on a restarted engine; and the sweeper never touches unlabelled, foreign-coordinator
// or excluded resources.
// Inputs:  burst.Engine over fakecloud and a real store; the burst.Fault hook is the kill switch.
// Outputs: assertions; burstCheckSweepSpares is the check function the planted regression reuses.
// Constraints: a kill is Fault returning burst.ErrKilled (the engine stops exactly there); a restart is
//          a new store handle and a new Engine on the same state file; no sleeps.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func burstKillAt(r *burstRig, point string) {
	r.fault = func(p string) error {
		if p == point {
			return burst.ErrKilled
		}
		return nil
	}
}

// burstKillAtPoint drives the machine's life up to point, kills the coordinator there and returns the intent token.
func burstKillAtPoint(t *testing.T, r *burstRig, point string) string {
	t.Helper()
	ctx := context.Background()
	switch point {
	case burst.PointAfterIntent, burst.PointAfterCreate:
		burstKillAt(r, point)
		res, err := r.eng.Provision(ctx, r.request("job-1"))
		if !errors.Is(err, burst.ErrKilled) {
			t.Fatalf("want the kill, got %v", err)
		}
		if want := map[string]int{burst.PointAfterIntent: 0, burst.PointAfterCreate: 1}[point]; len(r.cloud.All()) != want {
			t.Fatalf("machines at %s: %d, want %d", point, len(r.cloud.All()), want)
		}
		return res.Token
	case burst.PointAfterJoin:
		res := r.provision("job-1")
		burstKillAt(r, point)
		r.addNode("node-1")
		if err := r.eng.NodeJoined(ctx, res.Token, "node-1"); !errors.Is(err, burst.ErrKilled) {
			t.Fatalf("want the kill, got %v", err)
		}
		return res.Token
	case burst.PointMidJob:
		res := r.provision("job-1")
		r.join(res.Token, "node-1")
		a := queueAttempt(t, r.st, "pipe-kill", "project", "kill")
		if err := burstAdmit(ctx, r.st, a, "node-1", "owner"); err != nil {
			t.Fatal(err)
		}
		burstKillAt(r, point)
		r.clk.Advance(5 * time.Minute)
		if _, err := r.eng.Tick(ctx); !errors.Is(err, burst.ErrKilled) {
			t.Fatalf("want the kill, got %v", err)
		}
		return res.Token
	}
	t.Fatalf("unknown fault point %s", point)
	return ""
}

func TestBurstCoordinatorKillLeavesNoLabelledResourceAfterOneSweep(t *testing.T) {
	ctx := context.Background()
	points := []string{burst.PointAfterIntent, burst.PointAfterCreate, burst.PointAfterJoin, burst.PointMidJob}
	for _, mode := range []string{"restart-recovery", "sweep-only"} {
		for _, point := range points {
			t.Run(mode+"/"+point, func(t *testing.T) {
				r := burstNew(t)
				token := burstKillAtPoint(t, r, point)
				r.clk.Advance(r.cfg.OrphanAfter() + time.Minute) // the dead coordinator's machine outlives max_lifetime + grace
				r.restart()
				if mode == "restart-recovery" {
					if err := r.eng.Start(ctx); err != nil { // a dispatcher start: tear the open intents down, then ONE sweep
						t.Fatalf("recovery: %v", err)
					}
				} else if rep, err := r.eng.Sweep(ctx, false); err != nil {
					t.Fatalf("sweep: %v (%+v)", err, rep)
				}
				if left := r.labelled(burstCoord); len(left) != 0 {
					t.Fatalf("labelled machines of this coordinator after one sweep: %+v", left)
				}
				if point == burst.PointAfterJoin || point == burst.PointMidJob {
					if len(r.revoked) != 1 {
						t.Fatalf("the joined node must be revoked: %v", r.revoked)
					}
				}
				if mode == "restart-recovery" {
					if in := r.intent(token); in.State != store.BurstTerminal {
						t.Fatalf("intent %s is %s", token, in.State)
					}
					if row := r.resourceRow(token); row.ConfirmedTeardownAt == nil {
						t.Fatal("the reservation was not released")
					}
				}
			})
		}
	}
}

// burstCheckSweepSpares runs the sweeper over a cloud that holds one expired machine of this coordinator
// (it must go) plus machines it must never touch, and returns what it got wrong (empty = holds).
func burstCheckSweepSpares(r *burstRig) []string {
	t := r.t
	t.Helper()
	ctx := context.Background()
	var bad []string
	foreign := r.cloud.AddForeign(burst.IntentLabels(burstOther, burstToken(1)), burstAddr("192.0.2.201"))
	unlabelled := r.cloud.AddForeign(nil, burstAddr("192.0.2.202"))
	otherRole := r.cloud.AddForeign(map[string]string{"role": "builder", burst.LabelCoordinator: burstCoord}, burstAddr("192.0.2.203"))
	relabelled := r.cloud.AddForeign(burst.IntentLabels(burstCoord, burstToken(4)), burstAddr("192.0.2.205"))
	r.cloud.Relabel(relabelled, burst.IntentLabels(burstOther, burstToken(4))) // another coordinator took it over
	r.putIntent(burstCoord, burstToken(2), store.BurstTerminal)
	excluded := r.cloud.AddForeign(burst.IntentLabels(burstCoord, burstToken(2)), burstAddr("198.51.100.77")) // ours by label, but a deploy host
	r.exclude = []string{"198.51.100.77"}
	r.putIntent(burstCoord, burstToken(5), store.BurstTerminal)
	expired := r.cloud.AddForeign(burst.IntentLabels(burstCoord, burstToken(5)), burstAddr("192.0.2.206")) // ours, terminal intent: must be swept
	// labelled ours, but the intent row it names belongs to another coordinator and is terminal: only the
	// intent's owner check keeps the sweeper from reaping it
	r.putIntent(burstOther, burstToken(6), store.BurstTerminal)
	intentForeign := r.cloud.AddForeign(burst.IntentLabels(burstCoord, burstToken(6)), burstAddr("192.0.2.207"))
	spared := map[string]string{foreign: "foreign-coordinator", unlabelled: "unlabelled", otherRole: "other-role", relabelled: "relabelled", excluded: "excluded", intentForeign: "foreign-intent"}
	for id := range spared {
		r.cloud.Age(id, 10*time.Hour)
	}
	r.eng = r.newEngine() // pick up the exclusion set and any provider wrapper
	if _, err := r.eng.Sweep(ctx, false); err != nil {
		bad = append(bad, fmt.Sprintf("sweep failed: %v", err))
	}
	if _, err := r.eng.Tick(ctx); err != nil {
		bad = append(bad, fmt.Sprintf("tick failed: %v", err))
	}
	for id, why := range spared {
		if !r.cloud.Has(id) {
			bad = append(bad, fmt.Sprintf("the sweeper deleted a %s machine (%s)", why, id))
		}
	}
	if r.cloud.Has(expired) {
		bad = append(bad, "the sweeper left this coordinator's expired machine in place (the check is not exercising the sweep)")
	}
	sawExcluded := false
	for _, a := range r.alerts {
		sawExcluded = sawExcluded || a.Kind == burst.AlertExcluded && a.Resource == excluded
	}
	if !sawExcluded {
		bad = append(bad, "no burst.excluded alert for an own-labelled machine in the exclusion set")
	}
	return bad
}

func TestBurstSweeperNeverTouchesForeignUnlabelledOrExcluded(t *testing.T) {
	r := burstNew(t)
	if bad := burstCheckSweepSpares(r); len(bad) != 0 {
		t.Fatalf("sweeper touched what is not its own: %v", bad)
	}
}
