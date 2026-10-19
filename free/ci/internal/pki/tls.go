package pki

// Purpose: the TLS configs every listener and client uses (Epic D5, review F3 and F4).
// Inputs:  the CA, a server leaf, a Guard; for clients the pinned root and the client certificate.
// Outputs: TLSServerConfig(kind) for mTLS listeners, EnrollTLSConfig for the enrollment listener (no
//          client certificate), TLSClientConfig for nodes and user clients.
// Constraints: TLS 1.3 only; SessionTicketsDisabled on every server config; the server checks kind,
//          the exact intermediate, the deny list and node state in VerifyConnection, which runs on every
//          handshake, resumed or not; a client accepts only a coordinator-kind leaf chained to the root
//          it pinned, so a node or user leaf served as a server is refused (kind check and EKU).

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"
)

func (c *CA) baseServer(leaf tls.Certificate) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf}, SessionTicketsDisabled: true, Time: c.now}
}

// EnrollTLSConfig is the enrollment listener's config: TLS 1.3, the coordinator leaf, no client certificate.
func (c *CA) EnrollTLSConfig(leaf tls.Certificate) *tls.Config {
	cfg := c.baseServer(leaf)
	cfg.ClientAuth = tls.NoClientCert
	return cfg
}

// TLSServerConfig is the mTLS listener config for clients of one kind (node or user).
func (c *CA) TLSServerConfig(kind Kind, leaf tls.Certificate, g *Guard) (*tls.Config, error) {
	inter, ok := c.inter[kind]
	if !ok || kind == KindCoordinator {
		return nil, fail(ReasonWrongKind, "a server accepts node or user clients, not %q", kind)
	}
	pool := x509.NewCertPool()
	pool.AddCert(inter.cert)
	cfg := c.baseServer(leaf)
	cfg.ClientAuth, cfg.ClientCAs = tls.RequireAndVerifyClientCert, pool
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fail(ReasonNoCert, "no client certificate")
		}
		leaf := cs.PeerCertificates[0]
		if _, k := IdentityOf(leaf); k != kind {
			return fail(ReasonWrongKind, "client certificate is not of kind %s", kind)
		}
		if len(cs.VerifiedChains) == 0 || !bytes.Equal(cs.VerifiedChains[0][len(cs.VerifiedChains[0])-1].Raw, inter.cert.Raw) {
			return fail(ReasonBadChain, "client certificate does not chain to the %s intermediate", kind)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return g.Check(ctx, leaf)
	}
	return cfg, nil
}

// ClientOptions configure TLSClientConfig.
type ClientOptions struct {
	// RootDER is the pinned root certificate.
	RootDER []byte
	// Cert is the client certificate (nil for the enrollment connection).
	Cert *tls.Certificate
	// ServerName must match a SAN of the server leaf (a DNS name or an IP).
	ServerName string
	// Now is the clock (default time.Now).
	Now func() time.Time
}

// TLSClientConfig verifies the server against the pinned root and requires a coordinator-kind leaf.
func TLSClientConfig(o ClientOptions) (*tls.Config, error) {
	root, err := x509.ParseCertificate(o.RootDER)
	if err != nil {
		return nil, fail(ReasonBadChain, "pinned root does not parse")
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: o.ServerName, Time: o.Now}
	if o.Cert != nil {
		cfg.Certificates = []tls.Certificate{*o.Cert}
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || len(cs.VerifiedChains) == 0 {
			return fail(ReasonNoCert, "server presented no certificate")
		}
		_, k := IdentityOf(cs.PeerCertificates[0])
		chain := cs.VerifiedChains[0]
		if k != KindCoordinator || len(chain) < 3 || chain[1].Subject.CommonName != KindCN(KindCoordinator) {
			return fail(ReasonWrongKind, "server certificate is not a coordinator certificate")
		}
		return nil
	}
	return cfg, nil
}
