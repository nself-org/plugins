package main

// Purpose: `nself infra plan` — terraform plan for the hetzner module.
// Inputs: --provider, --domain, --state-bucket, --json.
// Outputs: terraform's own output (stderr under --json) and, under --json, one
// v1 envelope. Nothing is changed in the cloud.

import (
	"fmt"

	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	addJSONFlag(infraPlanCmd)
	infraPlanCmd.RunE = runInfraPlan
}

func runInfraPlan(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()

	providerStr, _ := cmd.Flags().GetString("provider")
	domain, _ := cmd.Flags().GetString("domain")
	stateBucket, _ := cmd.Flags().GetString("state-bucket")
	if providerStr == "" {
		return r.usage("--provider is required (hetzner)")
	}
	if domain == "" {
		return r.usage("--domain is required")
	}
	provider := infra.Provider(providerStr)
	if !infra.ValidProviders[provider] {
		return r.usage(fmt.Sprintf("unknown provider %q; supported: hetzner", providerStr))
	}
	opts := infra.PlanOptions{
		Provider:    provider,
		Domain:      domain,
		StateBucket: stateBucket,
	}
	applyTerraformTokenEnv()
	if !r.json {
		ui.Info(fmt.Sprintf("Planning nSelf infrastructure on %s for %s...", providerStr, domain))
	}
	if err := infra.Plan(cmd.Context(), opts); err != nil {
		return r.fail(err)
	}
	return r.done(map[string]any{"provider": providerStr, "domain": domain, "status": "planned"})
}
