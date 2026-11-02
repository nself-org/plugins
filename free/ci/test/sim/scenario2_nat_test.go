//go:build integration

package sim

// Purpose: the NAT'd paired agent leg of TestScenario2 (P7-NODE-17, NODE acceptance 1). A bare foreground run
//          pinned with `--where nat-agent` (the run.Coordinator `nself ci` builds, V15 dispatch) is completed by a
//          paired node that only ever dials OUT: the pull listener of the coordinator is the single route between
//          them. The node runs the real session over the real agent carrier (mTLS, long-poll lease, upload stream).
// Inputs:  INTEGRATION=1. No Docker, no ssh. The Docker variant of the NAT line is TestAgentPull (internal/transport/agent).
// Outputs: none beyond pass or fail.
// Constraints: the node has no inbound path (no ssh target in its capability, the ssh starter fails the test if it is
//          called, the node opens no listener); the job must run on that node, the lease must be held by it and the
//          evidence recorded for the attempt must name it and point at the placement decision record, which names the node.
//          (The run-level runner field of coordinator-recorded evidence still reads "local" for a remote run; see the report.)

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"errors"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
	"github.com/nself-org/plugins/free/ci/internal/where"
)

// natKey is the node's ed25519 identity as the coordinator checks it in hello.
type natKey struct{ pub ed25519.PublicKey }

func (k natKey) KeyID() string        { return pki.NodeKeyID(k.pub) }
func (k natKey) PublicString() string { return base64.StdEncoding.EncodeToString(k.pub) }

// natCoordinator is `ci serve` in this process: the agent listener on a loopback port over the shared state.db.
type natCoordinator struct {
	srv  *agentapi.Server
	ca   *pki.CA
	base string
}

// startNATCoordinator wires the agent transport the way cmd/transport_agent.go does and serves it until cleanup.
func startNATCoordinator(t *testing.T, st *store.Store) *natCoordinator {
	t.Helper()
	ctx := context.Background()
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := ca.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drv := agent.Default
	drv.Wire(agent.Wiring{Gate: &agent.Gate{Tokens: tokens, Bindings: st, Leases: st, Certs: st}, Minter: tokens, Leases: st, LogDir: t.TempDir()})
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord-nat", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leaf.Get, Leases: lease.New(st, nil, lease.Config{}), Sink: drv.Sink(), Source: drv,
		Store: st, Admit: drv.Admit, Blobs: drv.Blobs(), PollTimeout: 2 * time.Second, SourcePoll: 20 * time.Millisecond, GuardEvery: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
		<-done
		drv.Wire(agent.Wiring{})
		drv.SetCancel(nil)
	})
	return &natCoordinator{srv: srv, ca: ca, base: "https://" + ln.Addr().String()}
}

// cert issues the node's client certificate.
func (c *natCoordinator) cert(t *testing.T, id string) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	is, err := c.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		t.Fatal(err)
	}
	leafCert, _ := x509.ParseCertificate(is.CertDER)
	return tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leafCert}
}

// registerPullNode stores the node as an online, paired `agent` node with operator-assigned trust and no network address.
func registerPullNode(t *testing.T, f *fixture, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: f.repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	remoteKind, owned, uid, op := "remote", "owned", true, "operator"
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "agent", agent.Kind
	c.Identity.Ownership.Value = &op
	c.SecretsEligible = true
	c.Location.Kind.Value, c.Economics.Class.Value = &remoteKind, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	trust := model.CapabilityTrust{
		Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     assigned(model.Isolation("container")),
		Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: assigned([]model.SecretClass{"none"}),
		PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, id, trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
}

// startPullAgent runs the node: the real session over the real pull carrier, one slot, and a runner that records its jobs.
func startPullAgent(t *testing.T, coord *natCoordinator, id string, n *node) {
	t.Helper()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	cert := coord.cert(t, id)
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: coord.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	car := agent.NewCarrier(agent.CarrierConfig{Client: agent.Options{BaseURL: coord.base, TLS: tlsc, Wait: time.Second}, Backoff: agent.Backoff{Min: 10 * time.Millisecond, Max: 50 * time.Millisecond}})
	runner := session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		n.mu.Lock()
		n.jobs = append(n.jobs, j.Lease.LeaseID)
		n.mu.Unlock()
		n.started <- j.Lease.AttemptID
		return n.runner(ctx, n, j)
	})
	table, err := slots.Open(n.state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s, err := session.New(session.Config{Carrier: car, Key: natKey{pub}, Slots: table, Runner: runner, StateDir: n.state, Hooks: []session.JobHook{},
		Coordinator: session.Coordinator{ID: "coord-nat", RootFingerprint: pki.CertFingerprint(coord.ca.RootDER())}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: id}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Run(ctx) }()
	t.Cleanup(func() { cancel(); _ = car.Close(); <-done })
}

var errNoInbound = errors.New("the NAT'd node has no inbound path")

// natBareRun is the subtest "a NAT'd paired agent is reached through the bare run".
func natBareRun(t *testing.T) {
	const id = "nat-agent"
	f := newFixtureWith(t, nil, nil)
	var dialled atomic.Int32
	ssh.Default.Start = func(context.Context, remote.Target, string) (*ssh.Conn, error) {
		dialled.Add(1)
		return nil, errNoInbound
	}
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	registerPullNode(t, f, st, id)
	coord := startNATCoordinator(t, st)
	n := &node{id: id, state: t.TempDir(), started: make(chan string, 8), runner: passRunner("nat job")}
	startPullAgent(t, coord, id, n)
	deadline := time.Now().Add(30 * time.Second)
	for len(coord.srv.NodesConnected()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the node never connected to the coordinator")
		}
		time.Sleep(50 * time.Millisecond)
	}

	tgt, err := where.Values(id, where.Known{Nodes: []string{id}}, where.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": remoteJob(1)}}, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384, Remote: true}})
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: f.repo, Home: f.home, NoStatus: true, PolicyRun: tgt.Values}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: f.git, Reporter: noReport{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ch := make(chan run.Result, 1)
	go func() {
		res, rerr := c.Run(context.Background())
		if rerr != nil {
			t.Errorf("Run: %v\n%s", rerr, out)
		}
		ch <- res
	}()
	res := wait(t, ch, "the run")
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s\n%s\n%s", res.Exit, res.Summary.Result, out, dumpLogs(f.home))
	}
	if ran := n.ran(); len(ran) != 1 {
		t.Fatalf("the job ran %d times on %s: %v", len(ran), id, ran)
	}
	if dialled.Load() != 0 {
		t.Fatalf("the coordinator dialled the node %d times; it has no inbound path", dialled.Load())
	}
	lines := explainJob(t, f, res.RunID, "unit")
	t.Logf("ci explain:\n%s", lines)
	if !strings.Contains(lines, id) || !strings.Contains(lines, "placed") {
		t.Errorf("explain does not show the placement on %s", id)
	}
	if jo := res.Summary.Jobs[0]; jo.State != "passed" || !strings.Contains(jo.Tail, "nat job") {
		t.Errorf("job outcome %+v: the node's output did not reach the run", jo)
	}
	db, err := sql.Open("sqlite", filepath.Join(f.home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var attempt, state string
	if err = db.QueryRow("SELECT id, state FROM attempt").Scan(&attempt, &state); err != nil || state != "passed" {
		t.Fatalf("the attempt %q is %q (%v), want passed", attempt, state, err)
	}
	select {
	case got := <-n.started:
		if got != attempt {
			t.Errorf("the node ran attempt %q, the run recorded %q", got, attempt)
		}
	default:
		t.Error("the node reported no job start")
	}
	var body []byte
	if err = db.QueryRow("SELECT body FROM evidence WHERE attempt_id = ?", attempt).Scan(&body); err != nil {
		t.Fatalf("no evidence is recorded for attempt %s: %v", attempt, err)
	}
	var ev model.Evidence
	if err = json.Unmarshal(body, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Attempt == nil || ev.Attempt.ID != attempt || ev.Result != "pass" || ev.PlacementRef == nil || *ev.PlacementRef != "sched:decision/"+attempt {
		t.Errorf("evidence attempt %+v result %s placement_ref %v: want the attempt %s, pass and its decision record", ev.Attempt, ev.Result, ev.PlacementRef, attempt)
	}
}
