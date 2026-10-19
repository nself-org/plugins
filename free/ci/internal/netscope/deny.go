// Package netscope enforces the network scopes of a CI job without root (Epic D11).
package netscope

// Purpose: the deny decision. One pure function answers "may a job under this scope reach this address".
// Inputs:  a scope, a destination address in any spelling, and the Facts of the world (on-link prefixes,
//          the host's own addresses, the deploy exclusion set).
// Outputs: a Verdict with a stable reason; nothing is resolved, dialled or read from the environment here.
// Constraints: the address is unmapped (IPv4-mapped IPv6 becomes IPv4, zone dropped) before every check; the
// table denies whole ranges, so the mapped, NAT64, 6to4 and Teredo forms of a denied IPv4 address are denied
// with it; no environment variable loosens it (the SDK guard's NSELF_ALLOW_PRIVATE_URLS has no counterpart).

import (
	"net/netip"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Reason is why an address was refused. The values are stable event and doc vocabulary.
type Reason string

// The refusal reasons.
const (
	ReasonSpecial    Reason = "special_purpose"
	ReasonOnLink     Reason = "on_link"
	ReasonHostAddr   Reason = "host_address"
	ReasonExclusion  Reason = "deploy_exclusion"
	ReasonScope      Reason = "scope"
	ReasonInvalid    Reason = "invalid_address"
	ReasonNotAllowed Reason = "host_not_allowed"
	ReasonResolve    Reason = "resolve_failed"
	// ReasonDialFailed is an allowed destination the sidecar could not reach (an audit trail for diagnosis).
	ReasonDialFailed Reason = "dial_failed"
	// ReasonEventsDropped marks the last event of a sidecar that hit its event cap.
	ReasonEventsDropped Reason = "events_dropped"
)

// denyCIDRs is the special-purpose table: the IANA IPv4 and IPv6 special-purpose registries, every range the
// SDK's httpx.ValidateOutboundURL list holds (a verify line asserts that coverage, because the ci module does
// not require the SDK), and the transition ranges that embed an IPv4 address.
var denyCIDRs = []string{
	// IPv4
	"0.0.0.0/8",          // "this" network
	"10.0.0.0/8",         // RFC1918
	"100.64.0.0/10",      // CGNAT
	"127.0.0.0/8",        // loopback
	"169.254.0.0/16",     // link-local, cloud metadata (169.254.169.254)
	"172.16.0.0/12",      // RFC1918
	"192.0.0.0/24",       // IETF protocol assignments
	"192.0.2.0/24",       // TEST-NET-1
	"192.88.99.0/24",     // deprecated 6to4 relay anycast
	"192.168.0.0/16",     // RFC1918
	"198.18.0.0/15",      // benchmarking
	"198.51.100.0/24",    // TEST-NET-2
	"203.0.113.0/24",     // TEST-NET-3
	"224.0.0.0/4",        // multicast
	"240.0.0.0/4",        // reserved
	"255.255.255.255/32", // broadcast
	// IPv6
	"::/8",           // unspecified, loopback, IPv4-compatible and IPv4-mapped (::ffff:0:0/96) live here
	"::1/128",        // loopback
	"::ffff:0:0/96",  // IPv4-mapped
	"64:ff9b::/96",   // NAT64
	"64:ff9b:1::/48", // local-use NAT64
	"100::/64",       // discard-only
	"2001::/23",      // IETF protocol assignments (Teredo 2001::/32, ORCHID, benchmarking)
	"2001:db8::/32",  // documentation
	"2002::/16",      // 6to4
	"3fff::/20",      // documentation (RFC 9637)
	"5f00::/16",      // SRv6 SIDs
	"fc00::/7",       // unique local
	"fe80::/10",      // link-local
	"fec0::/10",      // deprecated site-local
	"ff00::/8",       // multicast
}

var denyPrefixes = mustPrefixes(denyCIDRs)

func mustPrefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// DenyTable returns the special-purpose CIDRs as written (for docs and the coverage test).
func DenyTable() []string { return append([]string(nil), denyCIDRs...) }

// Facts is what the world adds to the static table. Every list is optional; an empty Facts still denies the table.
type Facts struct {
	// OnLink are the prefixes configured on the interfaces of the host and of the proxy, v4 and v6 including
	// global unicast ones (a /64 GUA LAN is as private as a RFC1918 one).
	OnLink []netip.Prefix
	// HostAddrs are the addresses the host itself answers on.
	HostAddrs []netip.Addr
	// Exclusion are the deploy exclusion addresses and prefixes (deployguard.ExclusionSet).
	Exclusion []netip.Prefix
	// ExclusionHosts are the deploy exclusion host names: a name equal to one of them, or below one, is refused
	// before it is resolved (its addresses are refused as well, through Exclusion).
	ExclusionHosts []string
}

// Verdict is the answer of Allowed.
type Verdict struct {
	Allowed bool
	Reason  Reason
	// Detail names the table entry or prefix that matched ("" when allowed).
	Detail string
}

// Unmap returns addr in the one spelling every check uses: no zone, IPv4-mapped IPv6 as IPv4.
func Unmap(addr netip.Addr) netip.Addr { return addr.WithZone("").Unmap() }

// Allowed decides whether a job under scope may reach addr. Only restricted and internet reach a proxy;
// none has no route and lan and privileged are never served by it, so those four answers are all "no".
func Allowed(scope model.NetworkScope, addr netip.Addr, f Facts) Verdict {
	switch scope {
	case "restricted", "internet":
	default:
		return Verdict{Reason: ReasonScope, Detail: string(scope)}
	}
	if !addr.IsValid() {
		return Verdict{Reason: ReasonInvalid}
	}
	a := Unmap(addr)
	if a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() {
		return Verdict{Reason: ReasonSpecial, Detail: a.String()}
	}
	for _, p := range denyPrefixes {
		if p.Contains(a) {
			return Verdict{Reason: ReasonSpecial, Detail: p.String()}
		}
	}
	for _, h := range f.HostAddrs {
		if Unmap(h) == a {
			return Verdict{Reason: ReasonHostAddr, Detail: a.String()}
		}
	}
	if p, ok := firstContaining(f.Exclusion, a); ok {
		return Verdict{Reason: ReasonExclusion, Detail: p.String()}
	}
	if p, ok := firstContaining(f.OnLink, a); ok {
		return Verdict{Reason: ReasonOnLink, Detail: p.String()}
	}
	return Verdict{Allowed: true}
}

// firstContaining returns the first prefix of list that holds a. A prefix is compared in the unmapped
// spelling too, so a ::ffff:10.0.0.0/104 entry still matches 10.0.0.1.
func firstContaining(list []netip.Prefix, a netip.Addr) (netip.Prefix, bool) {
	for _, p := range list {
		if !p.IsValid() {
			continue
		}
		if p.Contains(a) {
			return p, true
		}
		if a.Is4() && p.Addr().Is6() && p.Bits() >= 96 {
			if m := netip.PrefixFrom(Unmap(p.Addr()), p.Bits()-96); m.Contains(a) {
				return p, true
			}
		}
	}
	return netip.Prefix{}, false
}
