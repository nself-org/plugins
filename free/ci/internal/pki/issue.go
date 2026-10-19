package pki

// Purpose: leaf issuance (Epic D5): node certificates from a CSR, user bundles, the coordinator server
//          leaf, renewal at 2/3 of life and revocation into the deny list.
// Inputs:  a CSR (DER), a node or user id, a TTL; for the server leaf the SANs of --public-url.
// Outputs: Issued{leaf, chain, root, serial}; Bundle{cert, key, root PEM}; deny-list rows.
// Constraints: leaf URI SAN is nself-ci:<kind>/<id>; node and user leaves are ClientAuth, the server
//          leaf ServerAuth; a CSR key must be ECDSA P-256 with a valid self-signature (its subject and
//          extensions are ignored, the coordinator writes the identity); TTL 0 is 7 days, above 7 days
//          or below 1 minute is E660 cert_ttl; a lineage issued with a TTL below 7 days is never
//          renewed; the server leaf key never leaves memory.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"net"
	"net/url"
	"regexp"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Certificate lifetimes (Epic D5).
const (
	DefaultTTL = 7 * 24 * time.Hour
	MinTTL     = time.Minute
)

// ClockSkew is the tolerated difference between node and coordinator clocks.
const ClockSkew = 120 * time.Second

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// ValidID reports whether id may be a node, user or coordinator id (it ends up in a URI SAN).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// URI is the SAN of a leaf: nself-ci:<kind>/<id>.
func URI(k Kind, id string) *url.URL {
	return &url.URL{Scheme: "nself-ci", Opaque: string(k) + "/" + id}
}

// Issued is a signed leaf with its chain.
type Issued struct {
	Serial   string
	CertDER  []byte
	Chain    [][]byte // leaf, intermediate
	RootDER  []byte
	NotAfter time.Time
}

// Bundle is a user credential: certificate, private key and root, PEM. The key is returned once and
// never stored by the coordinator.
type Bundle struct{ CertPEM, KeyPEM, RootPEM []byte }

// CheckTTL maps 0 to the default and refuses values outside [MinTTL, DefaultTTL].
func CheckTTL(ttl time.Duration) (time.Duration, error) {
	if ttl == 0 {
		return DefaultTTL, nil
	}
	if ttl < MinTTL || ttl > DefaultTTL {
		return 0, fail(ReasonCertTTL, "certificate lifetime %s is outside %s..%s", ttl, MinTTL, DefaultTTL)
	}
	return ttl, nil
}

func (c *CA) sign(kind Kind, id string, pub *ecdsa.PublicKey, eku x509.ExtKeyUsage, ttl time.Duration, sans *x509.Certificate) (*x509.Certificate, []byte, error) {
	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}
	now := c.now()
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: id}, NotBefore: now.Add(-backdate), NotAfter: now.Add(ttl),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku}, BasicConstraintsValid: true,
		URIs: []*url.URL{URI(kind, id)},
	}
	if sans != nil {
		tmpl.DNSNames, tmpl.IPAddresses = sans.DNSNames, sans.IPAddresses
	}
	signer := c.inter[kind]
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer.cert, pub, signer.key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, der, err
}

func csrKey(csrDER []byte) (*ecdsa.PublicKey, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, fail(ReasonBadCSR, "certificate request does not parse")
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, fail(ReasonBadCSR, "certificate request signature is invalid")
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, fail(ReasonBadCSR, "certificate request key must be ECDSA P-256")
	}
	return pub, nil
}

// CSRKeyFingerprint validates a CSR and returns the fingerprint of its public key (what the node prints).
func CSRKeyFingerprint(csrDER []byte) (string, error) {
	pub, err := csrKey(csrDER)
	if err != nil {
		return "", err
	}
	return KeyFingerprint(pub)
}

// NodeRequest is one node-certificate issuance.
type NodeRequest struct {
	CSR     []byte
	NodeID  string
	TTL     time.Duration
	OfferID string
}

// IssueNode signs a node certificate for the CSR key and records it.
func (c *CA) IssueNode(ctx context.Context, r NodeRequest) (*Issued, error) {
	if !ValidID(r.NodeID) {
		return nil, fail(ReasonBadCSR, "node id %q is not a valid identifier", r.NodeID)
	}
	ttl, err := CheckTTL(r.TTL)
	if err != nil {
		return nil, err
	}
	pub, err := csrKey(r.CSR)
	if err != nil {
		return nil, err
	}
	cert, der, err := c.sign(KindNode, r.NodeID, pub, x509.ExtKeyUsageClientAuth, ttl, nil)
	if err != nil {
		return nil, err
	}
	var capAfter int64
	if ttl < DefaultTTL { // an ephemeral lineage ends at its first expiry
		capAfter = cert.NotAfter.UnixNano()
	}
	row := store.PKICert{Serial: serialHex(cert), Kind: string(KindNode), NodeID: r.NodeID, Subject: cert.URIs[0].String(), OfferID: r.OfferID, NotBefore: cert.NotBefore.UnixNano(), NotAfter: cert.NotAfter.UnixNano(), CapAfter: capAfter}
	if err = c.st.PKICertPut(ctx, row); err != nil {
		return nil, err
	}
	return c.issued(cert, der, KindNode), nil
}

func (c *CA) issued(cert *x509.Certificate, der []byte, k Kind) *Issued {
	return &Issued{Serial: serialHex(cert), CertDER: der, Chain: [][]byte{der, c.inter[k].cert.Raw}, RootDER: c.root.cert.Raw, NotAfter: cert.NotAfter}
}

func serialHex(c *x509.Certificate) string { return hex.EncodeToString(c.SerialNumber.Bytes()) }

// IssueUser creates a user key pair and a ClientAuth certificate chained to the user intermediate.
func (c *CA) IssueUser(ctx context.Context, userID string, ttl time.Duration) (Bundle, error) {
	if !ValidID(userID) {
		return Bundle{}, fail(ReasonBadCSR, "user id %q is not a valid identifier", userID)
	}
	ttl, err := CheckTTL(ttl)
	if err != nil {
		return Bundle{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Bundle{}, err
	}
	cert, der, err := c.sign(KindUser, userID, &key.PublicKey, x509.ExtKeyUsageClientAuth, ttl, nil)
	if err != nil {
		return Bundle{}, err
	}
	row := store.PKICert{Serial: serialHex(cert), Kind: string(KindUser), NodeID: userID, Subject: cert.URIs[0].String(), NotBefore: cert.NotBefore.UnixNano(), NotAfter: cert.NotAfter.UnixNano()}
	if err = c.st.PKICertPut(ctx, row); err != nil {
		return Bundle{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return Bundle{}, err
	}
	chain := append(pemBlock("CERTIFICATE", der), pemBlock("CERTIFICATE", c.inter[KindUser].cert.Raw)...)
	return Bundle{CertPEM: chain, KeyPEM: pemBlock("PRIVATE KEY", keyDER), RootPEM: pemBlock("CERTIFICATE", c.root.cert.Raw)}, nil
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

// SANsFromURL returns the DNS and IP SANs for a public URL (host part only).
func SANsFromURL(publicURL string) (dns []string, ips []net.IP, err error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return nil, nil, fail(ReasonBadChain, "public URL %q has no host", publicURL)
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		return nil, []net.IP{ip}, nil
	}
	return []string{u.Hostname()}, nil, nil
}

// IssueServer makes the coordinator server leaf for one listener run. The key lives in memory only;
// the chain sent is leaf, coordinator intermediate, root (the joining node reads the root from it).
func (c *CA) IssueServer(coordinatorID string, dns []string, ips []net.IP, ttl time.Duration) (tls.Certificate, error) {
	if !ValidID(coordinatorID) {
		return tls.Certificate{}, fail(ReasonBadCSR, "coordinator id %q is not a valid identifier", coordinatorID)
	}
	ttl, err := CheckTTL(ttl)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, der, err := c.sign(KindCoordinator, coordinatorID, &key.PublicKey, x509.ExtKeyUsageServerAuth, ttl, &x509.Certificate{DNSNames: dns, IPAddresses: ips})
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, c.inter[KindCoordinator].cert.Raw, c.root.cert.Raw}, PrivateKey: key, Leaf: cert}, nil
}

// RenewAt is when a node should renew: 2/3 of the certificate's life.
func RenewAt(cert *x509.Certificate) time.Time {
	return cert.NotBefore.Add(cert.NotAfter.Sub(cert.NotBefore) * 2 / 3)
}

// Renew issues the next node certificate for the holder of a certificate that already passed the
// handshake checks. An expired or denied certificate, or one at the end of an ephemeral lineage,
// cannot renew (the node must pair again).
func (c *CA) Renew(ctx context.Context, peer *x509.Certificate, csrDER []byte) (*Issued, error) {
	id, kind := IdentityOf(peer)
	if kind != KindNode {
		return nil, fail(ReasonWrongKind, "only a node certificate renews")
	}
	now := c.now()
	if !now.Before(peer.NotAfter) {
		return nil, fail(ReasonExpired, "certificate of %s expired; pair the node again", id)
	}
	row, err := c.st.PKICertGet(ctx, serialHex(peer))
	if err != nil {
		return nil, fail(ReasonUnknownNode, "certificate of %s is not one of ours", id)
	}
	if denied, derr := c.st.PKIIsDenied(ctx, row.Serial); derr != nil || denied {
		return nil, fail(ReasonRevoked, "certificate of %s is revoked", id)
	}
	if row.CapAfter > 0 {
		return nil, fail(ReasonRenewCap, "certificate of %s is short-lived and cannot be renewed", id)
	}
	return c.IssueNode(ctx, NodeRequest{CSR: csrDER, NodeID: id, OfferID: row.OfferID})
}

// IdentityOf reads the id and kind from a leaf's nself-ci URI SAN ("", "" when it has none).
func IdentityOf(cert *x509.Certificate) (id string, kind Kind) {
	for _, u := range cert.URIs {
		if u.Scheme != "nself-ci" {
			continue
		}
		for _, k := range []Kind{KindNode, KindUser, KindCoordinator} {
			if p := string(k) + "/"; len(u.Opaque) > len(p) && u.Opaque[:len(p)] == p {
				return u.Opaque[len(p):], k
			}
		}
	}
	return "", ""
}

// Revoke denies every certificate issued for a node (a stand-alone revoke; `nodes revoke` uses RevokeHook).
func (c *CA) Revoke(ctx context.Context, actor, nodeID, reason string) (int64, error) {
	certs, err := c.st.PKICertsOfNode(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	serials := make([]string, len(certs))
	for i, cert := range certs {
		serials[i] = cert.Serial
	}
	return c.st.PKIDeny(ctx, actor, nodeID, reason, serials...)
}

// RevokeHook is the lifecycle revoker: inside the revoke transaction it adds every certificate serial
// of the node to the deny list, so `nodes revoke` and `nodes remove` cut the node off atomically.
func RevokeHook(now func() time.Time) store.RevokeHook {
	return func(ctx context.Context, tx *store.RevokeTx, nodeID string) error {
		_, err := tx.Exec(`INSERT OR IGNORE INTO pki_denied(serial,node_id,reason,denied_at) SELECT serial,node_id,'node_revoked',? FROM pki_cert WHERE node_id=? AND kind='node'`, now().UnixNano(), nodeID)
		return err
	}
}
