// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-alerts <args...>`
// with the leading "alerts" argument already stripped, so this root command
// takes the place `nself alerts` used to occupy in the core binary — its
// subcommands (list, silence, test) are what `nself alerts list` /
// `nself alerts silence` / `nself alerts test` resolve to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail).
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go):
// without them cobra prints its own "Error: ..." plus a usage block on every
// RunE error, then main() prints "Error: ..." again.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-alerts",
	Short: "Manage Prometheus alert rules and silences",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(alertsListCmd, alertsSilenceCmd, alertsTestCmd)
}
