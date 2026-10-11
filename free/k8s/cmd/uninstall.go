// Purpose: `nself-k8s uninstall` removes the nSelf Helm release (helm uninstall).
//
// Inputs: --release, --cluster, --yes, --json, and stdin.
//
// Outputs: helm's own output (stdout goes to stderr under --json), plus an
// Info/Success line in human mode; with --json one envelope whose data is
// {release, uninstalled}. Without --yes and without an interactive terminal (or
// with --json) the command refuses with E403 and exit status 4 and runs no
// helm command; on a terminal it asks for the release name instead.
//
// Constraints: destructive, so the refusal happens before helm is looked up.
// A refusal is exit 4 in human mode too. Persistent volumes the chart created
// may outlive the release (helm does not delete them); the help text says so.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/k8s"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the nSelf Helm release",
	Long: `Remove the nSelf Helm release with helm uninstall.

This is destructive. Pass --yes to confirm. Without --yes, a terminal is asked to
type the release name; a script (no terminal) or --json is refused with exit
status 4 and nothing is removed. Persistent volumes that the chart created can
outlive the release: check the cluster before reusing the namespace.

Example:
  nself k8s uninstall --yes
  nself k8s uninstall --release my-nself --cluster ~/.kube/config --yes --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		r := newInvocation(cmd, "uninstall")
		cluster, _ := cmd.Flags().GetString("cluster")
		release, _ := cmd.Flags().GetString("release")
		yes, _ := cmd.Flags().GetBool("yes")
		release = releaseOrDefault(release)
		if !yes {
			if r.json || !stdinIsTTY() || !confirmRelease(os.Stdin, os.Stderr, release) {
				return r.refuse(release)
			}
		}
		r.info(fmt.Sprintf("Uninstalling Helm release %s...", release))
		err := k8s.Uninstall(cmd.Context(), k8s.UninstallOptions{ReleaseName: release, Kubeconfig: cluster, Stdout: r.helmStdout()})
		if err != nil {
			cmd.SilenceErrors = r.json
			return r.fail(err)
		}
		r.success("nSelf release " + release + " uninstalled.")
		return r.done(map[string]any{"release": release, "uninstalled": true})
	},
}

// refuse reports an unconfirmed uninstall: E403, exit 4, in both modes.
func (r *invocation) refuse(release string) error {
	d := blockedDetail(release)
	if r.json {
		return r.emit(d)
	}
	r.cmd.SilenceErrors = true // main prints the message once
	return &exitErr{code: output.ExitCodeFor(d.Class), msg: d.Message + "; " + d.Remediation}
}

func init() {
	addJSONFlag(uninstallCmd)
	uninstallCmd.Flags().String("cluster", "", "Path to kubeconfig (default: helm reads the KUBECONFIG env var or ~/.kube/config)")
	uninstallCmd.Flags().String("release", k8s.HelmReleaseName, "Helm release name")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Confirm the removal (required without a terminal)")
}
