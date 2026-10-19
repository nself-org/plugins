package enroll

// Purpose: fixtures of the enroll tests: a real pki.CA over a real store, and a throwaway TLS server
// config. Inputs: testing.TB. Outputs: helpers only. Constraints: files under t.TempDir().

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func newCA(tb testing.TB, now func() time.Time) *pki.CA {
	tb.Helper()
	home := filepath.Join(tb.TempDir(), "ci")
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: home, Now: now})
	if err != nil {
		tb.Fatal(err)
	}
	return ca
}

// csrOf returns a fresh P-256 key and a CSR for it.
func csrOf(tb testing.TB) (*ecdsa.PrivateKey, []byte) {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		tb.Fatal(err)
	}
	return key, csr
}

func testServerTLS(tb testing.TB) *tls.Config {
	tb.Helper()
	ca := newCA(tb, nil)
	leaf, err := ca.IssueServer("local", nil, []net.IP{net.IPv4(127, 0, 0, 1)}, 0)
	if err != nil {
		tb.Fatal(err)
	}
	return ca.EnrollTLSConfig(leaf)
}

func derOf(tb testing.TB, pemBytes []byte) []byte {
	tb.Helper()
	b, _ := pem.Decode(pemBytes)
	if b == nil {
		tb.Fatal("not PEM")
	}
	return b.Bytes
}
