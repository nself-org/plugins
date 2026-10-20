package burst_test

// The test rig: a real store on an injected clock, the fake cloud, an engine with
// a recording revoker (the real store.NodeRevoke with the burst hook and a stand-in
// for NODE-07's deny list), a recording alert sink and a settable exclusion set.

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const coord = "01J00000000000000000COORD1"

type rig struct {
	t       *testing.T
	clk     *fakecloud.Clock
	st      *store.Store
	cloud   *fakecloud.Cloud
	eng     *burst.Engine
	alerts  []burst.Alert
	revoked []string
	denied  []string
	exclude []string
	team    bool
	pol     economics.Policy
	fault   func(string) error
	cfg     burst.Config
	path    string
	wrap    func(burst.Provider) burst.Provider
	// teamHook runs when the engine asks whether it is a TEAM coordinator: after the counters are read, before the commit.
	teamHook func()
}

func policy() economics.Policy {
	return economics.Policy{Currency: "USD", PaidEnabled: true, MaxPaidTimeoutMs: schedpolicy.DefaultMaxPaidTimeoutMs,
		Rates:   map[string]schedpolicy.Rate{"fakecloud": {Class: "ephemeral", PerHourMicro: 3_600_000, BillingIncrementMs: 60_000}},
		Budgets: schedpolicy.Budgets{Day: schedpolicy.Budget{Micro: 100_000_000, Set: true}, Provider: map[string]int64{}},
		Burst:   schedpolicy.BurstPolicy{Enabled: map[string]bool{"fakecloud": true}, MaxConcurrent: 2, MarginMs: 60_000}}
}

// newRig builds a TEAM rig; tweak may change the policy, the team flag and the config before the engine is built.
func newRig(t *testing.T, tweak ...func(*rig)) *rig {
	t.Helper()
	r := &rig{t: t, clk: fakecloud.NewClock(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)), team: true, pol: policy(), cfg: burst.Config{MaxLifetime: time.Hour, Grace: 10 * time.Minute, SweepEvery: 5 * time.Minute, RetryWindow: 30 * time.Minute, IdleTimeout: 5 * time.Minute}}
	r.cloud = fakecloud.New(r.clk)
	r.path = filepath.Join(t.TempDir(), "state.db")
	for _, f := range tweak {
		if f != nil {
			f(r)
		}
	}
	r.st = r.open()
	r.build()
	return r
}

func (r *rig) open() *store.Store {
	r.t.Helper()
	st, err := store.Open(r.path, store.Options{Clock: r.clk.Now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	return st
}

// build (re)creates the engine; a second call after a "kill" is the restarted coordinator.
func (r *rig) build() { r.eng = r.newEngine() }

// newEngine builds another engine over the same store, cloud and clock.
func (r *rig) newEngine() *burst.Engine {
	r.t.Helper()
	var prov burst.Provider = r.cloud
	if r.wrap != nil {
		prov = r.wrap(prov)
	}
	eng, err := burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{prov}, Coordinator: coord, Config: r.cfg, Now: r.clk.Now,
		Spend: economics.StaticSpend(r.pol), Team: func() bool {
			if r.teamHook != nil {
				r.teamHook()
			}
			return r.team
		}, Revoker: revoker{r}, Alert: func(a burst.Alert) { r.alerts = append(r.alerts, a) },
		Exclusion: func(context.Context) (deployguard.Set, error) {
			var es []deployguard.Entry
			for _, a := range r.exclude {
				es = append(es, deployguard.Entry{Kind: deployguard.KindAddress, Value: a, Source: "test"})
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

// revoker revokes through the real store aggregate with the burst hook and a recording deny-list stand-in.
type revoker struct{ r *rig }

func (v revoker) Revoke(ctx context.Context, id string) error {
	_, err := v.r.st.NodeRevoke(ctx, id, "cli:burst-test", []store.RevokeHook{store.BurstRevokeHook(), func(context.Context, *store.RevokeTx, string) error {
		v.r.denied = append(v.r.denied, id)
		return nil
	}})
	v.r.revoked = append(v.r.revoked, id)
	var se *store.Error
	if errors.As(err, &se) && se.Code == "E650" {
		return nil // already gone
	}
	return err
}

func (r *rig) request(job string) burst.Request {
	return burst.Request{Provider: r.cloud.ID(), Pipeline: "pipe-1", JobKey: job, Spec: burst.Spec{Platform: "linux/amd64", CPU: 2, MemMB: 4096, TimeoutMs: 600_000}, ExpectedWaitMs: 600_000, QueuedDemand: 1}
}

// provision runs one allowed provision and fails the test otherwise.
func (r *rig) provision(job string) burst.Result {
	r.t.Helper()
	res, err := r.eng.Provision(context.Background(), r.request(job))
	if err != nil {
		r.t.Fatalf("provision %s: %v", job, err)
	}
	return res
}

func (r *rig) intent(token string) store.BurstIntent {
	r.t.Helper()
	in, ok, err := r.st.BurstIntent(context.Background(), token)
	if err != nil || !ok {
		r.t.Fatalf("intent %s: %v %v", token, ok, err)
	}
	return in
}

// addNode registers a node row so a revoke has something to revoke.
func (r *rig) addNode(id string) {
	r.t.Helper()
	b, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:burst-test")
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
}

// join enrolls a node for the intent.
func (r *rig) join(token, node string) {
	r.t.Helper()
	r.addNode(node)
	if err := r.eng.NodeJoined(context.Background(), token, node); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) labelled(coordinator string) []burst.Resource {
	rs, err := r.cloud.List(context.Background(), burst.OwnLabels(coordinator))
	if err != nil {
		r.t.Fatal(err)
	}
	return rs
}

func (r *rig) resourceRow(token string) store.ResourceRow {
	r.t.Helper()
	row, ok, err := r.st.Resource(context.Background(), token)
	if err != nil || !ok {
		r.t.Fatalf("resource row %s: %v %v", token, ok, err)
	}
	return row
}

func (r *rig) auditCount(action string) int {
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

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

func (r *rig) eng2Spend() func(context.Context) (economics.Policy, error) {
	return economics.StaticSpend(r.pol)
}

func (r *rig) eng2Exclusion() func(context.Context) (deployguard.Set, error) {
	return func(context.Context) (deployguard.Set, error) { return deployguard.NewSet(nil), nil }
}
