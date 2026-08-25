// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-watchdog <args...>`
// with the leading "watchdog" argument already stripped, so this root
// command takes the place `nself watchdog` used to occupy in the core
// binary — its subcommands (status, reset-breakers, reset, history,
// test-alert) are what `nself watchdog status` etc. resolve to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail, 2 open circuit(s) on
// `status`, mirroring the exit codes documented before extraction).
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
	Use:   "nself-watchdog",
	Short: "Self-healing container watchdog with circuit breaker",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(
		watchdogStatusCmd,
		watchdogResetCmd,
		watchdogResetServiceCmd,
		watchdogHistoryCmd,
		watchdogTestAlertCmd,
	)
}
