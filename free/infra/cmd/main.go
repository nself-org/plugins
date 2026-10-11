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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

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
	// RunE is set in plan.go.
}

var infraApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Provision nSelf infrastructure via Terraform",
	Long: `Run 'terraform apply' for the nSelf module matching the given provider.

Outputs include NSELF_URL, ssh_host, and postgres_url after provisioning.

This command is gated as PLANNED. Use --force to bypass the gate and run
terraform against real infrastructure (requires UD-12 minor release approval).`,
	// RunE is set in apply.go.
}

var infraDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy nSelf infrastructure",
	Long: `Run 'terraform destroy' for the nSelf module matching the given provider.

WARNING: This removes all provisioned cloud resources. Data will be lost
unless you have taken a backup via 'nself backup'.`,
	// RunE is set in destroy.go.
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
	configureRoot()

	js := wantsJSON(os.Args[1:])
	if js {
		// Flag and argument errors are cobra's, raised before any RunE: keep
		// cobra from printing them; report prints the envelope instead.
		infraCmd.SilenceErrors = true
	}
	os.Exit(report(infraCmd.Execute(), js, os.Args[1:], os.Stdout, os.Stderr))
}

// report turns the result of Execute into the process exit code. Human mode:
// cobra has already printed a plain error (what RootCmd did in the CLI), so exit
// 1 without repeating it. A refusal or an error envelope arrives as exitErr and
// exits with its own code. With --json, an error cobra raised before a command
// ran (unknown flag) becomes an E401 envelope on stdout.
func report(err error, js bool, args []string, stdout, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(stderr, "Error:", ee.msg)
		}
		return ee.code
	}
	if js && !ran {
		d := usageDetail(err.Error())
		d.ExitCode = output.ExitCodeFor(d.Class)
		name := "infra"
		if c, _, ferr := infraCmd.Find(args); ferr == nil && c != infraCmd {
			name = c.CommandPath()
		}
		if werr := output.WriteError(stdout, name, d, nil); werr != nil {
			fmt.Fprintln(stderr, "Error:", err)
		}
		return d.ExitCode
	}
	return 1
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

// configureRoot applies the usage prefix and the help/completion/arguments
// settings below to the root. main calls it once; the in-process test harness
// calls it once too so its tree matches the binary's.
func configureRoot() {
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
	// Cobra's default Args validator rejects an unrecognised first argument
	// only for a ROOT command with subcommands; for a child it passes them to
	// RunE. Inside the CLI this was a child, so `nself infra nosuch` printed
	// help. As a root it would error instead, naming a binary the user never
	// typed. ArbitraryArgs restores the child behaviour.
	infraCmd.Args = cobra.ArbitraryArgs

	infraCmd.CompletionOptions.DisableDefaultCmd = true
	infraCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	infraCmd.SilenceUsage = true
}
