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
)

// Kinds lists the rule kinds in report order.
var Kinds = []string{KindBannedLiteral, KindRetiredTerm, KindDeadHost}

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

	File string `yaml:"-"` // the rule file that declared it
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
}

// RuleSet is every rule file merged.
type RuleSet struct {
	Rules []Rule
	Allow []Allow
	Skip  []string
	Files []string // repo-relative rule files read, in order
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
		if err := checkRule(clean, i, r); err != nil {
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

func checkRule(file string, i int, r Rule) error {
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
	default:
		return failf(file, key("kind"), "must be one of %s, not %q", strings.Join(Kinds, ", "), r.Kind)
	}
	for j, g := range r.Paths {
		if !validGlob(g) {
			return failf(file, fmt.Sprintf("rules[%d].paths[%d]", i, j), "not a valid glob: %q", g)
		}
	}
	return nil
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
