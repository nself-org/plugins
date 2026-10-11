// Purpose: `nself-k8s upgrade` upgrades the nSelf Helm release from the chart
// embedded in this binary, applying the current generated values.
//
// Inputs: the install flags (see install.go), --domain optional, and the
// project's .nself/generated/k8s/values.yaml and secrets.yaml.
//
// Outputs: helm's own stdout/stderr (inherited), plus an Info/Success line
// from the tui package around the helm invocation. Exit 1 when the generated
// values are missing ("run nself k8s values").
//
// Constraints: values are applied afresh, not with --reuse-values, so pass the
// same --domain and --plugins as at install time.
package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/k8s"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the nSelf Helm release",
	Long: `Upgrade the nSelf Helm release from the embedded chart with a rolling update.

Run 'nself k8s values' first so the generated values match the compose model.
The values are applied afresh (no --reuse-values): a service removed from the
compose model leaves the release. Pass the same --domain and --plugins as at
install time; the upgrade prints the
values it is not passing. The embedded chart does not consume domain or plugins
yet (D-0311), and the licence key is not written to the release at all until it does.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true // a missing values file is not a usage error
		r := newInvocation(cmd, "upgrade")
		if r.json && len(args) > 0 {
			return r.usageFail("upgrade takes no arguments")
		}
		opts := installOptions(cmd)
		opts.Stdout = r.helmStdout()
		warnUnusedLicence(r, opts)
		if missing := k8s.NotPassed(opts); len(missing) > 0 {
			r.warn("Not passing " + strings.Join(missing, ", ") + " (upgrade applies values afresh; repeat the install flags to pass them)")
		}
		r.info("Upgrading nSelf Helm release...")
		if err := k8s.Upgrade(cmd.Context(), opts); err != nil {
			return r.fail(err)
		}
		r.success("nSelf upgraded successfully.")
		return r.done(map[string]any{"release": releaseOrDefault(opts.ReleaseName), "upgraded": true})
	},
}

func init() {
	addJSONFlag(upgradeCmd)
	upgradeCmd.Flags().String("domain", "", "Domain, recorded in the release values (the chart does not consume it yet, D-0311)")
	addInstallFlags(upgradeCmd)
}
