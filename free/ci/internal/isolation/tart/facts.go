package tart

// Purpose: the network facts and the host-side proxy that scopes below lan are enforced with on Tart.
// Inputs:  the host's interfaces (netscope.Discover), the deploy exclusion set the run hook registered, a resolver.
// Outputs: netscope.Facts (on-link prefixes, host addresses, exclusion prefixes and names) and a ProxyStarter
//          that serves netscope's egress proxy on the Tart gateway address.
// Constraints: an exclusion set that exists and cannot be read or parsed refuses the scope (the same rule as the
//          container planner); the proxy listens only on the gateway address, never on 0.0.0.0, so a LAN host
//          cannot use it. The exclusion handling follows netscope's own (see debt: export it from netscope).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

// NetSource is where the profile gets what it needs to enforce a scope. A nil field makes the scopes that need
// it unavailable (E679), never weaker.
type NetSource struct {
	// Facts reads the host's on-link facts and the deploy exclusion set.
	Facts func(ctx context.Context) (netscope.Facts, error)
	// Hosts returns the hosts policy adds to the restricted allowlist (trust.providers).
	Hosts func(a isolation.Attempt) []string
	// Proxy serves the egress proxy for restricted and internet.
	Proxy ProxyStarter
}

// ProxyRequest is what a ProxyStarter is asked for.
type ProxyRequest struct {
	Scope   model.NetworkScope
	Attempt isolation.Attempt
	Gateway netip.Addr
	// VMAddr is the address of the attempt's VM: the proxy serves no other source.
	VMAddr netip.Addr
	Facts  netscope.Facts
	Hosts  []string
}

// ProxyHandle is a started proxy; Close stops it and is safe to call twice.
type ProxyHandle struct {
	Addr  netip.AddrPort
	Close func()
}

// ProxyStarter starts the proxy for one attempt.
type ProxyStarter func(ctx context.Context, req ProxyRequest) (ProxyHandle, error)

const exclusionLookupTimeout = 10 * time.Second

// BuildFacts returns a NetSource.Facts: the host facts merged with the exclusion set. excl may be nil (no
// exclusion source registered on this process); res defaults to net.DefaultResolver.
func BuildFacts(host func() (netscope.Facts, error), excl func(context.Context) (netscope.Exclusions, error), res netscope.Resolver) func(context.Context) (netscope.Facts, error) {
	if res == nil {
		res = net.DefaultResolver
	}
	return func(ctx context.Context) (netscope.Facts, error) {
		f, err := host()
		if err != nil {
			return netscope.Facts{}, err
		}
		if excl == nil {
			return f, nil
		}
		ex, err := excl(ctx)
		if err != nil {
			return netscope.Facts{}, isolation.ScopeUnenforceable("the deploy exclusion set cannot be read: %v", err)
		}
		xf, err := exclusionFacts(ctx, ex, res)
		if err != nil {
			return netscope.Facts{}, err
		}
		return netscope.Merge(f, xf), nil
	}
}

func exclusionFacts(ctx context.Context, ex netscope.Exclusions, res netscope.Resolver) (netscope.Facts, error) {
	var f netscope.Facts
	for _, raw := range ex.Addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil {
			return f, isolation.ScopeUnenforceable("exclusion address %q does not parse", raw)
		}
		a = netscope.Unmap(a)
		f.Exclusion = append(f.Exclusion, netip.PrefixFrom(a, a.BitLen()))
	}
	for _, raw := range ex.Prefixes {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return f, isolation.ScopeUnenforceable("exclusion prefix %q does not parse", raw)
		}
		f.Exclusion = append(f.Exclusion, p)
	}
	var names []string
	for _, h := range ex.Hosts {
		n := netscope.NormalizeHost(h)
		if n == "" {
			return f, isolation.ScopeUnenforceable("exclusion host %q does not parse", h)
		}
		f.ExclusionHosts = append(f.ExclusionHosts, n)
		if a, err := netip.ParseAddr(strings.Trim(n, "[]")); err == nil {
			a = netscope.Unmap(a)
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(a, a.BitLen()))
		} else {
			names = append(names, n)
		}
	}
	lctx, cancel := context.WithTimeout(ctx, exclusionLookupTimeout)
	defer cancel()
	type answer struct {
		addrs []netip.Addr
		err   error
	}
	answers := make([]answer, len(names))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i].addrs, answers[i].err = res.LookupNetIP(lctx, "ip", n)
		}()
	}
	wg.Wait()
	for i, a := range answers {
		var dnsErr *net.DNSError
		switch {
		case a.err == nil:
		case errors.As(a.err, &dnsErr) && dnsErr.IsNotFound:
			continue
		default:
			return f, isolation.ScopeUnenforceable("excluded host %s cannot be resolved: %v", names[i], a.err)
		}
		for _, addr := range a.addrs {
			addr = netscope.Unmap(addr)
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return f, nil
}

// HostProxy returns the ProxyStarter that serves netscope's proxy in this process on the gateway address. The
// gateway must be an address of this host (it exists once the VM is up) or the scope is unenforceable.
func HostProxy(res netscope.Resolver, discover func() (netscope.Facts, error), events io.Writer) ProxyStarter {
	if res == nil {
		res = net.DefaultResolver
	}
	if discover == nil {
		discover = netscope.Discover
	}
	return func(ctx context.Context, req ProxyRequest) (ProxyHandle, error) {
		own, err := discover()
		if err != nil {
			return ProxyHandle{}, err
		}
		if !hostHas(own, req.Gateway) {
			return ProxyHandle{}, isolation.ScopeUnenforceable("the Tart gateway %s is not an address of this host", req.Gateway)
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(req.Gateway.String(), "0"))
		if err != nil {
			return ProxyHandle{}, isolation.ScopeUnenforceable("proxy listen on %s: %v", req.Gateway, err)
		}
		addr, err := netip.ParseAddrPort(ln.Addr().String())
		if err != nil {
			_ = ln.Close()
			return ProxyHandle{}, fmt.Errorf("proxy address: %w", err)
		}
		var serve net.Listener = ln
		if req.VMAddr.IsValid() {
			serve = &sourceFilter{Listener: ln, allow: req.VMAddr}
		}
		p := &netscope.Proxy{
			Guard:  netscope.Guard{Scope: req.Scope, Facts: netscope.Merge(req.Facts, own), ExtraHosts: req.Hosts, Resolver: res},
			Dialer: &net.Dialer{Timeout: 15 * time.Second},
			Events: events,
		}
		pctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		done := make(chan struct{})
		go func() { defer close(done); _ = p.Serve(pctx, serve) }()
		var once sync.Once
		return ProxyHandle{Addr: addr, Close: func() {
			once.Do(func() { cancel(); _ = ln.Close(); <-done })
		}}, nil
	}
}

func hostHas(f netscope.Facts, a netip.Addr) bool {
	for _, h := range f.HostAddrs {
		if h == a {
			return true
		}
	}
	return false
}

// sourceFilter accepts only connections from one address. All VMs share the gateway address, so without it a
// second VM (under a narrower scope) could use this attempt's proxy; softnet pins a VM to its own source address.
type sourceFilter struct {
	net.Listener
	allow netip.Addr
}

func (s *sourceFilter) Accept() (net.Conn, error) {
	for {
		c, err := s.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if ap, perr := netip.ParseAddrPort(c.RemoteAddr().String()); perr == nil && netscope.Unmap(ap.Addr()) == s.allow {
			return c, nil
		}
		_ = c.Close()
	}
}
