// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-federation
// <args...>` with the leading "federation" argument already stripped, so
// this root command takes the place `nself federation` used to occupy in
// the core binary — its subcommands (compose, status, introspect) are what
// `nself federation <sub>` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself federation --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. federation.go
// depended on internal/config (FindNSelfRoot + Load), internal/plugin
// (LoadManifestsFromDir), and internal/ui; all are reimplemented standalone
// in internal/projectroot, internal/envcascade, internal/manifest, and
// internal/tui respectively. The domain package internal/federation
// (compose/registry/router/types) moved wholesale, unchanged.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-federation",
	Short: "Manage GraphQL Federation (opt-in, requires NSELF_FEDERATION=true)",
	Long: `Manage the Apollo Router supergraph for multi-service GraphQL Federation.

Federation is opt-in. Enable it by setting NSELF_FEDERATION=true in your .env.

When enabled, nself build:
  1. Collects all installed plugins with graphql.enabled: true.
  2. Adds Hasura as the core subgraph.
  3. Composes a supergraph schema via rover supergraph compose.
  4. Injects Apollo Router (CS_7, port 4000) into docker-compose.yml.
  5. Routes /v1/graphql through Apollo Router instead of directly to Hasura.

Subcommands:
  compose    Re-compose the supergraph schema (auto-runs on nself build).
  status     Show subgraph health and schema composition state.
  introspect Print the full supergraph schema to stdout.`,
	// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go):
	// without them cobra prints its own "Error: ..." plus a usage block on
	// every RunE error, then main() prints "Error: ..." again.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	federationStatusCmd.Flags().Bool("json", false, "Output status as JSON")
	rootCmd.AddCommand(federationComposeCmd)
	rootCmd.AddCommand(federationStatusCmd)
	rootCmd.AddCommand(federationIntrospectCmd)
}
