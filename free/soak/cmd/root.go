// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-soak <args...>` with
// the leading "soak" argument already stripped, so this root command takes
// the place `nself soak` used to occupy in the core binary — its subcommand
// (abort) is what `nself soak abort` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself soak abort --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// SilenceUsage/SilenceErrors are set to match the core CLI's RootCmd
// (cmd/commands/root.go) — without them, any RunE error prints a cobra
// usage block plus a duplicated "Error: ..." line (once from cobra, once
// from main.go), which the pre-extraction command never did.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "nself-soak",
	Short:         "Manage soak testing lifecycle",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Manage soak testing lifecycle for nSelf environments.

Subcommands:
  abort    Abort an active soak and optionally roll back to a prior version`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(soakAbortCmd)
}
