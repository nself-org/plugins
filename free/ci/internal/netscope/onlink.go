package netscope

// Purpose: discover, per attempt, the addresses and prefixes the host (or the sidecar) is directly connected to.
// Inputs:  the interface table of the machine it runs on, plus the kernel route table where one can be read.
// Outputs: Facts.OnLink (interface prefixes and non-default route prefixes, v4 and v6 incl. global unicast) and
//          Facts.HostAddrs (the machine's own addresses).
// Constraints: read-only and rootless; a table that cannot be read is an error, never an empty answer, so a
// scope is not proven on a guess; the default route (a /0) is not a LAN prefix and is skipped.

import (
	"net"
	"net/netip"
	"sort"
)

// Discover reads the interfaces of this machine and the routes the platform exposes.
func Discover() (Facts, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return Facts{}, &Error{Code: CodeScopeFailed, Message: "cannot read network interfaces: " + err.Error()}
	}
	var f Facts
	for _, ifc := range ifs {
		addrs, err := ifc.Addrs()
		if err != nil {
			return Facts{}, &Error{Code: CodeScopeFailed, Message: "cannot read addresses of " + ifc.Name + ": " + err.Error()}
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(n.IP)
			if !ok {
				continue
			}
			ones, _ := n.Mask.Size()
			ip = Unmap(ip)
			if n.IP.To4() != nil && ones > 32 {
				ones = 32
			}
			p, err := ip.Prefix(ones)
			if err != nil {
				continue
			}
			f.HostAddrs = append(f.HostAddrs, ip)
			f.OnLink = append(f.OnLink, p)
		}
	}
	routes, err := routePrefixes()
	if err != nil {
		return Facts{}, &Error{Code: CodeScopeFailed, Message: "cannot read the route table: " + err.Error()}
	}
	f.OnLink = append(f.OnLink, routes...)
	f.OnLink = uniquePrefixes(f.OnLink)
	sort.Slice(f.HostAddrs, func(i, j int) bool { return f.HostAddrs[i].Less(f.HostAddrs[j]) })
	return f, nil
}

// Merge returns a with b's lists added (deduplicated).
func Merge(a, b Facts) Facts {
	out := Facts{
		OnLink:    uniquePrefixes(append(append([]netip.Prefix(nil), a.OnLink...), b.OnLink...)),
		Exclusion: uniquePrefixes(append(append([]netip.Prefix(nil), a.Exclusion...), b.Exclusion...)),
	}
	hosts := map[string]bool{}
	for _, l := range [][]string{a.ExclusionHosts, b.ExclusionHosts} {
		for _, h := range l {
			if !hosts[h] {
				hosts[h] = true
				out.ExclusionHosts = append(out.ExclusionHosts, h)
			}
		}
	}
	seen := map[netip.Addr]bool{}
	for _, l := range [][]netip.Addr{a.HostAddrs, b.HostAddrs} {
		for _, h := range l {
			if !seen[h] {
				seen[h] = true
				out.HostAddrs = append(out.HostAddrs, h)
			}
		}
	}
	return out
}

func uniquePrefixes(in []netip.Prefix) []netip.Prefix {
	seen := map[netip.Prefix]bool{}
	var out []netip.Prefix
	for _, p := range in {
		if !p.IsValid() || p.Bits() == 0 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
