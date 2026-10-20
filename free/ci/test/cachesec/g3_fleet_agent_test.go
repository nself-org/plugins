//go:build cachesec

package cachesec

// Purpose: group g3 real-fleet agent carrier. The coordinator runs the real agent pull listener (mTLS 1.3, token
// gate, lease engine) on the host; the node is the shared simharness container running the real
// `nself-ci-agent pull` with an enrollment made for a test CA. The container reaches the host over the
// Colima/Docker host alias, the same way the production transport/agent integration test does.
// Inputs: the shared g3Fleet and a g3rig. Outputs: the driver result; frames land in the rig's g3Sink.
// Constraints: the container receives only an enrollment for a throwaway CA, never a secret; a missing route
// from the container to the host is t.Fatal, not a skip.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
	agenttr "github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// RunAgent delivers the rig's lease to the container's pull agent and returns the driver's result.
func (fl *g3Fleet) RunAgent(t *testing.T, r *g3rig, sink *g3Sink) error {
	t.Helper()
	ctx := context.Background()
	st := r.w.St
	var ips []net.IP
	for _, c := range strings.Fields(fl.sh(t, `(ip route | awk '/default/ {print $3}'; getent hosts host.docker.internal host.lima.internal | awk '{print $1}') | sort -u`)) {
		if ip := net.ParseIP(c); ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		t.Fatal("the node found no address of the host")
	}
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: t.TempDir(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := ca.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drv := &agenttr.Driver{Gate: &agenttr.Gate{Tokens: tokens, Bindings: st, Leases: st}, Minter: tokens, Leases: st, LogDir: t.TempDir(), Poll: 30 * time.Millisecond, CancelGrace: 2 * time.Second}
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord-1", IPs: ips}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leaf.Get, Leases: r.eng, Sink: drv.Sink(), Source: drv, Store: st,
		Admit: drv.Admit, Blobs: drv.Blobs(), PollTimeout: 2 * time.Second, SourcePoll: 20 * time.Millisecond, GuardEvery: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(c); err != nil {
			t.Errorf("fleet agent API shutdown: %v", err)
		}
		if err := <-done; err != nil {
			t.Errorf("fleet agent API serve: %v", err)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	host := ""
	for _, ip := range ips {
		if fl.sh(t, "timeout 3 bash -c 'exec 3<>/dev/tcp/"+ip.String()+"/"+strconv.Itoa(port)+"' && echo ok || echo no") == "ok" {
			host = ip.String()
			break
		}
	}
	if host == "" {
		t.Fatalf("the node cannot reach the coordinator on the host (tried %v)", ips)
	}
	// register the node and make its enrollment
	b, err := os.ReadFile("../../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var capab model.Capability
	if err = json.Unmarshal(b, &capab); err != nil {
		t.Fatal(err)
	}
	capab.Identity.ID, capab.Identity.Name, capab.Identity.Transport = r.node, r.node, agenttr.Kind
	capab.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(capab)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = st.NodeAdd(ctx, store.NodeRecord{ID: r.node, Name: r.node, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	is, err := ca.IssueNode(ctx, pki.NodeRequest{CSR: csr, NodeID: r.node, OfferID: "offer-" + r.node})
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	state := t.TempDir()
	res := enroll.JoinResult{NodeID: r.node, CertDER: is.CertDER, Chain: is.Chain, RootDER: is.RootDER, KeyDER: keyDER,
		AgentURL: "https://" + net.JoinHostPort(host, strconv.Itoa(port)), NotAfter: is.NotAfter, RootFingerprint: pki.CertFingerprint(is.RootDER)}
	if err = enroll.SaveCredential(state, res, time.Now()); err != nil {
		t.Fatal(err)
	}
	fl.sh(t, "rm -rf "+g3AgentDir+"/pki; install -d -m 700 "+g3AgentDir+"/pki")
	for _, name := range []string{"root.pem", "client.key.sealed", "node.crt", "enrollment.json", "kek"} {
		fl.f.CopyTo(t, "n1", filepath.Join(state, "pki", name), g3AgentDir+"/pki/"+name)
	}
	fl.sh(t, "chmod 600 "+g3AgentDir+"/pki/* && chown -R nself:nself /home/nself/.nself")
	fl.sh(t, "su nself -c 'nohup "+g3AgentDir+"/bin/nself-ci-agent pull --slots 1 >/tmp/agent.log 2>&1 &'")
	defer fl.sh(t, "pkill -f '[n]self-ci-agent pull'; sleep 1; echo done")
	end := time.Now().Add(60 * time.Second)
	for len(srv.NodesConnected()) != 1 {
		if time.Now().After(end) {
			t.Fatalf("the node never connected\n%s", fl.sh(t, "cat /tmp/agent.log"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	r.rev = strings.Repeat("a", 40) // the pull driver ships no bundle: only the revision id is leased
	rctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return drv.Run(rctx, capab, r.Proto(), sink)
}
