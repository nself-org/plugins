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
// Constraints: pure; no `go list` (exec); inside a module, testdata/, _x/ and
// .x/ directories are not packages (a module may itself be rooted in one);
// a go.mod under testdata/ is a fixture, not a module.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

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
