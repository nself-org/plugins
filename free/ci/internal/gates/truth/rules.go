package truth

// Purpose: the rule file schema and its loader.
// Inputs:  repo-relative rule file paths under a root directory.
// Outputs: a merged RuleSet (rules, allowlist, skip globs, the files read).
// Constraints: unknown keys, duplicate rule ids, an allowlist entry without a
// reason or for an unknown rule, a malformed hash and a rule file outside the
// root are all E644 naming the file and key; `include` pulls in other rule
// files (repo-relative, at most 8 deep, cycles refused); no network, no exec.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule kinds.
const (
	KindBannedLiteral = "banned-literal"
	KindRetiredTerm   = "retired-term"
	KindDeadHost      = "dead-host"

	// Claim kinds (P7-TRUTH-04, Epic TRUTH D5/D6): a regex pattern over prose.
	KindNumericClaim = "numeric-claim"
	KindVersionClaim = "version-claim"
)

// Kinds lists the base rule kinds in report order; ClaimKinds follow them in a
// report only when the loaded rules hold a rule of that kind.
var Kinds = []string{KindBannedLiteral, KindRetiredTerm, KindDeadHost}

// ClaimKinds are the rule kinds claims.yaml adds.
var ClaimKinds = []string{KindNumericClaim, KindVersionClaim}

const maxIncludeDepth = 8

// Rule is one rule. Exactly one of Literal, Term and SHA256 is set, by kind.
type Rule struct {
	ID      string   `yaml:"id"`
	Kind    string   `yaml:"kind"`
	Message string   `yaml:"message"`
	Literal string   `yaml:"literal,omitempty"` // banned-literal: case-sensitive substring
	Term    string   `yaml:"term,omitempty"`    // retired-term: case-insensitive whole term
	SHA256  []string `yaml:"sha256,omitempty"`  // dead-host: sha256 of the lower-cased IPv4 or host name
	Paths   []string `yaml:"paths,omitempty"`   // limits the rule to these globs (default: every file)

	Pattern       string        `yaml:"pattern,omitempty"`        // claim kinds: case-insensitive RE2
	Facts         string        `yaml:"facts,omitempty"`          // version-claim: repo-relative JSON/YAML file whose `version` is current
	ExemptPaths   []ExemptPath  `yaml:"exempt_paths,omitempty"`   // globs the rule skips, each with a reason
	ExemptMatches []ExemptMatch `yaml:"exempt_matches,omitempty"` // a hit inside a match of one of these regexes is exempt
	ExemptRegions *bool         `yaml:"exempt_regions,omitempty"` // honour GENERATED regions, JSX expressions and truth: exempt comments (default: claim kinds)

	File string         `yaml:"-"` // the rule file that declared it
	re   *regexp.Regexp // compiled Pattern (claim kinds)
	skip []*regexp.Regexp
}

// Allow exempts findings of one rule on matching paths, with a stated reason.
type Allow struct {
	Rule   string   `yaml:"rule"` // a rule id, or a glob over ids such as `retired-*`
	Paths  []string `yaml:"paths"`
	Reason string   `yaml:"reason"`
}

type ruleFile struct {
	Version int      `yaml:"version"`
	Include []string `yaml:"include,omitempty"`
	Skip    []string `yaml:"skip,omitempty"`
	Rules   []Rule   `yaml:"rules"`
	Allow   []Allow  `yaml:"allow,omitempty"`

	Overrides map[string]Override `yaml:"overrides,omitempty"` // per-rule extra exemptions for rules this file includes
}

// RuleSet is every rule file merged.
type RuleSet struct {
	Rules []Rule
	Allow []Allow
	Skip  []string
	Files []string // repo-relative rule files read, in order

	overrides []fileOverride
}

var (
	hashRe    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	idRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	yamlField = regexp.MustCompile(`line (\d+): field (\S+) not found`)
)

// Load reads the rule files (and what they include) under root.
func Load(root string, files []string) (RuleSet, error) {
	var rs RuleSet
	seen := map[string]bool{}
	ids := map[string]string{}
	for _, f := range files {
		if err := rs.load(root, f, 0, nil, seen, ids); err != nil {
			return RuleSet{}, err
		}
	}
	for i, a := range rs.Allow {
		if !anyRuleMatches(a.Rule, ids) {
			return RuleSet{}, failf(rs.Files[0], fmt.Sprintf("allow[%d].rule", i), "no rule has the id %q", a.Rule)
		}
	}
	if err := rs.applyOverrides(ids); err != nil {
		return RuleSet{}, err
	}
	return rs, nil
}

func (rs *RuleSet) load(root, rel string, depth int, stack []string, seen map[string]bool, ids map[string]string) error {
	clean := path.Clean(filepath.ToSlash(rel))
	if rel == "" || filepath.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") {
		return failf(rel, "", "a rule file path must be repo-relative and stay inside the repository")
	}
	for _, s := range stack {
		if s == clean {
			return failf(clean, "include", "include cycle: %s", strings.Join(append(stack, clean), " -> "))
		}
	}
	if depth > maxIncludeDepth {
		return failf(clean, "include", "include nesting is deeper than %d", maxIncludeDepth)
	}
	if seen[clean] {
		return nil
	}
	full := filepath.Join(root, filepath.FromSlash(clean))
	if info, err := os.Lstat(full); err == nil && !info.Mode().IsRegular() {
		return failf(clean, "", "a rule file must be a regular file (not a symlink or directory)")
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return failf(clean, "", "cannot read the rule file: %v", errors.Unwrap(err))
	}
	var f ruleFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return yamlError(clean, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return failf(clean, "", "a rule file holds one YAML document; remove the extra `---` document")
	}
	seen[clean] = true
	rs.Files = append(rs.Files, clean)
	if f.Version != 1 {
		return failf(clean, "version", "must be 1, not %d", f.Version)
	}
	for i := range f.Rules {
		r := f.Rules[i]
		r.File = clean
		if err := checkRule(clean, i, &r); err != nil {
			return err
		}
		if first, dup := ids[r.ID]; dup {
			return failf(clean, fmt.Sprintf("rules[%d].id", i), "duplicate rule id %q (first in %s)", r.ID, first)
		}
		ids[r.ID] = clean
		rs.Rules = append(rs.Rules, r)
	}
	for i, a := range f.Allow {
		if err := checkAllow(clean, i, a); err != nil {
			return err
		}
		rs.Allow = append(rs.Allow, a)
	}
	if err := rs.addOverrides(clean, f.Overrides); err != nil {
		return err
	}
	for i, g := range f.Skip {
		if !validGlob(g) {
			return failf(clean, fmt.Sprintf("skip[%d]", i), "not a valid glob: %q", g)
		}
		rs.Skip = append(rs.Skip, g)
	}
	for i, inc := range f.Include {
		next := append(append([]string(nil), stack...), clean)
		if err := rs.load(root, inc, depth+1, next, seen, ids); err != nil {
			var te *Error
			if errors.As(err, &te) && te.File == path.Clean(filepath.ToSlash(inc)) && te.Key == "" {
				return failf(clean, fmt.Sprintf("include[%d]", i), "%s: %s", te.File, te.Msg)
			}
			return err
		}
	}
	return nil
}

// anyRuleMatches reports whether the allowlist rule pattern (an id, or a
// glob such as `retired-*`) names at least one loaded rule.
func anyRuleMatches(pattern string, ids map[string]string) bool {
	for id := range ids {
		if ruleMatches(pattern, id) {
			return true
		}
	}
	return false
}

func ruleMatches(pattern, id string) bool {
	ok, _ := path.Match(pattern, id)
	return ok
}

func yamlError(file string, err error) error {
	var te *yaml.TypeError
	if errors.As(err, &te) {
		for _, m := range te.Errors {
			if g := yamlField.FindStringSubmatch(m); g != nil {
				return failf(file, g[2], "unknown key (line %s)", g[1])
			}
		}
		return failf(file, "", "%s", strings.Join(te.Errors, "; "))
	}
	return failf(file, "", "not valid YAML: %v", err)
}
