//go:build integration

package agent

// Purpose: TestAgentPull (P7-NODE-08): the acceptance lines of the pull carrier, one subtest per line. The loopback
//          lines run against the real listener; the NAT and netem lines run a real nself-ci-agent in a Docker
//          node (simharness) that has no published port and reaches the coordinator on the host.
// Inputs:  INTEGRATION=1, a Docker Engine for the container lines, go for the linux agent build.
// Constraints: images are digest-pinned by simharness; nothing here touches a real node; the container never
//          receives a secret, only an enrollment made for the test CA.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func itoa(n int) string { return strconv.Itoa(n) }

func TestAgentPull(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	t.Run("one token on every call of a session; each refusal has its own E661 reason", scenarioTokenTable)
	t.Run("a cancel issued while the agent is disconnected applies on reconnect", func(t *testing.T) { scenarioCancelDuringPartition(t, false) })
	t.Run("a cancel after the run gave up reaches the agent through the epoch", func(t *testing.T) { scenarioCancelDuringPartition(t, true) })
	t.Run("a cancel racing a result resolves by compare and swap", scenarioCancelRace)
	t.Run("a coordinator kill -9 mid-stream leaves no duplicate or missing log line", scenarioRestart)
	t.Run("a 30 s partition resumes without duplicate log lines or results", func(t *testing.T) { scenarioPartition(t, 30*time.Second) })
	t.Run("the job token never appears in a frame the agent sends", TestJobTokenTravelsInAHeaderAndNeverInAFrame)
	t.Run("a token bound before a coordinator restart is rejected from another session", TestGateBindingIsDurableAcrossARestart)
	t.Run("revoke while the agent long-polls closes the stream and refuses the handshake", scenarioRevokeWhilePolling)
	t.Run("an agent refuses a server leaf of node or user kind", scenarioServerLeafKinds)
	t.Run("a blob with a flipped byte is rejected by digest and an interrupted blob resumes by offset", scenarioBlob)
	t.Run("an agent behind NAT runs a job end to end", natScenario)
	t.Run("with its traffic dropped by netem the agent kills its job before the lease expires", netemScenario)
}

// dockerPull is a Docker node running `nself-ci-agent pull` against a coordinator on the host.
type dockerPull struct {
	t    *testing.T
	f    *simharness.Fleet
	e    *e2e
	node string
	url  string
}

func buildLinuxAgent(t *testing.T) string {
	t.Helper()
	arch := "amd64"
	if out, err := exec.Command("docker", "info", "--format", "{{.Architecture}}").Output(); err == nil && (strings.Contains(string(out), "aarch64") || strings.Contains(string(out), "arm64")) {
		arch = "arm64"
	}
	out := filepath.Join(t.TempDir(), "nself-ci-agent")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = "../../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

// newDockerPull starts the node, finds an address of the host it can reach, enrolls the node and starts the agent.
func newDockerPull(t *testing.T, heartbeat, coordTTL time.Duration) *dockerPull {
	t.Helper()
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "n1", Image: simharness.ImageDebian, CapAdd: []string{"NET_ADMIN"}}}})
	sh := func(script string) string { return strings.TrimSpace(f.Exec(t, "n1", "sh", "-c", script)) }
	sh("apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq procps >/dev/null")
	var ips []net.IP
	for _, c := range strings.Fields(sh(`(ip route | awk '/default/ {print $3}'; getent hosts host.docker.internal host.lima.internal | awk '{print $1}') | sort -u`)) {
		if ip := net.ParseIP(c); ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		t.Skip("the node found no address of the host")
	}
	e := newE2EWith(t, "0.0.0.0:0", ips, heartbeat, coordTTL)
	port := e.ln.Addr().(*net.TCPAddr).Port
	var host string
	for _, ip := range ips { // the first address the node can open a TCP connection to
		if sh("timeout 3 bash -c 'exec 3<>/dev/tcp/"+ip.String()+"/"+itoa(port)+"' && echo ok || echo no") == "ok" {
			host = ip.String()
			break
		}
	}
	if host == "" {
		t.Skipf("the node cannot reach the coordinator on the host (tried %v)", ips)
	}
	d := &dockerPull{t: t, f: f, e: e, node: "n1", url: "https://" + net.JoinHostPort(host, itoa(port))}
	d.enrollAndStart(buildLinuxAgent(t), sh)
	return d
}

// enrollAndStart writes the enrollment `join` would have saved, copies it and the agent into the node and
// starts `nself-ci-agent pull` as the node's user.
func (d *dockerPull) enrollAndStart(agentBin string, sh func(string) string) {
	t, e := d.t, d.e
	e.addNode(d.node)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	is, err := e.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: d.node, OfferID: "offer-" + d.node})
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	state := t.TempDir()
	res := enroll.JoinResult{NodeID: d.node, CertDER: is.CertDER, Chain: is.Chain, RootDER: is.RootDER, KeyDER: keyDER, AgentURL: d.url, NotAfter: is.NotAfter, RootFingerprint: pki.CertFingerprint(is.RootDER)}
	if err = enroll.SaveCredential(state, res, time.Now()); err != nil {
		t.Fatal(err)
	}
	const home = "/home/nself/.nself/ci-agent"
	sh("install -d -m 700 " + home + "/pki " + home + "/bin")
	for _, name := range []string{"root.pem", "client.key.sealed", "node.crt", "enrollment.json", "kek"} {
		d.f.CopyTo(t, d.node, filepath.Join(state, "pki", name), home+"/pki/"+name)
	}
	d.f.CopyTo(t, d.node, agentBin, home+"/bin/nself-ci-agent")
	sh("chmod 755 " + home + "/bin/nself-ci-agent && chmod 600 " + home + "/pki/* && chown -R nself:nself /home/nself/.nself")
	sh("su nself -c 'nohup " + home + "/bin/nself-ci-agent pull --slots 1 >/tmp/agent.log 2>&1 &'")
	eventuallyT(t, 60*time.Second, "the node to connect", func() bool { return len(e.srv.NodesConnected()) == 1 }, func() string { return sh("cat /tmp/agent.log") })
}

func eventuallyT(t *testing.T, within time.Duration, what string, cond func() bool, diag func() string) {
	t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s\n%s", what, diag())
}

func (d *dockerPull) sh(script string) string {
	d.t.Helper()
	return strings.TrimSpace(d.f.Exec(d.t, d.node, "sh", "-c", script))
}

func natScenario(t *testing.T) {
	d := newDockerPull(t, 0, 0)
	if listening := d.sh("ss -ltn | awk 'NR>1 {print $4}'"); strings.Contains(listening, ":3864") {
		t.Fatalf("the node listens on the agent port: %s", listening)
	}
	leaseID, att, ep := d.e.seedLease("1", d.node)
	done := d.e.runLease(context.Background(), d.node, leaseID, att, ep)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v\n%s", err, d.sh("cat /tmp/agent.log"))
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("the job never finished\n%s", d.sh("cat /tmp/agent.log"))
	}
	if d.e.sink.nResults() != 1 {
		t.Fatalf("%d results", d.e.sink.nResults())
	}
}

func netemScenario(t *testing.T) {
	d := newDockerPull(t, time.Second, 5*time.Second)
	loop := "while :; do date +%s%N >> /tmp/stamps; sleep 0.1; done"
	leaseID, att, ep := d.e.seedLease("1", d.node)
	l := protoLease(leaseID, att, ep)
	l.Job.Run = []string{"sh", "-c", loop}
	node := d.e.capability(d.node)
	go func() { _ = d.e.drv.Run(context.Background(), node, l, d.e.sink) }()
	eventuallyT(t, 60*time.Second, "the job to write", func() bool { return len(strings.Fields(d.sh("cat /tmp/stamps 2>/dev/null || true"))) > 5 }, func() string { return d.sh("cat /tmp/agent.log") })
	time.Sleep(2500 * time.Millisecond)
	d.f.Netem(t, d.node, 0, 100)
	cut := time.Now()
	eventuallyT(t, 20*time.Second, "the agent to kill its job", func() bool { return d.sh("pgrep -f '[s]leep 0.1' >/dev/null && echo up || echo down") == "down" }, func() string { return d.sh("cat /tmp/agent.log") })
	if took := time.Since(cut); took > 6*time.Second {
		t.Fatalf("the job lived %v after the traffic was dropped; the agent fences at agent_ttl (3 s)", took)
	}
	last := d.sh("tail -n 1 /tmp/stamps")
	if lease, err := d.e.st.LeaseGet(context.Background(), leaseID); err == nil && lease.ExpiresAt != 0 {
		if n, _ := strconv.ParseInt(last, 10, 64); n > lease.ExpiresAt {
			t.Fatalf("the job outlived its lease: last output %d, lease expiry %d", n, lease.ExpiresAt)
		}
	}
	d.f.Netem(t, d.node, 0, 0)
}

// scenarioTokenTable runs the whole R-DS-15 table: acceptance by call type, and every refusal with its reason.
func scenarioTokenTable(t *testing.T) {
	t.Run("one token on log, heartbeat and result", TestGateAdmitsTheSameTokenOnLogHeartbeatAndResult)
	t.Run("each refusal has its own reason", TestGateRefusalTable)
	t.Run("expired", TestGateExpiredToken)
	t.Run("terminal", TestGateTerminalAttempt)
	t.Run("spent", TestGateSpentTokenIsReplayed)
	t.Run("over the wire", func(t *testing.T) { scenarioWireRefusals(t) })
}
