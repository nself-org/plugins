package affected

// Purpose: the Go import graph (Epic D15). Every .go file, test files
// included, is parsed with go/parser ImportsOnly; build tags are ignored, so
// the graph is a superset of what any one build sees. Import paths map to
// directories through the `module` line of every go.mod in the tree, plus
// local `replace` targets.
// Inputs:  the walked tree and its fs.FS.
// Outputs: package directories, importer->imported directory edges and the
// reasons the graph cannot be trusted (unparsable file, import of an
// in-repo path with no package behind it).
// Constraints: pure; no `go list` (exec); testdata/, _x/ and .x/ directories
// are not packages (their files belong to the nearest package above).

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

func goGraph(fsys fs.FS, t tree, g *Graph) {
	mods := readModules(fsys, t, g)
	pkgFiles := map[string][]string{}
	for _, f := range t.files {
		if strings.HasSuffix(f, ".go") && isPackageDir(dirOf(f)) {
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
				targets, inRepo := resolveImport(mods, ip)
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

// isPackageDir is false for directories Go never builds as packages.
func isPackageDir(dir string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if seg == "testdata" || (len(seg) > 1 && (seg[0] == '_' || seg[0] == '.')) {
			return false
		}
	}
	return true
}

// readModules lists every go.mod module (and local replace) in the tree.
func readModules(fsys fs.FS, t tree, g *Graph) []goModule {
	var out []goModule
	for _, f := range t.files {
		if path.Base(f) != "go.mod" {
			continue
		}
		src, ok := readFile(fsys, f)
		if !ok {
			g.uncertain(f, "go.mod unreadable")
			continue
		}
		dir, found, inReplace := dirOf(f), false, false
		for _, line := range strings.Split(string(src), "\n") {
			if i := strings.Index(line, "//"); i >= 0 {
				line = line[:i]
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			switch {
			case fields[0] == "module" && len(fields) >= 2:
				out = append(out, goModule{dir, strings.Trim(fields[1], `"`)})
				found = true
			case fields[0] == "replace" && len(fields) >= 2 && fields[1] == "(":
				inReplace = true
			case inReplace && fields[0] == ")":
				inReplace = false
			case fields[0] == "replace":
				out = appendReplace(out, dir, fields[1:])
			case inReplace:
				out = appendReplace(out, dir, fields)
			}
		}
		if !found {
			g.uncertain(f, "go.mod without module line")
		}
	}
	return out
}

// appendReplace handles `old [version] => new [version]` for a local target.
func appendReplace(out []goModule, dir string, f []string) []goModule {
	for i, tok := range f {
		if tok != "=>" || i == 0 || i+1 >= len(f) {
			continue
		}
		if to := f[i+1]; strings.HasPrefix(to, "./") || strings.HasPrefix(to, "../") {
			if target := joinDir(dir, to); target != "" {
				out = append(out, goModule{target, strings.Trim(f[0], `"`)})
			}
		}
	}
	return out
}

// resolveImport maps an import path to repository directories through the
// longest matching module path (several when a path is declared and replaced
// in more than one place). inRepo is false for the standard library and
// third-party paths.
func resolveImport(mods []goModule, imp string) (dirs []string, inRepo bool) {
	longest := -1
	for _, m := range mods {
		if (imp == m.path || strings.HasPrefix(imp, m.path+"/")) && len(m.path) > longest {
			longest = len(m.path)
		}
	}
	if longest < 0 {
		return nil, false
	}
	for _, m := range mods {
		if len(m.path) == longest && (imp == m.path || strings.HasPrefix(imp, m.path+"/")) {
			if d := joinDir(m.dir, strings.TrimPrefix(strings.TrimPrefix(imp, m.path), "/")); d != "" {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs, true
}
