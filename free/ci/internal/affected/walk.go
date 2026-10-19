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
	truncated bool
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
		if d.Type()&fs.ModeSymlink != 0 {
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

// matchGlob reports whether the slash path p matches pattern; `*` and `?`
// stay inside one segment, `**` spans any number of segments (also zero) and
// `{a,b}` alternatives are expanded.
func matchGlob(pattern, p string) bool {
	for _, alt := range expandBraces(pattern) {
		if matchSegs(strings.Split(path.Clean(alt), "/"), strings.Split(p, "/")) {
			return true
		}
	}
	return false
}

// expandBraces expands the first (outermost) `{a,b,...}` group, recursively.
func expandBraces(p string) []string {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}
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
			if depth--; depth == 0 {
				parts = append(parts, p[last:i])
				var out []string
				for _, part := range parts {
					out = append(out, expandBraces(p[:open]+part+p[i+1:])...)
				}
				return out
			}
		}
	}
	return []string{p} // unbalanced: literal
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
