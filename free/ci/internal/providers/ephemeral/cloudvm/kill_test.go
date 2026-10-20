package cloudvm_test

// Purpose: TestCloudVMKillAtEachStep*: the coordinator is killed at each step of a VM's life (the engine's
// Fault hook returns ErrKilled, as a dead process would); after a restart and one start-up sweep no
// labelled server of this coordinator is left, every intent is terminal, and servers without both
// labels, or of another coordinator, are never touched. The cloud is the fake cloud behind the fake nself.
// Inputs:  the rig. Outputs: pass/fail. Constraints: the clock is stepped, never slept on.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func killAt(r *rig, point string) {
	r.fault = func(p string) error {
		if p == point {
			return burst.ErrKilled
		}
		return nil
	}
}

// restart is a new process: the store is reopened, everything rebuilt, the fault gone.
func (r *rig) restart() {
	r.t.Helper()
	_ = r.st.Close()
	r.fault = nil
	r.st = r.open()
	r.build()
}

func TestCloudVMKillAtEachStepLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	for _, point := range []string{burst.PointAfterIntent, burst.PointAfterCreate, burst.PointAfterJoin, burst.PointBeforeMark, burst.PointMidTeardown} {
		t.Run(point, func(t *testing.T) {
			r := newRig(t)
			other := r.cloud.AddForeign(map[string]string{"role": "ci-ephemeral", "coordinator_id": "another-coordinator", "intent": "X"})
			bare := r.cloud.AddForeign(nil)
			onlyRole := r.cloud.AddForeign(map[string]string{"role": "ci-ephemeral"})
			var token string
			switch point {
			case burst.PointAfterIntent, burst.PointAfterCreate:
				killAt(r, point)
				res, err := r.eng.Provision(ctx, r.request("job-1"))
				if !errors.Is(err, burst.ErrKilled) {
					t.Fatalf("want the kill, got %v", err)
				}
				token = res.Token
				if created := len(r.labelled()); (point == burst.PointAfterCreate) != (created == 1) {
					t.Fatalf("labelled servers at %s: %d", point, created)
				}
			default:
				res := r.provision("job-1")
				token = res.Token
				name := cloudvm.NodeName(token)
				r.addNode(name)
				if point == burst.PointAfterJoin {
					killAt(r, point)
				}
				if err := r.eng.NodeJoined(ctx, token, name); point == burst.PointAfterJoin != errors.Is(err, burst.ErrKilled) {
					t.Fatalf("join at %s: %v", point, err)
				}
				if point != burst.PointAfterJoin {
					killAt(r, point)
					if err := r.eng.Teardown(ctx, token, "job done"); !errors.Is(err, burst.ErrKilled) {
						t.Fatalf("want the kill, got %v", err)
					}
				}
			}
			r.clk.Advance(2 * time.Minute)
			r.restart()
			if err := r.eng.Start(ctx); err != nil {
				t.Fatalf("recovery: %v", err)
			}
			if left := r.labelled(); len(left) != 0 {
				t.Fatalf("labelled servers of this coordinator after restart and one sweep: %+v", left)
			}
			if in := r.intent(token); in.State != store.BurstTerminal {
				t.Fatalf("intent is %s", in.State)
			}
			if open, err := r.st.BurstIntents(ctx, coord); err != nil || len(open) != 0 {
				t.Fatalf("open intents %+v %v", open, err)
			}
			for _, id := range []string{other, bare, onlyRole} {
				if !r.cloud.Has(id) {
					t.Fatalf("the sweep deleted %s, which is not this coordinator's labelled server", id)
				}
			}
		})
	}
}

func TestCloudVMKillWhileTheApiIsDownConvergesAfterItReturns(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	res := r.provision("job-1")
	killAt(r, burst.PointMidTeardown)
	r.br.set(func(k *knobs) { k.outage = true })
	if err := r.eng.Teardown(ctx, res.Token, "job done"); err == nil {
		t.Fatal("the teardown cannot succeed during an outage")
	}
	r.clk.Advance(2 * time.Minute)
	r.restart()
	_ = r.eng.Start(ctx) // still down: the delete is retried, not lost
	if len(r.labelled()) != 1 {
		t.Fatal("the server cannot have gone while the API was down")
	}
	r.br.set(func(k *knobs) { k.outage = false })
	r.clk.Advance(6 * time.Minute)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if left := r.labelled(); len(left) != 0 {
		t.Fatalf("labelled servers left after the API returned: %+v", left)
	}
}

func TestCloudVMLostCreateResponseIsCleanedUp(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.br.set(func(k *knobs) { k.dieOnProvision = true })
	res, err := r.eng.Provision(ctx, r.request("job-1"))
	if err == nil {
		t.Fatal("a lost response cannot look like success")
	}
	if left := r.labelled(); len(left) != 0 {
		t.Fatalf("the engine left the machine whose response was lost: %+v", left)
	}
	if res.Token != "" && r.intent(res.Token).State != store.BurstTerminal {
		t.Fatalf("intent %+v", r.intent(res.Token))
	}
}

// TestCloudVMCrashAfterCreateIsAdoptedNotDuplicated: killed between the create and the recorded id; the
// retry finds the machine by its intent label (List, then no second create) and recovery removes it.
func TestCloudVMCrashAfterCreateIsAdoptedNotDuplicated(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	killAt(r, burst.PointAfterCreate)
	res, err := r.eng.Provision(ctx, r.request("job-1"))
	if !errors.Is(err, burst.ErrKilled) {
		t.Fatal(err)
	}
	r.restart()
	if _, err = r.eng.Resume(ctx, res.Token, r.request("job-1").Spec); err != nil {
		t.Fatal(err)
	}
	if len(r.labelled()) != 1 {
		t.Fatalf("the retry duplicated the machine: %d", len(r.labelled()))
	}
	if err = r.eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.labelled()) != 0 {
		t.Fatal("recovery left the machine")
	}
}
