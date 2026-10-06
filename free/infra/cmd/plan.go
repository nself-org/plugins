package main

import (
	"fmt"
	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	infraPlanCmd.RunE = func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		stateBucket, _ := cmd.Flags().GetString("state-bucket")
		if providerStr == "" {
			return fmt.Errorf("--provider is required (hetzner)")
		}
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: hetzner", providerStr)
		}
		opts := infra.PlanOptions{
			Provider:    provider,
			Domain:      domain,
			StateBucket: stateBucket,
		}
		applyTerraformTokenEnv()
		ui.Info(fmt.Sprintf("Planning nSelf infrastructure on %s for %s...", providerStr, domain))
		return infra.Plan(cmd.Context(), opts)
	}
}
