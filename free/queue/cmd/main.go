package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/nself-queue/internal/config"
	"github.com/nself-org/nself-queue/internal/queue"
	"github.com/nself-org/nself-queue/internal/ui"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage async job queues (pg-boss)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func getDBURL() string {
	cwd, _ := os.Getwd()
	cfg, err := config.Load(cwd)
	if err != nil {
		return "postgresql://postgres:postgres@localhost:5432/nself"
	}
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
		cfg.Postgres.User, cfg.Postgres.Password,
		cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DB)
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all queues with job counts",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		ctx := cmd.Context()

		queues, err := queue.ListQueues(ctx, getDBURL())
		if err != nil {
			return fmt.Errorf("list queues: %w", err)
		}

		if jsonOut {
			data, _ := json.MarshalIndent(queues, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(queues) == 0 {
			ui.Bullet("No queues found")
			return nil
		}

		ui.CommandHeader("Job Queues", fmt.Sprintf("%d queues", len(queues)))
		fmt.Printf("  %-30s %8s %8s %8s %8s\n", "QUEUE", "PENDING", "ACTIVE", "FAILED", "DEAD")
		ui.Separator()
		for _, q := range queues {
			fmt.Printf("  %-30s %8d %8d %8d %8d\n", q.Name, q.Pending, q.Active, q.Failed, q.Dead)
		}
		return nil
	},
}

var queueJobsCmd = &cobra.Command{
	Use:   "jobs <queue>",
	Short: "List jobs in a queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		state, _ := cmd.Flags().GetString("state")
		limit, _ := cmd.Flags().GetInt("limit")
		jsonOut, _ := cmd.Flags().GetBool("json")
		ctx := cmd.Context()

		jobs, err := queue.ListJobs(ctx, getDBURL(), args[0], state, limit)
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}

		if jsonOut {
			data, _ := json.MarshalIndent(jobs, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(jobs) == 0 {
			ui.Bullet("No jobs found")
			return nil
		}

		ui.CommandHeader("Jobs", fmt.Sprintf("queue=%s, %d jobs", args[0], len(jobs)))
		for _, j := range jobs {
			fmt.Printf("  %s  %-10s  retries=%d  %s\n", j.ID, j.State, j.RetryCount, j.CreatedAt.Format(time.RFC3339))
		}
		return nil
	},
}

var queueRetryCmd = &cobra.Command{
	Use:   "retry <job-id>",
	Short: "Retry a failed or dead job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := queue.RetryJob(cmd.Context(), getDBURL(), args[0]); err != nil {
			return fmt.Errorf("retry job: %w", err)
		}
		ui.Success(fmt.Sprintf("Job %s queued for retry", args[0]))
		return nil
	},
}

var queuePurgeCmd = &cobra.Command{
	Use:   "purge <queue>",
	Short: "Purge completed/dead jobs older than specified duration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		olderThanStr, _ := cmd.Flags().GetString("older-than")
		olderThan, err := time.ParseDuration(olderThanStr)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", olderThanStr, err)
		}

		count, err := queue.PurgeQueue(cmd.Context(), getDBURL(), args[0], olderThan)
		if err != nil {
			return fmt.Errorf("purge queue: %w", err)
		}
		ui.Success(fmt.Sprintf("Purged %d jobs from %s", count, args[0]))
		return nil
	},
}

var queueCronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage scheduled cron jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var queueCronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled cron jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		ctx := cmd.Context()

		crons, err := queue.ListCrons(ctx, getDBURL())
		if err != nil {
			return fmt.Errorf("list crons: %w", err)
		}

		if jsonOut {
			data, _ := json.MarshalIndent(crons, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(crons) == 0 {
			ui.Bullet("No cron jobs configured")
			return nil
		}

		ui.CommandHeader("Cron Jobs", fmt.Sprintf("%d jobs", len(crons)))
		for _, c := range crons {
			status := "enabled"
			if !c.Enabled {
				status = "disabled"
			}
			fmt.Printf("  %-30s  %-20s  %s\n", c.Name, c.Schedule, status)
		}
		return nil
	},
}

func init() {
	queueListCmd.Flags().Bool("json", false, "JSON output")
	queueJobsCmd.Flags().String("state", "", "Filter by state (pending, active, failed, dead)")
	queueJobsCmd.Flags().Int("limit", 50, "Maximum jobs to return")
	queueJobsCmd.Flags().Bool("json", false, "JSON output")
	queuePurgeCmd.Flags().String("older-than", "720h", "Purge jobs older than duration (default 30d)")
	queueCronListCmd.Flags().Bool("json", false, "JSON output")

	queueCronCmd.AddCommand(queueCronListCmd)
	queueCmd.AddCommand(queueListCmd, queueJobsCmd, queueRetryCmd, queuePurgeCmd, queueCronCmd)
}

func main() {
	prefixUsageWithNself(queueCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself queue nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	queueCmd.Args = cobra.ArbitraryArgs

	queueCmd.CompletionOptions.DisableDefaultCmd = true
	queueCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	queueCmd.SilenceUsage = true

	if err := queueCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without repeating
		// it. The CLI mirrors this status and stays silent, so the plugin's own
		// message is the only one the user sees.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself queue <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
