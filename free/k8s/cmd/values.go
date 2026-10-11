// Purpose: `nself-k8s values [--check]` generates the chart values from the
// project's resolved compose model, so k8s is a projection of the same source
// as `nself start` (Constitution 1.2, 1.4).
//
// Inputs: --project-dir (default "."), --out (default
// <project-dir>/.nself/generated/k8s), --check, and the project's
// .nself/compose-files.txt, .nself/compose-env-files.txt and (optional)
// .nself/generated/routes.json. Compose runs as `docker compose --env-file ...
// -f ... config --format json`.
//
// Outputs: values.yaml and secrets.yaml (0600) in --out; with --check nothing
// is written, differences are printed (service name first) and the command
// exits 1.
//
// With --json one v1 envelope replaces the text: data holds the files written
// and what could not be mapped (or the services count in --check); a parity
// failure is E723 with the differences in cause.
//
// Constraints: no cli import. Secret values are never printed. Without
// --check, services that cannot be mapped are listed on stdout with the
// reason and the command still succeeds (they are recorded in values.yaml).
package main

import (
	"github.com/spf13/cobra"

	"github.com/nself-org/nself-k8s/internal/values"
)

var valuesCmd = &cobra.Command{
	Use:   "values",
	Short: "Generate chart values from the project's compose model",
	Long: `Resolve the project's compose model (docker compose config) and write
.nself/generated/k8s/values.yaml (images, tags, env names, ports, volumes,
probes, service kinds) and secrets.yaml (env values, mode 0600).

--check compares an existing values.yaml with the compose model and exits 1,
naming every service that is missing, extra, or differs in image or tag.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true  // a failed check is not a usage error
		cmd.SilenceErrors = true // main prints the error once
		r := newInvocation(cmd, "values")
		dir, _ := cmd.Flags().GetString("project-dir")
		out, _ := cmd.Flags().GetString("out")
		check, _ := cmd.Flags().GetBool("check")
		res, err := values.Run(cmd.Context(), values.Options{ProjectDir: dir, OutDir: out, Check: check})
		if !r.json {
			if res != nil {
				res.WriteText(cmd.OutOrStdout())
			}
			return err
		}
		if err != nil {
			return r.valuesFail(res, err)
		}
		return r.done(valuesData(res))
	},
}

func init() {
	addJSONFlag(valuesCmd)
	valuesCmd.Flags().String("project-dir", ".", "nSelf project directory (holds .nself/)")
	valuesCmd.Flags().String("out", "", "Output directory (default <project-dir>/.nself/generated/k8s)")
	valuesCmd.Flags().Bool("check", false, "Compare an existing values.yaml with the compose model; exit 1 on any difference")
}
