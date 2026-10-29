package truth

// Purpose: per-line matching for the three rule kinds.
// Inputs:  one line of text and the compiled rules.
// Outputs: the rules the line breaks (with a hash prefix for dead hosts).
// Constraints: a dead-host rule stores sha256 hex of the lower-cased literal;
// the engine hashes every IPv4 and host-name token of the line and compares
// the whole token (a sub-domain is a different host), so no literal is ever held or
// printed; a retired term matches case-insensitively as a whole term (the
// characters on both sides are not letters or digits); a banned literal is a
// case-sensitive substring.

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ipv4Re = regexp.MustCompile(`[0-9]{1,3}(?:\.[0-9]{1,3}){3}`)
	hostRe = regexp.MustCompile(`[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+`)
)

// HashLiteral is the dead-host rule value of a host name or IPv4: the sha256
// of its lower-cased text. Authors compute it in their shell; it is exported
// for the tests and the runner's `hash` command.
func HashLiteral(literal string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(literal)))
	return hex.EncodeToString(sum[:])
}

// hostHit is a token whose hash is on a dead-host rule.
type hostHit struct {
	Rule   *Rule
	Prefix string // first 12 hex digits of the token hash
}

// compiled holds the rules grouped for the scan.
type compiled struct {
	literals []*Rule
	terms    []*Rule
	termsLow []string
	hosts    map[string][]*Rule // sha256 -> rules
	claims   []*Rule            // numeric-claim and version-claim rules

	regionRules []*Rule // rules that honour GENERATED regions, JSX and exempt comments
}

func compile(rs RuleSet) compiled {
	c := compiled{hosts: map[string][]*Rule{}}
	for i := range rs.Rules {
		r := &rs.Rules[i]
		if r.usesRegions() {
			c.regionRules = append(c.regionRules, r)
		}
		switch r.Kind {
		case KindBannedLiteral:
			c.literals = append(c.literals, r)
		case KindRetiredTerm:
			c.terms = append(c.terms, r)
			c.termsLow = append(c.termsLow, strings.ToLower(strings.TrimSpace(r.Term)))
		case KindNumericClaim, KindVersionClaim:
			c.claims = append(c.claims, r)
		case KindDeadHost:
			for _, h := range r.SHA256 {
				c.hosts[h] = append(c.hosts[h], r)
			}
		}
	}
	return c
}

// wholeTermAt reports whether low[start:end] is delimited by non-letters and non-digits.
func wholeTermAt(low string, start, end int) bool {
	before, _ := utf8.DecodeLastRuneInString(low[:start])
	after, _ := utf8.DecodeRuneInString(low[end:])
	return (start == 0 || !isWordRune(before)) && (end == len(low) || !isWordRune(after))
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// hostHits returns the dead-host rules the line breaks.
func (c compiled) hostHits(line string) []hostHit {
	if len(c.hosts) == 0 {
		return nil
	}
	low := strings.Map(func(r rune) rune {
		if isZeroWidth(r) {
			return -1 // zero-width characters cannot hide a token
		}
		return r
	}, strings.ToLower(line))
	var out []hostHit
	tried := map[string]bool{} // an IPv4 is also a dotted token: report it once
	try := func(tok string) {
		h := HashLiteral(tok)
		if tried[h] {
			return
		}
		tried[h] = true
		for _, r := range c.hosts[h] {
			out = append(out, hostHit{Rule: r, Prefix: h[:12]})
		}
	}
	for _, loc := range ipv4Re.FindAllStringIndex(low, -1) {
		if standalone(low, loc[0], loc[1]) {
			try(low[loc[0]:loc[1]])
		}
	}
	for _, tok := range hostRe.FindAllString(low, -1) {
		try(tok)
	}
	return out
}

// isZeroWidth reports a zero-width space, joiner, non-joiner, word joiner or BOM.
func isZeroWidth(r rune) bool {
	return r == 0x200B || r == 0x200C || r == 0x200D || r == 0x2060 || r == 0xFEFF
}

// hostsOnly keeps the dead-host rules: the one kind that reads skipped files.
func (c compiled) hostsOnly() compiled { return compiled{hosts: c.hosts} }

// standalone reports whether the IPv4 at [start,end) is not part of a longer
// dotted number (a version such as 1.2.3.4.5 is not an address).
func standalone(s string, start, end int) bool {
	if start > 0 && (s[start-1] == '.' && start > 1 && s[start-2] >= '0' && s[start-2] <= '9') {
		return false
	}
	if end < len(s) && s[end] == '.' && end+1 < len(s) && s[end+1] >= '0' && s[end+1] <= '9' {
		return false
	}
	return true
}

// forFile narrows the compiled rules to those that can fire on rel (the rule's
// paths match and no exemption path covers the file), so a line is never
// matched against a rule that cannot report on it.
func (c compiled) forFile(rel string) compiled {
	in := func(r *Rule) bool { return (len(r.Paths) == 0 || matchAny(r.Paths, rel)) && !r.pathExempt(rel) }
	out := compiled{hosts: c.hosts}
	for _, r := range c.literals {
		if in(r) {
			out.literals = append(out.literals, r)
		}
	}
	for i, r := range c.terms {
		if in(r) {
			out.terms = append(out.terms, r)
			out.termsLow = append(out.termsLow, c.termsLow[i])
		}
	}
	for _, r := range c.claims {
		if in(r) {
			out.claims = append(out.claims, r)
		}
	}
	for _, r := range c.regionRules {
		if in(r) {
			out.regionRules = append(out.regionRules, r)
		}
	}
	for _, rs := range c.hosts {
		for _, r := range rs {
			if !in(r) {
				out.hosts = c.hostsFor(in)
				return out
			}
		}
	}
	return out
}

// hostsFor rebuilds the dead-host table with only the rules in() accepts.
func (c compiled) hostsFor(in func(*Rule) bool) map[string][]*Rule {
	h := map[string][]*Rule{}
	for k, rs := range c.hosts {
		for _, r := range rs {
			if in(r) {
				h[k] = append(h[k], r)
			}
		}
	}
	return h
}
