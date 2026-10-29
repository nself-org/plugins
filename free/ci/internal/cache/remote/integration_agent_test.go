//go:build integration

package remote

// Purpose: carrier B (agent pull) of the simulation: the real agent driver and
// agentapi listener (TLS 1.3 mTLS, token gate, lease engine) on the coordinator,
// a real agent session over the real pull carrier on the node.

import (
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
	"net"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
	agenttr "github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

type nodeKey struct{ pub ed25519.PublicKey }

func (k nodeKey) KeyID() string        { return pki.NodeKeyID(k.pub) }
func (k nodeKey) PublicString() string { return base64.StdEncoding.EncodeToString(k.pub) }

// agentSide is the coordinator of carrier B around the rig's store and lease engine.
type agentSide struct {
	r    *rig
	ca   *pki.CA
	drv  *agenttr.Driver
	base string
	pub  ed25519.PublicKey
}

func (r *rig) agentSide() *agentSide {
	r.t.Helper()
	ctx := context.Background()
	dir := r.t.TempDir()
	ca, err := pki.New(ctx, pki.Options{Store: r.st, DataDir: dir, Now: time.Now})
	if err != nil {
		r.t.Fatal(err)
	}
	tokens, err := ca.Tokens(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	drv := &agenttr.Driver{Gate: &agenttr.Gate{Tokens: tokens, Bindings: r.st, Leases: r.st}, Minter: tokens, Leases: r.st, LogDir: r.t.TempDir(), Poll: 30 * time.Millisecond, CancelGrace: 2 * time.Second}
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord-1", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(r.st), Cert: leaf.Get, Leases: r.eng, Sink: drv.Sink(), Source: drv, Store: r.st,
		Admit: drv.Admit, Blobs: drv.Blobs(), PollTimeout: 2 * time.Second, SourcePoll: 20 * time.Millisecond, GuardEvery: 50 * time.Millisecond})
	if err != nil {
		r.t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		r.t.Fatal(err)
	}
	drv.SetCancel(srv.Cancel)
	drv.SetPush(srv.Push)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	r.t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
		<-done
	})
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	return &agentSide{r: r, ca: ca, drv: drv, base: "https://" + ln.Addr().String(), pub: pub}
}

// addNode registers the node of the lease's runner id and returns its capability and TLS certificate.
func (a *agentSide) addNode(id string) (model.Capability, tls.Certificate) {
	a.r.t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		a.r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		a.r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = id, id, agenttr.Kind
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = a.r.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		a.r.t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		a.r.t.Fatal(err)
	}
	is, err := a.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		a.r.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return c, tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leaf}
}

// runAgent delivers the rig's lease to a node over the agent pull carrier and returns the driver's result.
func (r *rig) runAgent(n nodeSide) error {
	r.t.Helper()
	side := r.agentSide()
	capab, cert := side.addNode("n-1")
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: side.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	if err != nil {
		r.t.Fatal(err)
	}
	car := agenttr.NewCarrier(agenttr.CarrierConfig{Client: agenttr.Options{BaseURL: side.base, TLS: tlsc, Wait: time.Second},
		Backoff: agenttr.Backoff{Min: 10 * time.Millisecond, Max: 50 * time.Millisecond}})
	r.t.Cleanup(func() { _ = car.Close() })
	state := r.t.TempDir()
	table, err := slots.Open(state, 2)
	if err != nil {
		r.t.Fatal(err)
	}
	s, err := session.New(session.Config{Carrier: n.tap(car), Key: nodeKey{side.pub}, Slots: table, Runner: n.runner(), StateDir: state, Hooks: n.hooks(),
		Coordinator: session.Coordinator{ID: "coord-1", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m-1", CapabilityDigest: "sha256:00"}})
	if err != nil {
		r.t.Fatal(err)
	}
	limit := 60 * time.Second
	if r.deadline > 0 {
		limit = r.deadline
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	sessDone := make(chan error, 1)
	go func() { sessDone <- s.Run(ctx) }()
	defer func() { cancel(); <-sessDone }()
	return side.drv.Run(ctx, capab, r.protoLease(), r.sink())
}
