//go:build cachesec

package cachesec

// Purpose: group g3 real-fleet harness. ONE simharness fleet (a Debian sshd node running the real
// nself-ci-agent binary) serves every g3 case that needs a real container carrier. The fleet starts
// lazily, runs all fleet scenarios inside the first fleet case (as nested subtests), and is closed when
// they end, so no fleet outlives the group and a -run filter cannot leak one. Each case then reports the
// Observed its scenario stored.
// Inputs: INTEGRATION=1, a Docker Engine, go for the linux agent build. Outputs: g3FleetCase.
// Constraints: images are digest-pinned by simharness; a missing prerequisite is t.Fatal, never a skip; the
// node never receives a secret; no checkout is leased, so the node needs no git.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdkremote "github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
	sshdrv "github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

const g3AgentDir = "/home/nself/.nself/ci-agent"

// g3Fleet is the shared fleet: node n1 with the agent installed and its host key pinned.
type g3Fleet struct {
	f    *simharness.Fleet
	home string
	repo string
	rev  string
}

type g3FleetResult struct {
	obs  Observed
	fail string
}

var g3fleetState struct {
	once sync.Once
	res  map[string]g3FleetResult
}

// g3FleetScenarios maps a case id to its scenario; each receives the shared fleet.
var g3FleetScenarios = map[string]func(t *testing.T, fl *g3Fleet) Observed{}

// g3FleetOrder is the order the scenarios run in (deterministic).
var g3FleetOrder []string

func g3RegisterFleet(id string, fn func(t *testing.T, fl *g3Fleet) Observed) {
	g3FleetScenarios[id] = fn
	g3FleetOrder = append(g3FleetOrder, id)
	register(id, func(t *testing.T) Observed { return g3FleetCase(t, id) })
}

// g3FleetCase starts the fleet on its first call, runs every fleet scenario there and replays the stored result.
func g3FleetCase(t *testing.T, id string) Observed {
	t.Helper()
	g3fleetState.once.Do(func() {
		g3fleetState.res = map[string]g3FleetResult{}
		t.Run("fleet", func(t *testing.T) {
			fl := g3StartFleet(t)
			for _, sid := range g3FleetOrder {
				sid := sid
				g3fleetState.res[sid] = g3FleetResult{fail: "scenario did not complete"}
				t.Run(sid, func(t *testing.T) {
					fl.waitAgentGone(t)
					obs := g3FleetScenarios[sid](t, fl)
					g3fleetState.res[sid] = g3FleetResult{obs: obs}
				})
			}
		})
	})
	r, ok := g3fleetState.res[id]
	if !ok || r.fail != "" {
		t.Fatalf("fleet scenario %s failed or did not run: %s (see the nested subtest of the first fleet case)", id, r.fail)
	}
	return r.obs
}

func g3NodeArch(t *testing.T) string {
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

func g3BuildAgent(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "g3-agent-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	out := filepath.Join(dir, "nself-ci-agent")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+g3NodeArch(t), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

func g3StartFleet(t *testing.T) *g3Fleet {
	t.Helper()
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "n1", Image: simharness.ImageDebian}}})
	fl := &g3Fleet{f: f, home: t.TempDir()}
	fl.repo, fl.rev = g3MakeRepo(t)
	// the node needs git to unpack the checkout bundle (the production ssh rig installs it the same way)
	fl.sh(t, "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git >/dev/null")
	fl.sh(t, "install -d "+g3AgentDir+"/bin && chown -R nself:nself /home/nself/.nself")
	f.CopyTo(t, "n1", g3BuildAgent(t), g3AgentDir+"/bin/nself-ci-agent")
	fl.sh(t, "chmod 755 "+g3AgentDir+"/bin/nself-ci-agent && chown nself:nself "+g3AgentDir+"/bin/nself-ci-agent")
	fs := strings.Fields(fl.sh(t, "cat /etc/ssh/ssh_host_ed25519_key.pub"))
	pin := sdkremote.PinnedHostKeys{Path: filepath.Join(fl.home, "known_hosts")}
	if err := pin.Replace("nself-ci-n1", fs[0]+" "+fs[1]); err != nil {
		t.Fatal(err)
	}
	return fl
}

func (fl *g3Fleet) sh(t *testing.T, script string) string {
	t.Helper()
	return strings.TrimSpace(fl.f.Exec(t, "n1", "sh", "-c", script))
}

// waitAgentGone waits until no agent of an earlier scenario still holds the node's host slot.
func (fl *g3Fleet) waitAgentGone(t *testing.T) {
	t.Helper()
	defer fl.sh(t, "rm -f "+g3AgentDir+"/slots.json; echo done") // a previous scenario's table must not hold a slot (E654 slots full)
	end := time.Now().Add(20 * time.Second)
	for fl.sh(t, "pgrep -f '[n]self-ci-agent serve' || echo none") != "none" {
		if time.Now().After(end) { // a failed earlier scenario must not poison the next one
			fl.sh(t, "pkill -f '[n]self-ci-agent serve'; sleep 2; echo done")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Run delivers the rig's lease to the node over the real ssh client and the real agent.
func (fl *g3Fleet) Run(t *testing.T, r *g3rig, sink *g3Sink) error {
	t.Helper()
	n := fl.f.Node(t, "n1")
	var c model.Capability
	c.Identity.ID, c.Identity.Transport = "n1", sshdrv.Kind
	op := "operator"
	c.Identity.Ownership.Value = &op
	c.SecretsEligible = true
	c.Identity.SSH = &model.CapabilitySSH{Hostname: n.Host, Port: n.Port, User: n.User}
	r.rev = fl.rev
	d := &sshdrv.Driver{Home: fl.home, RepoDir: fl.repo, IdentityFile: fl.f.KeyPath(), NoRenew: true, Heartbeat: time.Second, CancelGrace: 3 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return d.Run(ctx, c, r.Proto(), sink)
}

// g3MakeRepo is a one-commit repository on the host: the revision the lease checks out.
func g3MakeRepo(t *testing.T) (dir, rev string) {
	t.Helper()
	dir = t.TempDir()
	g := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", "f.txt")
	g("commit", "-q", "-m", "one")
	return dir, g("rev-parse", "HEAD")
}
