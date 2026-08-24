// Package manifest reimplements the narrow slice of the CLI's internal/
// plugin package that the federation plugin needs: scanning an installed-
// plugins directory for plugin.json files and extracting each one's graphql
// block.
//
// Purpose: the plugin is its own Go module and cannot import
// github.com/nself-org/cli/internal/plugin, whose PluginManifest type
// mirrors the full plugin.json schema (40+ fields covering licensing,
// dependencies, hooks, deprecation, and more) — entirely out of proportion
// to the five fields (name, port, graphql.{enabled,subgraph_name,
// subgraph_url,schema_path,entities}) federation.go actually reads. This
// file defines its own minimal manifest struct carrying only those fields,
// tagged with the identical JSON keys, and a directory scanner matching
// plugin.LoadManifestsFromDir's tolerant behavior (missing directory is not
// an error; a directory without a valid plugin.json is skipped, not fatal).
//
// One known, minor divergence from core: plugin.LoadManifestsFromDir's
// parseManifest also runs validateRequiredFields/validateManifest (checking
// name, version, description, category, license, and semver/enum shape) and
// skips any manifest that fails them. This scanner only requires Name to be
// non-empty. A plugin.json that is valid JSON with a populated graphql block
// but fails one of those other, unrelated required-field checks would be
// skipped by core but included here. In practice an installed plugin has
// already passed that validation at install time, so this is a narrow edge
// case, not a behavior users are expected to hit.
//
// Inputs: pluginDir, the resolved (unexpanded, matching core) plugin
// installation directory.
//
// Outputs: a slice of federation.ManifestGraphQLBlock, ready to pass to
// federation.FromManifestBlocks — the same shape cmd/commands/federation.go
// built in core.
//
// Constraints: read-only.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nself-org/nself-federation/internal/federation"
)

// pluginManifest is the minimal subset of plugin.PluginManifest that
// federation needs, with JSON tags copied from
// internal/plugin/interfaces.go's PluginManifest/PluginGraphQLBlock.
type pluginManifest struct {
	Name    string        `json:"name"`
	Port    int           `json:"port,omitempty"`
	GraphQL *graphQLBlock `json:"graphql,omitempty"`
}

type graphQLBlock struct {
	Enabled      bool        `json:"enabled"`
	SubgraphName string      `json:"subgraph_name"`
	SubgraphURL  string      `json:"subgraph_url"`
	SchemaPath   string      `json:"schema_path,omitempty"`
	Entities     []entityKey `json:"entities,omitempty"`
}

type entityKey struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// loadManifestsFromDir scans pluginDir for installed plugin manifests,
// matching plugin.LoadManifestsFromDir's tolerant behavior: a missing
// directory returns (nil, nil), and an entry without a valid plugin.json is
// skipped rather than failing the whole scan.
func loadManifestsFromDir(pluginDir string) ([]pluginManifest, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifests []pluginManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pluginDir, entry.Name(), "plugin.json"))
		if err != nil {
			continue // no plugin.json here, skip
		}
		var m pluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue // invalid JSON, skip
		}
		if m.Name == "" {
			continue // matches core's required-field check on Name
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// FederationBlocks scans the installed plugin manifests in pluginDir and
// returns federation blocks for plugins with graphql.enabled: true — the
// same conversion cmd/commands/federation.go's pluginFederationBlocks did in
// core.
func FederationBlocks(pluginDir string) ([]federation.ManifestGraphQLBlock, error) {
	manifests, err := loadManifestsFromDir(pluginDir)
	if err != nil {
		// Non-fatal: no plugins installed or dir missing returns empty list.
		return nil, nil
	}

	var blocks []federation.ManifestGraphQLBlock
	for _, m := range manifests {
		if m.GraphQL == nil || !m.GraphQL.Enabled {
			continue
		}
		entities := make([]federation.EntityKey, len(m.GraphQL.Entities))
		for i, e := range m.GraphQL.Entities {
			entities[i] = federation.EntityKey{Type: e.Type, Key: e.Key}
		}
		blocks = append(blocks, federation.ManifestGraphQLBlock{
			PluginName: m.Name,
			PluginPort: m.Port,
			GraphQL: federation.SubgraphConfig{
				Enabled:      m.GraphQL.Enabled,
				SubgraphName: m.GraphQL.SubgraphName,
				SubgraphURL:  m.GraphQL.SubgraphURL,
				SchemaPath:   m.GraphQL.SchemaPath,
				Entities:     entities,
			},
		})
	}
	return blocks, nil
}
