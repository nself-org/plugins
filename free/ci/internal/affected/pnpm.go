package affected

// Purpose: the pnpm workspace graph. Packages are the directories with a
// package.json that match the `packages:` globs of a pnpm-workspace.yaml
// (the workspace directory itself included); edges come from dependencies
// that name a workspace package (any specifier), from aliases
// (`workspace:real@*`, `npm:real@^1`) and from relative `link:`, `file:` and
// `portal:` specifiers.
// Inputs:  the walked tree and its fs.FS.
// Outputs: nodes and edges on the Graph, or uncertain reasons.
// Constraints: pure; an unresolvable `workspace:` dependency or relative
// link, an unparsable manifest, a workspace glob with extglob syntax, and
// npm/yarn `workspaces` without a pnpm-workspace.yaml are uncertain (the
// selector then returns a full run). Packages with the same name all get the
// edge.

import (
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type pkgJSON struct {
	Name       string
	Workspaces json.RawMessage
	Deps       map[string][]string // every dependency section: all specifiers per name
}

var depSections = []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}

func readPkg(fsys fs.FS, p string) (pkgJSON, bool) {
	b, ok := readFile(fsys, p)
	if !ok {
		return pkgJSON{}, false
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		return pkgJSON{}, false
	}
	pj := pkgJSON{Workspaces: raw["workspaces"], Deps: map[string][]string{}}
	if r, ok := raw["name"]; ok && json.Unmarshal(r, &pj.Name) != nil {
		return pkgJSON{}, false
	}
	for _, sec := range depSections {
		r, ok := raw[sec]
		if !ok {
			continue
		}
		var m map[string]string
		if json.Unmarshal(r, &m) != nil {
			return pkgJSON{}, false
		}
		for name, spec := range m {
			pj.Deps[name] = append(pj.Deps[name], spec)
		}
	}
	return pj, true
}

func pnpmGraph(fsys fs.FS, t tree, g *Graph) {
	pkgs := map[string]pkgJSON{} // dir -> manifest
	for _, f := range t.files {
		if path.Base(f) != "package.json" || !notTestdata(dirOf(f)) || hiddenDir(dirOf(f)) {
			continue
		}
		pj, ok := readPkg(fsys, f)
		if !ok {
			g.uncertain(f, "package.json unreadable or invalid")
			continue
		}
		pkgs[dirOf(f)] = pj
		g.addNode(dirOf(f), "pnpm")
	}
	roots := map[string]bool{} // directories holding a pnpm-workspace.yaml
	for _, f := range t.files {
		if path.Base(f) == "pnpm-workspace.yaml" && notTestdata(dirOf(f)) && !hiddenDir(dirOf(f)) {
			roots[dirOf(f)] = true
		}
	}
	for dir, pj := range pkgs {
		if !roots[dir] && len(pj.Workspaces) > 0 && string(pj.Workspaces) != "null" {
			g.uncertain(path.Join(dir, "package.json"), "npm or yarn workspaces are not supported without pnpm-workspace.yaml")
		}
	}
	dirsSorted := make([]string, 0, len(pkgs))
	for d := range pkgs {
		dirsSorted = append(dirsSorted, d)
	}
	sort.Strings(dirsSorted)
	inAnyWorkspace := map[string]bool{}
	for root := range roots {
		globs, ok := workspaceGlobs(fsys, path.Join(root, "pnpm-workspace.yaml"))
		if !ok {
			g.uncertain(path.Join(root, "pnpm-workspace.yaml"), "pnpm-workspace.yaml unreadable or unsupported")
			continue
		}
		var member []string
		members := map[string][]string{} // package name -> dirs
		for _, dir := range dirsSorted {
			if within(dir, root) && (dir == root || inWorkspace(globs, relTo(dir, root))) {
				member = append(member, dir)
				if n := pkgs[dir].Name; n != "" {
					members[n] = append(members[n], dir)
				}
			}
		}
		for _, dir := range member {
			inAnyWorkspace[dir] = true
			linkDeps(g, pkgs, members, dir, pkgs[dir])
		}
	}
	for _, dir := range dirsSorted { // links work without a workspace file too
		linkDeps(g, pkgs, nil, dir, pkgs[dir])
		if !inAnyWorkspace[dir] {
			for name, specs := range pkgs[dir].Deps {
				for _, spec := range specs {
					if strings.HasPrefix(spec, "workspace:") {
						g.uncertain(path.Join(dir, "package.json"), "workspace dependency "+name+" in a package outside every pnpm workspace")
					}
				}
			}
		}
	}
}

func relTo(dir, root string) string {
	if root == "." {
		return dir
	}
	if dir == root {
		return "."
	}
	return strings.TrimPrefix(dir, root+"/")
}

// workspaceGlobs reads the `packages:` list; an absent list means every
// subdirectory (pnpm's default), which is the widest reading. Extglob and
// escape syntax is not modelled: it is reported as not ok.
func workspaceGlobs(fsys fs.FS, p string) ([]string, bool) {
	b, ok := readFile(fsys, p)
	if !ok {
		return nil, false
	}
	var doc struct {
		Packages []string `yaml:"packages"`
	}
	if yaml.Unmarshal(b, &doc) != nil {
		return nil, false
	}
	for _, g := range doc.Packages {
		if _, within := expandBraces(g); strings.ContainsAny(g, "()|\\") || !within {
			return nil, false
		}
	}
	if len(doc.Packages) == 0 {
		return []string{"**"}, true
	}
	return doc.Packages, true
}

func inWorkspace(globs []string, rel string) bool {
	in := false
	for _, g := range globs {
		neg := strings.HasPrefix(g, "!")
		g = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(g, "!"), "./"), "/")
		if g != "" && matchGlob(g, rel) {
			in = !neg
		}
	}
	return in
}

// linkDeps adds the edges of one package. members is nil outside a workspace
// (only relative links apply).
func linkDeps(g *Graph, pkgs map[string]pkgJSON, members map[string][]string, dir string, pj pkgJSON) {
	names := make([]string, 0, len(pj.Deps))
	for n := range pj.Deps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		resolved := false
		for _, t := range members[name] { // the name is a workspace package, whatever the specifier
			g.addEdge(dir, t)
			resolved = true
		}
		workspaceSpec := false
		for _, spec := range pj.Deps[name] {
			workspaceSpec = workspaceSpec || strings.HasPrefix(spec, "workspace:")
			if linkSpec(g, pkgs, members, dir, name, spec) {
				resolved = true
			}
		}
		if !resolved && workspaceSpec && members != nil {
			g.uncertain(path.Join(dir, "package.json"), "workspace dependency "+name+" matches no workspace package")
		}
	}
}

// linkSpec handles the prefixed specifiers of one dependency; true when it
// produced an edge or was reported uncertain.
func linkSpec(g *Graph, pkgs map[string]pkgJSON, members map[string][]string, dir, name, spec string) bool {
	done := false
	for _, pre := range []string{"workspace:", "npm:", "link:", "file:", "portal:"} {
		rest := strings.TrimPrefix(spec, pre)
		if rest == spec {
			continue
		}
		pathForm := pre == "link:" || pre == "file:" || pre == "portal:" || isPathSpec(rest)
		switch {
		case pathForm && strings.Contains(rest, "://"):
			// a URL, not a path
		case pathForm:
			if linkPath(g, pkgs, dir, rest) {
				done = true
			} else if !isArchive(rest) {
				g.uncertain(path.Join(dir, "package.json"), "dependency "+name+" links to a path that is not a package")
				done = true // reported
			}
		default: // workspace:real@*, npm:real@^1, workspace:@s/real
			cands := []string{rest}
			if i := strings.LastIndex(rest, "@"); i > 0 {
				cands = append(cands, rest[:i])
			}
			for _, c := range cands {
				for _, t := range members[c] {
					g.addEdge(dir, t)
					done = true
				}
			}
		}
	}
	return done
}

func isPathSpec(rest string) bool {
	return rest == "." || rest == ".." || strings.HasPrefix(rest, "./") || strings.HasPrefix(rest, "../") || strings.HasPrefix(rest, "/")
}

// hiddenDir is true when a directory name in dir starts with a dot (pnpm
// globs do not match those).
func hiddenDir(dir string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if len(seg) > 1 && seg[0] == '.' {
			return true
		}
	}
	return false
}

func isArchive(rest string) bool {
	for _, ext := range []string{".tgz", ".tar.gz", ".tar", ".zip"} {
		if strings.HasSuffix(rest, ext) {
			return true
		}
	}
	return false
}

// linkPath adds the edge for a relative link; false when the target is not
// a package directory.
func linkPath(g *Graph, pkgs map[string]pkgJSON, dir, rest string) bool {
	target := joinDir(dir, rest)
	if _, ok := pkgs[target]; target == "" || !ok {
		return false
	}
	g.addEdge(dir, target)
	return true
}
