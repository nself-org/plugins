package legacy

// Package internal — gate_modules.go
//
// Purpose: Find the Go modules a repo holds when go.mod is not at the root
//   (D-0922). In a Rust-rooted repo such as nself-org/bundles the Go code
//   lives in paid/*/go.mod, so a root-only check detected only "rust" and
//   ran no Go gate at all.
// Inputs:  root string — repo root
// Outputs: goModuleDirs → sorted module dirs relative to root ("." = root)
// Constraints: A root go.mod wins and is the only module (behaviour
//   unchanged). Otherwise only depth 1 and 2 below root are searched;
//   vendor, node_modules, testdata and dot-dirs are skipped.

import (
	"os"
	"path/filepath"
	"sort"
)

// skipModuleDir reports whether a directory name is never searched for modules.
func skipModuleDir(name string) bool {
	if len(name) > 0 && name[0] == '.' {
		return true
	}
	return name == "vendor" || name == "node_modules" || name == "testdata"
}

// goModuleDirs returns the Go module dirs under root, relative to root.
func goModuleDirs(root string) []string {
	if fileExists(filepath.Join(root, "go.mod")) {
		return []string{"."}
	}
	var mods []string
	for _, d1 := range subDirs(root) {
		if fileExists(filepath.Join(root, d1, "go.mod")) {
			mods = append(mods, d1)
			continue
		}
		for _, d2 := range subDirs(filepath.Join(root, d1)) {
			if fileExists(filepath.Join(root, d1, d2, "go.mod")) {
				mods = append(mods, filepath.Join(d1, d2))
			}
		}
	}
	sort.Strings(mods)
	return mods
}

// subDirs lists searchable child directory names of dir.
func subDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !skipModuleDir(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// runGoModuleGates runs runGoGates once per module dir. The root module keeps
// its gate names; nested modules get the dir prefixed ("paid/chat: go:vet").
func runGoModuleGates(root string, timeout int, verbose bool) []GateResult {
	var all []GateResult
	for _, m := range goModuleDirs(root) {
		if m == "." {
			all = append(all, runGoGates(root, timeout, verbose)...)
			continue
		}
		for _, g := range runGoGates(filepath.Join(root, m), timeout, verbose) {
			g.Name = m + ": " + g.Name
			all = append(all, g)
		}
	}
	return all
}
