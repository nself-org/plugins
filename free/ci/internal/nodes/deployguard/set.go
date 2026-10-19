package deployguard

// Purpose: the exclusion set and the matcher over it: every identity that belongs to a deploy host
// (inventory of every known project plus the operator never-list), and the one question "is this node
// one of them".
// Inputs:  Entry rows (host, address, prefix, fingerprint, machine_id) and the Facts of a node.
// Outputs: Set, Facts, Result; View, the read-only export for P7-TRUST-08 and P7-SCHED-08.
// Constraints: pure (no network, no files); every name goes through Canon; a prefix entry that is not a
//          CIDR is dropped; Match checks addresses, names, fingerprints and machine ids, so one hit on
//          any identity makes the node a deploy host.

import (
	"net/netip"
	"sort"
	"strings"
)

// Match sources (the deploy_host.source enum of the capability document).
const (
	SourceInventory = "inventory"
	SourceNeverList = "never-list"
	SourceMarker    = "marker"
)

// Entry kinds.
const (
	KindHost        = "host"
	KindAddress     = "address"
	KindPrefix      = "prefix"
	KindFingerprint = "fingerprint"
	KindMachineID   = "machine_id"
)

// Entry is one excluded identity and where it came from.
type Entry struct{ Kind, Value, Source string }

// Facts are the identities of a node or of the local host.
type Facts struct {
	Names        []string
	Addrs        []netip.Addr
	Fingerprints []string
	MachineIDs   []string
}

// Result is the outcome of a match. Source is empty when nothing matched.
type Result struct {
	Matched      bool
	Source, Kind string
}

type prefixEntry struct {
	p      netip.Prefix
	source string
}

// Set is the exclusion set. The zero value is empty and usable.
type Set struct {
	hosts, fps, machines map[string]string
	addrs                map[netip.Addr]string
	prefixes             []prefixEntry
}

// NewSet builds a set from entries; an entry that normalises to nothing is dropped.
func NewSet(entries []Entry) Set {
	var s Set
	for _, e := range entries {
		s.Add(e)
	}
	return s
}

// Add puts one entry into the set. The first source for a value wins.
func (s *Set) Add(e Entry) {
	src := e.Source
	if src == "" {
		src = SourceInventory
	}
	switch e.Kind {
	case KindHost, KindAddress:
		if a, ok := parseAddr(bareHost(e.Value)); ok {
			if s.addrs == nil {
				s.addrs = map[netip.Addr]string{}
			}
			setOnce(s.addrs, a, src)
			return
		}
		if c := Canon(e.Value); c != "" {
			if s.hosts == nil {
				s.hosts = map[string]string{}
			}
			setOnce(s.hosts, c, src)
		}
	case KindPrefix:
		if p, err := netip.ParsePrefix(strings.TrimSpace(e.Value)); err == nil {
			s.prefixes = append(s.prefixes, prefixEntry{p.Masked(), src})
		}
	case KindFingerprint:
		if f := NormalizeFingerprint(e.Value); f != "" {
			if s.fps == nil {
				s.fps = map[string]string{}
			}
			setOnce(s.fps, f, src)
		}
	case KindMachineID:
		if m := NormalizeMachineID(e.Value); m != "" {
			if s.machines == nil {
				s.machines = map[string]string{}
			}
			setOnce(s.machines, m, src)
		}
	}
}

func setOnce[K comparable](m map[K]string, k K, v string) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

// Empty reports whether the set holds nothing.
func (s Set) Empty() bool {
	return len(s.hosts)+len(s.fps)+len(s.machines)+len(s.addrs)+len(s.prefixes) == 0
}

// Match reports whether any identity of f is in the set.
func (s Set) Match(f Facts) Result {
	for _, a := range f.Addrs {
		if r := s.matchAddr(a.Unmap()); r.Matched {
			return r
		}
	}
	for _, n := range f.Names {
		if r := s.MatchHost(n); r.Matched {
			return r
		}
	}
	for _, fp := range f.Fingerprints {
		if src, ok := s.fps[NormalizeFingerprint(fp)]; ok && NormalizeFingerprint(fp) != "" {
			return Result{true, src, KindFingerprint}
		}
	}
	for _, m := range f.MachineIDs {
		if src, ok := s.machines[NormalizeMachineID(m)]; ok && NormalizeMachineID(m) != "" {
			return Result{true, src, KindMachineID}
		}
	}
	return Result{}
}

// MatchHost checks one host string in any spelling.
func (s Set) MatchHost(raw string) Result {
	h := bareHost(raw)
	if h == "" {
		return Result{}
	}
	if a, ok := parseAddr(h); ok {
		return s.matchAddr(a)
	}
	if src, ok := s.hosts[h]; ok {
		return Result{true, src, KindHost}
	}
	return Result{}
}

func (s Set) matchAddr(a netip.Addr) Result {
	if src, ok := s.addrs[a]; ok {
		return Result{true, src, KindAddress}
	}
	for _, p := range s.prefixes {
		if p.p.Contains(a) {
			return Result{true, p.source, KindPrefix}
		}
	}
	return Result{}
}

// View is the read-only export of a set: sorted values, no sources.
type View struct {
	Hosts, Addresses, Prefixes, Fingerprints, MachineIDs []string
}

// View lists the set for readers outside this package (P7-TRUST-08, P7-SCHED-08).
func (s Set) View() View {
	v := View{}
	for h := range s.hosts {
		v.Hosts = append(v.Hosts, h)
	}
	for a := range s.addrs {
		v.Addresses = append(v.Addresses, a.String())
	}
	for _, p := range s.prefixes {
		v.Prefixes = append(v.Prefixes, p.p.String())
	}
	for f := range s.fps {
		v.Fingerprints = append(v.Fingerprints, f)
	}
	for m := range s.machines {
		v.MachineIDs = append(v.MachineIDs, m)
	}
	for _, l := range [][]string{v.Hosts, v.Addresses, v.Prefixes, v.Fingerprints, v.MachineIDs} {
		sort.Strings(l)
	}
	return v
}
