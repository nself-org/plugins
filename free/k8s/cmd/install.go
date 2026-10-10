// Purpose: `nself-k8s install` installs the nSelf Helm chart that is embedded
// in this binary, with the values `nself k8s values` generated.
//
// Inputs: --domain (required), --cluster, --release, --plugins, --project-dir,
// --wait, --timeout, the NSELF_PLUGIN_LICENSE_KEY env var, and the project's
// .nself/generated/k8s/values.yaml and secrets.yaml.
//
// Outputs: helm's own stdout/stderr (inherited), plus an Info/Success line
// from the tui package around the helm invocation; the success line lists the
// ingress hosts of the generated values, or no URL. Exit 1 when the generated
// values are missing ("run nself k8s values").
//
// Constraints: no chart repository, no secret and no licence key on argv (they
// travel in values files); the shared flag helpers also serve upgrade.go.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	nselfk8s "github.com/nself-org/nself-k8s"
	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/tui"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install nSelf on a Kubernetes cluster",
	Long: `Install the nSelf Helm chart on a Kubernetes cluster.

The chart is embedded in this plugin and is installed from a temporary
directory with the values that 'nself k8s values' generated
(.nself/generated/k8s/values.yaml and secrets.yaml). Run that command first.
The licence key (NSELF_PLUGIN_LICENSE_KEY) and the secrets reach helm through
values files, never on the command line.

Not consumed yet: --domain, --plugins and the licence key are recorded in the
release values, but the embedded chart does not read them (D-0311). The hosts
the stack answers on come from the generated ingress rules.

TLS is managed by cert-manager (must be pre-installed in the cluster).

Example:
  nself k8s values
  nself k8s install --domain myapp.com
  nself k8s install --domain myapp.com --cluster ~/.kube/config --release my-nself`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true // a missing values file is not a usage error
		opts := installOptions(cmd)
		if opts.Domain == "" {
			return fmt.Errorf("--domain is required")
		}
		tui.Info(fmt.Sprintf("Installing nSelf chart (domain=%s, recorded in the release values only)...", opts.Domain))
		if err := k8s.Install(cmd.Context(), opts); err != nil {
			return err
		}
		tui.Success(installedLine(opts.ProjectDir))
		return nil
	},
}

// installedLine is the success line: the ingress hosts of the generated
// values, or no URL when the project routes none. --domain is not used here
// because the chart does not consume it yet (D-0311).
func installedLine(projectDir string) string {
	hosts, err := k8s.IngressHosts(projectDir)
	if err != nil || len(hosts) == 0 {
		return "nSelf installed. The generated values route no ingress host."
	}
	urls := make([]string, len(hosts))
	for i, h := range hosts {
		urls[i] = "https://" + h
	}
	return "nSelf installed. Ingress hosts: " + strings.Join(urls, ", ")
}

// addInstallFlags registers the flags install and upgrade share.
func addInstallFlags(c *cobra.Command) {
	c.Flags().String("cluster", "", "Path to kubeconfig (default: helm reads the KUBECONFIG env var or ~/.kube/config)")
	c.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
	c.Flags().String("plugins", "", "Comma-separated plugins, recorded in the release values (the chart does not consume them yet, D-0311)")
	c.Flags().String("project-dir", ".", "nSelf project directory (holds .nself/generated/k8s)")
	c.Flags().Bool("wait", true, "Wait until every workload is ready")
	c.Flags().String("timeout", k8s.DefaultTimeout, "How long helm waits for readiness")
}

// installOptions reads the shared flags and the licence env var.
func installOptions(cmd *cobra.Command) k8s.InstallOptions {
	domain, _ := cmd.Flags().GetString("domain")
	cluster, _ := cmd.Flags().GetString("cluster")
	release, _ := cmd.Flags().GetString("release")
	pluginsRaw, _ := cmd.Flags().GetString("plugins")
	dir, _ := cmd.Flags().GetString("project-dir")
	wait, _ := cmd.Flags().GetBool("wait")
	timeout, _ := cmd.Flags().GetString("timeout")
	var plugins []string
	for _, p := range strings.Split(pluginsRaw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			plugins = append(plugins, p)
		}
	}
	return k8s.InstallOptions{
		ReleaseName: release,
		Domain:      domain,
		Kubeconfig:  cluster,
		LicenseKey:  os.Getenv("NSELF_PLUGIN_LICENSE_KEY"),
		Plugins:     plugins,
		ProjectDir:  dir,
		Chart:       nselfk8s.Chart,
		ChartRoot:   nselfk8s.ChartRoot,
		Wait:        wait,
		Timeout:     timeout,
	}
}

func init() {
	installCmd.Flags().String("domain", "", "Domain, recorded in the release values (required; the chart does not consume it yet, D-0311)")
	addInstallFlags(installCmd)
}
