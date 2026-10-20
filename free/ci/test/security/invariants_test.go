//go:build security

package security

// Purpose: I01, I03 and I07 against the real dispatcher (review R-SEC-22): every enumerated
// and generated world goes through Dispatcher.Plan (orderer plus scheduler), and a sample of
// the placed worlds goes through Submit, the store and Admit. The invariant is judged on the
// placement the dispatcher made, never on a model of it.
//   I01 untrusted only on a runner that accepts it
//   I03 private jobs never on a hosted runner
//   I07 untrusted never gets LAN
// Inputs:  invariants.World (enumerated and seeded), the fixtures.
// Outputs: one INVARIANT log line per test (enumerated, generated, seed, violations).
// Constraints: the test names keep the TestInvariant_ prefix: invariants.Check derives the id from it.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// invariantCase builds the one-runner world and the job of an invariants.World.
func invariantCase(kind string, f invariants.World) (sched.World, sched.Runner, sched.Job) {
	r := lanRunner("node", 1)
	r.Decl.Isolation, r.Decl.Network, r.Decl.Ownership = f.Isolation, f.Network, f.Ownership
	j := newJob("invariant", f.Trust, f.Network, f.Secrets...)
	if kind == "I03" {
		r = hostedRunner("provider")
		r.Decl.Isolation = f.Isolation
	}
	w := sched.World{Runners: []sched.Runner{r}, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	if kind == "I03" {
		w.Runners, w.Hosted = nil, []sched.Runner{r}
		w.Allowances = map[string]sched.Allowance{"fake": {RemainingEstimated: 1 << 50, Known: true, CeilingKnown: true}}
	}
	return w, r, j
}

func checkDispatcherInvariant(t *testing.T, kind string) {
	st := openStore(t)
	var current sched.World
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Transport: &fabricTransport{}, Poll: 5 * time.Millisecond,
		World: func(context.Context, sched.Pipeline) (sched.World, string, error) { return current, "invariant", nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	const sampleEvery = 37
	placedCases, admitted := 0, 0
	invariants.CheckDispatcher(t, func(f invariants.World) error {
		w, r, j := invariantCase(kind, f)
		queued := []dispatch.Queued{{Job: dispatch.Job{AttemptID: "invariant", Sched: j}, EnqueuedAt: time.Unix(1, 0), Seq: 1}}
		_, res := d.Plan(w, queued)
		placed := len(res.Placements) > 0
		var verr error
		switch kind {
		case "I01":
			if placed && len(trust.Admissible(trust.JobFacts{Trust: j.Trust, Network: j.Network, SecretClasses: j.SecretClasses, PersonalOwnRun: j.PersonalOwnRun}, r.Decl, w.Policy.Trust)) > 0 {
				verr = fmt.Errorf("I01: the dispatcher placed a job its runner does not accept %+v", f)
			}
		case "I03":
			if placed {
				verr = fmt.Errorf("I03: the dispatcher placed a private job on a hosted runner %+v", f)
			}
		case "I07":
			lan := trust.RankNetwork(j.Network) >= trust.RankNetwork("lan") || !r.Decl.NoLANRoute && r.Decl.Isolation != "hosted-disposable"
			if placed && j.Trust == "untrusted" && lan {
				verr = fmt.Errorf("I07: the dispatcher placed untrusted code on a LAN-reachable runner %+v", f)
			}
		}
		if !placed {
			return verr
		}
		placedCases++
		if verr != nil || placedCases%sampleEvery != 0 {
			return verr
		}
		current = w
		if r.Location == "lan" {
			registerNode(st, r)
		}
		id := queueAttempt(t, st, fmt.Sprintf("inv%d", placedCases), "project", "job", "owner")
		jq := j
		jq.Key = id
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a, serr := d.Submit(ctx, dispatch.Job{AttemptID: id, Sched: jq, Revision: "rev"})
		if serr != nil || a.RunnerID != res.Placements[0].RunnerID {
			return fmt.Errorf("a placed case was not admitted on its runner: %+v err=%v", a, serr)
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

func TestInvariant_I01(t *testing.T) { checkDispatcherInvariant(t, "I01") }
func TestInvariant_I03(t *testing.T) { checkDispatcherInvariant(t, "I03") }
func TestInvariant_I07(t *testing.T) { checkDispatcherInvariant(t, "I07") }
