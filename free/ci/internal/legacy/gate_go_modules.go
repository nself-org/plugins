package legacy

// Nested Go modules (D-0922): a repo may keep its Go code below the root
// (bundles: paid/*/go) next to a root manifest of another stack (Cargo.toml).
// Root-marker detection alone saw only that stack, so a Go-only change was
// gated by tools the host lacked and failed with gate:no-substantive-checks.

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// goModuleMaxDepth is how far below the root goModuleDirs looks for go.mod.
const goModuleMaxDepth = 4

// goModuleSkipDirs are never searched: dependencies and build output are not
// the repo's own modules.
var goModuleSkipDirs = map[string]bool{"node_modules": true, "vendor": true, "target": true}

// goModuleDirs returns the slash-separated directories, relative to root, of the
// repo's Go modules. A root go.mod is the only module there is ("."), as before;
// otherwise the modules found within goModuleMaxDepth levels. A found module is
// not searched further, so its own nested modules stay its business.
func goModuleDirs(root string) []string {
	if fileExists(filepath.Join(root, "go.mod")) {
		return []string{"."}
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return fs.SkipDir
		}
		name := d.Name()
		if goModuleSkipDirs[name] || strings.HasPrefix(name, ".") ||
			strings.Count(rel, string(filepath.Separator)) >= goModuleMaxDepth {
			return fs.SkipDir
		}
		if fileExists(filepath.Join(path, "go.mod")) {
			dirs = append(dirs, filepath.ToSlash(rel))
			return fs.SkipDir
		}
		return nil
	})
	return dirs
}

// runGoStack runs the Go gates for every module goModuleDirs finds. The root
// module keeps its plain gate names; a nested module's gates carry its path.
func runGoStack(root string, timeout int, verbose bool) []GateResult {
	var gates []GateResult
	for _, dir := range goModuleDirs(root) {
		if dir == "." {
			gates = append(gates, runGoGates(root, timeout, verbose)...)
			continue
		}
		for _, g := range runGoGates(filepath.Join(root, filepath.FromSlash(dir)), timeout, verbose) {
			g.Name += " (" + dir + ")"
			gates = append(gates, g)
		}
	}
	return gates
}
