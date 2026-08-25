// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-audit <args...>`
// with the leading "audit" argument already stripped, so this root command
// takes the place `nself audit` used to occupy in the core binary — its
// subcommand (docs) is what `nself audit docs` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 no findings, 1 findings/error), mirroring
// the exit codes documented on `nself audit docs --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// SilenceUsage/SilenceErrors are set to match the core CLI's RootCmd
// (cmd/commands/root.go) — without them, `audit docs` returning its
// findings-present error prints a cobra usage block plus a duplicated
// "Error: ..." line (once from cobra, once from main.go), which the
// pre-extraction command never did.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "nself-audit",
	Short:         "Run ecosystem audits (docs, origin, etc.)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Run ecosystem-level audits.

Subcommands:
  nself audit docs   Quarterly documentation audit (banned words, dead links,
                     missing anchors across README, wiki, docs, SPORT, PPI, PRI)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(auditDocsCmd)
}
