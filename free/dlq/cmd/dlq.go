// Purpose: the `replay` subcommand, moved verbatim from the core CLI's
// cmd/commands/dlq_replay.go under CLI-R11.
//
// Inputs: cobra flags (--max-rows, --filter, --dry-run, --base-url).
//
// Outputs: per-row replay results to stdout/stderr; a non-zero error if any
// row fails.
//
// Constraints: no dependency on the core CLI's internal/* packages.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-dlq/internal/dlq"
)

var dlqReplayCmd = &cobra.Command{
	Use:   "replay <plugin>",
	Short: "Re-enqueue DLQ rows for a plugin",
	Long: `Re-enqueue dead-letter queue rows for the named plugin back to the work queue.

WARNING: Only replay after the upstream bug causing the failures is fixed.
         Replaying before fixing the bug causes rows to re-DLQ immediately.

Safe defaults:
  --max-rows 100  Prevents accidental DLQ floods
  --dry-run       Preview without executing

Failures are reported per-row; the command exits non-zero if any row fails.
Operator-level authentication is required (set NSELF_API_TOKEN or NSELF_API_URL).

Examples:
  nself dlq replay mux
  nself dlq replay mux --dry-run
  nself dlq replay mux --max-rows 50
  nself dlq replay mux --filter status=quarantined
  nself dlq replay mux --filter status=quarantined --max-rows 10`,
	Args: cobra.ExactArgs(1),
	RunE: runDLQReplay,
}

func init() {
	f := dlqReplayCmd.Flags()
	f.Int("max-rows", 100, "Maximum rows to replay (prevents floods)")
	f.StringArray("filter", nil, "Filter as field=value (e.g. --filter status=quarantined)")
	f.Bool("dry-run", false, "Preview replay without executing")
	f.String("base-url", "", "nSelf API base URL (overrides NSELF_API_URL)")
}

func runDLQReplay(cmd *cobra.Command, args []string) error {
	plugin := args[0]
	maxRows, _ := cmd.Flags().GetInt("max-rows")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	filterArgs, _ := cmd.Flags().GetStringArray("filter")
	baseURL, _ := cmd.Flags().GetString("base-url")

	// Parse filter flags into map.
	filter := make(map[string]string)
	for _, f := range filterArgs {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid filter format %q: expected field=value", f)
		}
		filter[parts[0]] = parts[1]
	}

	_, err := dlq.Replay(cmd.Context(), dlq.ReplayOptions{
		Plugin:  plugin,
		MaxRows: maxRows,
		Filter:  filter,
		DryRun:  dryRun,
		BaseURL: baseURL,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	return err
}
