package clientapi

// Purpose: the transport of the user API (P7-NODE-19, Epic D14): the TLS 1.3 config of the `--api-addr`
// listener and the http.Server both listeners (TCP and unix socket) run.
// Inputs:  the CA (for the root), the coordinator server leaf, the deny-list Guard.
// Outputs: *tls.Config, Serve.
// Constraints: TLS 1.3 only, session tickets off; a client certificate is optional at the handshake so a
//          token client can connect, but one that is presented must chain to the CA root, carry a kind
//          that matches the intermediate that signed it, and (for a user) not be on the deny list; a node
//          or coordinator certificate passes the handshake only so that the handler can refuse it with an
//          E664 body (node_kind); streams have no write timeout and end at Config.MaxLife.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// TLSConfig is the config of the user API listener. leaf is asked on every handshake, so the caller can
// renew the server certificate while the listener runs.
func TLSConfig(ca *pki.CA, leaf func() (tls.Certificate, error), g *pki.Guard) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(ca.Root())
	return &tls.Config{
		MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			c, err := leaf()
			return &c, err
		},
		ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: pool,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return nil // a token client
			}
			peer := cs.PeerCertificates[0]
			_, kind := pki.IdentityOf(peer)
			if len(cs.VerifiedChains) == 0 || len(cs.VerifiedChains[0]) < 3 || kind == "" ||
				cs.VerifiedChains[0][1].Subject.CommonName != pki.KindCN(kind) {
				return Failf(ReasonBadCertificate, "the client certificate does not chain through the intermediate of its kind")
			}
			if kind != pki.KindUser {
				return nil // refused with a body by the handler: a node is not a user
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return g.Check(ctx, peer)
		},
	}
}

// Serve runs h on ln until ctx ends, then drains for up to ten seconds. ln may be a TLS or a unix listener.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 32 << 10}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
	case err := <-errc:
		return err
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	_ = srv.Close()
	return nil
}
