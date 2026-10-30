//go:build integration

package sim

// Purpose: helpers of TestScenario3 (P7-NODE-17): the Docker node's agent build and registration, the real ssh driver
//          settings, the link measurement and polling.
// Inputs:  a Docker Engine, the fixture repository and CI home.
// Outputs: a registered online ssh node with operator-assigned trust, durations.
// Constraints: nothing here touches a real host; trust fields are written only through registry.SetTrust.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// dockerArch is the architecture of the Docker node ("amd64" or "arm64").
func dockerArch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "info", "--format", "{{.Architecture}}").Output()
	if err != nil {
		t.Fatalf("docker info: %v (TestScenario3 needs a reachable Docker Engine)", err)
	}
	if a := strings.TrimSpace(string(out)); a == "aarch64" || a == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// buildVPSAgent builds the real agent for the node's architecture.
func buildVPSAgent(t *testing.T, arch string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "nself-ci-agent")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

// registerVPS stores the node as an online, paired ssh node with operator-assigned trust.
func registerVPS(t *testing.T, f *fixture, n *simharness.Node, arch string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: f.repo, Platform: "linux/" + arch, CPUs: 2, MemMB: 2048, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	remoteKind, owned, uid, op := "remote", "owned", true, "operator"
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = "vps", "vps", "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: n.Host, Port: n.Port, User: n.User}
	c.Identity.Ownership.Value = &op
	c.SecretsEligible = true
	c.Location.Kind.Value, c.Economics.Class.Value = &remoteKind, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: "vps", Name: "vps", State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	trust := model.CapabilityTrust{
		Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     assigned(model.Isolation("container")), // proved on the node by the caller before this runs
		Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: assigned([]model.SecretClass{"none"}),
		PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, "vps", trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, "vps", "online", "", "test"); err != nil {
		t.Fatal(err)
	}
}

// useRealSSH points the dispatch driver at the real ssh client and the node's key; the previous settings return at cleanup.
func useRealSSH(t *testing.T, f *fixture, key string) {
	t.Helper()
	d := ssh.Default
	home, repo, min, start, ver, renew, personal, ident, hb, ttl, grace := d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal, d.IdentityFile, d.Heartbeat, d.AgentTTL, d.CancelGrace
	d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.IdentityFile = f.home, f.repo, "", nil, nil, key
	d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = run.RenewLease, true, time.Second, 25*time.Second, 3*time.Second
	t.Cleanup(func() {
		d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.IdentityFile = home, repo, min, start, ver, ident
		d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = renew, personal, hb, ttl, grace
	})
}

// bannerMs is the time from opening a connection to the node until its ssh banner arrives. Docker's published port is
// answered by a local proxy, so a bare connect never crosses the node's network; the banner is sent by sshd inside the
// node, so netem delays it.
func bannerMs(t *testing.T, n *simharness.Node) float64 {
	t.Helper()
	begin := time.Now()
	c, err := net.DialTimeout("tcp", n.Addr(), 10*time.Second)
	if err != nil {
		t.Fatalf("connect %s: %v", n.Addr(), err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 4)
	if _, err = io.ReadFull(c, buf); err != nil || string(buf) != "SSH-" {
		t.Fatalf("no ssh banner from %s: %q %v", n.Addr(), buf, err)
	}
	return ms(time.Since(begin))
}

func minOf(v []float64) float64 {
	m := v[0]
	for _, x := range v {
		m = min(m, x)
	}
	return m
}

// eventually polls cond every 100 ms until it holds or budget ends.
func eventually(budget time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(budget)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// newRepoFixture is a git repository with one commit and an empty CI home; nodes are registered by the caller.
func newRepoFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, repo: t.TempDir(), home: t.TempDir()}
	gitIn(t, f.repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(f.repo, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.repo, "add", "f.txt")
	gitIn(t, f.repo, "commit", "-q", "-m", "init")
	f.sha = gitIn(t, f.repo, "rev-parse", "HEAD")
	return f
}

// dumpLogs is the text of every job log the run kept under home, for a failure message.
func dumpLogs(home string) string {
	var b strings.Builder
	_ = filepath.WalkDir(filepath.Join(home, "logs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			raw, _ := os.ReadFile(p)
			b.WriteString("== " + p + "\n" + string(raw) + "\n")
		}
		return nil
	})
	return b.String()
}

// vpsRig drives the Docker node with the real ssh driver, as the transport tests do, over the netem link.
type vpsRig struct {
	t               *testing.T
	sh              func(string) string
	node            *simharness.Node
	home, repo, sha string
	key             string
}

func (r *vpsRig) driver() *ssh.Driver {
	return &ssh.Driver{Home: r.home, RepoDir: r.repo, IdentityFile: r.key, NoRenew: true, Heartbeat: time.Second, CancelGrace: 3 * time.Second}
}

func (r *vpsRig) capability() model.Capability {
	var c model.Capability
	op := "operator"
	c.Identity.ID, c.Identity.Transport = "vps", ssh.Kind
	c.Identity.Ownership.Value = &op
	c.SecretsEligible = true
	c.Identity.SSH = &model.CapabilitySSH{Hostname: r.node.Host, Port: r.node.Port, User: r.node.User}
	return c
}

func (r *vpsRig) lease(id string, argv ...string) protocol.Lease {
	return protocol.Lease{LeaseID: id, AttemptID: "a-" + id, Epoch: 4, Token: "opaque", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static", Run: argv}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2099-01-01T00:00:00Z",
		Checkout: protocol.Checkout{Revision: r.sha}}
}

// logins counts the public-key logins sshd has accepted.
func (r *vpsRig) logins() int {
	out, _ := exec.Command("docker", "logs", r.node.Container).CombinedOutput()
	return strings.Count(string(out), "Accepted publickey")
}

func (r *vpsRig) requireJobRootGone(budget time.Duration) {
	r.t.Helper()
	if !eventually(budget, func() bool {
		return r.sh("find "+vpsAgentDir+"/jobs "+vpsAgentDir+"/checkout -mindepth 1 2>/dev/null | wc -l") == "0"
	}) {
		r.t.Fatalf("a job root is left on the node: %s", r.sh("ls -lA "+vpsAgentDir+"/jobs "+vpsAgentDir+"/checkout"))
	}
}

// lineSink is a transport.Sink that keeps the log lines with their arrival times; the rest is refused or dropped.
type lineSink struct {
	mu     sync.Mutex
	text   []string
	at     map[string]time.Time
	onLine func(string)
}

func newLineSink(on func(string)) *lineSink { return &lineSink{at: map[string]time.Time{}, onLine: on} }

func (s *lineSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	s.text = append(s.text, l.Text)
	s.at[strings.Fields(l.Text + " x")[0]] = time.Now()
	s.mu.Unlock()
	if s.onLine != nil {
		s.onLine(l.Text)
	}
	return nil
}
func (s *lineSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (s *lineSink) Result(context.Context, protocol.Result) error           { return nil }
func (s *lineSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (s *lineSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, errors.New("this sink keeps no blobs")
}

func (s *lineSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.text...)
}

func (s *lineSink) arrival(first string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.at[first]
}
