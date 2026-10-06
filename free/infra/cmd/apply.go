package main

import (
	"fmt"
	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	infraApplyCmd.Flags().Bool("i-accept-cloud-spend", false, "Accept cloud spend")

	infraApplyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		stateBucket, _ := cmd.Flags().GetString("state-bucket")
		autoApprove, _ := cmd.Flags().GetBool("auto-approve")
		location, _ := cmd.Flags().GetString("location")
		serverType, _ := cmd.Flags().GetString("server-type")
		acceptSpend, _ := cmd.Flags().GetBool("i-accept-cloud-spend")

		if !acceptSpend {
			ui.Warn("nself infra apply will incur cloud spend. Pass --i-accept-cloud-spend to proceed.")
			os.Exit(4)
		}

		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			ui.Warn("nself infra apply requires a TTY to confirm spend.")
			os.Exit(4)
		}

		if providerStr == "" {
			return fmt.Errorf("--provider is required (hetzner)")
		}
		if providerStr == "aws" || providerStr == "gcp" || providerStr == "azure" || providerStr == "do" || providerStr == "linode" {
			fmt.Fprintf(os.Stderr, "not shipped (DEF-005)\n")
			os.Exit(1)
		}
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: hetzner", providerStr)
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
		ui.Info(fmt.Sprintf("Provisioning nSelf on %s for %s...", providerStr, domain))
		return infra.Apply(cmd.Context(), opts)
	}
}
