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
	"slices"
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
	// DuplicateID marks rows that share one window id: only one host can really hold it, so the admin
	// must treat the rows with suspicion (a spoofer can copy an id it sniffed from the LAN).
	DuplicateID bool `json:"duplicate_id"`
}

// Result is what one browse found.
type Result struct {
	Candidates []Candidate `json:"candidates"`
	// Truncated counts candidates cut by MaxCandidates; Flooded is true when the raw record cap was hit
	// too, so more records may exist than were counted.
	Truncated int  `json:"truncated"`
	Flooded   bool `json:"flooded"`
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
	if a.Is6() && a.IsLinkLocalUnicast() {
		return false // no zone survives the wire, so fe80:: cannot be dialled
	}
	return a.IsValid() && (a.IsGlobalUnicast() || a.IsPrivate() || a.IsLinkLocalUnicast()) && !a.IsLoopback() && !a.IsMulticast()
}

// ofService reports whether a record name belongs to our service: the library does not check that an
// answer matches the queried type, so other devices on the LAN (Cast, printers) arrive too.
func ofService(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), "."+ServiceType+"."+Domain)
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

// candidates cleans, filters, de-duplicates, ranks and caps the raw records. Rows with a well-formed window
// id rank first, so a flood of id-less junk cannot push a real node out of the list.
func candidates(in []Entry) Result {
	seen := map[string]bool{}
	var out []Candidate
	for _, e := range in {
		if !ofService(e.Name) {
			continue
		}
		var addrs []string
		for _, a := range e.Addrs {
			if listable(a) {
				addrs = append(addrs, a.Unmap().WithZone("").String())
			}
		}
		sort.Strings(addrs)
		addrs = slices.Compact(addrs)
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
		if (out[i].WindowID != "") != (out[j].WindowID != "") {
			return out[i].WindowID != ""
		}
		if out[i].Instance != out[j].Instance {
			return out[i].Instance < out[j].Instance
		}
		if out[i].WindowID != out[j].WindowID {
			return out[i].WindowID < out[j].WindowID
		}
		return strings.Join(out[i].Addresses, ",") < strings.Join(out[j].Addresses, ",")
	})
	r := Result{}
	if len(out) > MaxCandidates {
		r.Truncated = len(out) - MaxCandidates
		out = out[:MaxCandidates]
	}
	ids := map[string]int{}
	for _, c := range out {
		if c.WindowID != "" {
			ids[c.WindowID]++
		}
	}
	for i := range out {
		out[i].DuplicateID = out[i].WindowID != "" && ids[out[i].WindowID] > 1
	}
	r.Candidates = out
	if r.Candidates == nil {
		r.Candidates = []Candidate{}
	}
	return r
}
