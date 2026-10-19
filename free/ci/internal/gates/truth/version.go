package truth

// Purpose: the version-claim rule kind: a semver literal in README-like prose
// that must equal the current version, or stand in a generated region.
// Inputs:  a claim hit and the facts file the rule names (or none).
// Outputs: whether the literal is the current version.
// Constraints: Epic TRUTH D6. `facts` is a repo-relative JSON or YAML file with
// a top-level `version`; the leading `v` is ignored on both sides; with no
// facts file every literal fails (nothing says which version is current); the
// facts file is read once per scan, never through a symlink, and an unreadable
// or version-less file is E644 naming the rule file and the rule.

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	semverRe  = regexp.MustCompile(`v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.]+)?(?:\+[0-9A-Za-z.]+)?`)
	semverAll = regexp.MustCompile(`^` + semverRe.String() + `$`)
)

// loadFacts reads the version of every facts file the version-claim rules name.
func loadFacts(root string, rs RuleSet) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rs.Rules {
		if r.Facts == "" {
			continue
		}
		clean := path.Clean(filepath.ToSlash(r.Facts))
		if _, done := out[clean]; done {
			continue
		}
		v, err := readFactsVersion(root, clean)
		if err != nil {
			file, key := r.File, "rules."+r.ID+".facts"
			if r.factsFile != "" {
				file, key = r.factsFile, "overrides."+r.ID+".facts"
			}
			return nil, failf(file, key, "%s: %v (resolved against the scanned root)", clean, err)
		}
		out[clean] = v
	}
	return out, nil
}

func readFactsVersion(root, rel string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	// A symlinked parent directory could lead out of the scanned tree (a clone chooses its own "current version").
	realRoot, rerr := filepath.EvalSymlinks(root)
	realFull, ferr := filepath.EvalSymlinks(full)
	if rerr != nil || ferr != nil || !strings.HasPrefix(realFull, realRoot+string(filepath.Separator)) {
		return "", errors.New("the facts file cannot be read inside the repository")
	}
	info, err := os.Lstat(full)
	if err != nil {
		return "", errors.New("the facts file cannot be read")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("the facts file must be a regular file (not a symlink or directory)")
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", errors.New("the facts file cannot be read")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", errors.New("the facts file is not a JSON or YAML mapping")
	}
	v, ok := doc["version"].(string)
	if !ok || !semverAll.MatchString(v) {
		return "", fmt.Errorf("the facts file has no top-level semver `version` string")
	}
	return strings.TrimPrefix(v, "v"), nil
}

// versionOK reports whether the semver inside the hit's text is the current
// version (false when the rule has no facts or the text holds no semver).
func versionOK(r *Rule, text string, facts map[string]string) bool {
	cur, ok := facts[path.Clean(filepath.ToSlash(r.Facts))]
	if r.Facts == "" || !ok {
		return false
	}
	lit := semverRe.FindString(text)
	return lit != "" && strings.TrimPrefix(lit, "v") == cur
}
