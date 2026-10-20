package update

// Purpose: the ssh carrier's half of an upgrade (hq NODE D11): the agent has no daemon, so "install" is a verified copy plus an
//          atomic swap over ssh and "restart" is nothing; the next `serve --stdio` starts the new binary.
// Inputs:  the pinned remote.Target of the node and the lifecycle collaborators (Run and Copy are cli sdk/go/remote's, drain, enable
//          and probe are the lifecycle commands), all injected.
// Outputs: a Node.
// Constraints: ssh and scp start only through the injected Run and Copy (no os/exec here); every remote command is built from the
//          constant agent paths below, never from node data; the copied file is checked against the signed sha256 on the node and
//          must print the target version before it replaces the installed binary; the previous binary is kept as .prev.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// Paths on the node, relative to the SSH user's home (the lifecycle installer's layout, Epic D4).
const (
	agentDir  = ".nself/ci-agent"
	agentBin  = agentDir + "/bin/nself-ci-agent"
	coordFile = agentDir + "/" + session.CoordinatorsFile
)

// SSHNode upgrades an agent that is started over ssh.
type SSHNode struct {
	NodeID string
	Plat   lifecycle.Platform
	Target remote.Target
	Run    func(ctx context.Context, t remote.Target, argv ...string) (string, error)
	Copy   func(ctx context.Context, t remote.Target, local, remotePath string) error
	// DrainFn, EnableFn and ProbeFn are the lifecycle commands for this node.
	DrainFn  func(ctx context.Context) (wasOnline bool, err error)
	EnableFn func(ctx context.Context) error
	ProbeFn  func(ctx context.Context) error
	// HelloPoll is the pause between version checks while waiting for hello.
	HelloPoll time.Duration
}

var _ Node = (*SSHNode)(nil)

// ID implements Node.
func (n *SSHNode) ID() string { return n.NodeID }

// Platform implements Node.
func (n *SSHNode) Platform() lifecycle.Platform { return n.Plat }

// Version asks the installed agent for its version.
func (n *SSHNode) Version(ctx context.Context) (string, error) {
	out, err := n.Run(ctx, n.Target, agentBin, "version")
	if err != nil {
		return "", fmt.Errorf("E656: the installed agent of %s did not answer `version`: %w", n.NodeID, err)
	}
	v, ok := ParseVersionLine(out)
	if !ok {
		return "", fmt.Errorf("E656: the agent of %s printed an unexpected version line", n.NodeID)
	}
	return v, nil
}

// Coordinators reads the agent's coordinators.json ("{}" when the agent has had no welcome yet).
func (n *SSHNode) Coordinators(ctx context.Context) ([]session.CoordinatorRecord, error) {
	out, err := n.Run(ctx, n.Target, "sh", "-c", `if [ -e "$1" ]; then cat "$1"; else echo '{}'; fi`, "sh", coordFile)
	if err != nil {
		return nil, fmt.Errorf("E656: cannot read the coordinators of %s: %w", n.NodeID, err)
	}
	var doc struct {
		Coordinators []session.CoordinatorRecord `json:"coordinators"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); err != nil {
		return nil, fmt.Errorf("E656: the coordinators file of %s is not readable JSON", n.NodeID)
	}
	return doc.Coordinators, nil
}

// Drain implements Node.
func (n *SSHNode) Drain(ctx context.Context) (bool, error) { return n.DrainFn(ctx) }

// Busy reads the host slot table (`slots` prints "cap N used M"): every coordinator and carrier shares it.
func (n *SSHNode) Busy(ctx context.Context) (int, error) {
	out, err := n.Run(ctx, n.Target, agentBin, "slots")
	if err != nil {
		return 0, fmt.Errorf("E656: cannot read the slots of %s: %w", n.NodeID, err)
	}
	var capacity, used int
	if _, err := fmt.Sscanf(strings.TrimSpace(lastLine(out)), "cap %d used %d", &capacity, &used); err != nil {
		return 0, fmt.Errorf("E656: the slots answer of %s is not readable", n.NodeID)
	}
	return used, nil
}

// Install copies the verified binary, checks it on the node and swaps it in; the old one stays as .prev.
func (n *SSHNode) Install(ctx context.Context, in InstallInput) error {
	if in.Staged == "" || in.SHA256 == "" {
		return fmt.Errorf("E656: no verified binary to install on %s", n.NodeID)
	}
	staged := agentBin + NewSuffix
	if err := n.Copy(ctx, n.Target, in.Staged, staged); err != nil {
		return fmt.Errorf("E656: cannot copy the agent to %s: %w", n.NodeID, err)
	}
	// Digest and smoke run on the copy; the swap is the last command and runs only when both passed.
	script := `set -e; f="$1"; want="$2"; ver="$3"; bin="$4"
chmod 700 "$f"
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$f"); else got=$(shasum -a 256 "$f"); fi
[ "${got%% *}" = "$want" ] || { rm -f "$f"; echo "checksum mismatch" >&2; exit 3; }
out=$("$f" version) || { rm -f "$f"; exit 4; }
case "$out" in "nself-ci-agent $ver "*|"nself-ci-agent $ver") ;; *) rm -f "$f"; echo "version mismatch" >&2; exit 5;; esac
chmod 755 "$f"
if [ -e "$bin" ]; then rm -f "$bin.prev"; ln "$bin" "$bin.prev" 2>/dev/null || cp -p "$bin" "$bin.prev"; fi
mv -f "$f" "$bin"`
	if _, err := n.Run(ctx, n.Target, "sh", "-c", script, "sh", staged, in.SHA256, in.Version, agentBin); err != nil {
		return fmt.Errorf("E656: the new agent was not installed on %s: %w", n.NodeID, err)
	}
	return nil
}

// Restart is nothing for the ssh carrier: no daemon runs, the next session starts the installed binary.
func (n *SSHNode) Restart(context.Context) error { return nil }

// Hello is the version answer of the installed agent: the ssh carrier's hello (lifecycle `nodes test` does the same).
func (n *SSHNode) Hello(ctx context.Context, wait time.Duration) (string, error) {
	poll := n.HelloPoll
	if poll <= 0 {
		poll = time.Second
	}
	end := time.Now().Add(wait)
	for {
		v, err := n.Version(ctx)
		if err == nil || !time.Now().Before(end) {
			return v, err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(poll):
		}
	}
}

// Probe implements Node.
func (n *SSHNode) Probe(ctx context.Context) error { return n.ProbeFn(ctx) }

// Restore puts the previous binary back with one rename.
func (n *SSHNode) Restore(ctx context.Context) error {
	if _, err := n.Run(ctx, n.Target, "mv", "-f", agentBin+PrevSuffix, agentBin); err != nil {
		return fmt.Errorf("E656: cannot restore the previous agent on %s: %w", n.NodeID, err)
	}
	return nil
}

// Enable implements Node.
func (n *SSHNode) Enable(ctx context.Context) error { return n.EnableFn(ctx) }

func lastLine(s string) string {
	s = strings.TrimRight(s, "\r\n ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
