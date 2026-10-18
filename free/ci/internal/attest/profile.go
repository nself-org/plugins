package attest

// Purpose: the publication profile (R-SEC-27): what may leave the machine in
// a document about a public repository. Publish applies it to evidence;
// ProfileFor exports the same functions so provenance and release documents
// (P7-CACHE-09/18) scrub non-evidence text the same way.
// Inputs:  RepoFacts (visibility, repo id, extra hostnames and usernames).
// Outputs: Profile with RunnerID, CoordinatorID, Public, Text and URL.
// Constraints: pure; fail closed: only the exact value Private turns the
// profile off, so an empty or unknown visibility scrubs; a zero Profile
// scrubs; pseudonyms are stable for one repo and differ between repos.

import (
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// Visibility is the repository's visibility as the caller knows it.
type Visibility string

const (
	Public  Visibility = "public"
	Private Visibility = "private"
)

// RepoFacts are the inputs of a Profile.
type RepoFacts struct {
	Visibility Visibility
	RepoID     string
	// Hostnames and Usernames are extra literals to redact from free text.
	Hostnames []string
	Usernames []string
}

// Profile scrubs identifying text. The zero value scrubs.
type Profile struct {
	private bool
	repoID  string
	hosts   []string
	users   []string
	aliases map[string]string // literal -> replacement, longest applied first
}

// ProfileFor builds the profile of one repository.
func ProfileFor(f RepoFacts) Profile {
	return Profile{private: f.Visibility == Private, repoID: f.RepoID,
		hosts: append([]string(nil), f.Hostnames...), users: append([]string(nil), f.Usernames...)}
}

// Public reports whether the profile scrubs.
func (p Profile) Public() bool { return !p.private }

// RunnerID is the stable pseudonym of a runner id (the id itself when private).
func (p Profile) RunnerID(id string) string { return p.pseudonym("runner", id) }

// CoordinatorID is the stable pseudonym of a coordinator id.
func (p Profile) CoordinatorID(id string) string { return p.pseudonym("coordinator", id) }

func (p Profile) pseudonym(kind, id string) string {
	if p.private || id == "" {
		return id
	}
	return kind + "-" + Digest([]byte("nself-ci/" + kind + "/v1\x00" + p.repoID + "\x00" + id))[:12]
}

// WithRunner is withRunner for callers outside the package (the status hook).
func (p Profile) WithRunner(id string) Profile { return p.withRunner(id) }

// withRunner returns a copy that also replaces the literal runner id in text
// with its pseudonym; the generic id "local" is not a host name and is left.
func (p Profile) withRunner(id string) Profile {
	q := p
	q.aliases = map[string]string{}
	for k, v := range p.aliases {
		q.aliases[k] = v
	}
	if id != "" && id != "local" && id != "localhost" {
		q.aliases[id] = p.RunnerID(id)
	}
	return q
}

// URL returns u when it may be published, and "" when it must not be: a
// private or local host, credentials in the URL, or anything not http(s).
// Query and fragment are dropped from a published URL.
func (p Profile) URL(u string) string {
	if p.private || u == "" {
		return u
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Host == "" {
		return ""
	}
	if p.privateHost(parsed.Hostname()) || p.privatePath(parsed.EscapedPath()) {
		return ""
	}
	parsed.RawQuery, parsed.Fragment, parsed.RawFragment = "", "", ""
	return parsed.String()
}

var privateSuffixes = []string{".local", ".lan", ".internal", ".home", ".home.arpa", ".localdomain", ".corp", ".intranet", ".private", ".localhost"}

// numericHostRe matches a host written only as numbers: decimal, hex or short
// IPv4 forms that netip refuses but a socket library may still resolve.
var numericHostRe = regexp.MustCompile(`^(0x[0-9a-f]+|[0-9]+)(\.(0x[0-9a-f]+|[0-9]+))*$`)

// privatePath is true when the path (raw, then percent-decoded up to three
// times) holds anything Text would redact: a home path, a private address, a
// known host or user name, a runner id. Such a URL is dropped whole.
func (p Profile) privatePath(path string) bool {
	for i := 0; i < 4; i++ {
		if p.Text(path) != path {
			return true
		}
		d, err := url.PathUnescape(path)
		if err != nil {
			return true
		}
		if d == path {
			return false
		}
		path = d
	}
	return true // still encoded after three layers: treat as hostile
}

// privateHost is true for a host that names a private network or machine.
func (p Profile) privateHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if h == "" || h == "localhost" || (!strings.Contains(h, ".") && !strings.Contains(h, ":")) {
		return true // empty, localhost or a bare single-label name
	}
	if addr, ok := parseLooseAddr(h); ok {
		return privateAddr(addr)
	}
	if numericHostRe.MatchString(h) {
		return true // a numeric form we cannot place is treated as private
	}
	for _, s := range privateSuffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	for _, x := range p.hosts {
		if x != "" && strings.EqualFold(strings.TrimSuffix(x, "."), h) {
			return true
		}
	}
	return false
}

// parseLooseAddr parses an address, accepting dotted IPv4 with leading zeros
// ("010.0.0.1" reads as decimal here: either reading is redacted as private
// when the decimal one is, and the octal one is never published as public).
func parseLooseAddr(s string) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(s); err == nil {
		return a, true
	}
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return netip.Addr{}, false
	}
	var b [4]byte
	for i, part := range parts {
		if part == "" || len(part) > 3 {
			return netip.Addr{}, false
		}
		n := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				return netip.Addr{}, false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return netip.Addr{}, false
		}
		b[i] = byte(n)
	}
	return netip.AddrFrom4(b), true
}

var extraPrivate = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
}

// privateAddr covers RFC1918, loopback, link-local, CGNAT, ULA, unspecified and
// the IPv6 forms that embed an IPv4 address (v4-mapped, NAT64, 6to4).
func privateAddr(a netip.Addr) bool {
	a = a.Unmap()
	if a.Is6() {
		raw := a.As16()
		switch {
		case a.Compare(netip.MustParseAddr("64:ff9b::")) >= 0 && a.Compare(netip.MustParseAddr("64:ff9b::ffff:ffff")) <= 0:
			return privateAddr(netip.AddrFrom4([4]byte{raw[12], raw[13], raw[14], raw[15]}))
		case raw[0] == 0x20 && raw[1] == 0x02:
			return privateAddr(netip.AddrFrom4([4]byte{raw[2], raw[3], raw[4], raw[5]}))
		}
	}
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsUnspecified() {
		return true
	}
	for _, pre := range extraPrivate {
		if pre.Contains(a) {
			return true
		}
	}
	return false
}
