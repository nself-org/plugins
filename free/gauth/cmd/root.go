// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-gauth <args...>`
// with the leading "gauth" argument already stripped, so this root command
// takes the place `nself gauth` used to occupy in the core binary — its
// subcommands (status, refresh, revoke) are what `nself gauth status` /
// `nself gauth refresh` / `nself gauth revoke` resolve to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself gauth --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// gauth.go had zero internal/* imports before extraction (a pure HTTP client
// to plugin-gauth), so this is a verbatim move, not an adaptation.
// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go).
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "nself-gauth",
	Short:         "Manage Google OAuth tokens for nSelf AI services",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Manage headless Google OAuth tokens used by plugin-gauth (:3762).

Subcommands:
  status   Show token expiry for all provisioned accounts
  refresh  Force-refresh a specific account's access token
  revoke   Revoke and remove a stored refresh token`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	// Status flags
	gauthStatusCmd.Flags().BoolVar(&gauthStatusJSON, "json", false, "Output as raw JSON")
	gauthStatusCmd.Flags().StringVar(&gauthStatusAccount, "account", "", "Filter to a single account ID")

	// Refresh flags
	gauthRefreshCmd.Flags().StringVar(&gauthRefreshAccount, "account", "", "Account ID to refresh (required)")
	gauthRefreshCmd.Flags().BoolVar(&gauthRefreshForce, "force", false, "Bypass cache and force a new token from Google")
	_ = gauthRefreshCmd.MarkFlagRequired("account")

	// Revoke flags
	gauthRevokeCmd.Flags().StringVar(&gauthRevokeAccount, "account", "", "Account ID to revoke (required)")
	_ = gauthRevokeCmd.MarkFlagRequired("account")

	rootCmd.AddCommand(gauthStatusCmd)
	rootCmd.AddCommand(gauthRefreshCmd)
	rootCmd.AddCommand(gauthRevokeCmd)
}
