// Purpose: `nself-k8s status` — shows the Helm release status. Behavior is
// unchanged from the pre-extraction `nself k8s status`.
//
// Inputs: --cluster, --release.
//
// Outputs: the release summary JSON line (five fields only); with --json the
// same object is the data member of a v1 envelope.
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
		r := newInvocation(cmd, "status")
		if r.json && len(args) > 0 {
			return r.usageFail("status takes no arguments")
		}
		cluster, _ := cmd.Flags().GetString("cluster")
		release, _ := cmd.Flags().GetString("release")
		if r.json {
			sum, err := k8s.ReleaseStatus(cmd.Context(), release, cluster)
			if err != nil {
				return r.fail(err)
			}
			return r.done(sum)
		}
		out, err := k8s.Status(cmd.Context(), release, cluster)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	},
}

func init() {
	addJSONFlag(statusCmd)
	// status prints its usage block on a failure (cobra default, kept for the
	// human-output goldens), so the new flag is hidden from that listing to keep
	// those bytes unchanged. It is documented in the wiki and the manifest.
	_ = statusCmd.Flags().MarkHidden("json")
	statusCmd.Flags().String("cluster", "", "Path to kubeconfig")
	statusCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
}
