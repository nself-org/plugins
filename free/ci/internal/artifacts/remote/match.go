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

// selects reports whether pattern selects rel (a file path).
func selects(pattern, rel string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchParts(pc, rc []string) bool {
	if len(pc) == 0 {
		return true // the pattern named rel or a directory above it
	}
	if pc[0] == "**" {
		if matchParts(pc[1:], rc) {
			return true
		}
		return len(rc) > 1 && matchParts(pc, rc[1:]) // "**" consumes directories, never the file itself
	}
	if len(rc) == 0 {
		return false
	}
	if hasMeta(pc[0]) {
		if ok, err := path.Match(pc[0], rc[0]); err != nil || !ok {
			return false
		}
	} else if pc[0] != rc[0] {
		return false
	}
	return matchParts(pc[1:], rc[1:])
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
