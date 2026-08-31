// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-encryption <args...>`
// with the leading "encryption" argument already stripped, so this root
// command takes the place `nself encryption` used to occupy in the core
// binary — its subcommands (configure, verify, rotate, status, key-events)
// are what `nself encryption <sub>` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// documented on `nself encryption --help` before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
// encryption.go had zero internal/ dependencies before extraction (it only
// ever talked to the BYOK plugin's own HTTP API), so this plugin needs no
// shim packages at all — a straight file move.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-encryption",
	Short: "Manage BYOK per-tenant encryption (Enterprise)",
	Long: `Bring Your Own Key (BYOK) encryption for nSelf Cloud.

Each tenant can supply their own Customer Managed Key (CMK) hosted in
AWS KMS, GCP Cloud KMS, or HashiCorp Vault Transit. nSelf uses envelope
encryption: data is encrypted with a Data Encryption Key (DEK), and the
DEK is wrapped by the tenant's CMK.

Requires an Enterprise license (NSELF_BYOK=true).

Subcommands:
  configure   Configure a KMS provider for the current tenant
  verify      Test KMS connectivity (wrap+unwrap round-trip)
  rotate      Rotate data encryption keys (re-wrap existing DEKs)
  status      Show current BYOK configuration and last verification
  key-events  List the key event audit trail`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	// SilenceUsage/SilenceErrors mirror core's RootCmd (cmd/commands/root.go):
	// without them cobra prints its own "Error: ..." plus a usage block on
	// every RunE error, then main() prints "Error: ..." again.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(encryptionConfigureCmd)
	rootCmd.AddCommand(encryptionVerifyCmd)
	rootCmd.AddCommand(encryptionRotateCmd)
	rootCmd.AddCommand(encryptionStatusCmd)
	rootCmd.AddCommand(encryptionKeyEventsCmd)
}
