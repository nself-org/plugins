package main

// Purpose: `nself claw migrate` — apply pending claw schema migrations.
// Moved from cli/cmd/commands/claw_migrate.go under CLI-R11: config.Load ->
// projectenv.Load, internal/ui -> local ui, version.Version -> this
// plugin's own pluginVersion (see below).

import (
	"fmt"
	"os"

	"github.com/nself-org/nself-claw/internal/claw"
	"github.com/nself-org/nself-claw/internal/projectenv"
	"github.com/nself-org/nself-claw/internal/ui"

	"github.com/spf13/cobra"
)

// pluginVersion is this plugin's own version, standing in for the core
// CLI's internal/version.Version (unreachable from this module) as the
// --to flag's default. Before extraction this defaulted to the running
// nself binary's version; there is no equivalent single version number
// once migrate runs from a separately-versioned plugin binary, so the
// plugin's own version (matched to plugin.json) is the closest analogue —
// "apply everything this plugin build knows about" by default.
const pluginVersion = "1.0.0"

var (
	clawMigrateFrom string
	clawMigrateTo   string
)

var clawMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply pending claw schema migrations",
	Long: `Apply pending claw database schema migrations in order.

By default the command reads the current schema version from the database
and applies every pending migration up to the latest available.

Use --from to skip migrations at or before a specific version.
Use --to to stop after applying a specific target version (default: this plugin's version).

Examples:
  nself claw migrate                     # apply all pending claw migrations
  nself claw migrate --to 005            # apply up to and including 005_*.sql
  nself claw migrate --from 002 --to 005 # apply only 003, 004, and 005`,
	RunE: runClawMigrate,
}

func init() {
	clawMigrateCmd.Flags().StringVar(&clawMigrateFrom, "from", "",
		"Skip migrations at or before this version (e.g. 003_add_index.sql)")
	clawMigrateCmd.Flags().StringVar(&clawMigrateTo, "to", pluginVersion,
		"Stop after applying this version (default: this plugin's version)")
}

func runClawMigrate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := projectenv.Load(cwd)
	if err != nil {
		return fmt.Errorf("loading project config: %w\nRun this command from your nself project directory", err)
	}

	ui.Section("claw migrate")
	ui.Info(fmt.Sprintf("From: %q  To: %q", clawMigrateFrom, clawMigrateTo))

	result, err := claw.Migrate(ctx, cfg, clawMigrateFrom, clawMigrateTo)
	if err != nil {
		return err
	}

	fmt.Println()
	if len(result.Applied) == 0 {
		ui.Success("No pending claw migrations — schema is up to date")
		return nil
	}

	ui.Success(fmt.Sprintf("Applied %d migration(s):", len(result.Applied)))
	for _, name := range result.Applied {
		fmt.Printf("  %s %s\n", ui.C(ui.Green, ui.IconSuccess), name)
	}
	if len(result.Skipped) > 0 {
		ui.Dimmed(fmt.Sprintf("Skipped %d (already applied or outside range)", len(result.Skipped)))
	}
	return nil
}
