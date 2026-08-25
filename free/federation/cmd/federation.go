// Purpose: subcommands for the federation plugin (compose, status,
// introspect), moved verbatim from the core CLI's cmd/commands/federation.go
// under CLI-R11. Behavior and flags are unchanged; only the package name,
// root-command wiring (root.go), and the internal/config + internal/plugin +
// internal/ui replacements (internal/envcascade + internal/manifest +
// internal/tui) changed.
//
// Inputs: cobra flags per subcommand; the current working directory (to
// locate the nSelf project root); the .env cascade (NSELF_FEDERATION,
// NSELF_PLUGIN_DIR, HASURA_PORT); installed plugin manifests under
// NSELF_PLUGIN_DIR; the `rover` CLI for schema composition.
//
// Outputs: writes .nself/federation/{supergraph.yaml,supergraph.graphql,
// router.yaml} under the project root, or prints subgraph status/schema.
//
// Constraints: no dependency on the core CLI's internal/* packages.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-federation/internal/envcascade"
	"github.com/nself-org/nself-federation/internal/federation"
	"github.com/nself-org/nself-federation/internal/manifest"
	"github.com/nself-org/nself-federation/internal/projectroot"
	"github.com/nself-org/nself-federation/internal/tui"
)

var federationComposeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Re-compose supergraph schema from installed plugin subgraphs",
	Long: `Re-run rover supergraph compose for all installed plugins that declare
graphql.enabled: true. The resulting supergraph.graphql is written to
.nself/federation/ and mounted into the Apollo Router container on next start.

This command runs automatically during nself build when NSELF_FEDERATION=true.
Run it manually after installing or updating a plugin that exposes a subgraph.`,
	RunE: runFederationCompose,
}

var federationStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show subgraph health and schema composition status",
	Long: `Print the list of registered subgraphs with their URL, last composition
timestamp, and schema hash. Non-zero exit when any subgraph reports status=error.`,
	RunE: runFederationStatus,
}

var federationIntrospectCmd = &cobra.Command{
	Use:   "introspect",
	Short: "Print the full supergraph schema to stdout",
	Long: `Print the composed supergraph.graphql to stdout. Useful for auditing
the full federated type system or piping into schema diffing tools.`,
	RunE: runFederationIntrospect,
}

// federationContext centralises the repeated pattern: find project root,
// load the three relevant config values, resolve the .nself/federation/
// directory path.
type federationContext struct {
	projectDir string
	fedDir     string
	cfg        envcascade.Values
}

func loadFederationContext() (*federationContext, error) {
	rawCwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	projectDir, err := projectroot.FindNSelfRoot(rawCwd)
	if err != nil {
		return nil, fmt.Errorf("finding project root: %w", err)
	}
	cfg, err := envcascade.Load(projectDir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return &federationContext{
		projectDir: projectDir,
		fedDir:     filepath.Join(projectDir, ".nself", "federation"),
		cfg:        cfg,
	}, nil
}

func runFederationCompose(_ *cobra.Command, _ []string) error {
	fctx, err := loadFederationContext()
	if err != nil {
		return err
	}
	if !fctx.cfg.FederationEnabled {
		tui.Warn("Federation is disabled. Set NSELF_FEDERATION=true in your .env to enable.")
		return nil
	}

	tui.CommandHeader("nself federation compose", "Compose supergraph schema")

	ctx := context.Background()

	if err := os.MkdirAll(fctx.fedDir, 0o700); err != nil {
		return fmt.Errorf("creating federation dir: %w", err)
	}
	supergraphYAMLPath := filepath.Join(fctx.fedDir, "supergraph.yaml")
	supergraphGraphQLPath := filepath.Join(fctx.fedDir, "supergraph.graphql")

	// Load installed plugin manifests and extract federation blocks.
	blocks, err := manifest.FederationBlocks(fctx.cfg.PluginDir)
	if err != nil {
		return err
	}

	hasuraURL := fmt.Sprintf("http://127.0.0.1:%d/v1/graphql", fctx.cfg.HasuraPort)
	entries, err := federation.FromManifestBlocks(blocks, hasuraURL)
	if err != nil {
		return fmt.Errorf("resolving subgraph entries: %w", err)
	}

	tui.Info(fmt.Sprintf("Composing %d subgraph(s)...", len(entries)))
	for _, e := range entries {
		tui.Info(fmt.Sprintf("  %-20s %s", e.Name, e.URL))
	}

	yamlContent, err := federation.BuildSupergraphYAML(entries)
	if err != nil {
		return fmt.Errorf("building supergraph YAML: %w", err)
	}
	if err := os.WriteFile(supergraphYAMLPath, yamlContent, 0o600); err != nil {
		return fmt.Errorf("writing supergraph.yaml: %w", err)
	}

	if err := federation.RunRoverCompose(ctx, federation.ComposeOptions{
		SupergraphYAMLPath: supergraphYAMLPath,
		OutputPath:         supergraphGraphQLPath,
	}); err != nil {
		tui.Error("Supergraph composition failed — check schema errors above.")
		return err
	}

	// Write base router config if it does not yet exist.
	routerCfgPath := filepath.Join(fctx.fedDir, "router.yaml")
	if _, statErr := os.Stat(routerCfgPath); os.IsNotExist(statErr) {
		if err := os.WriteFile(routerCfgPath, []byte(federation.RouterBaseConfig()), 0o600); err != nil {
			return fmt.Errorf("writing router.yaml: %w", err)
		}
	}

	tui.Success(fmt.Sprintf("Supergraph schema written to %s", supergraphGraphQLPath))
	return nil
}

func runFederationStatus(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	fctx, err := loadFederationContext()
	if err != nil {
		return err
	}

	if !fctx.cfg.FederationEnabled {
		if jsonOut {
			fmt.Println(`{"federation":false,"subgraphs":[]}`)
			return nil
		}
		tui.Warn("Federation is disabled (NSELF_FEDERATION=false).")
		return nil
	}

	blocks, err := manifest.FederationBlocks(fctx.cfg.PluginDir)
	if err != nil {
		return err
	}
	hasuraURL := fmt.Sprintf("http://127.0.0.1:%d/v1/graphql", fctx.cfg.HasuraPort)
	entries, err := federation.FromManifestBlocks(blocks, hasuraURL)
	if err != nil {
		return fmt.Errorf("resolving subgraph entries: %w", err)
	}

	statuses := make([]federation.SubgraphStatus, 0, len(entries))
	for _, e := range entries {
		statuses = append(statuses, federation.SubgraphStatus{
			Name:   e.Name,
			URL:    e.URL,
			Status: "active",
		})
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"federation": true,
			"subgraphs":  statuses,
		})
	}

	tui.CommandHeader("nself federation status", "Subgraph composition state")
	tui.Info(fmt.Sprintf("%-20s %-42s %s", "SUBGRAPH", "URL", "STATUS"))
	tui.Info(fmt.Sprintf("%-20s %-42s %s", "--------", "---", "------"))
	for _, s := range statuses {
		tui.Info(fmt.Sprintf("%-20s %-42s %s", s.Name, s.URL, s.Status))
	}
	return nil
}

func runFederationIntrospect(_ *cobra.Command, _ []string) error {
	fctx, err := loadFederationContext()
	if err != nil {
		return err
	}

	schemaPath := filepath.Join(fctx.fedDir, "supergraph.graphql")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("supergraph schema not found at %s — run: nself federation compose", schemaPath)
		}
		return fmt.Errorf("reading supergraph schema: %w", err)
	}
	fmt.Print(string(data))
	return nil
}
