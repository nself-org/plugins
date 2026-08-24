// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-costs <args...>`
// with the leading "costs" argument already stripped, so this root command
// takes the place `nself costs` used to occupy in the core binary. Unlike
// most CLI-R11 extractions, `costs` has no subcommands — the root command IS
// the whole feature, wired here with its RunE in costs.go.
//
// Inputs: os.Args, as passed through by the plugin router; --server-type and
// --format/-f flags.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself costs --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. The
// core's internal/plugin.ListInstalled() is reimplemented narrowly in
// internal/plugininfo (see that package's doc comment for the exact scope
// of the simplification). SilenceUsage/SilenceErrors are set to match the
// core CLI's RootCmd (cmd/commands/root.go) — see the CLI-R11 pentest-kit
// extraction's commit message for why this matters on any RunE error path.
package main

import (
	"github.com/spf13/cobra"
)

var costsServerType string
var costsFormat string

var rootCmd = &cobra.Command{
	Use:           "nself-costs",
	Short:         "Show estimated per-install operational costs",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Show an itemized breakdown of estimated monthly costs for your nself install.

Includes:
  - Hetzner VPS cost (based on server type detected from env or flag)
  - Stripe transaction fee structure
  - Plugin license costs for installed paid plugins
  - Operational misc (Cloudflare, Vercel, etc.)

Examples:
  nself costs                          # auto-detect server type from env
  nself costs --server-type cx23       # override server type
  nself costs --format json`,
	RunE: runCosts,
}

func init() {
	rootCmd.Flags().StringVar(&costsServerType, "server-type", "", "Hetzner server type (e.g. cx23, cax11). Auto-detected from HETZNER_SERVER_TYPE env var if unset.")
	rootCmd.Flags().StringVarP(&costsFormat, "format", "f", "table", "Output format: table|json")
}
