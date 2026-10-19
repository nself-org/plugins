package agentapi

// Purpose: the coordinator server certificate of the agent listener, renewed before it expires.
// Inputs:  the CA, the coordinator id and the DNS/IP SANs agents dial (from --public-url).
// Outputs: Get returns the current leaf, issuing a new one at 2/3 of its life (pki.RenewAt).
// Constraints: the key lives in memory only; a listener that runs longer than the certificate life keeps
//          serving a valid certificate; Get is safe for concurrent handshakes.

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// LeafSource issues and renews the server certificate.
type LeafSource struct {
	CA  *pki.CA
	ID  string
	DNS []string
	IPs []net.IP
	TTL time.Duration // 0: the PKI default

	mu  sync.Mutex
	cur tls.Certificate
}

// Get returns the current server certificate.
func (l *LeafSource) Get() (tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur.Leaf == nil || !l.CA.Now().Before(pki.RenewAt(l.cur.Leaf)) {
		c, err := l.CA.IssueServer(l.ID, l.DNS, l.IPs, l.TTL)
		if err != nil {
			return tls.Certificate{}, err
		}
		l.cur = c
	}
	return l.cur, nil
}
