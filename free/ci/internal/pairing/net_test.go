package pairing

// Purpose: loopback TLS helpers of the pairing tests: a counting wrapper around the service for the
// enrollment listener, attacker material for the MITM cases, and a tiny mTLS server whose handshake
// outcome the tests read from the client side.
// Inputs:  a rig. Outputs: listener addresses and handshake errors. Constraints: 127.0.0.1 only.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

var loopback = []net.IP{net.IPv4(127, 0, 0, 1)}

// countingRedeemer counts what reaches the real service.
type countingRedeemer struct {
	svc            *Service
	hellos, redeem atomic.Int32
}

func (c *countingRedeemer) Hello() enroll.Hello { c.hellos.Add(1); return c.svc.Hello() }

func (c *countingRedeemer) Redeem(ctx context.Context, q enroll.Request, src string) (enroll.Response, error) {
	c.redeem.Add(1)
	return c.svc.Redeem(ctx, q, src)
}

// realSrv is the real enrollment handler of a rig behind a counter.
type realSrv struct {
	URL string
	Cnt *countingRedeemer
	Srv *enroll.Server
}

// serveReal binds the real enrollment handler of r.svc behind a counter.
func (r *rig) serveReal() realSrv {
	r.tb.Helper()
	leaf, err := r.ca.IssueServer("local", nil, loopback, 0)
	if err != nil {
		r.tb.Fatal(err)
	}
	cnt := &countingRedeemer{svc: r.svc}
	srv, err := enroll.Listen("127.0.0.1:0", r.ca.EnrollTLSConfig(leaf), enroll.Handler(cnt, enroll.HandlerOptions{Now: r.clock.Now}))
	if err != nil {
		r.tb.Fatal(err)
	}
	r.tb.Cleanup(srv.Close)
	return realSrv{URL: "https://" + srv.Addr().String() + enroll.Route, Cnt: cnt, Srv: srv}
}

// proxy serves cert as an enrollment listener and counts the HTTP requests it receives.
func proxy(tb testing.TB, cert tls.Certificate) (url string, hits *atomic.Int32) {
	tb.Helper()
	hits = &atomic.Int32{}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	srv, err := enroll.Listen("127.0.0.1:0", cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(srv.Close)
	return "https://" + srv.Addr().String() + enroll.Route, hits
}

// forgedLeaf is a coordinator-looking leaf signed by a key nobody trusts, issued in the name of the real
// coordinator intermediate and presented with the REAL intermediate and root.
func forgedLeaf(tb testing.TB, real tls.Certificate) tls.Certificate {
	tb.Helper()
	inter, err := x509.ParseCertificate(real.Certificate[1])
	if err != nil {
		tb.Fatal(err)
	}
	attacker, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	parent := &x509.Certificate{Subject: inter.Subject, RawSubject: inter.RawSubject, SubjectKeyId: inter.SubjectKeyId}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(77), Subject: pkix.Name{CommonName: "local"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: loopback,
		URIs: []*url.URL{pki.URI(pki.KindCoordinator, "local")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &leafKey.PublicKey, attacker)
	if err != nil {
		tb.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, real.Certificate[1], real.Certificate[2]}, PrivateKey: leafKey}
}

// okServer accepts connections with cfg; a connection that completes the handshake is told "ok".
func okServer(tb testing.TB, cfg *tls.Config) string {
	tb.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if c.(*tls.Conn).Handshake() == nil {
					_, _ = c.Write([]byte("ok"))
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// dialOK dials addr and returns nil only when the server completed its handshake and said "ok". TLS 1.3
// lets the client finish first, so a server-side rejection of the client certificate shows up in the read.
func dialOK(addr string, cfg *tls.Config) error {
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, cfg)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = io.ReadFull(c, make([]byte, 2))
	return err
}

// clientCfg is the agent-side config: pinned root, optional client certificate, the rig clock.
func (r *rig) clientCfg(cert *tls.Certificate) *tls.Config {
	r.tb.Helper()
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), Cert: cert, ServerName: "127.0.0.1", Now: r.clock.Now})
	if err != nil {
		r.tb.Fatal(err)
	}
	return cfg
}
