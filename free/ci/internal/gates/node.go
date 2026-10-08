package gates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type nodeManifest struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
}

func nodeCommand(root string, p PlannedCheck, cfg model.PipelineConfig) ([]string, string) {
	file := filepath.Join(root, filepath.FromSlash(p.Workdir), "package.json")
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, "package.json unavailable"
	}
	var m nodeManifest
	if json.Unmarshal(b, &m) != nil {
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
	pm := strings.SplitN(m.PackageManager, "@", 2)[0]
	if pm == "" {
		for _, pair := range [][2]string{{"pnpm-lock.yaml", "pnpm"}, {"package-lock.json", "npm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}} {
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), pair[0])); err == nil {
				pm = pair[1]
				break
			}
		}
	}
	if pm == "" {
		pm = "npm"
	}
	if p.Def.ID == "node.coverage" {
		return []string{pm, "run", "test", "--", "--coverage"}, ""
	}
	return []string{pm, "run", script}, ""
}
