package main

import (
	"fmt"
	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	infraDestroyCmd.Flags().Bool("i-accept-cloud-spend", false, "Accept cloud spend")

	infraDestroyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		autoApprove, _ := cmd.Flags().GetBool("auto-approve")
		acceptSpend, _ := cmd.Flags().GetBool("i-accept-cloud-spend")

		if !acceptSpend {
			ui.Warn("nself infra destroy will incur cloud spend. Pass --i-accept-cloud-spend to proceed.")
			os.Exit(4)
		}

		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			ui.Warn("nself infra destroy requires a TTY to confirm spend.")
			os.Exit(4)
		}

		if providerStr == "" {
			return fmt.Errorf("--provider is required")
		}
		if providerStr == "aws" || providerStr == "gcp" || providerStr == "azure" || providerStr == "do" || providerStr == "linode" {
			fmt.Fprintf(os.Stderr, "not shipped (DEF-005)\n")
			os.Exit(1)
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: hetzner", providerStr)
		}
		if !autoApprove {
			ui.Warn("This will destroy all cloud resources. Use --auto-approve to confirm, or run 'nself backup' first.")
			return fmt.Errorf("requires --auto-approve to proceed")
		}
		applyTerraformTokenEnv()
		ui.Info(fmt.Sprintf("Destroying nSelf infrastructure on %s...", providerStr))
		return infra.Destroy(cmd.Context(), provider, domain, "", true)
	}
}
