// Package plugininfo reimplements the one read `nself costs` needs from the
// core CLI's internal/plugin package: the tier of each installed plugin.
//
// Purpose: the plugin is its own Go module and cannot import
// github.com/nself-org/cli/internal/plugin across the module boundary (Go's
// internal/ visibility rule forbids it), and that package is genuinely
// core-shared (plugin install/license/lifecycle management for the whole
// CLI), not something to fork wholesale. So this file replicates only the
// narrow slice cmd/costs.go actually reads: for each subdirectory of the
// plugin install dir, parse plugin.json and derive its tier with the same
// fallback internal/plugin/loader.go uses (explicit "tier" field, else
// "pro" if requires_license or licenseType=="pro", else "free").
//
// Deliberately NOT reimplemented (see internal/plugin/manifest.go in the
// core CLI for the full version): name/version/semver format checks, table
// name prefix checks, language/status allowlists, or the running-process
// status / dormant-expired lifecycle overlay. A plugin already on disk under
// ~/.nself/plugins/ was validated once at install time; re-validating it on
// every `nself costs` run buys nothing a cost estimate needs. A directory
// with a plugin.json that fails to parse as JSON is skipped, matching
// internal/plugin.ListInstalled's "directories without a valid manifest are
// silently skipped" contract for the one failure mode this command can hit
// in practice.
//
// Inputs: pluginDir (absolute path to the plugin install directory).
//
// Outputs: the tier string for each installed plugin, in the same order
// os.ReadDir returns entries (directory order, not sorted — matching
// ListInstalled, which does not sort either).
package plugininfo

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// manifest is the subset of plugin.json fields this command reads.
type manifest struct {
	Tier            string `json:"tier"`
	LicenseType     string `json:"licenseType"`
	RequiresLicense bool   `json:"requires_license"`
}

// InstalledTiers scans pluginDir and returns the derived tier of every
// installed plugin. A missing pluginDir returns (nil, nil), matching
// internal/plugin.ListInstalled's contract of treating "no plugins
// installed yet" as success, not an error.
func InstalledTiers(pluginDir string) ([]string, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tiers []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pluginDir, entry.Name(), "plugin.json"))
		if err != nil {
			continue // skip directories without a plugin.json, same as ListInstalled
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue // skip directories without a valid manifest, same as ListInstalled
		}

		tier := m.Tier
		if tier == "" {
			if m.RequiresLicense || m.LicenseType == "pro" {
				tier = "pro"
			} else {
				tier = "free"
			}
		}
		tiers = append(tiers, tier)
	}
	return tiers, nil
}
