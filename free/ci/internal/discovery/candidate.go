package discovery

// Purpose: turn raw network records into Candidates: advisory rows in state "discovered".
// Inputs:  []Entry from the network, which is untrusted: any host on the LAN can publish any record.
// Outputs: a bounded, sorted, de-duplicated []Candidate whose strings are safe to print to a terminal.
// Constraints: a Candidate has no trust field that can be true: discovered is never authorized, eligible
//          or selected (ADR 0025 §6, §10). Names are cleaned (control and escape characters never reach
//          the terminal), the window id must be exactly 8 lowercase hex characters or it is dropped, the
//          list is capped, and two responders claiming one name stay two rows (the key is the whole
//          record, never the name alone).

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// StateDiscovered is the only state this package produces.
const StateDiscovered = "discovered"

// MaxCandidates caps one browse so a flood of records cannot fill the terminal or memory.
const MaxCandidates = 64

// Candidate is one advisory row from `nself ci nodes discover`.
type Candidate struct {
	Instance   string   `json:"instance"`
	Host       string   `json:"host"`
	Addresses  []string `json:"addresses"`
	Port       int      `json:"port"`
	WindowID   string   `json:"window_id"`
	State      string   `json:"state"`
	Authorized bool     `json:"authorized"`
}

// clean keeps letters, digits, dot, dash and underscore, replaces anything else with '?' and cuts at max.
func clean(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= max {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// instanceOf is the DNS-SD instance label of a full service name.
func instanceOf(name string) string {
	if i := strings.Index(name, "."+ServiceType); i >= 0 {
		name = name[:i]
	}
	return clean(name, 63)
}

// listable keeps unicast addresses a candidate can really have; multicast, unspecified, loopback and
// invalid addresses are spoof or noise.
func listable(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	return a.IsValid() && (a.IsGlobalUnicast() || a.IsPrivate() || a.IsLinkLocalUnicast()) && !a.IsLoopback() && !a.IsMulticast()
}

// windowID is the first TXT field that is a well-formed window id, else "".
func windowID(txt []string) string {
	for _, t := range txt {
		if idPattern.MatchString(t) {
			return t
		}
	}
	return ""
}

func candidates(in []Entry) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	for _, e := range in {
		var addrs []string
		for _, a := range e.Addrs {
			if listable(a) {
				addrs = append(addrs, a.Unmap().WithZone("").String())
			}
		}
		sort.Strings(addrs)
		port := e.Port
		if port < 1 || port > 65535 {
			port = 0
		}
		c := Candidate{Instance: instanceOf(e.Name), Host: clean(strings.TrimSuffix(e.Host, "."), 253), Addresses: addrs, Port: port, WindowID: windowID(e.TXT), State: StateDiscovered}
		if c.Addresses == nil {
			c.Addresses = []string{}
		}
		key := strings.Join([]string{c.Instance, c.Host, c.WindowID, strings.Join(c.Addresses, ","), strconv.Itoa(port)}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Instance != out[j].Instance {
			return out[i].Instance < out[j].Instance
		}
		if out[i].WindowID != out[j].WindowID {
			return out[i].WindowID < out[j].WindowID
		}
		return strings.Join(out[i].Addresses, ",") < strings.Join(out[j].Addresses, ",")
	})
	if len(out) > MaxCandidates {
		out = out[:MaxCandidates]
	}
	return out
}
