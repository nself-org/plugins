package dispatch

// What the admission transaction re-reads or bounds beyond counters: the node
// row, a step that wedges the writer, an operator's cancel intent.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestDispatchNodeRecheck: a node drained, revoked or deauthorized for the project
// after the world was built gets no lease: admission re-reads the node row (D-1086).
func TestDispatchNodeRecheck(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(context.Context, *store.Store, store.NodeRecord) error
		reason model.PlacementReason
	}{
		{"drained", func(ctx context.Context, st *store.Store, n store.NodeRecord) error {
			n.State = "draining"
			return st.NodeSetState(ctx, n, "test", "drain")
		}, model.NodeDraining},
		{"revoked", func(ctx context.Context, st *store.Store, n store.NodeRecord) error {
			_, err := st.NodeRevoke(ctx, n.ID, "cli:test", nil)
			return err
		}, model.NodeRevoked},
		{"deauthorized", func(ctx context.Context, st *store.Store, n store.NodeRecord) error {
			var cap model.Capability
			if err := json.Unmarshal(n.Capability, &cap); err != nil {
				return err
			}
			cap.Lifecycle.AuthorizedProjects = []string{}
			doc, err := json.Marshal(cap)
			n.Capability = doc
			if err != nil {
				return err
			}
			return st.NodeSetAuthorization(ctx, n, "test")
		}, model.NodeDiscovered},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := openStore(t)
			live := liveWorld(st, nil, testRunner("node-r", "lan", 2))
			changed := false
			world := func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
				w, digest, err := live(ctx, p) // registers the node, online and authorized
				if err != nil || changed {
					return w, digest, err
				}
				changed = true
				n, err := st.NodeGet(ctx, "node-r")
				if err == nil {
					err = c.change(ctx, st, n) // the operator acts after the snapshot
				}
				return w, digest, err
			}
			d := newDispatcher(t, Config{Store: st, World: world, Transport: fakeTransport{}})
			id := queueAttempt(t, st, "pipe", "proj", "a")
			e, _ := d.enqueue(testQueued(id))
			if err := d.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if resolved(e) {
				t.Fatalf("a %s node got a lease: %+v", c.name, e.res)
			}
			if reason, _, ok, _ := st.Refusal(context.Background(), id); !ok || reason != string(c.reason) {
				t.Fatalf("reason %q (recorded %v), want %s", reason, ok, c.reason)
			}
			if snap, _ := st.WorldSnapshot(context.Background()); len(snap.Active) != 0 {
				t.Fatal("a lease exists for the unavailable node")
			}
		})
	}
}

// TestDispatchStepWedge: a step that waits for the writer its own transaction
// holds fails at the admission timeout instead of wedging every write.
func TestDispatchStepWedge(t *testing.T) {
	old := admitTimeout
	admitTimeout = 100 * time.Millisecond
	t.Cleanup(func() { admitTimeout = old })
	st := openStore(t)
	withSteps(t, map[string]Step{"zz-reenter": func(ctx context.Context, v View) error {
		_, _, err := st.AttemptStateEpoch(ctx, v.Demand.AttemptID) // needs the writer this transaction holds
		return err
	}})
	id := queueAttempt(t, st, "pipe", "proj", "a")
	done := make(chan error, 1)
	go func() {
		_, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: testRunner("R", "lan", 2), Job: testJob(id), Local: true, Host: "h", CapacityCPU: 4, CapacityMemMB: 4096})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a re-entrant step cannot succeed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the admission never returned: the writer is wedged")
	}
	if err := st.AppendAudit(context.Background(), "test", "after", "x", "the writer works"); err != nil {
		t.Fatalf("a later write after the timeout: %v", err)
	}
}

// TestDispatchCancelIntent: a queued attempt an operator cancelled ends the job.
func TestDispatchCancelIntent(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2))})
	id := queueAttempt(t, st, "pipe", "proj", "a")
	e, _ := d.enqueue(testQueued(id))
	if err := st.RequestCancel(context.Background(), id, "operator", 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(e) || e.err == nil {
		t.Fatalf("a cancelled attempt must end the job, not conflict for ever: resolved=%v err=%v", resolved(e), e.err)
	}
	if errors.Is(e.err, ErrConflict) {
		t.Fatal("the conflict leaked to the caller")
	}
}
