package netscope

// Purpose: the host facts and the deploy exclusion set a scope is set up with.
// Inputs:  the host's interfaces and routes (Discover), the exclusion entries a source supplies and a resolver.
// Outputs: Facts for the sidecar's Config; E679 when the exclusion set cannot be built.
// Constraints: an entry that does not parse refuses the scope; each excluded NAME is resolved once, with every
// name looked up at the same time under one deadline, and its addresses join the exclusion.

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// worldFacts is the host's own addresses and prefixes plus the node-global exclusion set.
// exclusionLookupTimeout bounds all excluded-name lookups of one setup together.
const exclusionLookupTimeout = 10 * time.Second

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
	xf, err := p.exclusionFacts(ctx, ex)
	if err != nil {
		return Facts{}, err
	}
	return Merge(f, xf), nil
}

// exclusionFacts turns exclusion entries into Facts. An entry that does not parse refuses the scope (a deploy
// host is never left out because its line was malformed). Each excluded NAME is also resolved here, once, and
// its addresses join the exclusion: a name-only entry would otherwise protect nothing from a job that dials
// the address (or another name for it). A name that does not exist protects nothing and is skipped; any other
// lookup failure refuses the scope.
func (p *Planner) exclusionFacts(ctx context.Context, ex Exclusions) (Facts, error) {
	var f Facts
	for _, raw := range ex.Addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil {
			return Facts{}, scopeFailed("exclusion address %q does not parse", raw)
		}
		f.Exclusion = append(f.Exclusion, netip.PrefixFrom(Unmap(a), Unmap(a).BitLen()))
	}
	for _, raw := range ex.Prefixes {
		pr, err := netip.ParsePrefix(raw)
		if err != nil {
			return Facts{}, scopeFailed("exclusion prefix %q does not parse", raw)
		}
		f.Exclusion = append(f.Exclusion, pr)
	}
	res := p.Resolver
	if res == nil {
		res = net.DefaultResolver
	}
	var names []string
	for _, h := range ex.Hosts {
		n := NormalizeHost(h)
		if n == "" {
			return Facts{}, scopeFailed("exclusion host %q does not parse", h)
		}
		f.ExclusionHosts = append(f.ExclusionHosts, n)
		if a, err := netip.ParseAddr(strings.Trim(n, "[]")); err == nil { // an address in the host list is an excluded address
			a = Unmap(a)
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(a, a.BitLen()))
		} else {
			names = append(names, n)
		}
	}
	// every name is looked up at the same time under one deadline, so a long never-list adds one round trip
	// (not one per name) to each attempt
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
			return Facts{}, scopeFailed("excluded host %s cannot be resolved: %v", names[i], a.err)
		}
		for _, addr := range a.addrs {
			addr = Unmap(addr)
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return f, nil
}
