package gates

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type nodeManifest struct {
	PackageManager  string                     `json:"packageManager"`
	Scripts         map[string]string          `json:"scripts"`
	Dependencies    map[string]json.RawMessage `json:"dependencies"`
	DevDependencies map[string]json.RawMessage `json:"devDependencies"`
}

// nodeLockfiles lists, per package manager, the lockfiles it freezes against.
var nodeLockfiles = map[string][]string{
	"npm":  {"npm-shrinkwrap.json", "package-lock.json"}, // npm prefers the shrinkwrap
	"pnpm": {"pnpm-lock.yaml"},
	"yarn": {"yarn.lock"},
	"bun":  {"bun.lock", "bun.lockb"},
}

// nodeLockOrder is the order lockfiles are tried when the manager is not declared.
var nodeLockOrder = []string{"pnpm", "npm", "yarn", "bun"}

func readNodeManifest(root, dir string) (nodeManifest, bool) {
	var m nodeManifest
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "package.json"))
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}

func hasFile(root, dir, name string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), name))
	return err == nil
}

// nodeManager resolves the package manager of a directory and the version its
// packageManager field pins ("" when none). The directory's own declaration or
// lockfile decides; a directory with neither (a workspace member) takes the
// nearest ancestor's; npm is the last resort.
func nodeManager(root, dir string, m nodeManifest) (manager, pin string) {
	for d := path.Clean(dir); ; d = path.Dir(d) {
		dm := m
		if d != path.Clean(dir) {
			dm, _ = readNodeManifest(root, d)
		}
		if dm.PackageManager != "" {
			name, version, _ := strings.Cut(dm.PackageManager, "@")
			version, _, _ = strings.Cut(version, "+") // corepack's +sha512 suffix is not part of the version
			return name, version
		}
		for _, pm := range nodeLockOrder {
			for _, lock := range nodeLockfiles[pm] {
				if hasFile(root, d, lock) {
					return pm, ""
				}
			}
		}
		if d == "." || d == "/" || d == "" {
			return "npm", ""
		}
	}
}

func nodeCommand(root string, p PlannedCheck, cfg model.PipelineConfig) ([]string, string) {
	m, ok := readNodeManifest(root, p.Workdir)
	if !ok {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p.Workdir), "package.json")); err != nil {
			return nil, "package.json unavailable"
		}
		return nil, "invalid package.json"
	}
	script := strings.TrimPrefix(p.Def.ID, "node.")
	if script == "coverage" {
		script = "test"
		if v, ok := cfg.Checks[p.Def.ID]; !ok || v.CoverageFloor == nil {
			return nil, "coverage floor not configured"
		}
	}
	if m.Scripts[script] == "" {
		return nil, "no " + script + " script in package.json"
	}
	pm, _ := nodeManager(root, p.Workdir, m)
	switch pm {
	case "npm", "pnpm", "yarn", "bun":
	default:
		return nil, "unsupported package manager"
	}
	if p.Def.ID == "node.coverage" {
		return []string{pm, "run", "test", "--", "--coverage"}, ""
	}
	return []string{pm, "run", script}, ""
}
