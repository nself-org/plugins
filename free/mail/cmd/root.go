// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-mail <args...>` with
// the leading "mail" argument already stripped, so this root command takes
// the place `nself mail` used to occupy in the core binary — its
// subcommands (send, broadcast, status, templates, dkim) are what
// `nself mail <sub>` resolves to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail, 2 no-license-configured,
// mirroring mailExitNoLicense in core).
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. mail.go
// depended on internal/license.CollectLicenseKeys, internal/plugin.
// ExitCodeError, and internal/ui; all are reimplemented standalone in
// internal/licensekeys, main.go's local exitCodeError, and internal/tui
// respectively. The domain package internal/mail (the ping_api HTTP client)
// moved wholesale, unchanged. SilenceUsage/SilenceErrors mirror core's
// RootCmd (cmd/commands/root.go): without them cobra prints its own
// "Error: ..." plus a usage block on every RunE error, then main() prints
// "Error: ..." again.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-mail",
	Short: "Send transactional and broadcast email through the nSelf stack",
	Long: `Send transactional and broadcast email through the nSelf stack.

The 'nself mail' command wraps the mux + Postmark plugins. ping_api proxies
each call to the running stack, so the Postmark plugin must be installed
and a valid license key must be configured.

Subcommands:
  send         Send a single transactional email
  broadcast    Send a broadcast to a list using a saved template
  status       Query delivery status for a message
  templates    Manage Postmark templates (list)
  dkim         Manage DKIM (verify)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// send flags
	mailSendCmd.Flags().String("to", "", "Recipient email address")
	mailSendCmd.Flags().String("subject", "", "Email subject")
	mailSendCmd.Flags().String("body", "", "Email body (inline)")
	mailSendCmd.Flags().String("body-file", "", "Read body from file ('-' for stdin)")
	mailSendCmd.Flags().String("body-type", "text", "Body type: text or html")
	mailSendCmd.Flags().Bool("json", false, "Output as JSON")

	// broadcast flags
	mailBroadcastCmd.Flags().String("list", "", "Mailing list ID")
	mailBroadcastCmd.Flags().String("template", "", "Postmark template ID")
	mailBroadcastCmd.Flags().Bool("json", false, "Output as JSON")

	// status flags
	mailStatusCmd.Flags().String("message-id", "", "Message ID returned by 'nself mail send'")
	mailStatusCmd.Flags().Bool("json", false, "Output as JSON")

	// templates list flags
	mailTemplatesListCmd.Flags().Bool("json", false, "Output as JSON")

	// dkim verify flags
	mailDKIMVerifyCmd.Flags().String("domain", "", "Domain to verify (e.g. example.com)")
	mailDKIMVerifyCmd.Flags().Bool("json", false, "Output as JSON")

	mailTemplatesCmd.AddCommand(mailTemplatesListCmd)
	mailDKIMCmd.AddCommand(mailDKIMVerifyCmd)

	rootCmd.AddCommand(mailSendCmd)
	rootCmd.AddCommand(mailBroadcastCmd)
	rootCmd.AddCommand(mailStatusCmd)
	rootCmd.AddCommand(mailTemplatesCmd)
	rootCmd.AddCommand(mailDKIMCmd)
}
