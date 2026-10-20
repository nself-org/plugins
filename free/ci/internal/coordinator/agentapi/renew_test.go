package agentapi

// Purpose: tests of POST /v1/agent/renew (D-1372): a node renews its certificate over the agent listener, and a
//          job it runs survives the reconnect that follows on the renewed certificate.
// Inputs:  the rig with the real CA, store, lease engine and listener.
// Outputs: pass or fail.
// Constraints: fake synthetic values only; the renewal must not depend on a session (hello comes later).

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// renewal posts a CSR of a fresh key to /v1/agent/renew with the agent's current certificate.
func (a *agent) renewal(body []byte) (*http.Response, *ecdsa.PrivateKey) {
	a.r.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.r.t.Fatal(err)
	}
	if body == nil {
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
		if err != nil {
			a.r.t.Fatal(err)
		}
		body, _ = json.Marshal(map[string][]byte{"csr": csr})
	}
	resp, err := a.cl.Post(a.r.base+"/v1/agent/renew", "application/json", bytes.NewReader(body))
	if err != nil {
		a.r.t.Fatalf("renew: %v", err)
	}
	return resp, key
}

// renewed decodes a 200 renewal into a client certificate for key.
func renewed(t *testing.T, resp *http.Response, key *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("renew = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Cert  []byte   `json:"cert"`
		Chain [][]byte `json:"chain"`
		Root  []byte   `json:"root"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("renew body: %v", err)
	}
	leaf, err := x509.ParseCertificate(out.Cert)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: append(append([][]byte{}, out.Chain...), out.Root), PrivateKey: key, Leaf: leaf}
}

func TestRenewIssuesTheNextCertificateOfTheSameNode(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	resp, key := a.renewal(nil)
	next := renewed(t, resp, key)
	if id, kind := pki.IdentityOf(next.Leaf); id != "n-1" || kind != pki.KindNode {
		t.Fatalf("renewed identity = %q %q, want node n-1", id, kind)
	}
	if next.Leaf.SerialNumber.Cmp(a.cert.Leaf.SerialNumber) == 0 {
		t.Fatal("renewal returned the same serial")
	}
	// the renewed certificate is accepted by the listener on its own
	a.cl, a.cert = r.client(&next), next
	a.connect()
}

func TestAJobSurvivesARenewalAcrossAReconnect(t *testing.T) {
	rec := &admitRec{}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok-1")
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("first heartbeat answer %s", m.Type)
	}
	old := a.cert.Leaf.SerialNumber
	resp, key := a.renewal(nil)
	next := renewed(t, resp, key)
	// the connection drops; the agent reconnects with the renewed certificate and says it kept the lease
	a.cl, a.cert = r.client(&next), next
	a.connect()
	u = a.openStreamTokens(0, leaseID+"=tok-1")
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("heartbeat on the renewed certificate answered %s %+v", m.Type, m.Body)
	}
	if l, err := r.st.LeaseGet(t.Context(), leaseID); err != nil || l.State != model.JobState("running") {
		t.Fatalf("lease after renewal = %+v %v, want still running", l, err)
	}
	c := rec.last()
	if !c.Reconcile || c.LeaseID != leaseID || !strings.HasSuffix(strings.ToLower(next.Leaf.SerialNumber.Text(16)), strings.TrimLeft(c.Serial, "0")) || next.Leaf.SerialNumber.Cmp(old) == 0 {
		t.Fatalf("last call = %+v, want a reconcile on the renewed serial", c)
	}
}

func TestRenewRefusals(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	for name, body := range map[string][]byte{
		"not json":      []byte("{"),
		"unknown field": []byte(`{"csr":"AA==","node":"n-2"}`),
		"empty csr":     []byte(`{"csr":""}`),
		"garbage csr":   []byte(`{"csr":"AAAA"}`),
		"too big":       bytes.Repeat([]byte("a"), 1<<20),
	} {
		resp, _ := a.renewal(body)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: renew = %d %s, want 400", name, resp.StatusCode, raw)
		}
	}
	// a short-lived (ephemeral) lineage cannot renew
	r.addNode("n-eph")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	is, err := r.ca.IssueNode(t.Context(), pki.NodeRequest{CSR: csr, NodeID: "n-eph", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	eph := &agent{r: r, id: "n-eph", cl: r.client(&tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leaf})}
	resp, _ := eph.renewal(nil)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(raw), pki.ReasonRenewCap) {
		t.Fatalf("ephemeral renew = %d %s, want 403 %s", resp.StatusCode, raw, pki.ReasonRenewCap)
	}
}
