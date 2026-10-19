package run

// Purpose: install the network scope planner and publish the scopes this node proved (P7-TRUST-08, Epic D11).
// Inputs:  the container profile's probed runtime; the deploy exclusion set this host can build for itself
//          (deployguard.LocalSet over the CI home: the cached entries of its state.db and the never-list), or a
//          source the serve wiring registers instead; optional policy hosts.
// Outputs: the isolation.AttemptPlanner every profile built with the default planner uses; the world
//          contributor that sets sched.World.Network["local"] from the probe, so SCHED derives NoLANRoute.
// Constraints: an exclusion set that exists and cannot be read makes restricted and internet unavailable (E679)
//          rather than run with less than the operator wrote down; the probe runs in the background and never blocks a world build; until it has a result the
// fact is absent (never guessed), so an untrusted job is not admitted on the strength of it; a node without a
// container runtime publishes nothing.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

var netscopeSources = struct {
	sync.RWMutex
	exclusion func(context.Context) (netscope.Exclusions, error)
	hosts     func(isolation.Attempt) []string
}{}

// RegisterNetscopeExclusions replaces where the deploy exclusion set is read from (a coordinator may wire
// deployguard.Guard.ExclusionSet here). The default is the host's own set (localExclusions).
func RegisterNetscopeExclusions(f func(context.Context) (netscope.Exclusions, error)) {
	netscopeSources.Lock()
	netscopeSources.exclusion = f
	netscopeSources.Unlock()
}

// RegisterNetscopeHosts sets the hosts policy adds to the restricted allowlist (trust.providers).
func RegisterNetscopeHosts(f func(isolation.Attempt) []string) {
	netscopeSources.Lock()
	netscopeSources.hosts = f
	netscopeSources.Unlock()
}

var netscopePlanner = &netscope.Planner{
	Exclusion: func(ctx context.Context) (netscope.Exclusions, error) {
		netscopeSources.RLock()
		f := netscopeSources.exclusion
		netscopeSources.RUnlock()
		if f == nil {
			return localExclusions(ctx)
		}
		return f(ctx)
	},
	ExtraHosts: func(a isolation.Attempt) []string {
		netscopeSources.RLock()
		f := netscopeSources.hosts
		netscopeSources.RUnlock()
		if f == nil {
			return nil
		}
		return f(a)
	},
}

// localExclusions is the exclusion set this host can build for itself; netscopeHome is the CI home ("" means
// $NSELF_CI_HOME or ~/.nself/ci). A missing state.db or config is an empty source; an unreadable one is an error.
func localExclusions(ctx context.Context) (netscope.Exclusions, error) {
	set, err := deployguard.LocalSet(ctx, netscopeHome)
	if err != nil {
		return netscope.Exclusions{}, err
	}
	v := set.View()
	return netscope.Exclusions{Hosts: v.Hosts, Addresses: v.Addresses, Prefixes: v.Prefixes}, nil
}

// netscopeHome is the CI home the local exclusion set reads; tests replace it.
var netscopeHome = ""

// scopeProof caches the background probe of this node.
type scopeProof struct {
	mu      sync.Mutex
	prober  *netscope.Prober
	scopes  []model.NetworkScope
	at      time.Time
	running bool
	binary  func(context.Context) string
}

const scopeProofTTL = 5 * time.Minute

func (p *scopeProof) get(ctx context.Context) []model.NetworkScope {
	binary := p.binary(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if binary == "" { // the runtime went away: an earlier proof no longer holds
		p.scopes, p.at = nil, time.Time{}
		return nil
	}
	if !p.running && (p.at.IsZero() || time.Since(p.at) > scopeProofTTL) {
		p.running = true
		go p.refresh(binary)
	}
	return append([]model.NetworkScope(nil), p.scopes...)
}

func (p *scopeProof) refresh(binary string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	scopes := p.prober.EnforcedScopes(ctx, binary)
	p.mu.Lock()
	p.scopes, p.at, p.running = scopes, time.Now(), false
	p.mu.Unlock()
}

// containerRuntime is the runtime binary the container profile probed ("" when it is unavailable).
func containerRuntime(ctx context.Context) string {
	for _, pr := range isolation.Default.Profiles() {
		if pr.Level() == "container" {
			return pr.Probe(ctx).Facts["runtime"]
		}
	}
	return ""
}

var localProof = &scopeProof{prober: &netscope.Prober{Planner: netscopePlanner}, binary: containerRuntime}

func contributeNetwork(ctx context.Context, w *sched.World) error {
	for _, r := range w.Runners {
		if r.ID != "local" || !hasLabel(r.Capability.Identity.Labels, world.LocalLabel) {
			continue // the proof is about this machine; a registered node that is called "local" is not it
		}
		if scopes := localProof.get(ctx); len(scopes) > 0 {
			w.Network = map[string]sched.NetworkFact{"local": {Enforced: scopes}}
		}
	}
	return nil
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func init() {
	isolation.InstallNetworkPlanner(netscopePlanner)
	if err := world.Register(world.Contributor{Name: "netscope", Field: "Network", Fn: contributeNetwork}); err != nil {
		panic("netscope: " + err.Error())
	}
}
