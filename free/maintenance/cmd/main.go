package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/nself-maintenance/internal/maintenance"
	"github.com/nself-org/nself-maintenance/internal/ui"
	"github.com/spf13/cobra"
)

// ── Parent command ────────────────────────────────────────────────────────────

var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Maintenance utilities: disk cleanup, scheduler",
	Long: `Maintenance utilities for nSelf deployments.

Subcommands:
  disk-cleanup   Free Docker images, old logs, and journal entries
  schedule       Install or remove the daily cleanup timer
  status         Show scheduler status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage: true,
}

// buildCaches is opt-in on purpose. On a workstation these caches are the
// difference between a fast build and a slow one; on a CI host they are
// usually the largest thing on the disk and the standard steps do not touch
// them (2026-09-02: standard cleanup freed ~1 GB on a host whose build caches
// held ~15 GB, after it had already hit 100%).
var buildCaches bool

// ── disk-cleanup ──────────────────────────────────────────────────────────────

var maintenanceDiskCleanupCmd = &cobra.Command{
	Use:   "disk-cleanup",
	Short: "Free disk space: Docker prune, log rotation, journal vacuum",
	Long: `Free disk space by running three cleanup steps:

  1. docker system prune -af --volumes=false
     Removes stopped containers and unused images. Volumes are kept.

  2. find /var/log -name '*.gz' -mtime +14 -delete
     Removes compressed log files older than 14 days.

  3. journalctl --vacuum-time=7d
     Removes journal entries older than 7 days (Linux only; no-op on macOS).

Disk usage is reported before and after the cleanup.

With --build-caches, additionally clears re-derivable build caches using each
tool's own cleanup command (go clean -cache -modcache, pnpm store prune, npm
cache clean, docker builder prune). Off by default: on a workstation those
caches are what make builds fast. On a CI host they are usually the largest
thing on the disk and none of the three steps above touch them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.CommandHeader("nSelf Maintenance", "Disk cleanup")

		ui.Section("Before cleanup")
		before, err := maintenance.GetDiskUsage()
		if err != nil {
			ui.Warn(fmt.Sprintf("Could not read disk usage: %v", err))
		} else {
			printDiskUsage(before)
		}

		ui.Section("Running cleanup steps")

		result := maintenance.DiskCleanup()

		if result.DockerPruneOut != "" {
			ui.Info("Docker prune:")
			for _, line := range splitLines(result.DockerPruneOut) {
				fmt.Printf("  %s\n", line)
			}
		}
		if result.LogRotationOut != "" {
			ui.Info("Log rotation:")
			for _, line := range splitLines(result.LogRotationOut) {
				fmt.Printf("  %s\n", line)
			}
		}
		if result.JournalVacuumOut != "" {
			ui.Info("Journal vacuum:")
			for _, line := range splitLines(result.JournalVacuumOut) {
				fmt.Printf("  %s\n", line)
			}
		}

		if len(result.Errors) > 0 {
			ui.Warn("Some steps completed with warnings:")
			for _, e := range result.Errors {
				ui.Warn(fmt.Sprintf("  %v", e))
			}
		}

		after := result.After
		if buildCaches {
			ui.Section("Clearing build caches")
			bc := maintenance.CleanBuildCaches()
			if len(bc.Ran) > 0 {
				ui.Info(fmt.Sprintf("Cleared: %s", strings.Join(bc.Ran, ", ")))
			}
			if len(bc.Skipped) > 0 {
				ui.Dimmed(fmt.Sprintf("  Not installed, skipped: %s", strings.Join(bc.Skipped, ", ")))
			}
			for _, e := range bc.Errors {
				ui.Warn(fmt.Sprintf("  %v", e))
			}
			after = bc.After
		}

		ui.Section("After cleanup")
		printDiskUsage(after)

		freed := result.Before.UsedGB - after.UsedGB
		if freed > 0.01 {
			ui.Success(fmt.Sprintf("Freed %.2f GB (disk %d%% → %d%% used)",
				freed, result.Before.UsedPercent, after.UsedPercent))
		} else {
			ui.Info(fmt.Sprintf("Disk usage unchanged at %d%%", after.UsedPercent))
		}

		if after.UsedPercent > 70 {
			ui.Warn(fmt.Sprintf(
				"Disk is still %d%% full. Run `nself maintenance schedule --daily` to enable automatic daily cleanup.",
				after.UsedPercent,
			))
		}

		return nil
	},
	SilenceUsage: true,
}

// ── schedule ──────────────────────────────────────────────────────────────────

var maintenanceScheduleDaily bool
var maintenanceScheduleOff bool

var maintenanceScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Install or remove the daily disk-cleanup timer",
	Long: `Install or remove a system-level timer that runs disk-cleanup every day at 03:00 local time.

On Linux, installs a systemd service + timer unit.
On macOS, installs a LaunchDaemon plist.

Flags:
  --daily   Install the daily timer (requires root/sudo)
  --off     Remove the daily timer (requires root/sudo)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !maintenanceScheduleDaily && !maintenanceScheduleOff {
			return cmd.Help()
		}

		if maintenanceScheduleDaily && maintenanceScheduleOff {
			return fmt.Errorf("specify either --daily or --off, not both")
		}

		if maintenanceScheduleDaily {
			ui.CommandHeader("nSelf Maintenance", "Installing daily timer")
			if err := maintenance.InstallDailyTimer(); err != nil {
				return fmt.Errorf("install daily timer: %w", err)
			}
			ui.Success("Daily disk-cleanup timer installed (runs at 03:00 local time).")
			ui.Info("Use `nself maintenance status` to verify.")
			return nil
		}

		// --off
		ui.CommandHeader("nSelf Maintenance", "Removing daily timer")
		if err := maintenance.RemoveDailyTimer(); err != nil {
			return fmt.Errorf("remove daily timer: %w", err)
		}
		ui.Success("Daily disk-cleanup timer removed.")
		return nil
	},
	SilenceUsage: true,
}

// ── status ────────────────────────────────────────────────────────────────────

var maintenanceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daily cleanup scheduler status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.CommandHeader("nSelf Maintenance", "Scheduler status")

		status := maintenance.GetScheduleStatus()
		fmt.Printf("  Platform : %s\n", status.Platform)
		if status.Enabled {
			ui.Success("Daily cleanup timer is enabled")
		} else {
			ui.Warn("Daily cleanup timer is NOT enabled")
			ui.Info("Run `nself maintenance schedule --daily` to enable it.")
		}
		if status.Detail != "" {
			fmt.Printf("  Detail   : %s\n", status.Detail)
		}

		fmt.Println()
		usage, err := maintenance.GetDiskUsage()
		if err == nil {
			ui.Section("Current disk usage")
			printDiskUsage(usage)
			if usage.UsedPercent > 70 {
				ui.Warn(fmt.Sprintf(
					"Disk is %d%% full. Run `nself maintenance schedule --daily` to enable automatic daily cleanup.",
					usage.UsedPercent,
				))
			}
		}
		return nil
	},
	SilenceUsage: true,
}

// ── helpers ───────────────────────────────────────────────────────────────────

func printDiskUsage(u maintenance.DiskUsage) {
	bar := diskBar(u.UsedPercent, 30)
	fmt.Printf("  Used: %5.1f GB / %.1f GB total  [%s] %d%%\n",
		u.UsedGB, u.TotalGB, bar, u.UsedPercent)
	fmt.Printf("  Free: %.1f GB\n", u.FreeGB)
}

func diskBar(pct, width int) string {
	filled := (pct * width) / 100
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// ── registration ──────────────────────────────────────────────────────────────

func init() {
	maintenanceScheduleCmd.Flags().BoolVar(&maintenanceScheduleDaily, "daily", false, "Install daily cleanup timer at 03:00 local time")
	maintenanceScheduleCmd.Flags().BoolVar(&maintenanceScheduleOff, "off", false, "Remove the daily cleanup timer")

	maintenanceDiskCleanupCmd.Flags().BoolVar(&buildCaches, "build-caches", false,
		"Also clear re-derivable build caches (go, pnpm, npm, docker builder). Off by default: on a workstation these make builds fast; on a CI host they are usually the largest consumer.")
	maintenanceCmd.AddCommand(maintenanceDiskCleanupCmd)
	maintenanceCmd.AddCommand(maintenanceScheduleCmd)
	maintenanceCmd.AddCommand(maintenanceStatusCmd)
}

func main() {
	// Users reach this binary by typing `nself maintenance ...` — the CLI proxies to
	// it — so every usage line has to read "nself maintenance ...". Setting Use is not
	// enough: cobra derives CommandPath from Name(), the first WORD of Use, so
	// the "[command]" line would disagree with the flags line. Prefixing the
	// template is correct at every depth.
	prefixUsageWithNself(maintenanceCmd)

	// Cobra's default Args validator rejects an unrecognised first argument only
	// for a ROOT command with subcommands; for a child it passes them to RunE.
	// Inside the CLI this was a child, so `nself maintenance nosuch` printed help.
	// ArbitraryArgs restores that.
	maintenanceCmd.Args = cobra.ArbitraryArgs

	// Cobra adds `completion` and `help` to any root. Inside the CLI those lived
	// on nself's root, not under this command, so advertising them here would
	// show subcommands that did not exist before.
	maintenanceCmd.CompletionOptions.DisableDefaultCmd = true
	maintenanceCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	maintenanceCmd.SilenceUsage = true

	if err := maintenanceCmd.Execute(); err != nil {
		// cobra already printed it; exit non-zero without repeating. The CLI
		// mirrors this status silently, so the plugin's message is the only one.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Cobra passes templates to subcommands, so one
// call covers the whole tree.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
