package main

// Purpose: `nself infra validate` — terraform validate for the hetzner module.
// Inputs: --provider (default hetzner), --json.
// Outputs: terraform's own output (stderr under --json) and, under --json, one
// v1 envelope. Nothing is changed in the cloud.

import (
	"fmt"

	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

var infraValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the Terraform module configuration",
	RunE:  runInfraValidate,
}

func runInfraValidate(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()

	providerStr, _ := cmd.Flags().GetString("provider")
	if providerStr == "" {
		providerStr = "hetzner" // default or just error
	}
	if providerStr != "hetzner" {
		return r.usageSafe(fmt.Sprintf("unknown provider %q; supported: hetzner", providerStr), "unknown provider; supported: hetzner")
	}

	provider := infra.Provider(providerStr)
	opts := infra.PlanOptions{
		Provider: provider,
	}

	applyTerraformTokenEnv()
	if !r.json {
		ui.Info(fmt.Sprintf("Validating nSelf infrastructure module on %s...", providerStr))
	}
	if err := infra.Validate(cmd.Context(), opts); err != nil {
		return r.fail(err)
	}
	return r.done(map[string]any{"provider": providerStr, "status": "valid"})
}

func init() {
	infraValidateCmd.Flags().String("provider", "hetzner", "Cloud provider (hetzner)")
	addJSONFlag(infraValidateCmd)
	infraCmd.AddCommand(infraValidateCmd)
}
