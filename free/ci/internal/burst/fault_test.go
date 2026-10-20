//go:build fault

package burst_test

// Fault injection (tag fault): the coordinator is killed at each step of a
// machine's life; after a restart and one sweep there is no labelled machine of
// this coordinator left, every intent is terminal and every reservation released.
// A kill is the Fault hook returning ErrKilled: the engine stops at that point
// and runs nothing after it, exactly as a dead process would.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
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

// restart is a new process: the store is reopened, the engine rebuilt, the fault gone.
func (r *rig) restart() {
	r.t.Helper()
	_ = r.st.Close()
	r.fault = nil
	r.st = r.open()
	r.build()
}

func (r *rig) assertClean(tokens ...string) {
	r.t.Helper()
	if left := r.labelled(coord); len(left) != 0 {
		r.t.Fatalf("labelled machines of this coordinator after restart and one sweep: %+v", left)
	}
	for _, tok := range tokens {
		if in := r.intent(tok); in.State != store.BurstTerminal {
			r.t.Fatalf("intent %s is %s", tok, in.State)
		}
		if row := r.resourceRow(tok); row.ConfirmedTeardownAt == nil {
			r.t.Fatalf("reservation of %s was not released", tok)
		}
	}
	if open, err := r.st.BurstIntents(context.Background(), coord); err != nil || len(open) != 0 {
		r.t.Fatalf("open intents %+v %v", open, err)
	}
}

func TestCoordinatorKill(t *testing.T) {
	ctx := context.Background()
	for _, point := range []string{burst.PointAfterIntent, burst.PointAfterCreate, burst.PointAfterJoin, burst.PointMidJob, burst.PointMidTeardown} {
		t.Run(point, func(t *testing.T) {
			r := newRig(t)
			killAt(r, point)
			var token string
			switch point {
			case burst.PointAfterIntent, burst.PointAfterCreate:
				res, err := r.eng.Provision(ctx, r.request("job-1"))
				if !errors.Is(err, burst.ErrKilled) {
					t.Fatalf("want the kill, got %v", err)
				}
				token = res.Token
				if point == burst.PointAfterCreate && len(r.cloud.All()) != 1 {
					t.Fatalf("the machine was created before the kill: %d", len(r.cloud.All()))
				}
				if point == burst.PointAfterIntent && len(r.cloud.All()) != 0 {
					t.Fatal("the kill came before any provider call")
				}
			case burst.PointAfterJoin:
				r.fault = nil
				res := r.provision("job-1")
				token = res.Token
				killAt(r, point)
				r.addNode("node-1")
				if err := r.eng.NodeJoined(ctx, token, "node-1"); !errors.Is(err, burst.ErrKilled) {
					t.Fatalf("want the kill, got %v", err)
				}
			case burst.PointMidJob:
				r.fault = nil
				res := r.provision("job-1")
				token = res.Token
				r.join(token, "node-1")
				a := queue(t, r.st, "pk")
				if _, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true)); err != nil {
					t.Fatal(err)
				}
				killAt(r, point)
				r.clk.Advance(5 * time.Minute)
				if _, err := r.eng.Tick(ctx); !errors.Is(err, burst.ErrKilled) {
					t.Fatalf("want the kill, got %v", err)
				}
			case burst.PointMidTeardown:
				r.fault = nil
				res := r.provision("job-1")
				token = res.Token
				r.join(token, "node-1")
				killAt(r, point)
				if err := r.eng.Teardown(ctx, token, "job done"); !errors.Is(err, burst.ErrKilled) {
					t.Fatalf("want the kill, got %v", err)
				}
				if len(r.cloud.All()) != 1 {
					t.Fatal("the kill came before the delete")
				}
			}
			r.clk.Advance(2 * time.Minute)
			r.restart()
			if err := r.eng.Start(ctx); err != nil {
				t.Fatalf("recovery: %v", err)
			}
			r.assertClean(token)
		})
	}
}

// TestCoordinatorKillCreateCrashIsAdoptedNotDuplicated: after a crash between the
// create and the recorded id, retrying the provision finds the machine by its intent
// label and adopts it; a second machine never appears.
func TestCoordinatorKillCreateCrashIsAdoptedNotDuplicated(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	killAt(r, burst.PointAfterCreate)
	res, err := r.eng.Provision(ctx, r.request("job-1"))
	if !errors.Is(err, burst.ErrKilled) {
		t.Fatal(err)
	}
	r.restart()
	got, err := r.eng.Resume(ctx, res.Token, r.request("job-1").Spec)
	if err != nil {
		t.Fatal(err)
	}
	if all := r.cloud.All(); len(all) != 1 || all[0].ID != got.ID {
		t.Fatalf("duplicated: %+v", all)
	}
	if err = r.eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	r.assertClean(res.Token)
}

// TestCoordinatorKillDuringOutageStillConverges: killed mid-teardown while the API is
// down; the restarted coordinator retries until the API is back, then leaves nothing.
func TestCoordinatorKillDuringOutageStillConverges(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	r.cloud.Outage(true)
	if err := r.eng.Teardown(ctx, res.Token, "job done"); burst.CodeOf(err) != burst.CodeTeardownFailed {
		t.Fatal(err)
	}
	r.restart()
	_ = r.eng.Start(ctx) // still down: alerts already given, retried
	r.clk.Advance(3 * time.Hour)
	r.cloud.Outage(false)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	r.assertClean(res.Token)
	if r.auditCount("burst.teardown_failed") != 1 {
		t.Fatalf("one audit row for the failure, got %d", r.auditCount("burst.teardown_failed"))
	}
}
