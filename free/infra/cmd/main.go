// nself-infra — Terraform-based infrastructure provisioning for nSelf.
//
// Purpose: this is the `nself infra` command family, moved out of the CLI core
// under CLI-R11. The core has to cover the self-hosted backend lifecycle with
// no added complexity; provisioning cloud infrastructure with Terraform is not
// part of that lifecycle, and the users who need it can install it.
//
// Inputs: the same flags and subcommands as `nself infra` — plan, apply and
// destroy, each taking --provider and --domain.
//
// Outputs: identical to the pre-extraction command, including the PLANNED gate
// on apply and the --auto-approve requirement on destroy.
//
// Constraints: the CLI proxies `nself infra ...` to this binary once installed,
// so the command tree here is rooted at `infra` and every flag name, default
// and message is the one the CLI used. Changing any of them changes what an
// existing script does.

package main

// infra.go — B51/B52: Terraform-based infrastructure provisioning for nSelf.
//
// Status: PLANNED — deferred (requires UD-12 minor release approval).
//
// Commands:
//   nself infra plan    --provider <provider> --domain <domain>
//   nself infra apply   --provider <provider> --domain <domain>
//   nself infra destroy --provider <provider> [--domain <domain>]

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/ui"

	"github.com/spf13/cobra"
)

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Provision nSelf infrastructure via Terraform",
	Long: `Provision cloud infrastructure for nSelf using provider-specific Terraform modules.

Supported providers: aws, gcp, azure, hetzner, do, linode

Requires terraform to be installed: https://developer.hashicorp.com/terraform

Examples:
  nself infra plan  --provider hetzner --domain myapp.com
  nself infra apply --provider hetzner --domain myapp.com
  nself infra destroy --provider hetzner`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var infraPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show the Terraform plan for the given provider",
	Long: `Run 'terraform plan' for the nSelf module matching the given provider.

The plan shows what infrastructure will be created without making any changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		stateBucket, _ := cmd.Flags().GetString("state-bucket")
		if providerStr == "" {
			return fmt.Errorf("--provider is required (aws, gcp, azure, hetzner, do, linode)")
		}
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: aws, gcp, azure, hetzner, do, linode", providerStr)
		}
		opts := infra.PlanOptions{
			Provider:    provider,
			Domain:      domain,
			StateBucket: stateBucket,
		}
		ui.Info(fmt.Sprintf("Planning nSelf infrastructure on %s for %s...", providerStr, domain))
		return infra.Plan(cmd.Context(), opts)
	},
}

var infraApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Provision nSelf infrastructure via Terraform",
	Long: `Run 'terraform apply' for the nSelf module matching the given provider.

Outputs include NSELF_URL, ssh_host, and postgres_url after provisioning.

This command is gated as PLANNED. Use --force to bypass the gate and run
terraform against real infrastructure (requires UD-12 minor release approval).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			ui.Warn("nself infra apply is PLANNED (requires UD-12 minor release approval).")
			ui.Warn("Re-run with --force to provision real infrastructure.")
			return fmt.Errorf("use --force to bypass the PLANNED gate")
		}

		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		stateBucket, _ := cmd.Flags().GetString("state-bucket")
		autoApprove, _ := cmd.Flags().GetBool("auto-approve")
		location, _ := cmd.Flags().GetString("location")
		serverType, _ := cmd.Flags().GetString("server-type")

		if providerStr == "" {
			return fmt.Errorf("--provider is required (aws, gcp, azure, hetzner, do, linode)")
		}
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: aws, gcp, azure, hetzner, do, linode", providerStr)
		}

		// Passthrough: HETZNER_NSELF_TOKEN → HCLOUD_TOKEN so the Hetzner Terraform
		// provider can authenticate without requiring users to export a second var.
		if tok := os.Getenv("HETZNER_NSELF_TOKEN"); tok != "" && os.Getenv("HCLOUD_TOKEN") == "" {
			os.Setenv("HCLOUD_TOKEN", tok)
		}

		deployPubkey, _ := cmd.Flags().GetString("deploy-pubkey")

		// Build optional Terraform variable overrides from flag values.
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
	},
}

var infraDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy nSelf infrastructure",
	Long: `Run 'terraform destroy' for the nSelf module matching the given provider.

WARNING: This removes all provisioned cloud resources. Data will be lost
unless you have taken a backup via 'nself backup'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		providerStr, _ := cmd.Flags().GetString("provider")
		domain, _ := cmd.Flags().GetString("domain")
		autoApprove, _ := cmd.Flags().GetBool("auto-approve")
		if providerStr == "" {
			return fmt.Errorf("--provider is required")
		}
		provider := infra.Provider(providerStr)
		if !infra.ValidProviders[provider] {
			return fmt.Errorf("unknown provider %q; supported: aws, gcp, azure, hetzner, do, linode", providerStr)
		}
		if !autoApprove {
			ui.Warn("This will destroy all cloud resources. Use --auto-approve to confirm, or run 'nself backup' first.")
			return fmt.Errorf("requires --auto-approve to proceed")
		}
		ui.Info(fmt.Sprintf("Destroying nSelf infrastructure on %s...", providerStr))
		return infra.Destroy(cmd.Context(), provider, domain, "", true)
	},
}

func init() {
	infraPlanCmd.Flags().String("provider", "", "Cloud provider (aws, gcp, azure, hetzner, do, linode)")
	infraPlanCmd.Flags().String("domain", "", "Domain for the nSelf deployment")
	infraPlanCmd.Flags().String("state-bucket", "", "S3/GCS bucket for Terraform state (optional)")

	infraApplyCmd.Flags().String("provider", "", "Cloud provider (aws, gcp, azure, hetzner, do, linode)")
	infraApplyCmd.Flags().String("domain", "", "Domain for the nSelf deployment")
	infraApplyCmd.Flags().String("state-bucket", "", "S3/GCS bucket for Terraform state (optional)")
	infraApplyCmd.Flags().Bool("auto-approve", false, "Skip interactive approval")
	infraApplyCmd.Flags().Bool("force", false, "Bypass the PLANNED gate and run terraform against real infrastructure")
	infraApplyCmd.Flags().String("location", "", "Hetzner datacenter location (e.g. nbg1, fsn1, hel1) — passed as TF var")
	infraApplyCmd.Flags().String("server-type", "", "Hetzner server type (e.g. cpx21, cx22) — passed as TF var")
	infraApplyCmd.Flags().String("deploy-pubkey", "", "SSH public key content for the nself deploy user's authorized_keys — passed as TF var deploy_ssh_pubkey")

	infraDestroyCmd.Flags().String("provider", "", "Cloud provider (aws, gcp, azure, hetzner, do, linode)")
	infraDestroyCmd.Flags().String("domain", "", "Domain for the nSelf deployment")
	infraDestroyCmd.Flags().Bool("auto-approve", false, "Confirm destruction without prompt")

	infraCmd.AddCommand(infraPlanCmd)
	infraCmd.AddCommand(infraApplyCmd)
	infraCmd.AddCommand(infraDestroyCmd)

}

func main() {
	// Users reach this binary by typing `nself infra ...` — the CLI proxies to
	// it — so every usage line has to read "nself infra ...", not "infra ..."
	// and not "nself-infra ...".
	//
	// Setting Use to "nself infra" is not enough: cobra derives CommandPath
	// from Name(), which is the first WORD of Use, so the "[command]" line and
	// the "--help" footer would say "nself" while the flags line said
	// "nself infra". Prefixing the template instead is correct at every depth —
	// the root renders "nself infra", and `nself infra plan --help` renders
	// "nself infra plan", because CommandPath still composes normally.
	prefixUsageWithNself(infraCmd)

	// Cobra adds `completion` and `help` to any root command. Inside the CLI
	// these lived on nself's root, not under `nself infra`, so advertising them
	// here would show the user two subcommands that did not exist before.
	infraCmd.CompletionOptions.DisableDefaultCmd = true
	infraCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	infraCmd.SilenceUsage = true

	if err := infraCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without
		// repeating it, which is what RootCmd did in the CLI.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the help and usage templates so every rendered
// command path is preceded by "nself ". Applied to the root; cobra passes the
// templates down to subcommands, so one call covers the whole tree.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
	root.SetHelpTemplate(root.HelpTemplate())
}
