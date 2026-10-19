package enroll

// Purpose: the node-side checks of join that a hostile or buggy coordinator would hit: what finish()
// accepts as an issued certificate, what servedRoot() takes as a root, post() on odd replies, and the
// fingerprint confirmation rules.
// Inputs:  material from a real CA. Outputs: pass/fail. Constraints: loopback only, no sleeps.

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func TestEnrollFinishChecksTheIssuedCertificate(t *testing.T) {
	now := time.Now()
	ca := newCA(t, func() time.Time { return now })
	other := newCA(t, func() time.Time { return now })
	ctx := context.Background()
	key, csr := csrOf(t)
	iss, err := ca.IssueNode(ctx, pki.NodeRequest{CSR: csr, NodeID: "node-f"})
	if err != nil {
		t.Fatal(err)
	}
	root := ca.Root()
	good := Response{NodeID: "node-f", Cert: iss.CertDER, Chain: iss.Chain, Root: iss.RootDER, AgentURL: "https://a", NotAfter: iss.NotAfter.Unix()}
	opts := JoinOptions{Name: "Node-F", Now: func() time.Time { return now }}
	res, err := finish(opts, good, root, key, "SHA256:r", "SHA256:c")
	if err != nil || res.NodeID != "node-f" || res.AgentURL != "https://a" || len(res.KeyDER) == 0 || !res.NotAfter.Equal(iss.NotAfter) {
		t.Fatalf("a good answer: %+v %v", res, err)
	}
	otherKey, otherCSR := csrOf(t)
	userCert, err := ca.IssueUser(ctx, "node-f", 0)
	if err != nil {
		t.Fatal(err)
	}
	userDER := derOf(t, userCert.CertPEM)
	foreign, err := other.IssueNode(ctx, pki.NodeRequest{CSR: csr, NodeID: "node-f"})
	if err != nil {
		t.Fatal(err)
	}
	wrongName, err := ca.IssueNode(ctx, pki.NodeRequest{CSR: csr, NodeID: "someone-else"})
	if err != nil {
		t.Fatal(err)
	}
	otherNode, err := ca.IssueNode(ctx, pki.NodeRequest{CSR: otherCSR, NodeID: "node-f"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		mut  func(*Response) *ecdsa.PrivateKey
		now  time.Time
	}{
		{"the answer names another root", func(r *Response) *ecdsa.PrivateKey { r.Root = foreign.RootDER; return key }, now},
		{"the certificate is for another key", func(r *Response) *ecdsa.PrivateKey { return otherKey }, now},
		{"the certificate is another node's", func(r *Response) *ecdsa.PrivateKey {
			r.Cert, r.Chain = wrongName.CertDER, wrongName.Chain
			return key
		}, now},
		{"a user certificate", func(r *Response) *ecdsa.PrivateKey {
			r.Cert, r.Chain = userDER, [][]byte{userDER, ca.Root().Raw}
			return key
		}, now},
		{"the intermediate is missing", func(r *Response) *ecdsa.PrivateKey { r.Chain = r.Chain[:1]; return key }, now},
		{"an empty chain", func(r *Response) *ecdsa.PrivateKey { r.Chain = nil; return key }, now},
		{"a certificate from another CA under our root", func(r *Response) *ecdsa.PrivateKey {
			r.Cert, r.Chain = foreign.CertDER, foreign.Chain
			return key
		}, now},
		{"the certificate does not parse", func(r *Response) *ecdsa.PrivateKey { r.Cert = []byte("junk"); return key }, now},
		{"the certificate is expired", func(r *Response) *ecdsa.PrivateKey { return key }, now.Add(8 * 24 * time.Hour)},
		{"the certificate is not yet valid", func(r *Response) *ecdsa.PrivateKey { return key }, now.Add(-time.Hour)},
		{"the certificate belongs to another key of the same node", func(r *Response) *ecdsa.PrivateKey {
			r.Cert, r.Chain = otherNode.CertDER, otherNode.Chain
			return key
		}, now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := good
			k := tc.mut(&resp)
			o := opts
			o.Now = func() time.Time { return tc.now }
			_, err := finish(o, resp, root, k, "SHA256:r", "SHA256:c")
			if pki.ReasonOf(err) != pki.ReasonBadChain {
				t.Fatalf("want E660 bad_chain, got %v", err)
			}
		})
	}
}

func TestEnrollServedRootMustBeASelfSignedRoot(t *testing.T) {
	ca := newCA(t, nil)
	leaf, err := ca.IssueServer("local", nil, []net.IP{net.IPv4(127, 0, 0, 1)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	serve := func(chain [][]byte) string {
		l := leaf
		l.Certificate = chain
		srv, err := Listen("127.0.0.1:0", ca.EnrollTLSConfig(l), http.NotFoundHandler())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(srv.Close)
		return srv.Addr().String()
	}
	ctx := context.Background()
	root, err := servedRoot(ctx, serve(leaf.Certificate))
	if err != nil || root.Raw == nil || pki.CertFingerprint(root.Raw) != ca.RootFingerprint() {
		t.Fatalf("a full chain: %v", err)
	}
	for name, chain := range map[string][][]byte{
		"leaf only":             leaf.Certificate[:1],
		"leaf and intermediate": leaf.Certificate[:2],
	} {
		if _, err = servedRoot(ctx, serve(chain)); pki.ReasonOf(err) != pki.ReasonBadChain {
			t.Errorf("%s: want bad_chain, got %v", name, err)
		}
	}
	dead := freePort(t)
	if _, err = servedRoot(ctx, dead); err == nil {
		t.Error("a closed port gave a root")
	}
	plain, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	go func() {
		for {
			c, err := plain.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	if _, err = servedRoot(ctx, plain.Addr().String()); err == nil {
		t.Error("a non-TLS server gave a root")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func TestEnrollPostHandlesOddReplies(t *testing.T) {
	var reply func(w http.ResponseWriter)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reply(w) }))
	defer srv.Close()
	post1 := func() (Hello, error) {
		var h Hello
		err := post(context.Background(), srv.Client(), srv.URL, Request{Phase: PhaseHello, Version: Version}, &h)
		return h, err
	}
	reply = func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(Hello{Version: 1, CoordinatorID: "c", ServerTime: 5})
	}
	if h, err := post1(); err != nil || h.CoordinatorID != "c" {
		t.Fatalf("%+v %v", h, err)
	}
	reply = func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ErrorBody{Code: "E659", Reason: "locked", Message: "the code is locked"})
	}
	_, err := post1()
	var re *RemoteError
	if !errors.As(err, &re) || re.Code != "E659" || re.Reason != "locked" || re.ErrCode() != "E659" || re.ErrReason() != "locked" || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("a coded error body: %v", err)
	}
	for name, fn := range map[string]func(http.ResponseWriter){
		"html error page":   func(w http.ResponseWriter) { w.WriteHeader(502); _, _ = w.Write([]byte("<html>bad gateway</html>")) },
		"json without code": func(w http.ResponseWriter) { w.WriteHeader(500); _, _ = w.Write([]byte(`{"message":"x"}`)) },
		"200 with garbage":  func(w http.ResponseWriter) { _, _ = w.Write([]byte("garbage")) },
		"200 empty":         func(w http.ResponseWriter) {},
	} {
		reply = fn
		if _, err = post1(); err == nil {
			t.Errorf("%s: no error", name)
		} else if errors.As(err, &re) {
			t.Errorf("%s: reported as a coded remote error", name)
		}
	}
	reply = func(w http.ResponseWriter) { w.WriteHeader(500); _, _ = w.Write([]byte(strings.Repeat("x", 3<<20))) }
	if _, err = post1(); err == nil || len(err.Error()) > 200 {
		t.Errorf("an oversized error body: %v", err)
	}
}

func TestEnrollConfirmRules(t *testing.T) {
	const fp = "SHA256:abc"
	asked := 0
	yes := func(string) (bool, error) { asked++; return true, nil }
	for _, tc := range []struct {
		name   string
		o      JoinOptions
		reason string
		asked  int
	}{
		{"expect equal", JoinOptions{ExpectFingerprint: fp}, "", 0},
		{"expect equal wins over a refusing prompt", JoinOptions{ExpectFingerprint: fp, Confirm: func(string) (bool, error) { asked++; return false, nil }}, "", 0},
		{"expect different, a yes prompt does not override it", JoinOptions{ExpectFingerprint: "SHA256:xyz", Confirm: yes}, pki.ReasonFingerprintMismatch, 0},
		{"no flag, no prompt", JoinOptions{}, pki.ReasonFingerprintUnconfirmed, 0},
		{"prompt says yes", JoinOptions{Confirm: yes}, "", 1},
		{"prompt says no", JoinOptions{Confirm: func(string) (bool, error) { asked++; return false, nil }}, pki.ReasonFingerprintUnconfirmed, 1},
		{"prompt fails", JoinOptions{Confirm: func(string) (bool, error) { asked++; return true, errors.New("EOF") }}, pki.ReasonFingerprintUnconfirmed, 1},
	} {
		asked = 0
		err := confirm(tc.o, fp)
		if pki.ReasonOf(err) != tc.reason || asked != tc.asked {
			t.Errorf("%s: err %v asked %d", tc.name, err, asked)
		}
		if err != nil && !strings.Contains(err.Error(), "E660") {
			t.Errorf("%s: not E660: %v", tc.name, err)
		}
	}
}

func TestEnrollRequestWireShape(t *testing.T) {
	b, _ := json.Marshal(Request{Phase: PhaseHello, Version: 1, ClientTime: 7})
	if string(b) != `{"phase":"hello","version":1,"client_time":7}` {
		t.Fatalf("a hello carries nothing else, in particular no code: %s", b)
	}
	b, _ = json.Marshal(Request{Phase: PhaseEnroll, Version: 1, ClientTime: 7, Code: "C", CSR: []byte{1}, NodeKey: []byte{2}, Name: "n"})
	if !strings.Contains(string(b), `"code":"C"`) || !strings.Contains(string(b), `"csr":"AQ=="`) {
		t.Fatalf("%s", b)
	}
}
