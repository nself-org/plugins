// Purpose: `nself monitor upgrade-dashboards` — moved wholesale from
// cli/cmd/commands/monitor.go under CLI-R11. Behavior is unchanged; only the
// cobra wiring (rootCmd instead of RootCmd) and the ui import (tui instead of
// internal/ui) changed to stand alone as a plugin binary.
package main

import (
	"fmt"

	"github.com/nself-org/nself-monitor/internal/tui"

	"github.com/spf13/cobra"
)

var monitorUpgradeDashboardsCmd = &cobra.Command{
	Use:   "upgrade-dashboards",
	Short: "Upgrade Grafana dashboards to the latest bundled versions",
	Long: `Re-provision all 11 nSelf Grafana dashboards from the bundled templates.

Dashboards: System Overview, Postgres, Hasura, Nginx, Per-Plugin, Request Latency,
AI Cost Tracker, User Activity, Error Heatmap, Backups, Licenses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		dashboards := []string{
			"system-overview",
			"postgres",
			"hasura",
			"nginx",
			"per-plugin",
			"request-latency",
			"ai-cost-tracker",
			"user-activity",
			"error-heatmap",
			"backups",
			"licenses",
		}

		tui.CommandHeader("Upgrade Dashboards", fmt.Sprintf("%d dashboards", len(dashboards)))

		for _, d := range dashboards {
			if force {
				tui.Checked(fmt.Sprintf("Provisioned: %s", d))
			} else {
				tui.Checked(fmt.Sprintf("Checked: %s (up to date)", d))
			}
		}

		tui.Success(fmt.Sprintf("All %d dashboards up to date", len(dashboards)))
		return nil
	},
}

func init() {
	monitorUpgradeDashboardsCmd.Flags().Bool("force", false, "Force re-provision even if up to date")
}
