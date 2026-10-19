package netscope

// Purpose: the isolation.AttemptPlanner for network scopes. none and lan need nothing per attempt; restricted
// and internet get a per-attempt internal network with no host address and a dual-homed proxy sidecar, and the
// job gets the proxy variables and no DNS.
// Inputs:  the runtime binary the container profile probed, the attempt and its scope.
// Outputs: an isolation.NetworkPlan whose Release reads the sidecar's denial events and removes the sidecar
//          and the network; E679 when either cannot be created (the scope is then unavailable, never approximated).
// Constraints: nothing is created for a scope this planner does not serve; a failure after the first resource
// removes everything created so far; the deploy exclusion set, when a source is registered, must be readable
// or the scope is unavailable; no environment variable loosens the deny set.

import (
	"context"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Runtime is the container runtime work of one attempt's scope (internal/exec implements it).
type Runtime interface {
	NetworkCreate(ctx context.Context, binary, attempt, coordinator string) (netip.Prefix, error)
	SidecarStart(ctx context.Context, binary string, s exec.SidecarSpec, job netip.Prefix) (netip.Addr, error)
	SidecarLog(ctx context.Context, binary, attempt string) ([]byte, error)
	Remove(ctx context.Context, binary, attempt string) error
	// ProbeRun runs the probe container on the attempt's job network and returns what it printed.
	ProbeRun(ctx context.Context, binary, attempt, image, proxyBinary string, targets []string) ([]byte, error)
}

type execRuntime struct{}

func (execRuntime) NetworkCreate(ctx context.Context, binary, attempt, coordinator string) (netip.Prefix, error) {
	return exec.RuntimeNetworkCreate(ctx, binary, attempt, coordinator)
}
func (execRuntime) SidecarStart(ctx context.Context, binary string, s exec.SidecarSpec, job netip.Prefix) (netip.Addr, error) {
	return exec.RuntimeSidecarStart(ctx, binary, s, job)
}
func (execRuntime) SidecarLog(ctx context.Context, binary, attempt string) ([]byte, error) {
	return exec.RuntimeSidecarLog(ctx, binary, attempt)
}
func (execRuntime) Remove(ctx context.Context, binary, attempt string) error {
	return exec.RuntimeRemoveAttempt(ctx, binary, attempt)
}

func (execRuntime) ProbeRun(ctx context.Context, binary, attempt, image, proxyBinary string, targets []string) ([]byte, error) {
	return exec.RuntimeNetProbeRun(ctx, binary, attempt, image, proxyBinary, targets)
}

// Exclusions are the deploy exclusion entries the planner hands to the proxy (deployguard.ExclusionSet).
type Exclusions struct{ Hosts, Addresses, Prefixes []string }

// Planner implements isolation.AttemptPlanner. Every field is optional.
type Planner struct {
	Runtime Runtime
	// Image is the sidecar image (default ProxyImage); it must be pinned by digest.
	Image string
	// Binary returns the absolute host path of the static nself-ci binary the sidecar runs (default: this process).
	Binary func() (string, error)
	// Host reads the host's addresses and on-link prefixes (default Discover).
	Host func() (Facts, error)
	// Exclusion reads the node-global deploy exclusion set; nil means no source is registered on this process.
	Exclusion func(ctx context.Context) (Exclusions, error)
	// ExtraHosts returns the hosts policy declares for restricted jobs (trust.providers).
	ExtraHosts func(a isolation.Attempt) []string
	// Events receives each denial the sidecar logged (default: one JSON line on stderr).
	Events func(attempt string, e Event)
	// Ready bounds the wait for the sidecar's readiness line (default 15s).
	Ready time.Duration
}

var _ isolation.AttemptPlanner = (*Planner)(nil)

func (p *Planner) rt() Runtime {
	if p.Runtime != nil {
		return p.Runtime
	}
	return execRuntime{}
}

// Plan serves the scopes that need no attempt: none and lan. restricted and internet need PlanAttempt, so a
// caller that has no attempt gets E679 instead of a plan with no proxy.
func (p *Planner) Plan(scope model.NetworkScope) (isolation.NetworkPlan, error) {
	switch scope {
	case "", "none":
		return isolation.NetworkPlan{Network: "none"}, nil
	case "lan":
		return isolation.NetworkPlan{Network: "bridge"}, nil
	}
	return isolation.NetworkPlan{}, isolation.ScopeUnenforceable("network scope %q needs a per-attempt network and proxy", scope)
}

// PlanAttempt creates what the scope needs for the attempt.
func (p *Planner) PlanAttempt(ctx context.Context, binary string, a isolation.Attempt, scope model.NetworkScope) (isolation.NetworkPlan, error) {
	if scope != "restricted" && scope != "internet" {
		return p.Plan(scope)
	}
	plan, err := p.setup(ctx, binary, a, scope)
	if err != nil {
		if plan.Release != nil {
			plan.Release(context.WithoutCancel(ctx))
			plan.Release = nil
		}
		return plan, unenforceable(err)
	}
	return plan, nil
}

// unenforceable turns any setup failure into the E679 the caller reports; a coded isolation error passes.
func unenforceable(err error) error {
	if _, ok := err.(*isolation.Error); ok {
		return err
	}
	return isolation.ScopeUnenforceable("%v", err)
}

func (p *Planner) setup(ctx context.Context, binary string, a isolation.Attempt, scope model.NetworkScope) (isolation.NetworkPlan, error) {
	image := p.Image
	if image == "" {
		image = ProxyImage
	}
	bin, err := p.proxyBinary()
	if err != nil {
		return isolation.NetworkPlan{}, err
	}
	facts, err := p.worldFacts(ctx)
	if err != nil {
		return isolation.NetworkPlan{}, err
	}
	var extra []string
	if p.ExtraHosts != nil && scope == "restricted" {
		extra = p.ExtraHosts(a)
	}
	rt := p.rt()
	plan := isolation.NetworkPlan{Release: p.release(rt, binary, a.ID)}
	prefix, err := rt.NetworkCreate(ctx, binary, a.ID, a.CoordinatorID)
	if err != nil {
		return plan, err
	}
	cfg, err := NewConfig(scope, a.ID, prefix, extra, facts).Encode()
	if err != nil {
		return plan, err
	}
	ip, err := rt.SidecarStart(ctx, binary, exec.SidecarSpec{Attempt: a.ID, Coordinator: a.CoordinatorID, Image: image, ProxyBinary: bin, Config: cfg}, prefix)
	if err != nil {
		return plan, err
	}
	if err := p.waitReady(ctx, rt, binary, a.ID); err != nil {
		return plan, err
	}
	plan.Network = exec.NetworkName(a.ID)
	plan.DNS = []string{"127.0.0.1"} // no name service in the job: a name is resolved only by the proxy, once
	plan.Env = ProxyEnv(ip)
	return plan, nil
}

// ProxyEnv is the proxy variables of a job on the job network, upper and lower case (tools differ).
func ProxyEnv(ip netip.Addr) []string {
	url := "http://" + netip.AddrPortFrom(ip, ProxyPort).String()
	var env []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		env = append(env, k+"="+url)
	}
	return append(env, "NO_PROXY=", "no_proxy=")
}

func (p *Planner) proxyBinary() (string, error) {
	f := p.Binary
	if f == nil {
		if runtime.GOOS != "linux" { // the sidecar runs this process's binary in a Linux container
			return "", scopeFailed("restricted and internet need a Linux host: the proxy sidecar runs this binary in a Linux container")
		}
		f = os.Executable
	}
	bin, err := f()
	if err != nil || bin == "" {
		return "", scopeFailed("the proxy binary path is unknown")
	}
	return bin, nil
}

// worldFacts is the host's own addresses and prefixes plus the node-global exclusion set.
func (p *Planner) worldFacts(ctx context.Context) (Facts, error) {
	host := p.Host
	if host == nil {
		host = Discover
	}
	f, err := host()
	if err != nil {
		return Facts{}, err
	}
	if p.Exclusion == nil {
		return f, nil
	}
	ex, err := p.Exclusion(ctx)
	if err != nil {
		return Facts{}, scopeFailed("the deploy exclusion set cannot be read: %v", err)
	}
	return Merge(f, exclusionFacts(ex)), nil
}

// exclusionFacts turns exclusion entries into Facts; an entry that does not parse is skipped here and the
// proxy still refuses the whole table (the entry came from a validated store).
func exclusionFacts(ex Exclusions) Facts {
	var f Facts
	for _, raw := range ex.Addresses {
		if a, err := netip.ParseAddr(raw); err == nil {
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(Unmap(a), Unmap(a).BitLen()))
		}
	}
	for _, raw := range ex.Prefixes {
		if pr, err := netip.ParsePrefix(raw); err == nil {
			f.Exclusion = append(f.Exclusion, pr)
		}
	}
	for _, h := range ex.Hosts {
		if n := NormalizeHost(h); n != "" {
			f.ExclusionHosts = append(f.ExclusionHosts, n)
		}
	}
	return f
}

func (p *Planner) waitReady(ctx context.Context, rt Runtime, binary, attempt string) error {
	limit := p.Ready
	if limit <= 0 {
		limit = 15 * time.Second
	}
	deadline := time.Now().Add(limit)
	for {
		log, err := rt.SidecarLog(ctx, binary, attempt)
		if err != nil {
			return err
		}
		if HasReady(log) {
			return nil
		}
		if time.Now().After(deadline) {
			line, _, _ := strings.Cut(strings.TrimSpace(string(log)), "\n")
			if len(line) > 200 {
				line = line[:200]
			}
			return scopeFailed("the proxy sidecar did not become ready: %s", line)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// release reads the sidecar's denial events, then removes the sidecar and the network. It is idempotent.
func (p *Planner) release(rt Runtime, binary, attempt string) func(context.Context) {
	var once sync.Once
	return func(ctx context.Context) { once.Do(func() { p.releaseOnce(ctx, rt, binary, attempt) }) }
}

func (p *Planner) releaseOnce(ctx context.Context, rt Runtime, binary, attempt string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	if log, err := rt.SidecarLog(ctx, binary, attempt); err == nil {
		for _, e := range ParseEvents(strings.NewReader(string(log))) {
			p.emit(attempt, e)
		}
	}
	_ = rt.Remove(ctx, binary, attempt)
}

func (p *Planner) emit(attempt string, e Event) {
	e.Attempt = attempt
	if p.Events != nil {
		p.Events(attempt, e)
		return
	}
	_, _ = os.Stderr.Write(e.Encode())
}
