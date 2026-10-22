package agentapi

// Purpose: TestVerifyPeer: the real listener refuses a client without a certificate, a revoked node
//          certificate and a user-kind certificate, and accepts a valid node certificate.
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: the real TLS listener and the real CA; no mocks of the verification.

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestVerifyPeer(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-valid")
	revoked := r.newAgent("n-revoked")
	revoked.connect()
	if _, err := r.ca.Revoke(t.Context(), "cli:test", "n-revoked", "stolen"); err != nil {
		t.Fatal(err)
	}
	user := r.userCert("u-1")
	coordinator, err := r.ca.IssueServer("coord-2", nil, loopbackIPs, 0) // a coordinator-kind leaf used as a client
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		client *http.Client
		wantOK bool
	}{
		{"no certificate", r.client(nil), false},
		{"revoked node certificate (new connection)", r.client(&revoked.cert), false},
		{"user-kind certificate", r.client(&user), false},
		{"coordinator-kind certificate", r.client(&coordinator), false},
		{"valid node certificate", a.cl, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ag := &agent{r: r, id: "n-valid", cl: c.client, pub: a.pub}
			resp, err := tryHello(ag)
			if c.wantOK {
				if err != nil || resp != 200 {
					t.Fatalf("valid node certificate: status %d err %v", resp, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("handshake or request succeeded with status %d, want a refusal", resp)
			}
		})
	}
	t.Run("TLS 1.2 is refused", func(t *testing.T) {
		cfg := r.client(nil)
		tr := cfg.Transport.(*http.Transport)
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
		tr.TLSClientConfig.MaxVersion = tls.VersionTLS12
		if _, err := cfg.Get(r.base + "/v1/agent/leases"); err == nil {
			t.Fatal("a TLS 1.2 client was served")
		}
	})
}

// tryHello posts a hello and returns the status code, or the transport error (a refused handshake
// surfaces on the first read in TLS 1.3).
func tryHello(a *agent) (int, error) {
	resp, err := a.helloErr(nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}
