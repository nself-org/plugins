package provision

// Purpose: a single command-execution seam (Executor) that provision.go and
//   verify.go run every host action through, so tests substitute a fake
//   recorder instead of shelling out, and so `verify` can be pointed at
//   several hosts (SSH) or the local machine with identical calling code.
// Inputs:  a shell command string, and for RunInput the bytes its stdin
//   receives (how a secret such as the runner registration token reaches
//   the host without appearing on any argv, D-0263).
// Outputs: combined stdout+stderr and an error (non-zero exit is returned
//   as an error, mirroring os/exec.CombinedOutput's contract).
// Constraints: SSHExecutor runs every ssh call through sdk/go/remote (cli
//   sdk/go/v2, vendored): the plugin has no ssh or scp exec site of its own
//   (TestSingleSSHExecSite). The ssh option set is remote.BaseOptions, the
//   set core `nself runner` used through internal/deploy, so key resolution
//   and StrictHostKeyChecking=accept-new are unchanged; a host is checked
//   with core's remote.ParseHostSpec first, so refusals read as core's.
//   Three deliberate differences from core: the destination also passes
//   remote.ValidateDest (an allowlist), ssh gets "--" before the
//   destination and runs with the sdk's environment allowlist (all from the
//   sdk), and the runner token travels on stdin (RunInput, D-0263; a token
//   with a line break is refused in Provision). A host with a port or a
//   legacy path, which core accepts, is refused by name (see target).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Executor runs one shell command against a bound host and reports its
// label (used in verify's parity matrix and provision's log lines).
type Executor interface {
	Run(ctx context.Context, command string) (output string, err error)
	// RunInput is Run with input written to the command's stdin, which is
	// then closed. Callers pass secrets this way so they never appear in the
	// command string, the local ssh argv or the remote process list.
	RunInput(ctx context.Context, command, input string) (output string, err error)
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

// RunInput implements Executor.
func (LocalExecutor) RunInput(ctx context.Context, command, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Stdin = strings.NewReader(input)
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

// target applies core's two checks in core's order (cli internal/deploy
// RunRemoteCommand: empty host, then remote.ParseHostSpec) so a malformed
// host is refused with core's message (D-0270), and returns the Target to
// run: the parsed destination, so an IPv6 host reaches the sdk bare (its
// allowlist refuses brackets) and every other host is unchanged. A port or
// legacy path, which core turns into ssh options, is refused by name: the
// sdk's default option set cannot carry -p (see D-0270's note).
func (e SSHExecutor) target() (remote.Target, error) {
	if e.Target.Dest == "" {
		return remote.Target{}, fmt.Errorf("remote target has no SSH host configured")
	}
	spec, err := remote.ParseHostSpec(e.Target.Dest)
	if err != nil {
		return remote.Target{}, err
	}
	if spec.Port > 0 || spec.LegacyPath != "" {
		return remote.Target{}, fmt.Errorf("host %q: a port or path in the host is not supported here; set Port for that host in ~/.ssh/config", e.Target.Dest)
	}
	t := e.Target
	t.Dest = spec.Dest()
	return t, nil
}

// Run implements Executor.
func (e SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	t, err := e.target()
	if err != nil {
		return "", err
	}
	return remote.Run(ctx, t, command)
}

// RunInput implements Executor through remote.Start, the sdk's only ssh
// session with a stdin pipe. Output and errors take Run's shape: stdout and
// stderr gathered into one trimmed buffer (two pipes, so the interleaving of
// stdout and stderr lines may differ from Run's single pipe), and a failure
// reported as "remote command on <dest> failed" with the process's exit status.
func (e SSHExecutor) RunInput(ctx context.Context, command, input string) (string, error) {
	t, err := e.target()
	if err != nil {
		return "", err
	}
	s, err := remote.Start(ctx, t, command)
	if err != nil {
		return "", err
	}
	var buf lockedBuffer
	var wg sync.WaitGroup
	for _, r := range []io.Reader{s.Stdout, s.Stderr} {
		wg.Add(1)
		go func(r io.Reader) { defer wg.Done(); _, _ = io.Copy(&buf, r) }(r)
	}
	_, werr := io.WriteString(s.Stdin, input)
	if cerr := s.Stdin.Close(); werr == nil {
		werr = cerr
	}
	wg.Wait() // every read completes before Wait, as os/exec requires
	err = s.Wait()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		return out, fmt.Errorf("remote command on %s failed: %w\n%s", e.Target.Dest, err, out)
	}
	if werr != nil {
		return out, fmt.Errorf("remote command on %s: send input: %w", e.Target.Dest, werr)
	}
	return out, nil
}

// Label implements Executor.
func (e SSHExecutor) Label() string { return e.Target.Dest }

// lockedBuffer is a bytes.Buffer two copy goroutines (stdout, stderr) write
// into, standing in for CombinedOutput's single buffer.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
