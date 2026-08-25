package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// webhookOutboxDir is the durable queue directory for webhook events
// that could not be written to Postgres at receipt time.
const webhookOutboxDir = "/var/lib/nself/webhook-outbox"

var webhooksCmd = &cobra.Command{
	Use:   "webhooks",
	Short: "Manage webhook processing and outbox",
}

var webhooksOutboxCmd = &cobra.Command{
	Use:   "outbox",
	Short: "Manage the webhook durable outbox queue",
}

var webhooksOutboxStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current outbox status",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")

		dir := webhookOutboxDir
		if envDir := os.Getenv("NSELF_WEBHOOK_OUTBOX_DIR"); envDir != "" {
			dir = envDir
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return outputOutboxStatus(format, dir, 0, nil)
			}
			return fmt.Errorf("reading outbox directory: %w", err)
		}

		var files []string
		for _, e := range entries {
			if !e.IsDir() {
				files = append(files, e.Name())
			}
		}

		return outputOutboxStatus(format, dir, len(files), files)
	},
}

type outboxStatus struct {
	Dir    string   `json:"dir"`
	Depth  int      `json:"depth"`
	Events []string `json:"events,omitempty"`
}

func outputOutboxStatus(format, dir string, depth int, files []string) error {
	status := outboxStatus{
		Dir:    dir,
		Depth:  depth,
		Events: files,
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Outbox directory: %s\n", dir)
	fmt.Printf("Pending events:   %d\n", depth)
	if depth > 0 {
		fmt.Println("\nQueued events:")
		for _, f := range files {
			fmt.Printf("  %s\n", f)
		}
	}
	return nil
}

// EnsureOutboxDir creates the outbox directory if it does not exist.
// Called during nself setup to ensure the durable queue is ready.
func EnsureOutboxDir() error {
	dir := webhookOutboxDir
	if envDir := os.Getenv("NSELF_WEBHOOK_OUTBOX_DIR"); envDir != "" {
		dir = envDir
	}
	return os.MkdirAll(dir, 0755)
}

func init() {
	webhooksOutboxStatusCmd.Flags().StringP("format", "f", "text", "Output format: text or json")
	webhooksOutboxCmd.AddCommand(webhooksOutboxStatusCmd)
	webhooksCmd.AddCommand(webhooksOutboxCmd)
}

func main() {
	prefixUsageWithNself(webhooksCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself webhooks nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	webhooksCmd.Args = cobra.ArbitraryArgs

	webhooksCmd.CompletionOptions.DisableDefaultCmd = true
	webhooksCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	webhooksCmd.SilenceUsage = true

	if err := webhooksCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without repeating
		// it. The CLI mirrors this status and stays silent, so the plugin's own
		// message is the only one the user sees.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself webhooks <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
