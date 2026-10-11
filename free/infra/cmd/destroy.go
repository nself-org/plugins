package main

// Purpose: `nself infra destroy` — terraform destroy for the hetzner module,
// behind the cloud-spend consent gate and --auto-approve.
// Inputs: --provider, --domain, --auto-approve, --i-accept-cloud-spend, --json.
// Outputs: terraform's own output (stderr under --json) and, under --json, one
// v1 envelope. A spend or terminal refusal exits 4, in both modes; a missing
// --auto-approve exits 1 in human mode (as always) and 4 with an E403 envelope
// under --json.
// Constraints: the order is spend flag, terminal, provider, shipped provider,
// --auto-approve; terraform destroy is reached only after all of them pass.

import (
	"fmt"

	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	infraDestroyCmd.Flags().Bool("i-accept-cloud-spend", false, "Accept cloud spend")
	addJSONFlag(infraDestroyCmd)
	infraDestroyCmd.RunE = runInfraDestroy
}

func runInfraDestroy(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()

	providerStr, _ := cmd.Flags().GetString("provider")
	domain, _ := cmd.Flags().GetString("domain")
	autoApprove, _ := cmd.Flags().GetBool("auto-approve")
	acceptSpend, _ := cmd.Flags().GetBool("i-accept-cloud-spend")

	if err := r.spendGate("destroy", acceptSpend); err != nil {
		return err
	}

	if providerStr == "" {
		return r.usage("--provider is required")
	}
	if shippedElsewhere(providerStr) {
		return r.notShipped(providerStr)
	}
	provider := infra.Provider(providerStr)
	if !infra.ValidProviders[provider] {
		return r.usage(fmt.Sprintf("unknown provider %q; supported: hetzner", providerStr))
	}
	if !autoApprove {
		if r.json {
			return r.refuse("", "nself infra destroy needs --auto-approve: nothing was destroyed",
				"take a backup with nself backup, then pass --auto-approve")
		}
		ui.Warn("This will destroy all cloud resources. Use --auto-approve to confirm, or run 'nself backup' first.")
		return fmt.Errorf("requires --auto-approve to proceed")
	}
	applyTerraformTokenEnv()
	if !r.json {
		ui.Info(fmt.Sprintf("Destroying nSelf infrastructure on %s...", providerStr))
	}
	if err := infra.Destroy(cmd.Context(), provider, domain, "", true); err != nil {
		return r.fail(err)
	}
	return r.done(map[string]any{"provider": providerStr, "domain": domain, "status": "destroyed"})
}
