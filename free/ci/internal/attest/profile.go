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
	if p.privateHost(parsed.Hostname()) {
		return ""
	}
	parsed.RawQuery, parsed.Fragment, parsed.RawFragment = "", "", ""
	return parsed.String()
}

var privateSuffixes = []string{".local", ".lan", ".internal", ".home", ".home.arpa", ".localdomain", ".corp", ".intranet", ".private", ".localhost"}

// privateHost is true for a host that names a private network or machine.
func (p Profile) privateHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "" || h == "localhost" || (!strings.Contains(h, ".") && !strings.Contains(h, ":")) {
		return true // empty, localhost or a bare single-label name
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		return privateAddr(addr)
	}
	for _, s := range privateSuffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	for _, x := range p.hosts {
		if x != "" && strings.EqualFold(x, h) {
			return true
		}
	}
	return false
}

// privateAddr covers RFC1918, loopback, link-local, CGNAT, ULA and unspecified.
func privateAddr(a netip.Addr) bool {
	a = a.Unmap()
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsUnspecified() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(a)
}
