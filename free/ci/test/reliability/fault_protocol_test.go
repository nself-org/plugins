//go:build integration

package reliability

// Purpose: an agent that offers an incompatible protocol. Its hello is refused (E652), so it never has a session
//          and never receives a lease; the scheduler does not place on it, and `nself ci explain` names the
//          protocol reason (node.protocol) for it.

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
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// nodeCert issues a node certificate for id from the lab's CA.
func (l *lab) nodeCert(id string) tls.Certificate {
	l.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		l.t.Fatal(err)
	}
	is, err := l.c.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		l.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	return tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leaf}
}

func TestFaultIncompatibleAgent(t *testing.T) {
	l := newLab(t, fast, 2)
	v2 := "n3-v2-" + l.f.ID
	l.registerNode(v2, func(c *model.Capability) {
		c.Protocol = model.CapabilityProtocol{Versions: []int{2}, Min: 2, AgentVersion: "2.0.0"}
	})

	// the handshake: an agent that speaks only protocol 2 is refused with E652 and has no session
	cert := l.nodeCert(v2)
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: l.c.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	car := agent.NewCarrier(agent.CarrierConfig{Client: agent.Options{BaseURL: "https://127.0.0.1:" + itoa(l.c.port), TLS: tlsc, Wait: time.Second}, Logf: t.Logf})
	t.Cleanup(func() { _ = car.Close() })
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	hello := protocol.Message{V: 1, Type: "hello", Seq: 1, Body: protocol.Hello{Versions: []int{2}, AgentVersion: "2.0.0",
		NodeKey:          protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(pub), Public: base64.StdEncoding.EncodeToString(pub)},
		CapabilityDigest: "sha256:00", Identity: protocol.Identity{MachineID: "m-v2", HostKeyFingerprints: []string{}}, CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 1}}}
	if err = car.Send(hello); err != nil {
		t.Fatal(err)
	}
	got := make(chan protocol.Message, 1)
	go func() {
		m, rerr := car.Recv()
		if rerr == nil {
			got <- m
		}
	}()
	select {
	case m := <-got:
		rej, ok := m.Body.(*protocol.Reject)
		if !ok || rej.Code != protocol.CodeVersion || rej.Reason != "no_common_version" {
			t.Fatalf("the answer to an incompatible hello is %T %+v, want E652 no_common_version", m.Body, m.Body)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the incompatible hello was not answered")
	}
	for _, n := range l.c.connected() {
		if n == v2 {
			t.Fatalf("the incompatible agent has a session: %v", l.c.connected())
		}
	}

	// placement: the job runs on a compatible node, and the decision names the protocol reason for the other
	h := l.start(jobSpec{iter: 10, idempotent: true, infraMax: 1})
	res := h.result(2 * time.Minute)
	if res.Exit != 0 {
		t.Fatalf("exit %d\n%s", res.Exit, h.out)
	}
	atts := h.attempts()
	if len(atts) != 1 {
		t.Fatalf("attempts: %+v", atts)
	}
	doc := h.decision(atts[0].ID)
	if !strings.Contains(doc, "node.protocol") || !strings.Contains(doc, v2) {
		t.Fatalf("the placement decision does not name node.protocol for %s:\n%s", v2, doc)
	}
	if n := l.fleetSideEffects(); n != 1 {
		t.Fatalf("the side effect was written %d times", n)
	}

	// `nself ci explain <run>/<job>` prints the same reason
	cmd := exec.Command(ciBinary(t), "explain", res.RunID+"/unit", "--json")
	cmd.Env = append(os.Environ(), "NSELF_CI_HOME="+l.c.home, "NSELF_V15=1", "NSELF_CI_REPO="+l.root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "node.protocol") {
		t.Fatalf("`ci explain` does not name node.protocol:\n%s", out)
	}
}
