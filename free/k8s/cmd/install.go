// Purpose: `nself-k8s install` — installs the nSelf Helm chart on a
// Kubernetes cluster. Behavior is unchanged from the pre-extraction
// `nself k8s install`: same flags, same output.
//
// Inputs: --domain (required), --cluster, --release, --plugins, and the
// NSELF_PLUGIN_LICENSE_KEY env var.
//
// Outputs: helm's own stdout/stderr (inherited), plus an Info/Success line
// from the tui package around the helm invocation.
//
// Constraints: pure move from cli/cmd/commands/k8s.go's k8sInstallCmd.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/tui"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install nSelf on a Kubernetes cluster",
	Long: `Install the nSelf Helm chart on a Kubernetes cluster.

The chart deploys Postgres (StatefulSet), Hasura, Auth, and Nginx Ingress.
TLS is managed by cert-manager (must be pre-installed in the cluster).

Example:
  nself k8s install --domain myapp.com
  nself k8s install --domain myapp.com --cluster ~/.kube/config --release my-nself`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		cluster, _ := cmd.Flags().GetString("cluster")
		release, _ := cmd.Flags().GetString("release")
		pluginsRaw, _ := cmd.Flags().GetString("plugins")
		licenseKey := os.Getenv("NSELF_PLUGIN_LICENSE_KEY")

		var plugins []string
		if pluginsRaw != "" {
			for _, p := range strings.Split(pluginsRaw, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					plugins = append(plugins, p)
				}
			}
		}

		tui.Info("Adding nSelf Helm repository...")
		if err := k8s.RepoAdd(cmd.Context()); err != nil {
			return err
		}

		opts := k8s.InstallOptions{
			ReleaseName: release,
			Domain:      domain,
			Kubeconfig:  cluster,
			LicenseKey:  licenseKey,
			Plugins:     plugins,
		}
		tui.Info(fmt.Sprintf("Installing nSelf chart (domain=%s)...", domain))
		if err := k8s.Install(cmd.Context(), opts); err != nil {
			return err
		}
		tui.Success(fmt.Sprintf("nSelf installed. Access your stack at https://%s", domain))
		return nil
	},
}

func init() {
	installCmd.Flags().String("domain", "", "Domain for the nSelf deployment (required)")
	installCmd.Flags().String("cluster", "", "Path to kubeconfig (defaults to KUBECONFIG env or ~/.kube/config)")
	installCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
	installCmd.Flags().String("plugins", "", "Comma-separated list of plugins to install")
}
