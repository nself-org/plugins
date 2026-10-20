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
	"crypto/rand"
	"encoding/hex"
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
	// DrainFn and ProbeFn are the lifecycle commands for this node.
	DrainFn func(ctx context.Context) (undo func(context.Context) error, err error)
	ProbeFn func(ctx context.Context) error
	// HelloPoll is the pause between version checks while waiting for hello; SettleWait bounds the wait for a cut-off install script.
	HelloPoll, SettleWait time.Duration

	// from is the version seen first; Restore puts back a .prev only when the marker next to it names this version.
	from string
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
	if n.from == "" {
		n.from = v
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
	// A login banner may precede the document: it starts at the first brace.
	raw := out
	if i := strings.Index(raw, "{"); i >= 0 {
		raw = raw[i:]
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("E656: the coordinators file of %s is not readable JSON", n.NodeID)
	}
	return doc.Coordinators, nil
}

// Drain implements Node.
func (n *SSHNode) Drain(ctx context.Context) (func(context.Context) error, error) {
	return n.DrainFn(ctx)
}

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
	// A name of its own per run: two coordinators upgrading one node never share a staged file.
	var nonce [6]byte
	_, _ = rand.Read(nonce[:])
	staged := agentBin + NewSuffix + "-" + hex.EncodeToString(nonce[:])
	if err := n.Copy(ctx, n.Target, in.Staged, staged); err != nil {
		return fmt.Errorf("E656: cannot copy the agent to %s: %w", n.NodeID, err)
	}
	// Digest, smoke and a last look at the slot table run on the copy; the swap is the last command and runs only when all passed
	// (another coordinator may have leased a job since the idle check). The marker names the version that .prev holds.
	script := `set -e; f="$1"; want="$2"; ver="$3"; bin="$4"; from="$5"
chmod 700 "$f"
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$f"); else got=$(shasum -a 256 "$f"); fi
[ "${got%% *}" = "$want" ] || { rm -f "$f"; echo "checksum mismatch" >&2; exit 3; }
out=$("$f" version) || { rm -f "$f"; exit 4; }
last=$(printf '%s' "$out" | tail -n 1)
case "$last" in "nself-ci-agent $ver "*|"nself-ci-agent $ver") ;; *) rm -f "$f"; echo "version mismatch" >&2; exit 5;; esac
if [ -e "$bin" ]; then
  used=$("$bin" slots | tail -n 1 | awk '{print $4}')
  [ "$used" = 0 ] || { rm -f "$f"; echo BUSY; exit 0; }
fi
chmod 755 "$f"
if [ -e "$bin" ]; then rm -f "$bin.prev" "$bin.prev.version"; ln "$bin" "$bin.prev" 2>/dev/null || cp -p "$bin" "$bin.prev"; echo "$from" > "$bin.prev.version"; fi
mv -f "$f" "$bin"
echo SWAPPED`
	out, err := n.Run(ctx, n.Target, "sh", "-c", script, "sh", staged, in.SHA256, in.Version, agentBin, n.from)
	if err != nil {
		// A dropped or killed ssh does not stop the remote script (no tty): let it finish before anyone looks at the node.
		n.settle(staged)
		return fmt.Errorf("E656: the new agent was not installed on %s: %w", n.NodeID, err)
	}
	switch lastLine(out) { // the markers are the last line, so a login banner cannot fake one
	case "BUSY":
		return refuse(ReasonRunningJob, "a job started on %s during the copy; nothing was installed", n.NodeID)
	case "SWAPPED":
		return nil
	}
	return fmt.Errorf("E656: the install on %s did not report a finished swap", n.NodeID)
}

// settle waits (bounded, on a context that outlives a cancelled run) until the staged file is gone: the install script removes it as
// its last act, whether it swapped or gave up. A partial copy that no script ever picked up stays and is simply left.
func (n *SSHNode) settle(staged string) {
	wait := n.SettleWait
	if wait <= 0 {
		wait = 30 * time.Second
	}
	poll := n.HelloPoll
	if poll <= 0 {
		poll = time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait+10*time.Second)
	defer cancel()
	for end := time.Now().Add(wait); time.Now().Before(end); time.Sleep(poll) {
		out, err := n.Run(ctx, n.Target, "sh", "-c", `[ ! -e "$1" ] && echo GONE`, "sh", staged)
		if err == nil && lastLine(out) == "GONE" {
			return
		}
	}
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

// Restore puts the previous binary back with one rename, and only when the marker next to it says it is the version this node ran
// before the upgrade (a stale .prev from an older upgrade is never put back).
func (n *SSHNode) Restore(ctx context.Context) error {
	if n.from == "" {
		return fmt.Errorf("E656: %s has no recorded previous version", n.NodeID)
	}
	script := `[ "$(cat "$1.prev.version" 2>/dev/null)" = "$2" ] || { echo NOPREV; exit 0; }
mv -f "$1.prev" "$1" && rm -f "$1.prev.version" && echo RESTORED`
	out, err := n.Run(ctx, n.Target, "sh", "-c", script, "sh", agentBin, n.from)
	if err != nil {
		return fmt.Errorf("E656: cannot restore the previous agent on %s: %w", n.NodeID, err)
	}
	switch lastLine(out) {
	case "RESTORED":
		return nil
	case "NOPREV":
		return nil // the swap never happened: the installed agent already is the previous one
	}
	return fmt.Errorf("E656: the restore on %s did not report a result", n.NodeID)
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\r\n ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
