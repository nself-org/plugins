package truth

// Purpose: validate one rule and one allowlist entry of a rule file.
// Inputs:  the declaring file, the index, the decoded rule or entry.
// Outputs: an E644 *Error naming file and key, or nil; checkRule also compiles
// the claim pattern and the exempt_matches regexes into the rule.
// Constraints: a kind takes only its own keys; a dead-host rule takes no text
// exemption (exempt_matches, exempt_regions) because its literal is never held.

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

func checkRule(file string, i int, r *Rule) error {
	key := func(k string) string { return fmt.Sprintf("rules[%d].%s", i, k) }
	if !idRe.MatchString(r.ID) {
		return failf(file, key("id"), "must be lower-case letters, digits, '.', '_' or '-': %q", r.ID)
	}
	if strings.TrimSpace(r.Message) == "" {
		return failf(file, key("message"), "a rule needs a message that says what to write instead")
	}
	only := func(set, other string, ok bool) error {
		if !ok {
			return failf(file, key(other), "%s rules do not take this key (use %s)", r.Kind, set)
		}
		return nil
	}
	switch r.Kind {
	case KindBannedLiteral:
		if r.Literal == "" {
			return failf(file, key("literal"), "a banned-literal rule needs a literal")
		}
		if err := only("literal", "term", r.Term == ""); err != nil {
			return err
		}
		if err := only("literal", "sha256", len(r.SHA256) == 0); err != nil {
			return err
		}
	case KindRetiredTerm:
		if strings.TrimSpace(r.Term) == "" {
			return failf(file, key("term"), "a retired-term rule needs a term")
		}
		if err := only("term", "literal", r.Literal == ""); err != nil {
			return err
		}
		if err := only("term", "sha256", len(r.SHA256) == 0); err != nil {
			return err
		}
	case KindDeadHost:
		if len(r.SHA256) == 0 {
			return failf(file, key("sha256"), "a dead-host rule lists the sha256 of each lower-cased host or IPv4 (the rule file never holds the literal)")
		}
		for j, h := range r.SHA256 {
			if !hashRe.MatchString(h) {
				return failf(file, fmt.Sprintf("rules[%d].sha256[%d]", i, j), "must be 64 lower-case hex digits")
			}
		}
		if err := only("sha256", "literal", r.Literal == ""); err != nil {
			return err
		}
		if err := only("sha256", "term", r.Term == ""); err != nil {
			return err
		}
	case KindNumericClaim, KindVersionClaim:
		if err := checkClaim(file, i, r); err != nil {
			return err
		}
	default:
		return failf(file, key("kind"), "must be one of %s, not %q", strings.Join(append(append([]string(nil), Kinds...), ClaimKinds...), ", "), r.Kind)
	}
	if r.Pattern != "" && !isClaimKind(r.Kind) {
		return failf(file, key("pattern"), "%s rules do not take this key (use numeric-claim or version-claim)", r.Kind)
	}
	if r.Facts != "" && r.Kind != KindVersionClaim {
		return failf(file, key("facts"), "only version-claim rules take a facts file")
	}
	for j, g := range r.Paths {
		if !validGlob(g) {
			return failf(file, fmt.Sprintf("rules[%d].paths[%d]", i, j), "not a valid glob: %q", g)
		}
	}
	return checkExemptions(file, i, r)
}

// checkClaim validates a numeric-claim or version-claim rule and compiles its pattern.
func checkClaim(file string, i int, r *Rule) error {
	key := func(k string) string { return fmt.Sprintf("rules[%d].%s", i, k) }
	if strings.TrimSpace(r.Pattern) == "" {
		return failf(file, key("pattern"), "a %s rule needs a pattern (RE2, matched case-insensitively)", r.Kind)
	}
	re, err := regexp.Compile("(?i)" + r.Pattern)
	if err != nil {
		return failf(file, key("pattern"), "not a valid RE2 pattern: %v", err)
	}
	r.re = re
	if r.Literal != "" || r.Term != "" || len(r.SHA256) > 0 {
		return failf(file, key("pattern"), "%s rules take only a pattern, not literal, term or sha256", r.Kind)
	}
	if len(r.Paths) == 0 {
		return failf(file, key("paths"), "a %s rule lists the prose paths it applies to (Epic TRUTH D6)", r.Kind)
	}
	if r.Facts != "" && !relPathOK(r.Facts) {
		return failf(file, key("facts"), "must be a repo-relative path inside the repository: %q", r.Facts)
	}
	return nil
}

// checkExemptions validates exempt_paths, exempt_matches and exempt_regions.
func checkExemptions(file string, i int, r *Rule) error {
	if err := checkExemptPaths(file, fmt.Sprintf("rules[%d].exempt_paths", i), r.ExemptPaths); err != nil {
		return err
	}
	if (len(r.ExemptMatches) > 0 || (r.ExemptRegions != nil && *r.ExemptRegions)) && r.Kind == KindDeadHost {
		return failf(file, fmt.Sprintf("rules[%d].exempt_matches", i), "a dead-host rule takes no text exemption (exempt_paths only)")
	}
	for j, m := range r.ExemptMatches {
		key := fmt.Sprintf("rules[%d].exempt_matches[%d]", i, j)
		if len(strings.TrimSpace(m.Reason)) < 8 {
			return failf(file, key+".reason", "an exemption needs a reason of at least 8 characters")
		}
		re, err := regexp.Compile("(?i)" + m.Regex)
		if err != nil || m.Regex == "" {
			return failf(file, key+".regex", "not a valid RE2 pattern: %q", m.Regex)
		}
		r.skip = append(r.skip, re)
	}
	return nil
}

// checkExemptPaths validates a list of {glob, reason} entries under key.
func checkExemptPaths(file, key string, list []ExemptPath) error {
	for j, e := range list {
		if !validGlob(e.Glob) {
			return failf(file, fmt.Sprintf("%s[%d].glob", key, j), "not a valid glob: %q", e.Glob)
		}
		if len(strings.TrimSpace(e.Reason)) < 8 {
			return failf(file, fmt.Sprintf("%s[%d].reason", key, j), "an exemption needs a reason of at least 8 characters")
		}
	}
	return nil
}

func relPathOK(p string) bool {
	c := path.Clean(p)
	return p != "" && !strings.HasPrefix(p, "/") && c != ".." && !strings.HasPrefix(c, "../")
}

func checkAllow(file string, i int, a Allow) error {
	key := func(k string) string { return fmt.Sprintf("allow[%d].%s", i, k) }
	if a.Rule == "" {
		return failf(file, key("rule"), "an allowlist entry names the rule it exempts")
	}
	if _, err := path.Match(a.Rule, "x"); err != nil {
		return failf(file, key("rule"), "not a valid rule id or glob: %q", a.Rule)
	}
	if len(strings.TrimSpace(a.Reason)) < 8 {
		return failf(file, key("reason"), "an allowlist entry needs a reason of at least 8 characters")
	}
	if len(a.Paths) == 0 {
		return failf(file, key("paths"), "an allowlist entry names the paths it exempts")
	}
	for j, g := range a.Paths {
		if !validGlob(g) {
			return failf(file, fmt.Sprintf("allow[%d].paths[%d]", i, j), "not a valid glob: %q", g)
		}
	}
	return nil
}
