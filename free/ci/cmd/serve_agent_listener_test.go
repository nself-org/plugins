package main

// Purpose: tests of the agent listener wiring: the flag registration, the certificate names, the refusing
//          default sink, and a real `nodes revoke` (lifecycle revoker hooks plus the session closer this
//          file registers) ending a live long-poll of the node within 1 s on the listener `ci serve` starts.
// Inputs:  a temp store and data dir handed over with useAgentStore. Outputs: pass or fail.
// Constraints: the real listener, CA and lifecycle; synthetic ids only; the listener is stopped and the
//          store source restored by t.Cleanup.

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
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestAgentListenerIsRegistered(t *testing.T) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	coordinator.AddFlags(fs)
	f := fs.Lookup("agent-addr")
	if f == nil || f.DefValue != ":3864" {
		t.Fatalf("--agent-addr = %+v, want default :3864", f)
	}
}

func TestAgentSANs(t *testing.T) {
	cases := []struct {
		name, public, bind string
		wantDNS            []string
		wantIPs            []string
		wantErr            bool
	}{
		{"default bind", "", ":3864", []string{"localhost"}, []string{"127.0.0.1", "::1"}, false},
		{"public name", "https://ci.example.test:3864", ":3864", []string{"ci.example.test", "localhost"}, []string{"127.0.0.1", "::1"}, false},
		{"public ip", "https://192.0.2.7", "0.0.0.0:3864", []string{"localhost"}, []string{"192.0.2.7", "127.0.0.1", "::1"}, false},
		{"bind host", "", "ci.example.test:3864", []string{"ci.example.test", "localhost"}, []string{"127.0.0.1", "::1"}, false},
		{"public url without host", "not a url", ":3864", nil, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dns, ips, err := agentSANs(c.public, c.bind)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v", err)
			}
			var got []string
			for _, ip := range ips {
				got = append(got, ip.String())
			}
			if !c.wantErr && (!reflect.DeepEqual(dns, c.wantDNS) || !reflect.DeepEqual(got, c.wantIPs)) {
				t.Fatalf("dns %v ips %v, want %v %v", dns, got, c.wantDNS, c.wantIPs)
			}
		})
	}
}

func TestRefusingSinkRefusesEverything(t *testing.T) {
	var s refusingSink
	ctx := context.Background()
	if s.Log(ctx, protocol.Log{}) == nil || s.Result(ctx, protocol.Result{}) == nil || s.ArtifactRef(ctx, protocol.ArtifactRef{}) == nil || s.CacheRef(ctx, protocol.CacheRef{}) == nil {
		t.Fatal("the default sink accepted agent output")
	}
}

func TestAgentListenerStartRevokeStop(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	prev := agentStoreSource
	t.Cleanup(func() { agentStoreSource = prev })
	useAgentStore(st, dir, "")
	ctx := context.Background()
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	addAgentTestNode(t, st, "n-1")
	cert := agentTestNodeCert(t, ca, "n-1")

	stop, err := startAgentListener(ctx, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = stop(ctx)
		}
	})
	srv := activeAgent.Load()
	if srv == nil {
		t.Fatal("no active agent server")
	}
	var addr net.Addr
	waitFor(t, "the listener to bind", func() bool { addr = srv.Addr(); return addr != nil })

	client := func(c *tls.Certificate) *http.Client {
		cfg, cerr := pki.TLSClientConfig(pki.ClientOptions{RootDER: ca.RootDER(), Cert: c, ServerName: "127.0.0.1"})
		if cerr != nil {
			t.Fatal(cerr)
		}
		tr := &http.Transport{TLSClientConfig: cfg, ForceAttemptHTTP2: true}
		t.Cleanup(tr.CloseIdleConnections)
		return &http.Client{Transport: tr, Timeout: 10 * time.Second}
	}
	base := "https://" + addr.String()

	user, err := ca.IssueUser(ctx, "u-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	userPair, err := tls.X509KeyPair(append(append([]byte{}, user.CertPEM...), user.RootPEM...), user.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*tls.Certificate{"no certificate": nil, "user certificate": &userPair} {
		if resp, rerr := client(c).Get(base + "/v1/agent/leases"); rerr == nil {
			resp.Body.Close()
			t.Fatalf("%s was served with status %d", name, resp.StatusCode)
		}
	}

	cl := client(&cert)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	hello := protocol.Hello{Versions: []int{1}, AgentVersion: "1.5.0", NodeKey: protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(pub), Public: base64.StdEncoding.EncodeToString(pub)},
		CapabilityDigest: "sha256:00", Identity: protocol.Identity{MachineID: "m-1", HostKeyFingerprints: []string{}}, CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 1}}
	var body bytes.Buffer
	if err = protocol.NewEncoder(&body).Encode(protocol.Message{V: 1, Type: "hello", Seq: 1, Body: hello}); err != nil {
		t.Fatal(err)
	}
	resp, err := cl.Post(base+"/v1/agent/hello", "application/x-ndjson", &body)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("hello: %v %v", resp, err)
	}
	session := resp.Header.Get(agentapi.HeaderSession)
	resp.Body.Close()
	if session == "" {
		t.Fatal("no session")
	}

	polled := make(chan time.Time, 1)
	go func() {
		rq, _ := http.NewRequest(http.MethodGet, base+"/v1/agent/leases", nil)
		rq.Header.Set(agentapi.HeaderSession, session)
		rq.Header.Set(agentapi.HeaderWait, "30")
		if r, derr := cl.Do(rq); derr == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			r.Body.Close()
		}
		polled <- time.Now()
	}()
	waitFor(t, "the long-poll to be live", func() bool { return len(srv.NodesConnected()) == 1 })
	time.Sleep(100 * time.Millisecond) // the request is registered a moment after the session

	start := time.Now()
	d := lifecycle.NewDeps(st, dir, "proj-1", "cli:test")
	if _, err = d.Revoke(ctx, "n-1"); err != nil {
		t.Fatalf("nodes revoke: %v", err)
	}
	select {
	case at := <-polled:
		if took := at.Sub(start); took > time.Second {
			t.Fatalf("long-poll ended %v after the revoke, want < 1 s", took)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the long-poll of a revoked node is still open 3 s after the revoke")
	}
	if got := srv.NodesConnected(); len(got) != 0 {
		t.Fatalf("NodesConnected after the revoke = %v", got)
	}

	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = stop(sctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	stopped = true
	if activeAgent.Load() != nil {
		t.Fatal("the stopped server is still the active one")
	}
	if c, derr := net.DialTimeout("tcp", addr.String(), 500*time.Millisecond); derr == nil {
		c.Close()
		t.Fatal("the agent port still accepts connections after stop")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func addAgentTestNode(t *testing.T, st *store.Store, id string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "internal", "model", "testdata", "capability", "valid", "laptop.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		t.Fatal(err)
	}
}

func agentTestNodeCert(t *testing.T, ca *pki.CA, id string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	is, err := ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	return tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leaf}
}
