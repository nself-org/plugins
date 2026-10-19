package affected

// Purpose: the bounded file walk shared by the Go and pnpm graph builders.
// Inputs:  an fs.FS snapshot of the repository (P7-CI-33 snapshot source).
// Outputs: a sorted list of slash-separated file paths and a flag that says
// the walk was cut short (more than maxFiles files).
// Constraints: pure over fs.FS; never follows symlinks; skips vendor/,
// node_modules/, .git/ and .nself/ by name and a Cargo `target/`; walks
// dist/, build/ and target/ otherwise (a Go package named `build` is still a
// package, a workspace package may live there) under a per-directory budget
// of maxSoft files, past which the walk is uncertain. The depth is not
// capped (a package deeper than any limit would be invisible and its
// dependants missed, the unsafe direction); the file cap bounds the work.

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	maxFiles = 50000
	maxSoft  = 20000
	maxRead  = 8 << 20
)

var hardIgnored = map[string]bool{"vendor": true, "node_modules": true, ".git": true, ".nself": true}
var softIgnored = map[string]bool{"dist": true, "build": true, "target": true}

type tree struct {
	files     []string
	set       map[string]bool
	links     []string // symlinks named like source or manifests: not followed
	truncated bool
}

// isSourceName is true for the file names the graph reads.
func isSourceName(base string) bool {
	return strings.HasSuffix(base, ".go") || base == "go.mod" || base == "go.work" || base == "package.json" || base == "pnpm-workspace.yaml"
}

func walk(fsys fs.FS) (tree, error) {
	t := tree{set: map[string]bool{}}
	soft, softCount, softFull := "", 0, false
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if soft != "" && strings.HasPrefix(p, soft+"/") {
			if softCount++; softCount > maxSoft {
				t.truncated, softFull = true, true
			}
			if softFull {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			switch {
			case hardIgnored[d.Name()]:
				return fs.SkipDir
			case d.Name() == "target" && fileExists(fsys, path.Join(path.Dir(p), "Cargo.toml")):
				return fs.SkipDir // cargo build output
			case softIgnored[d.Name()]:
				soft, softCount, softFull = p, 0, false
			}
			return nil
		}
		if !d.Type().IsRegular() { // symlinks, FIFOs and devices are never read
			if d.Type()&fs.ModeSymlink != 0 && isSourceName(path.Base(p)) {
				t.links = append(t.links, p)
			}
			return nil
		}
		if len(t.files) >= maxFiles {
			t.truncated = true
			return fs.SkipAll
		}
		t.files = append(t.files, p)
		t.set[p] = true
		return nil
	})
	sort.Strings(t.files)
	return t, err
}

func fileExists(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	return err == nil
}

// readFile reads p up to maxRead bytes; ok is false when it cannot be read
// whole.
func readFile(fsys fs.FS, p string) ([]byte, bool) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil || len(b) > maxRead {
		return nil, false
	}
	return b, true
}

// dirOf is the directory of a file path, "." for the repository root.
func dirOf(p string) string { return path.Dir(p) }

// within reports whether dir is base or lies under it; "." holds everything.
func within(dir, base string) bool {
	return base == "." || dir == base || strings.HasPrefix(dir, base+"/")
}

// joinDir joins a directory and a relative path, returning "" when the
// result leaves the repository.
func joinDir(dir, rel string) string {
	out := path.Clean(path.Join(dir, rel))
	if out == ".." || strings.HasPrefix(out, "../") || strings.HasPrefix(out, "/") {
		return ""
	}
	return out
}

// maxBraceAlts bounds the alternatives one glob may expand to.
const maxBraceAlts = 1024

// matchGlob reports whether the slash path p matches pattern; `*` and `?`
// stay inside one segment, `**` spans any number of segments (also zero),
// `{a,b}` alternatives and `{1..3}` ranges are expanded.
func matchGlob(pattern, p string) bool {
	alts, _ := expandBraces(pattern)
	for _, alt := range alts {
		if matchSegs(strings.Split(path.Clean(alt), "/"), strings.Split(p, "/")) {
			return true
		}
	}
	return false
}

// expandBraces expands the first (outermost) `{...}` group, recursively. ok
// is false when the result would pass maxBraceAlts (the list is cut there).
func expandBraces(p string) (out []string, ok bool) {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}, true
	}
	depth, last := 0, open+1
	var parts []string
	for i := open; i < len(p); i++ {
		switch p[i] {
		case '{':
			depth++
		case ',':
			if depth == 1 {
				parts, last = append(parts, p[last:i]), i+1
			}
		case '}':
			if depth--; depth != 0 {
				continue
			}
			parts = append(parts, p[last:i])
			if len(parts) == 1 && strings.Contains(parts[0], "..") {
				r, isRange, wide := braceRange(parts[0])
				if wide || !isRange {
					return []string{p}, false // a range form this matcher does not model
				}
				parts = r
			}
			for _, part := range parts {
				sub, subOK := expandBraces(p[:open] + part + p[i+1:])
				out = append(out, sub...)
				if !subOK || len(out) > maxBraceAlts {
					if len(out) > maxBraceAlts {
						out = out[:maxBraceAlts]
					}
					return out, false // stop at once: the cut propagates up
				}
			}
			return out, true
		}
	}
	return []string{p}, true // unbalanced: literal
}

// braceRange expands `1..3` (integers, either direction) or `a..c` (single
// letters). isRange is false when s is not a range; tooWide when it would
// pass maxBraceAlts.
func braceRange(s string) (out []string, isRange, tooWide bool) {
	lo, hi, found := strings.Cut(s, "..")
	if !found {
		return nil, false, false
	}
	if a, err1 := strconv.Atoi(lo); err1 == nil {
		b, err2 := strconv.Atoi(hi)
		if err2 != nil || (len(lo) > 1 && lo[0] == '0') || (len(hi) > 1 && hi[0] == '0') {
			return nil, false, false // padded or step forms: not modelled
		}
		if a < -(1<<30) || a > 1<<30 || b < -(1<<30) || b > 1<<30 || abs(b-a) >= maxBraceAlts {
			return nil, true, true
		}
		step := 1
		if b < a {
			step = -1
		}
		for n := a; ; n += step {
			out = append(out, strconv.Itoa(n))
			if n == b {
				return out, true, false
			}
		}
	}
	if len(lo) == 1 && len(hi) == 1 {
		step := 1
		if hi[0] < lo[0] {
			step = -1
		}
		for c := int(lo[0]); ; c += step {
			out = append(out, string(rune(c)))
			if c == int(hi[0]) {
				return out, true, false
			}
		}
	}
	return nil, false, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func matchSegs(pat, segs []string) bool {
	memo := map[[2]int]bool{}
	var rec func(i, j int) bool
	rec = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		v := false
		switch {
		case i == len(pat):
			v = j == len(segs)
		case pat[i] == "**":
			for k := j; k <= len(segs) && !v; k++ {
				v = rec(i+1, k)
			}
		case j < len(segs):
			ok, err := path.Match(pat[i], segs[j])
			v = err == nil && ok && rec(i+1, j+1)
		}
		memo[key] = v
		return v
	}
	return rec(0, 0)
}
