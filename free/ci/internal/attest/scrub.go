package attest

// Purpose: Profile.Text, the free-text half of the publication profile. It
// removes private network addresses, private URLs, host names, home paths and
// user names from log excerpts and similar text.
// Inputs:  a string.
// Outputs: the string with each hit replaced by a fixed placeholder; the same
// input always gives the same output.
// Constraints: pure, RE2 only (linear time); a private profile returns the
// text unchanged. Public addresses and public URLs are kept: they identify
// nothing private. Over-redaction is the safe direction.

import (
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

const (
	redactedIP   = "[redacted-ip]"
	redactedHost = "[redacted-host]"
	redactedHome = "[redacted-home]"
	redactedUser = "[redacted-user]"
	redactedURL  = "[redacted-url]"
	// redactedText replaces a whole text that names more users than Text will track: fail closed
	redactedText = "[redacted-text]"
)

var (
	urlRe      = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>\x60)\]}]+`)
	ipv4Re     = regexp.MustCompile(`(?:[0-9]{1,3}\.){3}[0-9]{1,3}`)
	ipv6Re     = regexp.MustCompile(`[0-9A-Fa-f:]*:[0-9A-Fa-f:]*:[0-9A-Fa-f:.]*`)
	homeRe     = regexp.MustCompile(`(/Users|/home)/([^/\s:"'\x60?#&]+)|/root\b|[A-Za-z]:\\Users\\([^\\\s"'?#&]+)`)
	userHostRe = regexp.MustCompile(`([A-Za-z0-9._-]+)@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,})`)
	localHost  = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.(?:` + privateSuffixAlternation() + `)\b`)
)

// privateSuffixAlternation turns privateSuffixes (the one list URL also uses)
// into a regexp alternation, longest first so "home.arpa" wins over "home".
func privateSuffixAlternation() string {
	parts := make([]string, 0, len(privateSuffixes))
	for _, s := range privateSuffixes {
		parts = append(parts, regexp.QuoteMeta(strings.TrimPrefix(s, ".")))
	}
	sort.Slice(parts, func(i, j int) bool { return len(parts[i]) > len(parts[j]) })
	return strings.Join(parts, "|")
}

// Text scrubs identifying text for publication.
func (p Profile) Text(s string) string {
	if p.private || s == "" {
		return s
	}
	// user names in home paths anywhere in the text (URL paths, queries and
	// fragments that are about to be dropped, percent-encoded or not) still
	// identify the user in the rest of the text
	seen, over := learnUsers(s)
	if over {
		return redactedText
	}
	s = urlRe.ReplaceAllStringFunc(s, func(m string) string {
		trimmed := strings.TrimRight(m, ".,;:")
		if u := p.URL(trimmed); u != "" {
			return u + m[len(trimmed):] // query and fragment are dropped, as for target_url
		}
		return redactedURL + m[len(trimmed):]
	})
	return p.scrub(s, seen)
}

// textNoURL is Text without the URL pass. Profile.URL uses it on a path, so a
// URL cannot recurse into itself: a path that still holds "://" is dropped.
func (p Profile) textNoURL(s string) string {
	seen, over := learnUsers(s)
	if over {
		return redactedText
	}
	return p.scrub(s, seen)
}

func (p Profile) scrub(s string, seen []string) string {
	users := append([]string(nil), p.users...)
	for _, u := range seen {
		users = appendUser(users, u)
	}
	s = homeRe.ReplaceAllStringFunc(s, func(m string) string {
		if sub := homeRe.FindStringSubmatch(m); sub[2] != "" {
			users = appendUser(users, sub[2])
		} else if sub[3] != "" {
			users = appendUser(users, sub[3])
		}
		return redactedHome
	})
	s = userHostRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := userHostRe.FindStringSubmatch(m)
		user, host := sub[1], sub[2]
		if user != "git" {
			users = appendUser(users, user)
			user = redactedUser
		}
		if p.privateHost(host) {
			host = redactedHost
		}
		return user + "@" + host
	})
	s = localHost.ReplaceAllString(s, redactedHost)
	s = ipv4Re.ReplaceAllStringFunc(s, func(m string) string {
		if a, ok := parseLooseAddr(m); ok && (privateAddr(a) || p.knownAddr(a)) {
			return redactedIP
		}
		return m
	})
	s = ipv6Re.ReplaceAllStringFunc(s, func(m string) string {
		for _, c := range []string{m, strings.TrimSuffix(m, ":"), strings.TrimPrefix(m, ":"), strings.TrimPrefix(strings.TrimSuffix(m, ":"), ":")} {
			if a, err := netip.ParseAddr(c); err == nil && !a.IsUnspecified() && (privateAddr(a) || p.knownAddr(a)) {
				return redactedIP
			}
		}
		return m
	})
	for lit, repl := range p.aliases {
		s = replaceToken(s, lit, repl)
	}
	for _, h := range p.hosts {
		s = replaceToken(s, h, redactedHost)
	}
	sort.Slice(users, func(i, j int) bool { return len(users[i]) > len(users[j]) })
	for _, u := range users {
		if len(u) >= 2 {
			s = replaceToken(s, u, redactedUser)
		}
	}
	return s
}

// genericUsers are account names every CI image has; they identify nobody, and
// replacing them as words would mangle ordinary log lines ("runner", "node").
var genericUsers = map[string]bool{"runner": true, "node": true, "build": true, "ci": true, "app": true, "user": true, "root": true,
	"admin": true, "ubuntu": true, "runneradmin": true, "github": true, "builder": true, "jenkins": true, "circleci": true, "vagrant": true}

// maxLearnedUsers bounds the user names one Text call collects (each costs a pass over the text).
const maxLearnedUsers = 256

// learnUsers lists the user names of home paths in s, looking through up to
// three layers of percent-encoding. over is true when the text names more than
// maxLearnedUsers distinct users: the caller then redacts the whole text.
func learnUsers(s string) (users []string, over bool) {
	for i := 0; i < 4; i++ {
		for _, sub := range homeRe.FindAllStringSubmatch(s, -1) {
			var ok bool
			if users, ok = addUser(users, sub[2]+sub[3]); !ok {
				return users, true
			}
		}
		for _, sub := range userHostRe.FindAllStringSubmatch(s, -1) {
			if sub[1] == "git" {
				continue
			}
			var ok bool
			if users, ok = addUser(users, sub[1]); !ok {
				return users, true
			}
		}
		d := unescapeLoose(s)
		if d == s {
			break
		}
		s = d
	}
	return users, false
}

// unescapeLoose decodes every valid %XX and leaves any other '%' alone.
func unescapeLoose(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if hi, lo := unhex(s[i+1]), unhex(s[i+2]); hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// addUser adds a user name once (case-insensitive), skipping empty and generic
// account names; ok is false when a new name does not fit under maxLearnedUsers.
func addUser(users []string, name string) ([]string, bool) {
	if name == "" || genericUsers[strings.ToLower(name)] {
		return users, true
	}
	for _, u := range users {
		if strings.EqualFold(u, name) {
			return users, true
		}
	}
	if len(users) >= maxLearnedUsers {
		return users, false
	}
	return append(users, name), true
}

// appendUser merges a name into a user list without the cap: the names it sees
// were all counted by learnUsers on the same text, which already failed closed
// past maxLearnedUsers; configured names never count against the cap.
func appendUser(users []string, name string) []string {
	if name == "" || genericUsers[strings.ToLower(name)] {
		return users
	}
	for _, u := range users {
		if strings.EqualFold(u, name) {
			return users
		}
	}
	return append(users, name)
}

// replaceToken replaces lit where it stands alone: a letter, digit or
// underscore on either side keeps it as part of another word.
func replaceToken(s, lit, repl string) string {
	if lit == "" {
		return s
	}
	re, err := regexp.Compile(`(?i)(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(lit) + `($|[^A-Za-z0-9_])`)
	if err != nil {
		return s
	}
	for i := 0; i < 64; i++ { // overlapping neighbours need a second pass
		next := re.ReplaceAllString(s, "${1}"+repl+"${2}")
		if next == s {
			break
		}
		s = next
	}
	return s
}
