package truth

// Purpose: walk a tree and apply a RuleSet to every text file.
// Inputs:  a root directory and a RuleSet.
// Outputs: Findings (rule, path, line) with allowlisted hits counted apart.
// Constraints: only regular files are read (symlinks are not followed); `.git`,
// `node_modules` and `.claude/worktrees` are never entered; files over 4 MiB or
// holding a NUL byte in the first 8 KiB are skipped as binary and counted;
// the rule files themselves are never scanned (a retired term in a rule file is
// the rule); a finding carries no matched text for a dead host; no network.

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const maxFileBytes = 4 << 20

var errNoFiles = errors.New("no text file was read (an empty tree, or every file skipped)")

// Finding is one rule broken at one line.
type Finding struct {
	RuleID  string
	Kind    string
	Path    string // slash-separated, relative to the scanned root
	Line    int
	Message string
	Hash    string // dead-host: the first 12 hex digits of the token hash
}

// Result is the outcome of one scan.
type Result struct {
	Findings []Finding
	Allowed  int // hits an allowlist entry exempted
	Files    int // files read
	Skipped  int // files skipped as binary or too large
}

// Scan applies rs to every file under root. selfFiles are repo-relative paths
// of rule files: a file at one of those paths is never scanned, in any tree
// (a clone of the repository that holds the rules carries its own copy).
func Scan(root string, rs RuleSet, selfFiles []string) (Result, error) {
	c := compile(rs)
	self := map[string]bool{}
	for _, f := range selfFiles {
		self[filepath.ToSlash(f)] = true
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Result{}, err
	}
	root = real
	var res Result
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || rel == ".claude/worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || self[rel] || matchAny(rs.Skip, rel) {
			return nil
		}
		return scanFile(p, rel, rs, c, &res)
	})
	if err != nil {
		return Result{}, err
	}
	if res.Files == 0 {
		return Result{}, errNoFiles
	}
	sort.SliceStable(res.Findings, func(i, j int) bool {
		a, b := res.Findings[i], res.Findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.RuleID < b.RuleID
	})
	return res, nil
}

func scanFile(p, rel string, rs RuleSet, c compiled, res *Result) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if info.Size() > maxFileBytes {
		res.Skipped++
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if bytes.IndexByte(raw[:min(len(raw), 8192)], 0) >= 0 {
		res.Skipped++
		return nil
	}
	res.Files++
	for n, b := range bytes.Split(raw, []byte{'\n'}) {
		n++
		line := strings.TrimSuffix(string(b), "\r")
		add := func(r *Rule, hash string) {
			if len(r.Paths) > 0 && !matchAny(r.Paths, rel) {
				return
			}
			if allowed(rs.Allow, r.ID, rel) {
				res.Allowed++
				return
			}
			res.Findings = append(res.Findings, Finding{RuleID: r.ID, Kind: r.Kind, Path: rel, Line: n, Message: r.Message, Hash: hash})
		}
		for _, r := range c.literalHits(line) {
			add(r, "")
		}
		for _, r := range c.termHits(line) {
			add(r, "")
		}
		for _, h := range c.hostHits(line) {
			add(h.Rule, h.Prefix)
		}
	}
	return nil
}

func allowed(list []Allow, rule, rel string) bool {
	for _, a := range list {
		if ruleMatches(a.Rule, rule) && matchAny(a.Paths, rel) {
			return true
		}
	}
	return false
}

// Excerpt renders up to max findings, one per line, without any matched text.
func Excerpt(found []Finding, max int) string {
	var b strings.Builder
	for i, f := range found {
		if i == max {
			b.WriteString("... and more\n")
			break
		}
		b.WriteString(f.Path + ":" + strconv.Itoa(f.Line) + ": " + f.RuleID)
		if f.Hash != "" {
			b.WriteString(" (token sha256 " + f.Hash + ")")
		}
		b.WriteString(": " + f.Message + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
