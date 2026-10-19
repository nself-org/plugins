package deployguard

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type rig struct {
	t      *testing.T
	st     *store.Store
	g      *Guard
	run    *FixtureRunner
	home   string
	rootA  string
	rootB  string
	lookup map[string][]netip.Addr
	ssh    map[string]remote.Resolved
	audits []store.AuditRow
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := &rig{t: t, st: st, home: t.TempDir(), rootA: t.TempDir(), rootB: t.TempDir(), lookup: map[string][]netip.Addr{}, ssh: map[string]remote.Resolved{}}
	st.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		r.audits = append(r.audits, row)
		return store.AuditChain{Head: []byte("h")}, nil
	})
	r.run = &FixtureRunner{HelpSide: "read", HostsByRoot: map[string][]string{r.rootA: {FixtureHostV4}, r.rootB: {FixtureHostName, "192.0.2.9"}}}
	r.g = &Guard{Store: st, Reader: Reader{Run: r.run.Run}, Home: r.home, Actor: "cli:test",
		Resolver: Resolver{
			SSH: func(_ context.Context, a string) (remote.Resolved, error) {
				if v, ok := r.ssh[Canon(a)]; ok {
					return v, nil
				}
				return remote.Resolved{}, errors.New("no such host")
			},
			Lookup: func(_ context.Context, h string) ([]netip.Addr, error) {
				if v, ok := r.lookup[h]; ok {
					return v, nil
				}
				return nil, errors.New("nxdomain")
			}}}
	return r
}

func (r *rig) writeConfig(text string) {
	r.t.Helper()
	if err := os.WriteFile(ConfigPath(r.home), []byte(text), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) capability(id string, mutate func(*model.Capability)) *model.Capability {
	r.t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.SSH, c.Identity.HostKeyFingerprints = nil, nil
	c.Identity.MachineID = model.Fact[string]{Source: "self", ObservedAt: time.Now().UTC(), Confidence: "unknown"}
	if mutate != nil {
		mutate(&c)
	}
	return &c
}

func (r *rig) addNode(id string, mutate func(*model.Capability)) {
	r.t.Helper()
	doc, _ := json.Marshal(r.capability(id, mutate))
	n := store.NodeRecord{ID: id, Name: id, State: "offline", Capability: doc, Lifecycle: []byte(`{}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err := r.st.NodePut(context.Background(), n, "operator"); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) deployHost(id string) model.CapabilityDeployHost {
	r.t.Helper()
	n, err := r.st.NodeGet(context.Background(), id)
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(n.Capability, &c); err != nil {
		r.t.Fatal(err)
	}
	var col model.CapabilityDeployHost
	if err = json.Unmarshal(n.DeployHost, &col); err != nil || col.Matched != c.DeployHost.Matched || col.Override != c.DeployHost.Override {
		r.t.Fatalf("column %s and document %+v disagree", n.DeployHost, c.DeployHost)
	}
	return c.DeployHost
}

func ssh(host string) func(*model.Capability) {
	return func(c *model.Capability) {
		c.Identity.SSH = &model.CapabilitySSH{Alias: host, Hostname: host, Port: 22, User: "ci"}
	}
}

func TestRefreshIsNodeGlobalUnionOfEveryProjectAndTheNeverList(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.writeConfig("ci:\n  never_hosts:\n    - 198.51.100.0/24\n    - Never.Example.Test\n")
	// A node added from project A (no deploy config there) sits on project B's deploy host.
	r.addNode("n-b", ssh(FixtureHostName))
	r.addNode("n-never", ssh("never.example.test."))
	r.addNode("n-net", ssh("198.51.100.5"))
	r.addNode("n-ok", ssh(FixtureOther))
	for _, root := range []string{r.rootA, r.rootB} {
		if err := r.g.Seen(ctx, root); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"n-b": SourceInventory, "n-never": SourceNeverList, "n-net": SourceNeverList, "n-ok": ""} {
		dh := r.deployHost(id)
		got := ""
		if dh.Source != nil {
			got = *dh.Source
		}
		if dh.Matched != (want != "") || got != want || dh.Override {
			t.Errorf("%s: %+v", id, dh)
		}
	}
	v, err := r.g.ExclusionSet(ctx)
	if err != nil || len(v.Addresses) != 2 || len(v.Prefixes) != 1 {
		t.Fatalf("export %+v %v", v, err)
	}
	// Dispatch reads the cache: no further nself call is made by Set.
	calls := len(r.run.Calls)
	if _, err = r.g.Set(ctx); err != nil || len(r.run.Calls) != calls {
		t.Fatal("Set ran a process")
	}
}

func TestFailedRefreshKeepsEntriesAndRaisesAWarning(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.addNode("n-a", ssh(FixtureHostV4))
	_ = r.g.Seen(ctx, r.rootA)
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !r.deployHost("n-a").Matched || len(r.g.Warnings(ctx)) != 0 {
		t.Fatal("setup")
	}
	r.run.HostsByRoot = map[string][]string{} // the project's nself now fails
	err := r.g.Refresh(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("refresh error = %v", err)
	}
	if !r.deployHost("n-a").Matched {
		t.Fatal("the set shrank on a failed refresh")
	}
	w := r.g.Warnings(ctx)
	if len(w) != 1 || !strings.Contains(w[0], r.rootA) || !strings.Contains(w[0], "using entries cached") {
		t.Fatalf("warnings %v", w)
	}
	// A project that really dropped the host clears the node on the next good read.
	r.run.HostsByRoot = map[string][]string{r.rootA: {FixtureOther}}
	if err = r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if r.deployHost("n-a").Matched || len(r.g.Warnings(ctx)) != 0 {
		t.Fatal("a good read did not clear the match")
	}
	// A root that vanished from disk fails without touching its entries.
	if err = os.RemoveAll(r.rootA); err != nil {
		t.Fatal(err)
	}
	if err = r.g.RefreshProject(ctx, r.rootA); ReasonOf(err) != ReasonInventory {
		t.Fatalf("missing root: %v", err)
	}
}

func TestBrokenNeverListKeepsTheCachedList(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.writeConfig("ci:\n  never_hosts: [203.0.113.77]\n")
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	r.writeConfig("ci: [unterminated")
	if err := r.g.Refresh(ctx); err == nil {
		t.Fatal("broken config not reported")
	}
	s, _ := r.g.Set(ctx)
	if !s.MatchHost("203.0.113.77").Matched {
		t.Fatal("never-list dropped on a parse error")
	}
	if w := r.g.Warnings(ctx); len(w) != 1 || !strings.Contains(w[0], NeverListRoot) {
		t.Fatalf("warnings %v", w)
	}
}

func TestIdentityReachesTheHostThroughEveryDoor(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	r.ssh["ci-deploy-box"] = remote.Resolved{Hostname: FixtureHostV4, ProxyJump: "ci@" + FixtureJumpHost + ":22", HostKeyAlias: "box.example.test"}
	r.ssh["via-jump"] = remote.Resolved{Hostname: "elsewhere.example.test", ProxyJump: "ci@" + FixtureHostV4 + ",other.example.test"}
	r.ssh["by-alias-only"] = remote.Resolved{Hostname: "alias.example.test", HostKeyAlias: FixtureHostV4}
	r.lookup["lb.example.test"] = []netip.Addr{netip.MustParseAddr("::ffff:" + FixtureHostV4)}
	cases := map[string]Identity{
		"ip":            {Dest: "ci@" + FixtureHostV4},
		"ssh alias":     {Dest: "ci-deploy-box"},
		"proxy jump":    {Dest: "via-jump"},
		"hostkey alias": {Dest: "by-alias-only"},
		"dns alias":     {Dest: "lb.example.test"},
		"name only":     {Names: []string{"LB.example.test."}},
		"fingerprint":   {Names: []string{"unrelated"}, Fingerprints: []string{FixtureFingerprint}},
	}
	for name, id := range cases {
		res, _, err := r.g.Match(ctx, id)
		if err != nil || !res.Matched || res.Source != SourceInventory {
			t.Errorf("%s: %+v %v", name, res, err)
		}
	}
	// A pull agent reports the host-key fingerprint (listed in the fixture inventory) and a machine id
	// that a never-list entry names.
	r.writeConfig("ci:\n  never_hosts:\n    - machine-id:" + FixtureMachineID + "\n")
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]Identity{
		"agent fingerprint": {Names: []string{"pull-node"}, Fingerprints: []string{FixtureFingerprint}},
		"agent machine id":  {Names: []string{"pull-node"}, MachineID: strings.ToUpper(FixtureMachineID)},
	} {
		res, _, _ := r.g.Match(ctx, id)
		if !res.Matched {
			t.Errorf("%s did not match", name)
		}
	}
	if res, _, _ := r.g.Match(ctx, Identity{Dest: "ssh-gone", Names: []string{"quiet.example.test"}, Fingerprints: []string{"SHA256:" + strings.Repeat("B", 43)}}); res.Matched {
		t.Error("an unknown host matched")
	}
}

func TestLookupFailureNeverFailsTheMatchAndIsAWarning(t *testing.T) {
	r := newRig(t)
	f, warn := r.g.Resolver.Facts(context.Background(), Identity{Dest: "ghost", Names: []string{"a.example.test"}})
	if len(f.Names) != 2 || len(warn) != 2 {
		t.Fatalf("facts %+v warn %v", f, warn)
	}
	var none Resolver
	if f2, w2 := none.Facts(context.Background(), Identity{Dest: "ci@" + FixtureHostV4}); len(f2.Addrs) != 1 || len(w2) != 0 {
		t.Fatalf("zero resolver: %+v %v", f2, w2)
	}
}

func TestOverrideSurvivesWhileMatchedAndClearsWithIt(t *testing.T) {
	src := SourceInventory
	prev := model.CapabilityDeployHost{Matched: true, Source: &src, Override: true}
	if got := DeployHostFor(Result{Matched: true, Source: SourceNeverList}, prev); !got.Override || *got.Source != SourceNeverList {
		t.Errorf("kept override: %+v", got)
	}
	if got := DeployHostFor(Result{}, prev); got.Matched || got.Override || got.Source != nil {
		t.Errorf("cleared: %+v", got)
	}
}

func TestRunRefreshesNowAndStopsWithItsContext(t *testing.T) {
	r := newRig(t)
	_ = r.g.Seen(context.Background(), r.rootA)
	ctx, cancel := context.WithCancel(context.Background())
	var errs []error
	done := make(chan struct{})
	go func() { r.g.Run(ctx, func(e error) { errs = append(errs, e) }); close(done) }()
	deadline := time.After(5 * time.Second)
	for {
		if s, _ := r.g.Set(context.Background()); s.MatchHost(FixtureHostV4).Matched {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no refresh at start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	for _, e := range errs { // the cancel may land inside the first refresh
		if !errors.Is(e, context.Canceled) && !strings.Contains(e.Error(), "context canceled") {
			t.Fatalf("errors %v", errs)
		}
	}
}

func TestProjectRootsFromConfigJoinTheSet(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.writeConfig("ci:\n  project_roots:\n    - " + r.rootB + "\n    - relative/ignored\nci.never_hosts: [flat.example.test]\n")
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	s, _ := r.g.Set(ctx)
	if !s.MatchHost(FixtureHostName).Matched || !s.MatchHost("flat.example.test").Matched || s.MatchHost(FixtureHostV4).Matched {
		t.Fatalf("set %+v", s.View())
	}
}
