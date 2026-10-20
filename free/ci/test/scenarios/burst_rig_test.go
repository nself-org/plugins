//go:build scenarios

package scenarios

// Purpose: the rig of the BURST scenarios (P7-SCHED-15, Epic acceptance 6): a TEAM coordinator with a
// real store on an injected clock, the fake cloud, a real burst.Engine with a recording revoker (the
// real store.NodeRevoke plus the burst revoke hook), a recording alert sink and a settable exclusion
// set. Nothing here is shared with internal/burst's own _test files; the shapes are the same on purpose.
// Inputs:  none. Outputs: helpers the burst*_test.go files share.
// Constraints: no sleeps, no network; time moves only through fakecloud.Clock; fakecloud ids are
//          deterministic; every helper fails with t.Fatal, never t.Skip.

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	burstCoord = "01J00000000000000000COORD1"
	burstOther = "01J00000000000000000COORD2"
)

// burstRig is one TEAM coordinator and its cloud. The tests set fields before burstNew builds the engine.
type burstRig struct {
	t       *testing.T
	clk     *fakecloud.Clock
	st      *store.Store
	cloud   *fakecloud.Cloud
	eng     *burst.Engine
	alerts  []burst.Alert
	revoked []string
	exclude []string
	team    bool
	pol     economics.Policy
	fault   func(string) error
	cfg     burst.Config
	path    string
	wrap    func(burst.Provider) burst.Provider
}

func burstPolicy() economics.Policy {
	return economics.Policy{Currency: "USD", PaidEnabled: true, MaxPaidTimeoutMs: schedpolicy.DefaultMaxPaidTimeoutMs,
		Rates:   map[string]schedpolicy.Rate{"fakecloud": {Class: "ephemeral", PerHourMicro: 3_600_000, BillingIncrementMs: 60_000}},
		Budgets: schedpolicy.Budgets{Day: schedpolicy.Budget{Micro: 100_000_000, Set: true}, Provider: map[string]int64{}},
		Burst:   schedpolicy.BurstPolicy{Enabled: map[string]bool{"fakecloud": true}, MaxConcurrent: 2, MarginMs: 60_000}}
}

// burstNew builds a TEAM rig; tweak edits the policy, config or team flag before the engine exists.
func burstNew(t *testing.T, tweak ...func(*burstRig)) *burstRig {
	t.Helper()
	r := &burstRig{t: t, clk: fakecloud.NewClock(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)), team: true, pol: burstPolicy(),
		cfg: burst.Config{MaxLifetime: time.Hour, Grace: 10 * time.Minute, SweepEvery: 5 * time.Minute, RetryWindow: 30 * time.Minute, IdleTimeout: 5 * time.Minute}}
	r.cloud = fakecloud.New(r.clk)
	r.path = filepath.Join(t.TempDir(), "state.db")
	for _, f := range tweak {
		if f != nil {
			f(r)
		}
	}
	r.open()
	r.eng = r.newEngine()
	return r
}

func (r *burstRig) open() {
	r.t.Helper()
	st, err := store.Open(r.path, store.Options{Clock: r.clk.Now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	r.st = st
}

// restart is a new process: the store is reopened, the engine rebuilt, the fault switched off.
func (r *burstRig) restart() {
	r.t.Helper()
	_ = r.st.Close()
	r.fault = nil
	r.open()
	r.eng = r.newEngine()
}

func (r *burstRig) newEngine() *burst.Engine {
	r.t.Helper()
	var prov burst.Provider = r.cloud
	if r.wrap != nil {
		prov = r.wrap(prov)
	}
	eng, err := burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{prov}, Coordinator: burstCoord, Config: r.cfg, Now: r.clk.Now,
		Spend: economics.StaticSpend(r.pol), Team: func() bool { return r.team }, Revoker: burstRevoker{r}, Alert: func(a burst.Alert) { r.alerts = append(r.alerts, a) },
		Exclusion: func(context.Context) (deployguard.Set, error) {
			var es []deployguard.Entry
			for _, a := range r.exclude {
				es = append(es, deployguard.Entry{Kind: deployguard.KindAddress, Value: a, Source: "scenario"})
			}
			return deployguard.NewSet(es), nil
		}, Fault: func(p string) error {
			if r.fault != nil {
				return r.fault(p)
			}
			return nil
		}})
	if err != nil {
		r.t.Fatal(err)
	}
	return eng
}

// burstRevoker revokes through the real store aggregate with the burst hook, and records each revoke.
type burstRevoker struct{ r *burstRig }

func (v burstRevoker) Revoke(ctx context.Context, id string) error {
	_, err := v.r.st.NodeRevoke(ctx, id, "cli:burst-scenario", []store.RevokeHook{store.BurstRevokeHook()})
	v.r.revoked = append(v.r.revoked, id)
	var se *store.Error
	if errors.As(err, &se) && se.Code == "E650" {
		return nil // already gone
	}
	return err
}

func (r *burstRig) request(job string) burst.Request {
	return burst.Request{Provider: r.cloud.ID(), Pipeline: "pipe-1", JobKey: job, Spec: burst.Spec{Platform: "linux/amd64", CPU: 2, MemMB: 4096, TimeoutMs: 600_000}, ExpectedWaitMs: 600_000, QueuedDemand: 1}
}

// provision runs one allowed provision and fails the test otherwise.
func (r *burstRig) provision(job string) burst.Result {
	r.t.Helper()
	res, err := r.eng.Provision(context.Background(), r.request(job))
	if err != nil {
		r.t.Fatalf("provision %s: %v", job, err)
	}
	return res
}

// join registers a node row and enrolls it for the intent.
func (r *burstRig) join(token, node string) {
	r.t.Helper()
	r.addNode(node)
	if err := r.eng.NodeJoined(context.Background(), token, node); err != nil {
		r.t.Fatal(err)
	}
}

// addNode registers a node row, so a revoke has something to revoke.
func (r *burstRig) addNode(node string) {
	r.t.Helper()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "project", Platform: "linux/amd64", CPUs: 8, MemMB: 8192, Now: r.clk.Now()})
	if err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = node, node
	doc, err := json.Marshal(c)
	if err != nil {
		r.t.Fatal(err)
	}
	admin, _ := store.NewAdminAuthority("cli:burst-scenario")
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: node, Name: node, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
}

func (r *burstRig) intent(token string) store.BurstIntent {
	r.t.Helper()
	in, ok, err := r.st.BurstIntent(context.Background(), token)
	if err != nil || !ok {
		r.t.Fatalf("intent %s: %v %v", token, ok, err)
	}
	return in
}

func (r *burstRig) resourceRow(token string) store.ResourceRow {
	r.t.Helper()
	row, ok, err := r.st.Resource(context.Background(), token)
	if err != nil || !ok {
		r.t.Fatalf("resource row %s: %v %v", token, ok, err)
	}
	return row
}

// labelled lists what the cloud holds under coordinator's labels.
func (r *burstRig) labelled(coordinator string) []burst.Resource {
	r.t.Helper()
	rs, err := r.cloud.List(context.Background(), burst.OwnLabels(coordinator))
	if err != nil {
		r.t.Fatal(err)
	}
	return rs
}

// dayHeld is what the day budget holds on the day of the intent's resource row.
func (r *burstRig) dayHeld(token string) int64 {
	r.t.Helper()
	n, err := r.st.Reserved(context.Background(), store.ScopeDay, "", r.resourceRow(token).WindowDay)
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *burstRig) auditCount(action string) int {
	r.t.Helper()
	es, err := r.st.AuditEntries(context.Background(), 0, 100000)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Action == action {
			n++
		}
	}
	return n
}

// putIntent records an intent row directly (the sweeper reads rows, whoever wrote them).
func (r *burstRig) putIntent(coordinator, token, state string) {
	r.t.Helper()
	err := r.st.BurstWrite(context.Background(), func(b store.BurstTx) error {
		now := b.NowMs()
		if err := b.PutIntent(store.BurstIntent{Token: token, Coordinator: coordinator, Provider: "fakecloud", JobKey: "j-" + token, Platform: "linux/amd64", LifetimeMs: r.cfg.LifetimeMs(), CreatedAt: now},
			store.BurstLease{IdleUntil: now + 300_000, MaxUntil: now + 3_600_000}); err != nil {
			return err
		}
		switch state {
		case store.BurstTearingDown:
			return b.MarkTearingDown(token, "scenario", now)
		case store.BurstTerminal:
			return b.MarkTerminal(token, now)
		}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func burstAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

// burstToken is a deterministic intent token for seeded rows.
func burstToken(i int) string { return "01J0000000000000000TOK" + leftPad(strconv.Itoa(i), 5) }

func leftPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// burstAdmit asks the real dispatch admission (the registered `ephemeral` step included) to lease a
// queued attempt to node, the way the dispatcher does for a burst VM.
func burstAdmit(ctx context.Context, st *store.Store, attempt, node string, tr model.TrustClass, labels ...string) error {
	rn := sched.Runner{ID: node, Provider: "fakecloud", Ephemeral: true}
	rn.Capability.Identity.Labels = labels
	_, err := dispatch.Admit(ctx, st, dispatch.Request{AttemptID: attempt, Placement: sched.Placement{JobKey: attempt, RunnerID: node, CPU: 1, MemMB: 256}, Runner: rn,
		Job: sched.Job{Key: attempt, Trust: tr, TimeoutMs: 60_000}, Local: true, Host: "host-" + attempt, CapacityCPU: 8, CapacityMemMB: 8192})
	return err
}
