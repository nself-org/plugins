package run

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func withCIHome(t *testing.T, config string) {
	t.Helper()
	home := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := netscopeHome
	netscopeHome = home
	t.Cleanup(func() { netscopeHome = old })
}

// The default source is this host's own set: its never-list reaches the proxy, and an unreadable file makes
// the scope unavailable instead of running with less.
func TestLocalExclusionsComeFromTheHostsOwnSet(t *testing.T) {
	withCIHome(t, "ci:\n  never_hosts:\n    - 5.75.235.42\n    - 167.235.195.0/24\n    - prod.example.com\n")
	ex, err := netscopePlanner.Exclusion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ex.Addresses, []string{"5.75.235.42"}) || !reflect.DeepEqual(ex.Prefixes, []string{"167.235.195.0/24"}) || !reflect.DeepEqual(ex.Hosts, []string{"prod.example.com"}) {
		t.Fatalf("%+v", ex)
	}
	withCIHome(t, "ci: [this is: not valid")
	if _, err := netscopePlanner.Exclusion(context.Background()); err == nil {
		t.Fatal("an unreadable never-list was read as empty")
	}
	withCIHome(t, "")
	if ex, err := netscopePlanner.Exclusion(context.Background()); err != nil || len(ex.Addresses)+len(ex.Prefixes)+len(ex.Hosts) != 0 {
		t.Fatalf("a missing config is an empty source: %+v %v", ex, err)
	}
}

func TestRegisteredExclusionSourceReplacesTheLocalOne(t *testing.T) {
	withCIHome(t, "ci:\n  never_hosts: [5.75.235.42]\n")
	RegisterNetscopeExclusions(func(context.Context) (netscope.Exclusions, error) {
		return netscope.Exclusions{Addresses: []string{"1.2.3.4"}}, nil
	})
	t.Cleanup(func() { RegisterNetscopeExclusions(nil) })
	ex, _ := netscopePlanner.Exclusion(context.Background())
	if !reflect.DeepEqual(ex.Addresses, []string{"1.2.3.4"}) {
		t.Fatalf("%+v", ex)
	}
}

func TestContributorPublishesTheProofForTheLocalRunnerOnly(t *testing.T) {
	old := localProof
	t.Cleanup(func() { localProof = old })
	localProof = &scopeProof{binary: func(context.Context) string { return "" }, scopes: []model.NetworkScope{"none", "restricted"}, at: time.Now()}
	local := sched.Runner{ID: "local"}
	local.Capability.Identity = model.CapabilityIdentity{ID: "local", Provider: "local", Transport: "local", Labels: []string{world.LocalLabel}}
	impostor := sched.Runner{ID: "local"} // a registered node called "local" without the reserved label
	labelled := sched.Runner{ID: "local"} // one that even carries the label, but is an ssh endpoint
	labelled.Capability.Identity = model.CapabilityIdentity{ID: "local", Provider: "operator", Transport: "ssh", Labels: []string{world.LocalLabel}}
	w := sched.World{Runners: []sched.Runner{impostor}}
	if err := contributeNetwork(context.Background(), &w); err != nil || len(w.Network) != 0 {
		t.Fatalf("a runner without the local label got a proof: %+v %v", w.Network, err)
	}
	w = sched.World{Runners: []sched.Runner{labelled}}
	if err := contributeNetwork(context.Background(), &w); err != nil || len(w.Network) != 0 {
		t.Fatalf("a registered ssh node called local got a proof: %+v %v", w.Network, err)
	}
	// no runtime: the stale proof is dropped and nothing is published
	w = sched.World{Runners: []sched.Runner{local}}
	if err := contributeNetwork(context.Background(), &w); err != nil || len(w.Network) != 0 {
		t.Fatalf("a proof outlived its runtime: %+v %v", w.Network, err)
	}
	localProof = &scopeProof{proven: "docker", binary: func(context.Context) string { return "docker" }, scopes: []model.NetworkScope{"none", "restricted"}, at: time.Now()}
	w = sched.World{Runners: []sched.Runner{labelled, local}} // two runners with the id: ambiguous, no proof
	if err := contributeNetwork(context.Background(), &w); err != nil || len(w.Network) != 0 {
		t.Fatalf("an ambiguous local id got a proof: %+v %v", w.Network, err)
	}
	w = sched.World{Runners: []sched.Runner{local}}
	if err := contributeNetwork(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	if got := w.Network["local"].Enforced; !reflect.DeepEqual(got, []model.NetworkScope{"none", "restricted"}) {
		t.Fatalf("%+v", w.Network)
	}
}

// D-1332: a registered node's probe result (capability network.enforced_scopes) reaches World.Network, but only
// while it is fresh, stated by a probe, and from a registered ssh or agent node that is not called "local".
func TestContributorReadsTheProbeResultOfRegisteredNodes(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	node := func(id, transport string, source string, age time.Duration, scopes ...model.NetworkScope) sched.Runner {
		r := sched.Runner{ID: id}
		r.Capability.Identity = model.CapabilityIdentity{ID: id, Provider: transport, Transport: transport}
		f := model.Fact[[]model.NetworkScope]{Source: source, ObservedAt: now.Add(-age), Confidence: "known", Value: &scopes}
		if scopes == nil {
			f.Value, f.Confidence = nil, "unknown"
		}
		r.Capability.Network = &model.CapabilityNetwork{EnforcedScopes: f}
		return r
	}
	w := sched.World{NowMs: now.UnixMilli(), Runners: []sched.Runner{
		node("agent-ok", "agent", "probe", time.Minute, "none", "restricted"),
		node("ssh-ok", "ssh", "probe", time.Hour, "none"),
		node("stale", "agent", "probe", 48*time.Hour, "none", "restricted"),
		node("future", "agent", "probe", -time.Hour, "none", "restricted"),
		node("self-said", "agent", "self", time.Minute, "none", "restricted"),
		node("unknown", "agent", "probe", time.Minute),
		node("hosted", "provider", "probe", time.Minute, "none", "restricted"),
		node("local", "ssh", "probe", time.Minute, "none", "restricted"),
		{ID: "no-section"},
	}}
	if err := contributeNetwork(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Network) != 2 || !reflect.DeepEqual(w.Network["agent-ok"].Enforced, []model.NetworkScope{"none", "restricted"}) || !reflect.DeepEqual(w.Network["ssh-ok"].Enforced, []model.NetworkScope{"none"}) {
		t.Fatalf("%+v", w.Network)
	}
}
