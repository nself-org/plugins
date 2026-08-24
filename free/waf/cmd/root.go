// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-waf <args...>` with
// the leading "waf" argument already stripped, so this root command takes
// the place `nself waf` used to occupy in the core binary — its subcommands
// (enable, mode, report) are what `nself waf <sub>` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself waf --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// waf.go depended on internal/config.FindNSelfRoot and internal/ui; both are
// reimplemented in internal/projectroot and internal/tui respectively.
// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go):
// without them cobra prints its own "Error: ..." plus a usage block on every
// RunE error, then main() prints "Error: ..." again — a visible regression
// from the single, clean error line `nself waf ...` produced before
// extraction.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-waf",
	Short: "Manage the Web Application Firewall (Coraza + OWASP CRS)",
	Long: `Manage the Coraza WAF integrated with nginx.

Subcommands:
  enable    Enable the WAF in detection mode
  mode      Switch between detection and blocking mode
  report    Show recent WAF events`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(wafEnableCmd)
	rootCmd.AddCommand(wafModeCmd)
	rootCmd.AddCommand(wafReportCmd)
}
