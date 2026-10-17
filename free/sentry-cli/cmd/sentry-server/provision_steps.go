package main

// provision_steps.go: the Terraform and SSH-wait steps of 'sentry-server provision'.
//
// Purpose: runSentryTerraform applies the Hetzner ops module; waitForSSH blocks
//   until a fresh server accepts SSH.
// Inputs:  project, location, server type, key path, dry-run flag, host.
// Outputs: an error, or nil when the step is done (or only printed by --dry-run).
// Constraints: HETZNER_NSELF_TOKEN or HCLOUD_TOKEN must be set for Terraform;
//   ssh must be on PATH. Moved out of main.go unchanged (P7-SURF-31 size split).

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/nself-sentry-cli/internal/infra"
	"github.com/nself-org/nself-sentry-cli/internal/ui"
)

// runSentryTerraform runs terraform apply for the Hetzner ops module.
//
// Purpose:  Provision Hetzner infrastructure for the ops server.
// Inputs:   project name, location, serverType, keyPath (SSH private key path), dryRun flag.
// Outputs:  Terraform apply executed against terraform/modules/hetzner.
// Constraints: HETZNER_NSELF_TOKEN or HCLOUD_TOKEN must be set.
func runSentryTerraform(ctx context.Context, project, location, serverType, keyPath string, dryRun bool) error {
	// Passthrough: HETZNER_NSELF_TOKEN → HCLOUD_TOKEN.
	if tok := os.Getenv("HETZNER_NSELF_TOKEN"); tok != "" && os.Getenv("HCLOUD_TOKEN") == "" {
		os.Setenv("HCLOUD_TOKEN", tok)
	}

	// Read the public key for the deploy user's authorized_keys.
	pubKeyPath := keyPath + ".pub"
	deployPubKey := ""
	if data, err := os.ReadFile(pubKeyPath); err == nil {
		deployPubKey = strings.TrimSpace(string(data))
	}

	vars := map[string]string{}
	if location != "" {
		vars["location"] = location
	}
	if serverType != "" {
		vars["server_type"] = serverType
	}
	if project != "" {
		vars["project"] = project
	}
	if deployPubKey != "" {
		vars["deploy_ssh_pubkey"] = deployPubKey
	}

	opts := infra.ApplyOptions{
		PlanOptions: infra.PlanOptions{
			Provider:   infra.ProviderHetzner,
			Domain:     project,
			ModulesDir: "terraform/modules/hetzner",
		},
		AutoApprove: true,
		Vars:        vars,
	}

	if dryRun {
		ui.Info(fmt.Sprintf("DRY-RUN: would run terraform apply (provider=hetzner, location=%s, server-type=%s)", location, serverType))
		return nil
	}

	ui.Info(fmt.Sprintf("Running terraform apply (location=%s, server-type=%s)...", location, serverType))
	return infra.Apply(ctx, opts)
}

// waitForSSH polls the given host until SSH is reachable or the timeout expires.
//
// Purpose:  Block until a freshly provisioned server accepts SSH connections.
// Inputs:   host in user@host or user@host:/path format, keyPath, timeout.
// Outputs:  nil when SSH is ready; error after timeout.
// Constraints: polls every 10 seconds; requires ssh binary in PATH.
func waitForSSH(ctx context.Context, host, keyPath string, timeout time.Duration) error {
	// Extract the SSH target (strip any :/path suffix).
	sshTarget := host
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			sshTarget = host[:i]
			break
		}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		args := []string{
			"-i", keyPath,
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "ForwardAgent=no",
			"-o", "BatchMode=yes",
			sshTarget,
			"true",
		}
		c := exec.CommandContext(ctx, "ssh", args...)
		c.Env = os.Environ()
		if err := c.Run(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for SSH")
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("SSH on %s did not become reachable within %s", sshTarget, timeout)
}
