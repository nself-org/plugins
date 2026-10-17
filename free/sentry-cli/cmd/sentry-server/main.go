package main

// sentry_server.go — 'nself sentry-server' command group.
//
// Purpose: end-to-end provisioning of an ops/sentry server. Orchestrates
// Terraform infrastructure provisioning, secrets push, and ops-profile
// deployment in a single workflow. Intended for spinning up ɳSentry servers
// on Hetzner Cloud.
//
// Commands:
//   nself sentry-server provision <project>
//
// Inputs:
//   <project> positional arg (required)
//   --location, --server-type, --dry-run, --host, --key-path flags
// Outputs: provisioned server running the ops docker-compose profile.
// Constraints: terraform, ssh, scp, rsync must be in PATH.

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/nself-sentry-cli/cmd/jsonout"
	"github.com/nself-org/nself-sentry-cli/internal/deploy"
	"github.com/nself-org/nself-sentry-cli/internal/ui"

	"github.com/spf13/cobra"
)

var sentryServerCmd = &cobra.Command{
	Use:   "sentry-server",
	Short: "Provision and manage ɳSentry ops servers",
	Long: `Provision and manage ɳSentry observability/ops servers.

The sentry-server command orchestrates the full lifecycle of an ops server:
Terraform provisioning, secrets push, and ops-profile deployment.

Examples:
  nself sentry-server provision myproject
  nself sentry-server provision myproject --dry-run
  nself sentry-server provision myproject --host nself@1.2.3.4 --key-path ~/.ssh/ops_key`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var sentryServerProvisionCmd = &cobra.Command{
	Use:   "provision <project>",
	Short: "Provision an ops/sentry server for a project",
	Long: `Provision a full ɳSentry ops server for the given project.

Steps:
  1. Provision Hetzner server via Terraform (skipped when --host is given)
  2. Wait for SSH to become reachable (up to 3 minutes)
  3. Push secrets (.env.ops or .env) to the remote host via scp
  4. Deploy with the ops profile via rsync + docker compose

Use --host to skip Terraform and redeploy to an existing server.
Use --dry-run to preview steps without executing any real operations.`,
	Args: cobra.ExactArgs(1),
	RunE: runSentryServerProvision,
}

func init() {
	sentryServerProvisionCmd.Flags().String("location", "nbg1", "Hetzner datacenter location (nbg1, fsn1, hel1)")
	sentryServerProvisionCmd.Flags().String("server-type", "cpx21", "Hetzner server type (e.g. cpx21, cx22)")
	sentryServerProvisionCmd.Flags().Bool("dry-run", false, "Print what would happen without executing")
	sentryServerProvisionCmd.Flags().String("host", "", "Skip Terraform and deploy to this existing host (user@host:/path)")
	sentryServerProvisionCmd.Flags().String("key-path", "", "SSH key path (defaults to NSELF_DEPLOY_KEY_PATH env var)")

	// provision printed no JSON before: the envelope is its --json output in
	// v1.4 and v1.5 mode alike.
	jsonout.AddFlag(sentryServerProvisionCmd, "Print one v1 JSON envelope on stdout (progress and step output go to stderr)")
	jsonout.Always(sentryServerProvisionCmd)

	sentryServerCmd.AddCommand(sentryServerProvisionCmd)
}

// provisionResult is the data of the provision envelope.
type provisionResult struct {
	Project   string `json:"project"`
	DryRun    bool   `json:"dry_run"`
	Terraform bool   `json:"terraform"`
	Deployed  bool   `json:"deployed"`
}

// runSentryServerProvision implements 'nself sentry-server provision <project>'.
//
// Purpose:  Full ops-server provisioning pipeline: Terraform → SSH wait →
//
//	secrets push → ops deploy.
//
// Inputs:   project arg, flags (location, server-type, dry-run, host, key-path).
// Outputs:  Provisioned and running ops server.
// Constraints: Steps are sequential; failure at any step returns immediately.
func runSentryServerProvision(cmd *cobra.Command, args []string) error {
	if !jsonout.Envelope(cmd) {
		_, err := provision(cmd, args)
		return err
	}
	// JSON mode: stdout carries the envelope only. Progress lines and the
	// output of terraform, ssh and nself go to stderr for the length of the run.
	saved := os.Stdout
	os.Stdout = os.Stderr
	res, err := provision(cmd, args)
	os.Stdout = saved
	if err != nil {
		return err
	}
	return jsonout.Done(cmd, res)
}

// provision runs the four steps and reports what it did.
func provision(cmd *cobra.Command, args []string) (*provisionResult, error) {
	project := args[0]
	res := &provisionResult{Project: project}

	location, _ := cmd.Flags().GetString("location")
	serverType, _ := cmd.Flags().GetString("server-type")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	host, _ := cmd.Flags().GetString("host")
	keyPath, _ := cmd.Flags().GetString("key-path")
	res.DryRun = dryRun

	// Resolve SSH key from flag → env → default.
	if keyPath == "" {
		keyPath = os.Getenv("NSELF_DEPLOY_KEY_PATH")
	}
	if keyPath == "" {
		home, _ := os.UserHomeDir()
		keyPath = home + "/.ssh/id_ed25519"
	}

	ui.Info(fmt.Sprintf("Provisioning ops/sentry server for project %s...", project))

	// ── Step 1: Terraform (unless --host skips it) ─────────────────────────
	if host == "" {
		if err := runSentryTerraform(cmd.Context(), project, location, serverType, keyPath, dryRun); err != nil {
			return nil, fmt.Errorf("sentry-server provision: terraform: %w", err)
		}
		res.Terraform = !dryRun
		if dryRun {
			ui.Info("DRY-RUN: would wait for SSH, push secrets, and deploy ops profile")
			return res, nil
		}
		// After Terraform, the host is read from env (set by the module output or
		// the NSELF_DEPLOY_HOST_OPS env var the operator configures).
		host = os.Getenv("NSELF_DEPLOY_HOST_OPS")
		if host == "" {
			host = os.Getenv("OPS_DEPLOY_HOST")
		}
		if host == "" {
			return nil, fmt.Errorf("sentry-server provision: terraform complete but NSELF_DEPLOY_HOST_OPS is not set; export it and re-run with --host")
		}
	}

	cfg := deploy.SSHConfig{
		Host:    host,
		KeyPath: keyPath,
	}

	// ── Step 2: Wait for SSH ───────────────────────────────────────────────
	if dryRun {
		ui.Info(fmt.Sprintf("DRY-RUN: would wait for SSH on %s (up to 3 minutes)", host))
	} else {
		ui.Info(fmt.Sprintf("Waiting for SSH on %s (up to 3 minutes)...", host))
		if err := waitForSSH(cmd.Context(), host, keyPath, 3*time.Minute); err != nil {
			return nil, fmt.Errorf("sentry-server provision: SSH wait: %w", err)
		}
		ui.Success("SSH reachable")
	}

	// ── Step 3: Push secrets ───────────────────────────────────────────────
	workdir, err := projectRoot()
	if err != nil {
		return nil, fmt.Errorf("sentry-server provision: %w", err)
	}

	pushOpts := deploy.PushSecretsOptions{
		Target:      "ops",
		DryRun:      dryRun,
		ProjectRoot: workdir,
	}
	ui.Info("Pushing secrets to remote host...")
	if err := deploy.PushSecrets(cmd.Context(), cfg, pushOpts); err != nil {
		return nil, fmt.Errorf("sentry-server provision: push secrets: %w", err)
	}

	// ── Step 4: Deploy ops profile ─────────────────────────────────────────
	ui.Info("Deploying ops profile...")
	if dryRun {
		ui.Info(fmt.Sprintf("DRY-RUN: would run ops deploy to %s", host))
	} else {
		if err := runCLISelf(cmd.Context(), workdir, "ops", "deploy"); err != nil {
			return nil, fmt.Errorf("sentry-server provision: ops deploy: %w", err)
		}
	}

	if !dryRun {
		res.Deployed = true
		ui.Success(fmt.Sprintf("ɳSentry ops server for project %q is live at %s", project, host))
	}
	return res, nil
}

var setupOnce sync.Once

func setup() { setupOnce.Do(setupRoot) }

func setupRoot() {
	prefixUsageWithNself(sentryServerCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself sentry-server nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	sentryServerCmd.Args = cobra.ArbitraryArgs

	sentryServerCmd.CompletionOptions.DisableDefaultCmd = true
	sentryServerCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	sentryServerCmd.SilenceUsage = true
	jsonout.Wrap(sentryServerCmd)
}

func main() {
	setup()
	// cobra has already printed the error; exit non-zero without repeating
	// it. The CLI mirrors this status and stays silent, so the plugin's own
	// message is the only one the user sees. With --json in v1.5 mode the
	// error is one envelope on stdout and the status is its class.
	os.Exit(jsonout.Execute(sentryServerCmd, os.Args[1:]))
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself sentry-server <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
