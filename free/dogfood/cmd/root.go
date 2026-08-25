// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-dogfood <args...>`
// with the leading "dogfood" argument already stripped, so this root command
// takes the place `nself dogfood` used to occupy in the core binary — its
// subcommands (audit, report) are what `nself dogfood audit` /
// `nself dogfood report` resolve to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail, 2 warn-only, mirroring the
// exit codes documented on `nself dogfood audit --help` before extraction).
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go):
// without them cobra prints its own "Error: ..." plus a usage block on every
// RunE error, then main() prints "Error: ..." again — a fidelity bug found
// during the CLI-R11 mail/waf/encryption/federation slices and backfilled
// here since every later extraction copies this file as its template.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-dogfood",
	Short: "Production dogfood audit and reporting",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(auditCmd, reportCmd)
}
