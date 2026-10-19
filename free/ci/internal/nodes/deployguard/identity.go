package deployguard

// Purpose: turn what is known about a node into the identities Match compares: the host as typed, what
// `ssh -G` says it is (hostname, HostKeyAlias, every ProxyJump hop), the addresses its names resolve to,
// the pinned and agent-reported host-key fingerprints and the machine id.
// Inputs:  Identity (from the capability document or the add command) and two seams, SSH and Lookup.
// Outputs: Facts and a list of warnings (a lookup that failed never fails the match).
// Constraints: no connection to the host: ssh -G only reads config and DNS asks the resolver (2 s per
//          name); a failure keeps the names already known; an IP literal is not looked up.

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Identity is what is known about a node before it is matched.
type Identity struct {
	// Dest is the destination as typed or stored: an ssh alias, host or user@host.
	Dest string
	// Names are further names (hostname, node id, node name).
	Names        []string
	Fingerprints []string
	MachineID    string
}

// Resolver is the pair of seams that read ssh config and DNS.
type Resolver struct {
	SSH    func(ctx context.Context, alias string) (remote.Resolved, error)
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
}

// DefaultResolver reads `ssh -G` and the system resolver.
func DefaultResolver() Resolver {
	return Resolver{SSH: remote.ResolveSSHHost, Lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}}
}

const lookupTimeout = 2 * time.Second

// Facts expands an identity into every name, address, fingerprint and machine id it stands for.
func (r Resolver) Facts(ctx context.Context, id Identity) (Facts, []string) {
	var f Facts
	var warn []string
	seen, quiet := map[string]bool{}, map[string]bool{}
	name := func(n string) {
		c := Canon(n)
		if c == "" || seen[c] {
			return
		}
		seen[c] = true
		f.Names = append(f.Names, c)
	}
	name(id.Dest)
	// Labels (node id, node name) are looked up too, but a label with no address is not worth a warning.
	for _, n := range id.Names {
		before := len(f.Names)
		name(n)
		if len(f.Names) > before {
			quiet[f.Names[before]] = true
		}
	}
	if _, literal := parseAddr(bareHost(id.Dest)); id.Dest != "" && !literal && r.SSH != nil {
		if res, err := r.SSH(ctx, id.Dest); err != nil {
			warn = append(warn, "ssh -G could not resolve "+Canon(id.Dest))
		} else {
			name(res.Hostname)
			name(res.HostKeyAlias)
			for _, hop := range strings.Split(res.ProxyJump, ",") {
				if hop = strings.TrimSpace(hop); hop != "" && !strings.EqualFold(hop, "none") {
					name(hop)
				}
			}
		}
	}
	for _, n := range append([]string(nil), f.Names...) {
		if a, ok := parseAddr(n); ok {
			f.Addrs = append(f.Addrs, a)
			continue
		}
		if r.Lookup == nil {
			continue
		}
		lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		addrs, err := r.Lookup(lctx, n)
		cancel()
		if err != nil {
			if !quiet[n] {
				warn = append(warn, "no address for "+n)
			}
			continue
		}
		for _, a := range addrs {
			f.Addrs = append(f.Addrs, a.Unmap())
		}
	}
	f.Fingerprints = append(f.Fingerprints, id.Fingerprints...)
	if id.MachineID != "" {
		f.MachineIDs = append(f.MachineIDs, id.MachineID)
	}
	return f, warn
}
