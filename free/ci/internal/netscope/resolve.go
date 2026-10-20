package netscope

// Purpose: turn a requested host into the addresses that may be dialled: parse or resolve once, unmap,
// check every address, return them pinned.
// Inputs:  a scope, a host (name or literal), the world Facts, a Resolver.
// Outputs: the addresses the proxy may dial, or the Verdict that refused the host.
// Constraints: a name is resolved exactly once per request and the caller dials the returned addresses, never
// the name (no second resolution between check and dial: DNS rebinding); one denied address in the answer
// refuses the whole host, so a split answer cannot smuggle a private address in beside a public one.

import (
	"context"
	"net"
	"net/netip"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Resolver looks a name up once. *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Dialer opens a connection to a literal address. *net.Dialer satisfies it.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Guard decides destinations for one attempt.
type Guard struct {
	Scope      model.NetworkScope
	Facts      Facts
	ExtraHosts []string
	Resolver   Resolver
}

// Pin returns the addresses of host that the scope allows, or the refusing Verdict. host has no port.
func (g Guard) Pin(ctx context.Context, host string) ([]netip.Addr, Verdict) {
	h := NormalizeHost(strings.Trim(host, "[]"))
	if h == "" {
		return nil, Verdict{Reason: ReasonInvalid}
	}
	if hostExcluded(g.Facts.ExclusionHosts, h) {
		return nil, Verdict{Reason: ReasonExclusion, Detail: h}
	}
	if !HostAllowed(g.Scope, h, g.ExtraHosts) {
		// a literal under restricted lands here too: the name policy has no address entries
		return nil, Verdict{Reason: ReasonNotAllowed, Detail: h}
	}
	if a, err := netip.ParseAddr(h); err == nil {
		if v := Allowed(g.Scope, a, g.Facts); !v.Allowed {
			return nil, v
		}
		return []netip.Addr{Unmap(a)}, Verdict{Allowed: true}
	}
	if g.Resolver == nil {
		return nil, Verdict{Reason: ReasonResolve, Detail: "no resolver"}
	}
	addrs, err := g.Resolver.LookupNetIP(ctx, "ip", h)
	if err != nil || len(addrs) == 0 {
		detail := h
		if err != nil {
			detail += ": " + err.Error()
		}
		return nil, Verdict{Reason: ReasonResolve, Detail: detail}
	}
	pinned := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if v := Allowed(g.Scope, a, g.Facts); !v.Allowed {
			v.Detail = h + " -> " + a.String() + " " + v.Detail
			return nil, v
		}
		pinned = append(pinned, Unmap(a))
	}
	return pinned, Verdict{Allowed: true}
}

// dialPinned tries the pinned addresses in order and re-checks the address it actually connected to.
func (g Guard) dialPinned(ctx context.Context, d Dialer, addrs []netip.Addr, port string) (net.Conn, error) {
	var last error
	for _, a := range addrs {
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(a.String(), port))
		if err != nil {
			last = err
			continue
		}
		if ta, ok := c.RemoteAddr().(*net.TCPAddr); ok {
			if got, ok := netip.AddrFromSlice(ta.IP); ok && !Allowed(g.Scope, got, g.Facts).Allowed {
				_ = c.Close()
				last = &Error{Code: CodeScopeFailed, Message: "connected address left the allowed set"}
				continue
			}
		}
		return c, nil
	}
	if last == nil {
		last = &Error{Code: CodeScopeFailed, Message: "no address to dial"}
	}
	return nil, last
}

// hostExcluded reports whether h is one of the excluded names or below one.
func hostExcluded(list []string, h string) bool {
	for _, x := range list {
		x = strings.TrimLeft(x, ".") // ".corp.internal" means the domain and everything below it
		if x != "" && (h == x || strings.HasSuffix(h, "."+x)) {
			return true
		}
	}
	return false
}
