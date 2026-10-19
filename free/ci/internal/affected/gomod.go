package affected

// Purpose: go.mod and go.work reading for the Go import graph: which modules
// exist, which directories they cover and which of them a package can see
// (its own module, that module's local replaces, and the members of a go.work
// that lists it).
// Inputs:  the walked tree and its fs.FS.
// Outputs: modSet with visibleFrom (the modules a directory may import) and
// resolveImport (import path -> repository directories).
// Constraints: pure; testdata/ manifests are fixtures and are ignored; a
// replace or use target that is not a ./ or ../ directory is uncertain.

import (
	"io/fs"
	"path"
	"strconv"
	"strings"
)

type goModule struct{ dir, path string }

// modFile is one go.mod or go.work: the module it declares, the directories
// it uses (go.work) and its local replace targets.
type modFile struct {
	dir      string
	module   string
	uses     []string
	replaces []goModule
	badPath  bool // a replace or use target that is not a plain relative directory
}

// modSet indexes the go.mod and go.work files of a tree.
type modSet struct {
	byDir map[string]modFile // go.mod by directory
	works []modFile
}

func readModFiles(fsys fs.FS, t tree, g *Graph) modSet {
	ms := modSet{byDir: map[string]modFile{}}
	for _, f := range t.files {
		base := path.Base(f)
		if (base != "go.mod" && base != "go.work") || !notTestdata(dirOf(f)) {
			continue
		}
		src, ok := readFile(fsys, f)
		if !ok {
			g.uncertain(f, base+" unreadable")
			continue
		}
		mf := parseModFile(dirOf(f), string(src))
		if mf.badPath {
			g.uncertain(f, base+" names a path that is not a relative directory")
		}
		switch {
		case base == "go.work":
			ms.works = append(ms.works, mf)
		case mf.module == "":
			g.uncertain(f, "go.mod without module line")
		default:
			ms.byDir[mf.dir] = mf
		}
	}
	return ms
}

// parseModFile reads the module, use and local replace directives.
func parseModFile(dir, src string) modFile {
	mf := modFile{dir: dir}
	block := ""
	for _, line := range strings.Split(src, "\n") {
		f, bad := tokenize(line)
		mf.badPath = mf.badPath || bad
		if len(f) == 0 {
			continue
		}
		if block != "" {
			if f[0] == ")" {
				block = ""
			} else {
				mf.directive(block, f)
			}
			continue
		}
		if len(f) >= 2 && f[1] == "(" {
			block = f[0]
			continue
		}
		mf.directive(f[0], f[1:])
	}
	return mf
}

// tokenize splits a go.mod line into fields: whitespace separates, a
// double-quoted string (Go escapes decoded) or a backquoted string groups,
// `//` outside quotes starts a comment. bad is true for an unterminated or
// invalid quoted string.
func tokenize(line string) (out []string, bad bool) {
	var cur strings.Builder
	in := false
	flush := func() {
		if in {
			out = append(out, cur.String())
			cur.Reset()
			in = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"' || c == '`':
			j := i + 1
			for j < len(line) && line[j] != c {
				if c == '"' && line[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(line) {
				return out, true
			}
			s, err := strconv.Unquote(line[i : j+1])
			if err != nil {
				return out, true
			}
			cur.WriteString(s)
			in, i = true, j
		case c == '(' || c == ')': // `replace(` and `use(` are valid: the paren is its own token
			flush()
			out = append(out, string(c))
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			flush()
			return out, false
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	flush()
	return out, false
}

func (mf *modFile) directive(kind string, f []string) {
	switch kind {
	case "module":
		if len(f) >= 1 {
			mf.module = f[0]
		}
	case "use":
		if len(f) >= 1 {
			if d := joinDir(mf.dir, f[0]); d != "" && isRelDir(f[0]) {
				mf.uses = append(mf.uses, d)
			} else {
				mf.badPath = true
			}
		}
	case "replace":
		for i, tok := range f {
			if tok != "=>" || i == 0 || i+1 >= len(f) {
				continue
			}
			to := f[i+1]
			switch {
			case isRelDir(to):
				if d := joinDir(mf.dir, to); d != "" {
					mf.replaces = append(mf.replaces, goModule{d, f[0]})
				} else {
					mf.badPath = true
				}
			case strings.HasPrefix(to, "/") || strings.Contains(to, `\`) || (len(to) > 1 && to[1] == ':'):
				mf.badPath = true // an absolute or Windows path: not modelled
			}
		}
	}
}

// isRelDir reports whether s is a ./ or ../ directory path (or . and ..).
func isRelDir(s string) bool {
	return s == "." || s == ".." || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}

// visibleFrom lists the in-repo modules a package in dir can import: the
// module that holds it, that module's local replaces, and (through a go.work
// that uses the module) the other used modules and the go.work replaces.
func (ms modSet) visibleFrom(dir string) []goModule {
	var holder *modFile
	for d := dir; ; d = path.Dir(d) {
		if mf, ok := ms.byDir[d]; ok {
			holder = &mf
			break
		}
		if d == "." || d == "/" {
			return nil
		}
	}
	out := []goModule{{holder.dir, holder.module}}
	out = append(out, holder.replaces...)
	for _, w := range ms.works {
		member := false
		for _, u := range w.uses {
			member = member || u == holder.dir
		}
		if !member {
			continue
		}
		for _, u := range w.uses {
			if mf, ok := ms.byDir[u]; ok {
				out = append(out, goModule{mf.dir, mf.module})
				out = append(out, mf.replaces...) // workspace mode merges every member's replaces
			}
		}
		out = append(out, w.replaces...)
	}
	return out
}

// resolveImport maps an import path to repository directories through the
// longest matching visible module path (several when a path is declared and
// replaced in more than one place). inRepo is false for the standard library
// and every module the repository does not hold.
func resolveImport(visible []goModule, imp string) (dirs []string, inRepo bool) {
	longest := -1
	for _, m := range visible {
		if (imp == m.path || strings.HasPrefix(imp, m.path+"/")) && len(m.path) > longest {
			longest = len(m.path)
		}
	}
	if longest < 0 {
		return nil, false
	}
	for _, m := range visible {
		if len(m.path) == longest && (imp == m.path || strings.HasPrefix(imp, m.path+"/")) {
			if d := joinDir(m.dir, strings.TrimPrefix(strings.TrimPrefix(imp, m.path), "/")); d != "" {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs, true
}
