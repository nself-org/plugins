package main

import (
	"fmt"
	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

var infraValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the Terraform module configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		if providerStr == "" {
			providerStr = "hetzner" // default or just error
		}
		if providerStr != "hetzner" {
			return fmt.Errorf("unknown provider %q; supported: hetzner", providerStr)
		}

		provider := infra.Provider(providerStr)
		opts := infra.PlanOptions{
			Provider: provider,
		}

		applyTerraformTokenEnv()
		ui.Info(fmt.Sprintf("Validating nSelf infrastructure module on %s...", providerStr))
		return infra.Validate(cmd.Context(), opts)
	},
}

func init() {
	infraValidateCmd.Flags().String("provider", "hetzner", "Cloud provider (hetzner)")
	infraCmd.AddCommand(infraValidateCmd)
}
