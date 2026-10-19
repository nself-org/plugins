package affected

// Purpose: the Go import graph (Epic D15). Every .go file, test files
// included, is parsed with go/parser ImportsOnly; build tags are ignored, so
// the graph is a superset of what any one build sees. An import path is
// resolved the way the go command does for the importing module: through
// that module's own path, its local `replace` targets and the modules of a
// go.work that lists it. Anything else is external (a released version from
// the module cache or vendor/), so a change here cannot reach it.
// Inputs:  the walked tree and its fs.FS.
// Outputs: package directories, importer->imported directory edges and the
// reasons the graph cannot be trusted (unparsable file, an import of an
// in-repo path with no package behind it).
// Constraints: pure; no `go list` (exec); testdata/, _x/ and .x/ directories
// are not packages and their go.mod files are not modules.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
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
}

func goGraph(fsys fs.FS, t tree, g *Graph) {
	mods := readModFiles(fsys, t, g)
	pkgFiles := map[string][]string{}
	for _, f := range t.files {
		if strings.HasSuffix(f, ".go") && mods.isPackage(dirOf(f)) {
			pkgFiles[dirOf(f)] = append(pkgFiles[dirOf(f)], f)
		}
	}
	dirs := make([]string, 0, len(pkgFiles))
	for d := range pkgFiles {
		dirs = append(dirs, d)
		g.addNode(d, "go")
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		visible := mods.visibleFrom(d)
		for _, f := range pkgFiles[d] {
			src, ok := readFile(fsys, f)
			if !ok {
				g.uncertain(f, "go source unreadable")
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
			if err != nil {
				g.uncertain(f, "go source does not parse")
				continue
			}
			for _, imp := range parsed.Imports {
				ip, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					g.uncertain(f, "bad import path")
					continue
				}
				targets, inRepo := resolveImport(visible, ip)
				if !inRepo {
					continue
				}
				linked := false
				for _, target := range targets {
					if pkgFiles[target] != nil {
						linked = true
						if target != d {
							g.addEdge(d, target)
						}
					}
				}
				if !linked {
					g.uncertain(f, "import of in-repo path with no package: "+ip)
				}
			}
		}
	}
}

// isPackageDir is false for directories Go's `./...` never matches: testdata
// and names starting with _ or . (such a directory can still be a module
// root, and everything below that root is judged from the root, not from the
// repository root).
func isPackageDir(dir string) bool {
	if dir == "." {
		return true
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == "testdata" || (len(seg) > 1 && (seg[0] == '_' || seg[0] == '.')) {
			return false
		}
	}
	return true
}

// notTestdata is false for a path inside a testdata directory, whose
// manifests are fixtures, not part of the repository's build.
func notTestdata(dir string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if seg == "testdata" {
			return false
		}
	}
	return true
}

// isPackage reports whether dir is a Go package directory: judged relative
// to the module root that holds it, or to the repository root when none does.
func (ms modSet) isPackage(dir string) bool {
	if !notTestdata(dir) {
		return false
	}
	for d := dir; ; d = path.Dir(d) {
		if _, ok := ms.byDir[d]; ok {
			rel := strings.TrimPrefix(strings.TrimPrefix(dir, d), "/")
			return rel == "" || isPackageDir(rel)
		}
		if d == "." || d == "/" {
			return isPackageDir(dir)
		}
	}
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
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
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

func (mf *modFile) directive(kind string, f []string) {
	switch kind {
	case "module":
		if len(f) >= 1 {
			mf.module = unquote(f[0])
		}
	case "use":
		if len(f) >= 1 {
			if d := joinDir(mf.dir, unquote(f[0])); d != "" {
				mf.uses = append(mf.uses, d)
			}
		}
	case "replace":
		for i, tok := range f {
			if tok != "=>" || i == 0 || i+1 >= len(f) {
				continue
			}
			if to := unquote(f[i+1]); strings.HasPrefix(to, "./") || strings.HasPrefix(to, "../") {
				if d := joinDir(mf.dir, to); d != "" {
					mf.replaces = append(mf.replaces, goModule{d, unquote(f[0])})
				}
			}
		}
	}
}

func unquote(s string) string { return strings.Trim(s, "\"`") }

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
