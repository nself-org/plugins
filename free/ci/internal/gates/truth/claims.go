package truth

// Purpose: the shared types and defaults of the claim rules (numeric-claim,
// version-claim) and of the exemptions every rule kind may carry.
// Inputs:  decoded rule files.
// Outputs: ExemptPath, ExemptMatch, Override, the default exempt paths and the
// helpers the scanner uses to decide whether a hit is exempt.
// Constraints: Epic TRUTH D6. A claim rule never reads Go source, go.mod,
// go.sum or go.work, whatever its paths say; changelogs, release notes, blog,
// ADRs, history, testdata, vendor and node_modules are exempt by default; every
// exemption states a reason (8+ characters), or the rule file is E644.

import (
	"regexp"
	"sort"
)

// ExemptPath skips a rule on files matching Glob, with the reason written down.
type ExemptPath struct {
	Glob   string `yaml:"glob"`
	Reason string `yaml:"reason"`
}

// ExemptMatch exempts a hit that sits inside a match of Regex on its line.
type ExemptMatch struct {
	Regex  string `yaml:"regex"`
	Reason string `yaml:"reason"`
}

// Override adds exemptions (and, for a version-claim, the facts file) to a rule
// declared in a file this file includes.
type Override struct {
	ExemptPaths []ExemptPath `yaml:"exempt_paths,omitempty"`
	Facts       string       `yaml:"facts,omitempty"`
}

type fileOverride struct {
	File string
	Rule string
	Override
}

// defaultExemptPaths are exempt for the claim kinds: history is correct as of
// its date, test data states anything, vendored trees are not ours, and source
// code is not prose (D6).
var defaultExemptPaths = []string{
	"CHANGELOG*", "Changelog*", "changelog*", "RELEASE-*", "RELEASE_NOTES*", "release-notes*", "RELEASES*",
	"**/releases/**", "**/blog/**", "**/adr/**", "**/adrs/**", "**/history/**", "**/HISTORY*",
	"**/testdata/**", "**/vendor/**", "**/node_modules/**",
	"*.go", "go.mod", "go.sum", "go.work", "go.work.sum",
}

func isClaimKind(kind string) bool { return kind == KindNumericClaim || kind == KindVersionClaim }

// usesRegions reports whether the rule honours GENERATED regions, JSX
// expressions and truth: exempt comments.
func (r *Rule) usesRegions() bool {
	if r.ExemptRegions != nil {
		return *r.ExemptRegions
	}
	return isClaimKind(r.Kind)
}

// pathExempt reports whether the rule skips the file.
func (r *Rule) pathExempt(rel string) bool {
	if isClaimKind(r.Kind) && matchAny(defaultExemptPaths, rel) {
		return true
	}
	for _, e := range r.ExemptPaths {
		if matchGlob(e.Glob, rel) {
			return true
		}
	}
	return false
}

// textExempt reports whether the hit [start,end) of line sits inside a match
// of one of the rule's exempt_matches regexes.
func (r *Rule) textExempt(line string, start, end int) bool {
	return anySpanCovers(r.skip, line, start, end)
}

func anySpanCovers(res []*regexp.Regexp, line string, start, end int) bool {
	for _, re := range res {
		for _, loc := range re.FindAllStringIndex(line, -1) {
			if loc[0] <= start && end <= loc[1] {
				return true
			}
		}
	}
	return false
}

// addOverrides records the overrides block of one rule file; they apply once
// every file is loaded, so a file can override a rule it includes.
func (rs *RuleSet) addOverrides(file string, ov map[string]Override) error {
	ids := make([]string, 0, len(ov))
	for id := range ov {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		o := ov[id]
		if err := checkExemptPaths(file, "overrides."+id+".exempt_paths", o.ExemptPaths); err != nil {
			return err
		}
		if o.Facts != "" && !relPathOK(o.Facts) {
			return failf(file, "overrides."+id+".facts", "must be a repo-relative path inside the repository: %q", o.Facts)
		}
		rs.overrides = append(rs.overrides, fileOverride{File: file, Rule: id, Override: o})
	}
	return nil
}

// applyOverrides merges every override into its rule; a rule id nobody
// declared is E644.
func (rs *RuleSet) applyOverrides(ids map[string]string) error {
	for _, o := range rs.overrides {
		if _, ok := ids[o.Rule]; !ok {
			return failf(o.File, "overrides."+o.Rule, "no rule has the id %q", o.Rule)
		}
		for i := range rs.Rules {
			r := &rs.Rules[i]
			if r.ID != o.Rule {
				continue
			}
			if o.Facts != "" && r.Kind != KindVersionClaim {
				return failf(o.File, "overrides."+o.Rule+".facts", "only a version-claim rule takes a facts file")
			}
			r.ExemptPaths = append(r.ExemptPaths, o.ExemptPaths...)
			if o.Facts != "" {
				r.Facts = o.Facts
			}
		}
	}
	return nil
}
