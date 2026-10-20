package remote

// Purpose: decide which declaration selects a path an agent reports, and what
// record name the collector would have given it, without touching a file: the
// coordinator sees only `artifact-ref{path}`.
// Inputs:  the attempt's declarations (records.Decl, in order) and a
// workdir-relative path.
// Outputs: the declaration, the record name, whether any declaration selects it.
// Constraints: mirrors records.expand (components match with path.Match, `**`
// spans directories only, a matched directory selects everything below it) and
// records.recordName; match_test.go checks the two agree on a real tree.

import (
	"path"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

func hasMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// maxComponents bounds the path a node may name and the pattern a job may declare (the collector
// bounds the pattern at 64 parts; a path deeper than this selects nothing real).
const maxComponents = 256

// selects reports whether pattern selects rel (a file path).
func selects(pattern, rel string) bool {
	pc, rc := strings.Split(pattern, "/"), strings.Split(rel, "/")
	if len(pc) > maxComponents || len(rc) > maxComponents {
		return false
	}
	memo := map[[2]int]bool{}
	seen := map[[2]int]bool{}
	var match func(i, j int) bool
	match = func(i, j int) bool {
		key := [2]int{i, j}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
		memo[key] = matchAt(pc, rc, i, j, match)
		return memo[key]
	}
	return match(0, 0)
}

// matchAt decides whether pc[i:] selects rc[j:], asking match for the sub-problems (memoised, so a
// pattern with several "**" parts is polynomial).
func matchAt(pc, rc []string, i, j int, match func(i, j int) bool) bool {
	if i == len(pc) {
		return true // the pattern named rel or a directory above it
	}
	if pc[i] == "**" {
		if match(i+1, j) {
			return true
		}
		return len(rc)-j > 1 && match(i, j+1) // "**" consumes directories, never the file itself
	}
	if j == len(rc) {
		return false
	}
	if hasMeta(pc[i]) {
		if ok, err := path.Match(pc[i], rc[j]); err != nil || !ok {
			return false
		}
	} else if pc[i] != rc[j] {
		return false
	}
	return match(i+1, j+1)
}

// literalPrefix is the leading components of a pattern that hold no wildcard.
func literalPrefix(p string) string {
	var keep []string
	for _, c := range strings.Split(p, "/") {
		if hasMeta(c) {
			break
		}
		keep = append(keep, c)
	}
	return strings.Join(keep, "/")
}

func recordName(d records.Decl, rel string) string {
	literal := !hasMeta(d.Path) && rel == d.Path
	switch {
	case d.Name == "":
		return rel
	case literal:
		return d.Name
	}
	return d.Name + "/" + strings.TrimPrefix(rel, strings.TrimSuffix(literalPrefix(d.Path), "/")+"/")
}

// Select returns the nth (0-based) declaration, in order, that selects rel and the record name it
// gives: a path two declarations select is reported once per declaration, as the collector stores it.
func Select(decls []records.Decl, rel string, nth int) (records.Decl, string, bool) {
	for _, d := range decls {
		if !selects(d.Path, rel) {
			continue
		}
		if nth == 0 {
			return d, recordName(d, rel), true
		}
		nth--
	}
	return records.Decl{}, "", false
}
