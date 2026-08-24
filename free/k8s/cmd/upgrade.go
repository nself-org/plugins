// Purpose: `nself-k8s upgrade` — upgrades the nSelf Helm release with a
// rolling update. Behavior is unchanged from the pre-extraction
// `nself k8s upgrade`.
//
// Inputs: --cluster, --release.
//
// Outputs: helm's own stdout/stderr (inherited), plus an Info/Success line
// from the tui package around the helm invocation.
//
// Constraints: pure move from cli/cmd/commands/k8s.go's k8sUpgradeCmd.
package main

import (
	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/tui"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the nSelf Helm release",
	Long:  `Upgrade the nSelf Helm chart to the latest version with a rolling update.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cluster, _ := cmd.Flags().GetString("cluster")
		release, _ := cmd.Flags().GetString("release")
		opts := k8s.InstallOptions{
			ReleaseName: release,
			Kubeconfig:  cluster,
		}
		tui.Info("Upgrading nSelf Helm release...")
		if err := k8s.Upgrade(cmd.Context(), opts); err != nil {
			return err
		}
		tui.Success("nSelf upgraded successfully.")
		return nil
	},
}

func init() {
	upgradeCmd.Flags().String("cluster", "", "Path to kubeconfig")
	upgradeCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
}
