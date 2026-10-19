package dispatch

// Purpose: the invariants the dispatcher owns. I01, I03 and I07 run through
// invariants.CheckDispatcher against the real Dispatcher.Plan (orderer + Place,
// with a fake transport driver behind every non-local runner), and a sample of
// the placed worlds goes through a full Tick and Admit against a real store.
// I14 (runner capacity enforced) is the concurrent race test further down.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// fakeTransport is the fake driver registry: every kind is known, deliveries are counted.
type fakeTransport struct {
	known func(string) bool
	run   func(Delivery) error
}

func (f fakeTransport) Known(kind string) bool {
	return f.known == nil || f.known(kind)
}
func (f fakeTransport) Deliver(_ context.Context, d Delivery) error {
	if f.run == nil {
		return nil
	}
	return f.run(d)
}

// invariantCase builds the one-runner world and one-job queue of an invariants.World.
func invariantCase(kind string, f invariants.World) (sched.World, sched.Runner, sched.Job) {
	r := testRunner("node", "lan", 1)
	r.Decl.Isolation, r.Decl.Network, r.Decl.Ownership = f.Isolation, f.Network, f.Ownership
	j := testJob("invariant")
	j.Trust, j.Network, j.SecretClasses = f.Trust, f.Network, f.Secrets
	if kind == "I03" {
		r.Location, r.Class, r.Provider = "hosted", "included", "provider"
		r.Decl.Hosted = true
	}
	w := sched.World{Runners: []sched.Runner{r}, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	if kind == "I03" {
		w.Runners, w.Hosted = nil, []sched.Runner{r}
		w.Allowances = map[string]sched.Allowance{"provider": {RemainingEstimated: 1000000, Known: true, CeilingKnown: true}}
	}
	return w, r, j
}

// checkDispatchInvariant runs one invariant against the dispatcher. Every case
// is planned; every sampleEvery-th placed case is also admitted for real.
func checkDispatchInvariant(t *testing.T, kind string) {
	st := openStore(t)
	var current sched.World
	d := newDispatcher(t, Config{Store: st, Transport: fakeTransport{}, World: func(context.Context, sched.Pipeline) (sched.World, string, error) { return current, "invariant", nil }})
	const sampleEvery = 37
	placedCases, admitted := 0, 0
	invariants.CheckDispatcher(t, func(f invariants.World) error {
		w, r, j := invariantCase(kind, f)
		_, res := d.Plan(w, []Queued{{Job: Job{AttemptID: "invariant", Sched: j}, EnqueuedAt: time.Unix(1, 0), Seq: 1}})
		placed := len(res.Placements) > 0
		var err error
		switch kind {
		case "I01":
			if placed && len(trust.Admissible(trust.JobFacts{Trust: j.Trust, Network: j.Network, SecretClasses: j.SecretClasses, PersonalOwnRun: j.PersonalOwnRun}, r.Decl, w.Policy.Trust)) > 0 {
				err = fmt.Errorf("I01: dispatcher placed a job its runner does not accept %+v", f)
			}
		case "I03":
			if placed {
				err = fmt.Errorf("I03: dispatcher placed a private job on a hosted runner %+v", f)
			}
		case "I07":
			lan := trust.RankNetwork(j.Network) >= trust.RankNetwork("lan") || !r.Decl.NoLANRoute && r.Decl.Isolation != "hosted-disposable"
			if placed && j.Trust == "untrusted" && lan {
				err = fmt.Errorf("I07: dispatcher placed untrusted code on a LAN-reachable runner %+v", f)
			}
		}
		if !placed {
			return err
		}
		placedCases++
		if err != nil || placedCases%sampleEvery != 0 {
			return err
		}
		current = w
		if r.Location == "lan" {
			registerNode(st, r)
		}
		id := queueAttempt(t, st, fmt.Sprintf("inv%d", placedCases), "proj", "job")
		jq := Job{AttemptID: id, Sched: j}
		jq.Sched.Key = id
		e, qerr := d.enqueue(jq)
		if qerr != nil {
			return qerr
		}
		if terr := d.Tick(context.Background()); terr != nil {
			return terr
		}
		if !resolved(e) || e.err != nil || e.res.RunnerID != res.Placements[0].RunnerID {
			return fmt.Errorf("a placed case was not admitted on its runner: %+v err=%v", e.res, e.err)
		}
		admitted++
		pass(t, st, id)
		return nil
	})
	t.Logf("%s: %d placed cases, %d admitted through the store", kind, placedCases, admitted)
	if placedCases > 0 && admitted == 0 {
		t.Fatal("no placed case reached admission")
	}
}

func TestInvariant_I01(t *testing.T) { checkDispatchInvariant(t, "I01") }
func TestInvariant_I03(t *testing.T) { checkDispatchInvariant(t, "I03") }
func TestInvariant_I07(t *testing.T) { checkDispatchInvariant(t, "I07") }
