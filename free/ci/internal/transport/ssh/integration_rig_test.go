//go:build integration

package ssh

// Purpose: the Docker rig of TestSSHTransport: a Debian sshd node with the real
// nself-ci-agent installed, a pinned host key and a driver that talks to it
// through the real ssh client.
// Inputs:  INTEGRATION=1, a reachable Docker Engine, ssh and git on the host.
// Constraints: images are digest-pinned by simharness; nothing here touches a real
// node; the agent binary is built for the node's architecture.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const agentDir = "/home/nself/.nself/ci-agent"

type itRig struct {
	t         *testing.T
	fleet     *simharness.Fleet
	home      string
	repo, rev string
	drv       *Driver
}

func nodeArch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "info", "--format", "{{.Architecture}}").Output()
	if err != nil {
		t.Fatalf("docker info: %v", err)
	}
	if a := strings.TrimSpace(string(out)); a == "aarch64" || a == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func buildAgent(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "nself-ci-agent")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = "../../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+nodeArch(t), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

func newITRig(t *testing.T) *itRig {
	t.Helper()
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{
		{Name: "n1", Image: simharness.ImageDebian},
		{Name: "n2", Image: simharness.ImageDebian, CapAdd: []string{"NET_ADMIN"}},
	}})
	r := &itRig{t: t, fleet: f, home: t.TempDir()}
	r.repo, r.rev = makeRepo(t)
	r.sh("n1", "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git procps >/dev/null")
	r.sh("n1", "install -d "+agentDir+"/bin && chown -R nself:nself /home/nself/.nself")
	f.CopyTo(t, "n1", buildAgent(t), agentDir+"/bin/nself-ci-agent")
	r.sh("n1", "chmod 755 "+agentDir+"/bin/nself-ci-agent && chown nself:nself "+agentDir+"/bin/nself-ci-agent")
	r.pin("n1")
	r.pin("n2")
	r.drv = r.driver()
	return r
}

// driver returns a fresh driver for the rig (a Driver holds a sync.Once, so it is never copied).
func (r *itRig) driver() *Driver {
	return &Driver{Home: r.home, RepoDir: r.repo, IdentityFile: r.fleet.KeyPath(), NoRenew: true, Heartbeat: time.Second, CancelGrace: 3 * time.Second}
}

// sh runs script as root on the node and returns its stdout.
func (r *itRig) sh(node, script string) string {
	r.t.Helper()
	return strings.TrimSpace(r.fleet.Exec(r.t, node, "sh", "-c", script))
}

// pin records the node's current ed25519 host key under its alias.
func (r *itRig) pin(node string) {
	r.t.Helper()
	f := strings.Fields(r.sh(node, "cat /etc/ssh/ssh_host_ed25519_key.pub"))
	pin := remote.PinnedHostKeys{Path: filepath.Join(r.home, "known_hosts")}
	if err := pin.Replace("nself-ci-"+node, f[0]+" "+f[1]); err != nil {
		r.t.Fatal(err)
	}
}

func (r *itRig) node(name string) model.Capability {
	r.t.Helper()
	n := r.fleet.Node(r.t, name)
	var c model.Capability
	c.Identity.ID = name
	c.Identity.Transport = Kind
	c.Identity.SSH = &model.CapabilitySSH{Hostname: n.Host, Port: n.Port, User: n.User}
	return c
}

func (r *itRig) lease(id string, argv ...string) protocol.Lease {
	l := testLease()
	l.LeaseID, l.AttemptID = id, "a-"+id
	l.Job = model.Job{Kind: "static", Run: argv}
	l.Checkout.Revision = r.rev
	return l
}

// pgrepGone polls the node until no process matches pattern or the budget ends.
func (r *itRig) pgrepGone(pattern string, budget time.Duration) bool {
	r.t.Helper()
	deadline := time.Now().Add(budget)
	for {
		if r.sh("n1", "pgrep -f '"+pattern+"' || echo none") == "none" {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// lineSink wraps recSink with arrival times and a per-line callback.
type lineSink struct {
	*recSink
	mu2    sync.Mutex
	at     map[string]time.Time
	onLine func(string)
}

func newLineSink(on func(string)) *lineSink {
	return &lineSink{recSink: &recSink{}, at: map[string]time.Time{}, onLine: on}
}

func (s *lineSink) Log(ctx context.Context, l protocol.Log) error {
	s.mu2.Lock()
	s.at[l.Text] = time.Now()
	s.mu2.Unlock()
	if s.onLine != nil {
		s.onLine(l.Text)
	}
	return s.recSink.Log(ctx, l)
}
