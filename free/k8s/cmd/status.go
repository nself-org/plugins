// Purpose: `nself-k8s status` — shows the Helm release status. Behavior is
// unchanged from the pre-extraction `nself k8s status`.
//
// Inputs: --cluster, --release.
//
// Outputs: the Helm release status JSON, printed as-is.
//
// Constraints: pure move from cli/cmd/commands/k8s.go's k8sStatusCmd.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/k8s"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the Helm release status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cluster, _ := cmd.Flags().GetString("cluster")
		release, _ := cmd.Flags().GetString("release")
		out, err := k8s.Status(cmd.Context(), release, cluster)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	},
}

func init() {
	statusCmd.Flags().String("cluster", "", "Path to kubeconfig")
	statusCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
}
