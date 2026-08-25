package main

// sentry.go — 'nself sentry' command group.
//
// Purpose: Parent command for ɳSentry operations (status, alerts, etc.).
//   Prints help when invoked without a subcommand.
// Inputs:  subcommand
// Outputs: help text or delegates to subcommand
// Constraints: no flags at parent level; add flags on subcommands only.
// SPORT: CLI-CMD-SENTRY-001

import (
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var sentryCmd = &cobra.Command{
	Use:   "sentry",
	Short: "ɳSentry ops: status, alerts, and observability",
	Long: `Manage and inspect ɳSentry observability resources.

Cloud subcommands (login, monitors, incidents, status-pages, alerts, whoami)
target the hosted SaaS at ` + "`https://api.sentry.nself.org`" + ` by default, or a
self-hosted/local sentry bundle via --api-url / NSELF_SENTRY_API_URL.
Auth: API key (nsk_*) via 'nself sentry login', --api-key, or NSELF_SENTRY_API_KEY.

A symlinked binary named 'nsentry' behaves as this command group:
  ln -s $(which nself) /usr/local/bin/nsentry
  nsentry monitors list

Examples:
  nself sentry login --api-key nsk_abc123...
  nself sentry monitors add --name api --url https://api.example.com --interval 60s
  nself sentry incidents list --status open
  nself sentry whoami --json
  nself sentry status --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
}

func main() {
	prefixUsageWithNself(sentryCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself sentry nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	sentryCmd.Args = cobra.ArbitraryArgs

	sentryCmd.CompletionOptions.DisableDefaultCmd = true
	sentryCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	sentryCmd.SilenceUsage = true

	if err := sentryCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without repeating
		// it. The CLI mirrors this status and stays silent, so the plugin's own
		// message is the only one the user sees.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself sentry <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
