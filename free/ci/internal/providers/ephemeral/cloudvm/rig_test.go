package cloudvm_test

// Purpose: the shared rig of the cloud VM tests: a TEAM coordinator in miniature. One steppable clock
// feeds a real SQLite store, the node CA and pairing service, the burst engine and the fake cloud; the
// provider under test reaches the cloud only through the fake `nself` on PATH (fakenself_test.go).
// Inputs:  tweak functions that change the rig before it is built. Outputs: helpers only.
// Constraints: no sleeps (the clock is stepped), no fixed ports, every secret-looking value is synthetic.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	coord      = "01J00000000000000000COORD1"
	fakeToken  = "synthetic-token-0001-not-real"
	pluginHome = "/home/coordinator-test"
	agentSHA   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	agentURL   = "https://agents.example.test/nself-ci-agent-linux-amd64"
	providerID = "hetzner"
)

type rig struct {
	t         *testing.T
	clk       *fakecloud.Clock
	cloud     *fakecloud.Cloud
	br        *bridge
	st        *store.Store
	ca        *pki.CA
	reg       *registry.Registry
	pair      *pairing.Service
	eng       *burst.Engine
	prov      *cloudvm.Provider
	pol       economics.Policy
	team      bool
	cfg       burst.Config
	dir, home string
	tokenFile string
	enrollURL string
	enrollAdr string
	path      string
	fault     func(string) error
	denied    []string
	alerts    []burst.Alert
}

func basePolicy(tokenFile, enrollURL string) economics.Policy {
	return economics.Policy{Currency: "USD", PaidEnabled: true, MaxPaidTimeoutMs: schedpolicy.DefaultMaxPaidTimeoutMs,
		Rates:   map[string]schedpolicy.Rate{providerID: {Class: "ephemeral", PerHourMicro: 5_000_000, BillingIncrementMs: 3_600_000, ExtrasMicro: 100_000}},
		Budgets: schedpolicy.Budgets{Day: schedpolicy.Budget{Micro: 100_000_000, Set: true}, Provider: map[string]int64{}},
		Burst: schedpolicy.BurstPolicy{Enabled: map[string]bool{providerID: true}, MaxConcurrent: 4, MarginMs: 60_000,
			Trust: schedpolicy.BurstTrust{Accepts: []string{"owner", "collaborator", "internal"}, Secrets: []string{"none"}, Isolation: "ephemeral-vm", PrivacyZone: "provider-allowlist"},
			CloudVM: schedpolicy.CloudVM{Type: "cx22", Location: "fsn1", Image: "ubuntu-24.04", EnrollURL: enrollURL, TokenFile: tokenFile,
				Agent: map[string]schedpolicy.AgentBinary{"amd64": {URL: agentURL, SHA256: agentSHA}}}}}
}

// newRig builds a TEAM rig; tweak may change the policy, the team flag and the engine config before the build.
func newRig(t *testing.T, tweak ...func(*rig)) *rig {
	t.Helper()
	r := &rig{t: t, clk: fakecloud.NewClock(time.Now().Truncate(time.Second)), team: true,
		cfg: burst.Config{MaxLifetime: time.Hour, Grace: 10 * time.Minute, SweepEvery: 5 * time.Minute, RetryWindow: 30 * time.Minute, IdleTimeout: 5 * time.Minute}}
	r.dir = shortDir(t)
	r.home = filepath.Join(t.TempDir(), "ci")
	r.path = filepath.Join(t.TempDir(), "state.db")
	r.tokenFile = filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(r.tokenFile, []byte(fakeToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.enrollAdr = freeAddr(t)
	r.enrollURL = "https://" + r.enrollAdr + "/v1/enroll"
	r.pol = basePolicy(r.tokenFile, r.enrollURL)
	r.cloud = fakecloud.New(r.clk)
	r.cloud.SetID(providerID)
	r.br = newBridge(t, r.dir, r.cloud, fakeToken)
	t.Setenv("PATH", r.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, f := range tweak {
		if f != nil {
			f(r)
		}
	}
	r.st = r.open()
	r.build()
	return r
}

// shortDir is a temp directory whose path is short enough for a unix socket.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "cv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

func freeAddr(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
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

// build (re)creates CA, pairing, provider and engine over the open store: a second call after a "kill" is the restarted coordinator.
func (r *rig) build() {
	r.t.Helper()
	ctx := context.Background()
	ca, err := pki.New(ctx, pki.Options{Store: r.st, DataDir: r.home, Now: r.clk.Now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.ca, r.reg = ca, registry.New(r.st)
	r.pair = pairing.New(pairing.Config{CA: ca, Store: r.st, Registry: r.reg, CoordinatorID: "local", AgentURL: "https://agent.test:3865", Actor: "cli:burst-test",
		Poll: 20 * time.Millisecond, AfterIssue: func(ctx context.Context, id string) error { return r.prov.AfterIssue(ctx, id) }})
	r.prov, err = cloudvm.New(cloudvm.Config{CoordinatorID: coord, ProviderID: providerID, Spend: r.spend, Pairing: r.pair,
		Intents: cloudvm.StoreIntents{Store: r.st, Coordinator: coord}, Lifetime: r.cfg, Home: r.home, Registry: r.reg, Now: r.clk.Now, PluginHome: pluginHome, DestroyWait: -1})
	if err != nil {
		r.t.Fatal(err)
	}
	r.eng, err = burst.New(burst.Options{Store: r.st, Providers: []burst.Provider{r.prov}, Coordinator: coord, Config: r.cfg, Now: r.clk.Now,
		Spend: r.spend, Team: func() bool { return r.team }, Revoker: revoker{r}, Alert: func(a burst.Alert) { r.alerts = append(r.alerts, a) },
		Exclusion: func(context.Context) (deployguard.Set, error) { return deployguard.NewSet(nil), nil },
		Fault: func(p string) error {
			if r.fault != nil {
				return r.fault(p)
			}
			return nil
		}})
	if err != nil {
		r.t.Fatal(err)
	}
	r.prov.SetJoiner(r.eng)
}

func (r *rig) spend(context.Context) (economics.Policy, error) { return r.pol, nil }

// revoker revokes through the real store aggregate with the burst hook and the CA's deny-list hook.
type revoker struct{ r *rig }

func (v revoker) Revoke(ctx context.Context, id string) error {
	_, err := v.r.st.NodeRevoke(ctx, id, "cli:burst-test", []store.RevokeHook{store.BurstRevokeHook(), pki.RevokeHook(v.r.clk.Now)})
	v.r.denied = append(v.r.denied, id)
	var se *store.Error
	if errors.As(err, &se) && se.Code == "E650" {
		return nil
	}
	return err
}

func (r *rig) request(job string) burst.Request {
	return burst.Request{Provider: providerID, Pipeline: "pipe-1", JobKey: job, Spec: burst.Spec{Platform: "linux/amd64", CPU: 2, MemMB: 4096, TimeoutMs: 600_000}, ExpectedWaitMs: 600_000, QueuedDemand: 5}
}

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

// labelled lists this coordinator's labelled servers straight from the fake cloud.
func (r *rig) labelled() []burst.Resource {
	rs, err := r.cloud.List(context.Background(), burst.OwnLabels(coord))
	if err != nil {
		r.t.Fatal(err)
	}
	return rs
}

// addNode registers a node row so a revoke has something to revoke.
func (r *rig) addNode(id string) {
	r.t.Helper()
	b, err := os.ReadFile("../../../model/testdata/capability/valid/laptop.valid.json")
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
