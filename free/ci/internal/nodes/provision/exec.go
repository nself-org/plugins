package provision

// Purpose: a single command-execution seam (Executor) that provision.go and
//   verify.go run every host action through, so tests substitute a fake
//   recorder instead of shelling out, and so `verify` can be pointed at
//   several hosts (SSH) or the local machine with identical calling code.
// Inputs:  a shell command string.
// Outputs: combined stdout+stderr and an error (non-zero exit is returned
//   as an error, mirroring os/exec.CombinedOutput's contract).
// Constraints: SSHExecutor runs every ssh call through sdk/go/remote (cli
//   sdk/go/v2, vendored): the plugin has no ssh or scp exec site of its own
//   (TestSingleSSHExecSite). The ssh option set is remote.BaseOptions, the
//   set core `nself runner` used through internal/deploy, so key resolution
//   and StrictHostKeyChecking=accept-new are unchanged. Two deliberate
//   differences from core, both from the sdk: the destination also passes
//   remote.ValidateDest (an allowlist), and ssh gets "--" before the
//   destination and runs with the sdk's environment allowlist.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Executor runs one shell command against a bound host and reports its
// label (used in verify's parity matrix and provision's log lines).
type Executor interface {
	Run(ctx context.Context, command string) (output string, err error)
	Label() string
}

// LocalExecutor runs commands on the current machine via `bash -c`. Bash
// (not POSIX sh) is required because the chromium-cache check uses
// `shopt -s nullglob`; every provisioning target (Ubuntu runner hosts) and
// every dev machine this ships on has bash available.
type LocalExecutor struct{}

// Run implements Executor.
func (LocalExecutor) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Label implements Executor.
func (LocalExecutor) Label() string { return "local" }

// SSHExecutor runs commands on a remote host over SSH through sdk/go/remote.
type SSHExecutor struct {
	// Target is the destination and key; its Options stay nil, which the sdk
	// reads as remote.BaseOptions(KeyPath) (the option allowlist for an explicit
	// Options slice refuses StrictHostKeyChecking and ForwardAgent).
	Target remote.Target
	// KeyPath is the private key passed with -i.
	KeyPath string
}

// NewSSHExecutor builds an SSHExecutor from a "user@host" string and an SSH
// key path. An empty keyPath resolves remote.DefaultKeyPath: the
// NSELF_DEPLOY_KEY_PATH / NSELF_DEPLOY_SSH_KEY env vars, then
// ~/.ssh/id_ed25519, so every remote-targeting command shares one rule.
func NewSSHExecutor(sshTarget, keyPath string) SSHExecutor {
	if keyPath == "" {
		keyPath = remote.DefaultKeyPath()
	}
	return SSHExecutor{
		Target:  remote.Target{Dest: sshTarget, KeyPath: keyPath}, // nil Options: the sdk uses BaseOptions(KeyPath)
		KeyPath: keyPath,
	}
}

// Run implements Executor. The two checks before the sdk call are core's own
// (empty host, then the legacy destination check) so their messages are
// unchanged; the sdk then applies its stricter destination allowlist.
func (e SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	if e.Target.Dest == "" {
		return "", fmt.Errorf("remote target has no SSH host configured")
	}
	if err := remote.ValidateLegacyDest(e.Target.Dest); err != nil {
		return "", err
	}
	return remote.Run(ctx, e.Target, command)
}

// Label implements Executor.
func (e SSHExecutor) Label() string { return e.Target.Dest }
