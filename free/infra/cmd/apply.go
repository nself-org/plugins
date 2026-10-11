package main

// Purpose: `nself infra apply` — terraform apply for the hetzner module, behind
// the cloud-spend consent gate.
// Inputs: --provider, --domain, --state-bucket, --auto-approve, --location,
// --server-type, --deploy-pubkey, --i-accept-cloud-spend, --force (accepted and
// ignored, kept for scripts written against the PLANNED gate), --json.
// Outputs: terraform's own output (stderr under --json) and, under --json, one
// v1 envelope. A refusal exits 4.
// Constraints: the order is spend flag, terminal, provider, domain; nothing
// reaches terraform before all of them pass (confirm.go).

import (
	"fmt"
	"os"

	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	infraApplyCmd.Flags().Bool("i-accept-cloud-spend", false, "Accept cloud spend")
	addJSONFlag(infraApplyCmd)
	infraApplyCmd.RunE = runInfraApply
}

func runInfraApply(cmd *cobra.Command, args []string) error {
	r := begin(cmd)
	defer r.end()

	providerStr, _ := cmd.Flags().GetString("provider")
	domain, _ := cmd.Flags().GetString("domain")
	stateBucket, _ := cmd.Flags().GetString("state-bucket")
	autoApprove, _ := cmd.Flags().GetBool("auto-approve")
	location, _ := cmd.Flags().GetString("location")
	serverType, _ := cmd.Flags().GetString("server-type")
	acceptSpend, _ := cmd.Flags().GetBool("i-accept-cloud-spend")

	if err := r.spendGate("apply", acceptSpend); err != nil {
		return err
	}

	if providerStr == "" {
		return r.usage("--provider is required (hetzner)")
	}
	if shippedElsewhere(providerStr) {
		return r.notShipped(providerStr)
	}
	if domain == "" {
		return r.usage("--domain is required")
	}
	provider := infra.Provider(providerStr)
	if !infra.ValidProviders[provider] {
		return r.usageSafe(fmt.Sprintf("unknown provider %q; supported: hetzner", providerStr), "unknown provider; supported: hetzner")
	}

	applyTerraformTokenEnv()
	if tok := os.Getenv("HETZNER_NSELF_TOKEN"); tok != "" && os.Getenv("HCLOUD_TOKEN") == "" {
		os.Setenv("HCLOUD_TOKEN", tok)
	}

	deployPubkey, _ := cmd.Flags().GetString("deploy-pubkey")

	vars := map[string]string{}
	if location != "" {
		vars["location"] = location
	}
	if serverType != "" {
		vars["server_type"] = serverType
	}
	if deployPubkey != "" {
		vars["deploy_ssh_pubkey"] = deployPubkey
	}

	opts := infra.ApplyOptions{
		PlanOptions: infra.PlanOptions{
			Provider:    provider,
			Domain:      domain,
			StateBucket: stateBucket,
		},
		AutoApprove: autoApprove,
		Vars:        vars,
	}
	if !r.json {
		ui.Info(fmt.Sprintf("Provisioning nSelf on %s for %s...", providerStr, domain))
	}
	if err := infra.Apply(cmd.Context(), opts); err != nil {
		return r.fail(err)
	}
	return r.done(map[string]any{"provider": providerStr, "domain": domain, "status": "applied", "auto_approve": autoApprove})
}
