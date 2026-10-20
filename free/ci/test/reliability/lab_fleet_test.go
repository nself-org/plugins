//go:build integration

package reliability

// Purpose: the node side of the lab: a simharness fleet of Debian nodes, each running the real nself-ci-agent
//          (pull carrier) enrolled against the lab's CA, registered in the store as an online agent node.
// Inputs:  a timing profile and a node count.
// Outputs: a lab that the fault tests drive (pause, partition, kill, restart) and probe (job files, agent log).
// Constraints: images are digest-pinned by simharness; the nodes receive only an enrollment made for the test CA;
//          a node that cannot reach the host is a t.Fatal, never a skip.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const agentHome = "/home/nself/.nself/ci-agent"

func itoa(n int) string { return strconv.Itoa(n) }

// lab is one fleet and one coordinator.
type lab struct {
	t     *testing.T
	f     *simharness.Fleet
	c     *coord
	host  string   // the address of the test host the nodes dial
	ids   []string // coordinator-side node ids, in fleet order (n1, n2, ...)
	names []string // container names, same order
	state map[string]string
	root  string       // the repository every run uses; nodes are authorized for it
	runs  atomic.Int32 // commits made so far
}

// newLab starts count nodes, the coordinator and an agent on every node, and waits until all are connected.
func newLab(t *testing.T, p timing, count int) *lab {
	t.Helper()
	needIntegration(t)
	bin := linuxAgent(t)
	specs := make([]simharness.NodeSpec, count)
	for i := range specs {
		specs[i] = simharness.NodeSpec{Name: "n" + itoa(i+1), Image: simharness.ImageDebian, CapAdd: []string{"NET_ADMIN"}}
	}
	l := &lab{t: t, f: simharness.Start(t, simharness.Config{Nodes: specs}), state: map[string]string{}}
	l.initRepo()
	for _, s := range specs {
		l.names = append(l.names, s.Name)
		l.ids = append(l.ids, s.Name+"-"+l.f.ID)
	}
	ips := l.hostCandidates()
	l.c = newCoord(t, t.TempDir(), ips, p)
	l.c.up()
	l.host = l.reachableHost(ips)
	for i, name := range l.names {
		l.enroll(i, name, bin)
	}
	for i := range l.names {
		l.startAgent(i)
	}
	l.waitConnected(60*time.Second, l.ids...)
	t.Cleanup(l.c.down)
	return l
}

// hostCandidates are the addresses of the test host as the first node sees them.
func (l *lab) hostCandidates() []net.IP {
	out := l.sh(0, `(ip route | awk '/default/ {print $3}'; getent hosts host.docker.internal host.lima.internal | awk '{print $1}') | sort -u`)
	var ips []net.IP
	for _, f := range strings.Fields(out) {
		if ip := net.ParseIP(f); ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		l.t.Fatal("the node found no address of the host")
	}
	return ips
}

// reachableHost is the first candidate every node can open a TCP connection to.
func (l *lab) reachableHost(ips []net.IP) string {
	for _, ip := range ips {
		ok := true
		for i := range l.names {
			if l.sh(i, "timeout 3 bash -c 'exec 3<>/dev/tcp/"+ip.String()+"/"+itoa(l.c.port)+"' && echo ok || echo no") != "ok" {
				ok = false
			}
		}
		if ok {
			return ip.String()
		}
	}
	l.t.Fatalf("the nodes cannot reach the coordinator on the host (tried %v)", ips)
	return ""
}

// sh runs script in node i's container as root and returns trimmed stdout; a failing script fails the test.
func (l *lab) sh(i int, script string) string {
	l.t.Helper()
	return strings.TrimSpace(l.f.Exec(l.t, l.names[i], "sh", "-c", script))
}

// enroll issues node i a certificate, registers it and copies the enrollment and the agent into the container.
func (l *lab) enroll(i int, name, bin string) {
	t := l.t
	id := l.ids[i]
	l.registerNode(id)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	is, err := l.c.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	dir := t.TempDir()
	res := enroll.JoinResult{NodeID: id, CertDER: is.CertDER, Chain: is.Chain, RootDER: is.RootDER, KeyDER: keyDER, AgentURL: l.c.url(l.host), NotAfter: is.NotAfter, RootFingerprint: pki.CertFingerprint(is.RootDER)}
	if err = enroll.SaveCredential(dir, res, time.Now()); err != nil {
		t.Fatal(err)
	}
	l.state[name] = dir
	l.sh(i, "install -d -m 700 "+agentHome+"/pki "+agentHome+"/bin")
	for _, f := range []string{"root.pem", "client.key.sealed", "node.crt", "enrollment.json", "kek"} {
		l.f.CopyTo(t, name, filepath.Join(dir, "pki", f), agentHome+"/pki/"+f)
	}
	l.f.CopyTo(t, name, bin, agentHome+"/bin/nself-ci-agent")
	l.sh(i, "chmod 755 "+agentHome+"/bin/nself-ci-agent && chmod 600 "+agentHome+"/pki/* && chown -R nself:nself /home/nself/.nself")
}

// registerNode adds the node to the store as an online agent node that accepts owner and internal work.
func (l *lab) registerNode(id string) {
	t := l.t
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "reliability", Platform: hostPlatform(), CPUs: 4, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	lan, owned, uid := "lan", "owned", true
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "agent", "agent"
	c.SecretsEligible = true
	c.Lifecycle.AuthorizedProjects = []string{l.root}
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = l.c.st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	trust := model.CapabilityTrust{
		Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     assigned(model.Isolation("container")),
		Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: assigned([]model.SecretClass{"none"}),
		PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
	}
	reg := registry.New(l.c.st)
	if _, err = reg.SetTrust(ctx, id, trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
}

func assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

// startAgent starts `nself-ci-agent pull` in node i as the node user; its pid goes to /tmp/agent.pid.
func (l *lab) startAgent(i int) {
	l.sh(i, "su nself -c 'nohup "+agentHome+"/bin/nself-ci-agent pull --slots 1 >>/tmp/agent.log 2>&1 & echo $! >/tmp/agent.pid'")
}

// waitConnected waits until every named node has a live session.
func (l *lab) waitConnected(within time.Duration, ids ...string) {
	l.t.Helper()
	until(l.t, within, "the nodes to connect", func() bool {
		have := map[string]bool{}
		for _, n := range l.c.connected() {
			have[n] = true
		}
		for _, id := range ids {
			if !have[id] {
				return false
			}
		}
		return true
	}, l.diag)
}

// diag is the agent log of every node, for a failure message.
func (l *lab) diag() string {
	var b strings.Builder
	for i, n := range l.names {
		out, err := l.tryExec(i, "tail -n 40 /tmp/agent.log")
		if err != nil {
			out = "(" + err.Error() + ")"
		}
		b.WriteString("--- agent log of " + n + " (" + l.ids[i] + ")\n" + out + "\n")
	}
	return b.String()
}

// until polls cond every 50 ms until it holds or within elapses (then it fails with diag's text).
func until(t *testing.T, within time.Duration, what string, cond func() bool, diag func() string) {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	d := ""
	if diag != nil {
		d = diag()
	}
	t.Fatalf("timed out after %v waiting for %s\n%s", within, what, d)
}

// tryExec runs script in node i's container and returns its output and error (a paused or stopped node fails).
func (l *lab) tryExec(i int, script string) (string, error) {
	out, err := exec.Command("docker", "exec", l.f.Nodes[i].Container, "sh", "-c", script).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// hostPlatform is the platform the run asks every job for (run.platform: the coordinator host). The nodes declare
// it, so placement works on a macOS developer machine too; the fixture job is a shell script and runs anywhere.
func hostPlatform() string { return runtime.GOOS + "/" + runtime.GOARCH }
