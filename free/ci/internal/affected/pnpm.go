package affected

// Purpose: the pnpm workspace graph. Packages are the directories with a
// package.json that match the `packages:` globs of a pnpm-workspace.yaml
// (the workspace directory itself included); edges come from dependencies
// that name another workspace package, `workspace:` ones included, and from
// relative `link:`/`file:`/`portal:` specifiers.
// Inputs:  the walked tree and its fs.FS.
// Outputs: nodes and edges on the Graph, or uncertain reasons.
// Constraints: pure; an unresolvable `workspace:` dependency, an unparsable
// manifest, and npm/yarn `workspaces` without a pnpm-workspace.yaml are
// uncertain (the selector then returns a full run).

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type pkgJSON struct {
	Name       string                     `json:"name"`
	Workspaces json.RawMessage            `json:"workspaces"`
	Deps       [4]map[string]string       `json:"-"`
	Raw        map[string]json.RawMessage `json:"-"`
}

var depSections = []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}

func readPkg(fsys fs.FS, p string) (pkgJSON, bool) {
	var pj pkgJSON
	b, ok := readFile(fsys, p)
	if !ok || json.Unmarshal(b, &pj) != nil {
		return pj, false
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	for i, sec := range depSections {
		if r, ok := raw[sec]; ok {
			_ = json.Unmarshal(r, &pj.Deps[i])
		}
	}
	return pj, true
}

func pnpmGraph(fsys fs.FS, t tree, g *Graph) {
	pkgs := map[string]pkgJSON{} // dir -> manifest
	for _, f := range t.files {
		if path.Base(f) != "package.json" {
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
		if path.Base(f) == "pnpm-workspace.yaml" {
			roots[dirOf(f)] = true
		}
	}
	if len(roots) == 0 {
		if pj, ok := pkgs["."]; ok && len(pj.Workspaces) > 0 && string(pj.Workspaces) != "null" {
			g.uncertain("package.json", "npm or yarn workspaces are not supported without pnpm-workspace.yaml")
		}
	}
	for root := range roots {
		globs, ok := workspaceGlobs(fsys, path.Join(root, "pnpm-workspace.yaml"))
		if !ok {
			g.uncertain(path.Join(root, "pnpm-workspace.yaml"), "pnpm-workspace.yaml unreadable or invalid")
			continue
		}
		members := map[string]string{} // package name -> dir
		for dir, pj := range pkgs {
			if within(dir, root) && (dir == root || inWorkspace(globs, relTo(dir, root))) && pj.Name != "" {
				members[pj.Name] = dir
			}
		}
		for dir, pj := range pkgs {
			if !within(dir, root) || (dir != root && !inWorkspace(globs, relTo(dir, root))) {
				continue
			}
			linkWorkspaceDeps(g, pkgs, members, dir, pj)
		}
	}
	for dir, pj := range pkgs { // links work without a workspace file too
		linkRelative(g, pkgs, dir, pj)
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
// subdirectory (pnpm's default), which is the widest reading.
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
		if g == "" {
			continue
		}
		if matchGlob(g, rel) {
			in = !neg
		}
	}
	return in
}

func linkWorkspaceDeps(g *Graph, pkgs map[string]pkgJSON, members map[string]string, dir string, pj pkgJSON) {
	for _, deps := range pj.Deps {
		for name, spec := range deps {
			target, ok := members[name]
			switch {
			case ok:
				g.addEdge(dir, target)
			case strings.HasPrefix(spec, "workspace:"):
				if strings.HasPrefix(spec, "workspace:.") || strings.HasPrefix(spec, "workspace:/") {
					continue // a path form is resolved as a relative link
				}
				g.uncertain(path.Join(dir, "package.json"), "workspace dependency "+name+" matches no workspace package")
			}
		}
	}
}

func linkRelative(g *Graph, pkgs map[string]pkgJSON, dir string, pj pkgJSON) {
	for _, deps := range pj.Deps {
		for name, spec := range deps {
			for _, pre := range []string{"link:", "file:", "portal:", "workspace:"} {
				rel := strings.TrimPrefix(spec, pre)
				if rel == spec || !(strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../")) {
					continue
				}
				target := joinDir(dir, rel)
				if _, ok := pkgs[target]; ok {
					g.addEdge(dir, target)
				} else if target != "" {
					g.uncertain(path.Join(dir, "package.json"), "link dependency "+name+" is not a package directory")
				}
			}
		}
	}
}
