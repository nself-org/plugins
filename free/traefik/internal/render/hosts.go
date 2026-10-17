package render

// Server-name handling: host classes and tiers, duplicate detection, listeners.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// hostsFree refuses a server name that another route already serves on the same
// entrypoint: Traefik would pick one of two equal rules at random, nginx keeps
// the first and warns. Names compare case-insensitively (nginx lowercases them, so
// does Traefik's Host()) and a leading-dot name ".x" registers both "x" and "*.x".
// It also refuses a custom TLS option on a wildcard name, because Traefik selects
// TLS options by SNI from Host() rules only.
func (b *builder) hostsFree(r contract.Route, ls []listener) bool {
	ok := true
	for _, l := range ls {
		for _, n := range r.ServerNames {
			ln := strings.ToLower(n)
			keys := []string{"exact|" + ln}
			switch {
			case dotRE.MatchString(n):
				keys = []string{"exact|" + ln[1:], "wild|" + ln}
			case wildRE.MatchString(n):
				keys = []string{"wild|" + ln[1:]}
			}
			for _, k := range keys {
				key := l.entry + "|" + k
				if prev, dup := b.hosts[key]; dup && prev != r.ID {
					b.refuse(r.ID, "server_names", fmt.Sprintf("%q on %s is already served by route %s", n, l.entry, prev))
					ok = false
				}
				b.hosts[key] = r.ID
			}
			if l.opt != "" && !nameRE.MatchString(n) {
				b.refuse(r.ID, "tls", fmt.Sprintf("wildcard %q with non-default protocols or ciphers: Traefik picks TLS options by SNI from Host() rules only", n))
				ok = false
			}
		}
	}
	return ok
}

func (b *builder) listeners(r contract.Route) []listener {
	var ls []listener
	if !r.Listen.HTTP && !r.Listen.HTTPS {
		b.refuse(r.ID, "listen", "neither http nor https (nginx would listen on *:80); the route is not guessed")
	}
	if r.Listen.HTTP {
		ls = append(ls, listener{entry: "web", tag: "web"})
	}
	if r.Listen.HTTPS {
		opt, ok := b.tlsFor(r)
		if ok {
			ls = append(ls, listener{entry: "websecure", tag: "tls", opt: opt, secure: true})
		}
	}
	return ls
}

// The name regexes allow no empty label (no "..", no trailing dot): see badShape.
var wildRE = regexp.MustCompile(`^\*\.[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*$`)
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(?:\.[A-Za-z0-9-]+)*$`)

// dotRE is nginx's ".example.test": the name itself plus every subdomain.
var dotRE = regexp.MustCompile(`^\.[A-Za-z0-9][A-Za-z0-9-]*(?:\.[A-Za-z0-9-]+)*$`)

// badShape reports a server name with an empty label or a last character that is not a letter
// or digit. Traefik's Host() ignores a trailing dot on the rule and on the request, nginx keeps
// it in the name, and duplicate detection keys on the written name: "a.example.test" and
// "a.example.test." would be two hosts here and one host in Traefik (a deny_all route and an
// upstream route would then tie). nginx answers a request for "a..example.test" with a 400.
func badShape(raw string) bool {
	if raw == "" || strings.Contains(raw, "..") {
		return true
	}
	c := raw[len(raw)-1]
	return (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9')
}

// Host specificity. nginx picks the server by exact name, then by the longest
// wildcard, and only then the location; Traefik ranks by one priority number, so
// the number is tier + location priority and a tier step is larger than every
// location priority (locPri is capped at maxLocPri, the redirect sits at
// redirectPri, one below the next tier). A wildcard tier grows with the suffix
// length, so the longer suffix wins as in nginx; exactTier beats every wildcard
// tier. Defaults routes (priority 1 to 4) sit at tier 0.
const (
	tierStep    = 100000
	maxLocPri   = tierStep - 3
	redirectPri = tierStep - 1
	maxName     = 253
	exactTier   = tierStep * (maxName + 3)
)

func wildTier(suffixLen int) int { return tierStep * (1 + suffixLen) }

type hostGroup struct {
	tier int
	rule string
}

// hostGroups splits a route's server names by name class and wildcard suffix
// length. Each group becomes its own router set at its own tier.
func (b *builder) hostGroups(r contract.Route) ([]hostGroup, bool) {
	if len(r.ServerNames) == 0 {
		b.refuse(r.ID, "server_names", "empty")
		return nil, false
	}
	byTier := map[int][]string{}
	for _, raw := range r.ServerNames {
		if len(raw) > maxName {
			b.refuse(r.ID, "server_names", fmt.Sprintf("%q is longer than %d characters", raw[:20]+"...", maxName))
			return nil, false
		}
		// The name is checked as written: strings.ToLower folds U+212A (Kelvin sign) to an ASCII "k",
		// so a check after lowercasing would accept a name that is not the ASCII name. Only an
		// all-ASCII name reaches ToLower.
		if badShape(raw) {
			b.refuse(r.ID, "server_names", fmt.Sprintf("%q has an empty label or does not end in a letter or digit (a trailing dot names the same host to Traefik and a different one to nginx)", raw))
			return nil, false
		}
		switch {
		case nameRE.MatchString(raw):
			n := strings.ToLower(raw)
			byTier[exactTier] = append(byTier[exactTier], "Host(`"+n+"`)")
		case dotRE.MatchString(raw):
			n := strings.ToLower(raw)
			byTier[exactTier] = append(byTier[exactTier], "Host(`"+n[1:]+"`)")
			byTier[wildTier(len(n))] = append(byTier[wildTier(len(n))], wildRule(n))
		case wildRE.MatchString(raw):
			n := strings.ToLower(raw)
			byTier[wildTier(len(n)-1)] = append(byTier[wildTier(len(n)-1)], wildRule(n[1:]))
		default:
			b.refuse(r.ID, "server_names", fmt.Sprintf("%q is not a plain or leading-wildcard name", raw))
			return nil, false
		}
	}
	tiers := make([]int, 0, len(byTier))
	for t := range byTier {
		tiers = append(tiers, t)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(tiers)))
	var out []hostGroup
	for _, t := range tiers {
		rule := byTier[t][0]
		if len(byTier[t]) > 1 {
			rule = "(" + strings.Join(byTier[t], " || ") + ")"
		}
		out = append(out, hostGroup{tier: t, rule: rule})
	}
	return out, true
}

// wildRule matches any name ending in suffix (".example.test"). The optional trailing dot
// matches a Host header written as an absolute name ("x.example.test."), which nginx strips
// and serves; Traefik's Host() tolerates it but a HostRegexp does not unless the pattern does.
func wildRule(suffix string) string {
	return "HostRegexp(`^.+" + regexp.QuoteMeta(suffix) + `\.?$` + "`)"
}
