// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-dlq <args...>` with
// the leading "dlq" argument already stripped, so this root command takes
// the place `nself dlq` used to occupy in the core binary — its subcommand
// (replay) is what `nself dlq replay` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself dlq replay --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. dlq_replay.go
// had zero internal/ dependencies beyond internal/dlq itself before
// extraction (it talks only to the nSelf REST API over HTTP, authenticated
// via NSELF_API_TOKEN/NSELF_API_URL), so this plugin needs no shim packages
// at all. SilenceUsage/SilenceErrors mirror core's RootCmd
// (cmd/commands/root.go): without them cobra prints its own "Error: ..."
// plus a usage block on every RunE error, then main() prints "Error: ..."
// again.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-dlq",
	Short: "Manage dead-letter queues for nSelf plugins",
	Long: `Manage dead-letter queues (DLQ) for nSelf plugins.

Dead-letter queues accumulate rows that failed processing. Use 'nself dlq replay'
to re-enqueue rows after fixing the upstream bug that caused the failures.

Subcommands:
  replay   Re-enqueue DLQ rows for a plugin back to the work queue`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(dlqReplayCmd)
}
