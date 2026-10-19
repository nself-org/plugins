package pki

// Purpose: the coordinator CA hierarchy (Epic D5): one root (10 years) over three intermediates, node,
//          user and coordinator, all ECDSA P-256, every private key sealed under the KEK in the store.
// Inputs:  Options{Store, DataDir, Now}; the first New against an empty store creates the hierarchy in
//          one transaction, later calls load it.
// Outputs: *CA: the root, the intermediates, and the signing methods in issue.go.
// Constraints: stdlib crypto/x509 only; node and user intermediates carry ExtKeyUsage ClientAuth and
//          the coordinator intermediate ServerAuth, so Go's chain building refuses a leaf of the wrong
//          kind even before our own kind check; intermediates are path length 0; a missing KEK over an
//          existing hierarchy is ErrNoKEK; serials are 127-bit random.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Kind is the role a leaf certificate has; it is the first path segment of its URI SAN.
type Kind string

// The three leaf kinds, one intermediate each.
const (
	KindNode        Kind = "node"
	KindUser        Kind = "user"
	KindCoordinator Kind = "coordinator"
)

const (
	rootName  = "root"
	rootLife  = 10 * 365 * 24 * time.Hour
	interLife = 5 * 365 * 24 * time.Hour
	backdate  = time.Minute
	rootCN    = "nself-ci root"
)

// Options configure New.
type Options struct {
	Store   *store.Store
	DataDir string
	// Now is the clock (default time.Now); tests inject one.
	Now func() time.Time
}

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// CA is the loaded hierarchy.
type CA struct {
	st    *store.Store
	kek   []byte
	now   func() time.Time
	root  authority
	inter map[Kind]authority
}

// KindCN is the common name of the intermediate that signs leaves of kind k.
func KindCN(k Kind) string { return "nself-ci " + string(k) + " CA" }

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

func issueAuthority(parent *authority, key *ecdsa.PrivateKey, cn string, life time.Duration, now time.Time, usage []x509.ExtKeyUsage, pathLen int) (*x509.Certificate, []byte, error) {
	serial, err := newSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: cn}, NotBefore: now.Add(-backdate), NotAfter: now.Add(life),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, BasicConstraintsValid: true, IsCA: true, ExtKeyUsage: usage,
		MaxPathLen: pathLen, MaxPathLenZero: pathLen == 0,
	}
	signer, signKey := tmpl, key
	if parent != nil {
		signer, signKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, der, err
}

func sealKey(kek []byte, kind string, key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return Seal(kek, der, "authority:"+kind)
}

// New loads the hierarchy of the store, creating it (and the KEK) on first use.
func New(ctx context.Context, o Options) (*CA, error) {
	if o.Store == nil || o.DataDir == "" {
		return nil, errors.New("pki: Store and DataDir are required")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	have, err := o.Store.PKIAuthorities(ctx)
	if err != nil {
		return nil, err
	}
	kek, err := LoadKEK(o.DataDir, len(have) == 0)
	if err != nil {
		return nil, err
	}
	c := &CA{st: o.Store, kek: kek, now: o.Now, inter: map[Kind]authority{}}
	if len(have) == 0 {
		if _, err = c.bootstrap(ctx); err != nil { // a lost race still loads the winner's rows below
			return nil, err
		}
		if have, err = o.Store.PKIAuthorities(ctx); err != nil {
			return nil, err
		}
	}
	return c, c.load(have)
}

func (c *CA) bootstrap(ctx context.Context) (bool, error) {
	now := c.now()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, err
	}
	rootCert, rootDER, err := issueAuthority(nil, rootKey, rootCN, rootLife, now, nil, 1)
	if err != nil {
		return false, err
	}
	root := authority{rootCert, rootKey}
	sealed, err := sealKey(c.kek, rootName, rootKey)
	if err != nil {
		return false, err
	}
	rows := []store.PKIAuthority{{Kind: rootName, CertDER: rootDER, KeySealed: sealed, NotAfter: rootCert.NotAfter.UnixNano()}}
	for _, k := range []Kind{KindNode, KindUser, KindCoordinator} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return false, err
		}
		usage := x509.ExtKeyUsageClientAuth
		if k == KindCoordinator {
			usage = x509.ExtKeyUsageServerAuth
		}
		cert, der, err := issueAuthority(&root, key, KindCN(k), interLife, now, []x509.ExtKeyUsage{usage}, 0)
		if err != nil {
			return false, err
		}
		if sealed, err = sealKey(c.kek, string(k), key); err != nil {
			return false, err
		}
		rows = append(rows, store.PKIAuthority{Kind: string(k), CertDER: der, KeySealed: sealed, NotAfter: cert.NotAfter.UnixNano()})
	}
	return c.st.PKIAuthoritiesInsert(ctx, rows)
}

func (c *CA) load(rows []store.PKIAuthority) error {
	byKind := map[string]authority{}
	for _, r := range rows {
		cert, err := x509.ParseCertificate(r.CertDER)
		if err != nil {
			return fmt.Errorf("pki: authority %s: %w", r.Kind, err)
		}
		plain, err := Unseal(c.kek, r.KeySealed, "authority:"+r.Kind)
		if err != nil {
			return fmt.Errorf("pki: cannot unseal the %s key (wrong or damaged KEK): %w", r.Kind, err)
		}
		k, err := x509.ParsePKCS8PrivateKey(plain)
		if err != nil {
			return fmt.Errorf("pki: authority %s key: %w", r.Kind, err)
		}
		ek, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("pki: authority %s key is not ECDSA", r.Kind)
		}
		byKind[r.Kind] = authority{cert, ek}
	}
	var ok bool
	if c.root, ok = byKind[rootName]; !ok {
		return errors.New("pki: stored hierarchy has no root")
	}
	for _, k := range []Kind{KindNode, KindUser, KindCoordinator} {
		if c.inter[k], ok = byKind[string(k)]; !ok {
			return fmt.Errorf("pki: stored hierarchy has no %s intermediate", k)
		}
	}
	return nil
}

// RootDER is the root certificate, DER.
func (c *CA) RootDER() []byte { return c.root.cert.Raw }

// Root is the parsed root certificate.
func (c *CA) Root() *x509.Certificate { return c.root.cert }

// RootFingerprint is the fingerprint the operator compares on both sides.
func (c *CA) RootFingerprint() string { return CertFingerprint(c.root.cert.Raw) }

// Now is the CA clock.
func (c *CA) Now() time.Time { return c.now() }
